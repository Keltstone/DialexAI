package hunt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sentinix/pkg/ai"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

// CandidateFinding tracks a finding through the multi-stage validation pipeline.
type CandidateFinding struct {
	Finding        engines.Finding
	Confidence     float64
	Exploitability string // "confirmed", "likely", "unconfirmed"
	Mitigated      bool
	MitigationNote string
	RawAnswer      answer
	CVSSScore      float64
}

// ValidationPipeline manages multi-stage false-positive reduction, PoC execution, and FP memory.
type ValidationPipeline struct {
	cfg   *HuntConfig
	root  string
	store *store.Store
}

// NewValidationPipeline initializes the validation pipeline with config and store.
func NewValidationPipeline(cfg *HuntConfig, root string, store *store.Store) *ValidationPipeline {
	return &ValidationPipeline{
		cfg:   cfg,
		root:  root,
		store: store,
	}
}

// Stage 1: SchemaFilter validates JSON completeness, reason non-emptiness, and confidence threshold.
func (vp *ValidationPipeline) FilterSchemaAndConfidence(candidates []CandidateFinding) []CandidateFinding {
	minConf := 0.60
	if vp.cfg != nil && vp.cfg.Validation.MinConfidence > 0 {
		minConf = vp.cfg.Validation.MinConfidence
	}

	var valid []CandidateFinding
	for _, c := range candidates {
		if c.Confidence < minConf {
			continue // Drop low confidence
		}
		if strings.TrimSpace(c.Finding.Message) == "" {
			continue // Drop empty reasons
		}
		if c.Finding.Severity < engines.Low {
			continue // Drop unparseable or sub-low severity
		}
		valid = append(valid, c)
	}
	return valid
}

// Stage 2: VerifyMitigations checks deterministic security mitigations in manifest and code.
func (vp *ValidationPipeline) VerifyMitigations(candidates []CandidateFinding) []CandidateFinding {
	if vp.cfg != nil && !vp.cfg.Validation.MitigationCheck {
		return candidates
	}

	manifestPath := filepath.Join(vp.root, "AndroidManifest.xml")
	manifestBytes, _ := os.ReadFile(manifestPath)
	manifestStr := string(manifestBytes)

	var retained []CandidateFinding
	for _, c := range candidates {
		mitigated := false
		note := ""

		// Android Manifest mitigation: exported=false or signature permission
		if len(manifestBytes) > 0 {
			base := filepath.Base(c.Finding.File)
			baseName := strings.TrimSuffix(base, filepath.Ext(base))

			// Check if this component has exported="false" explicitly
			compPattern := regexp.MustCompile(fmt.Sprintf(`(?s)<(activity|service|receiver|provider)[^>]*android:name="[^"]*%s"[^>]*>`, regexp.QuoteMeta(baseName)))
			if match := compPattern.FindString(manifestStr); match != "" {
				if strings.Contains(match, `android:exported="false"`) && !strings.Contains(match, `<intent-filter`) {
					mitigated = true
					note = "Component explicitly unexported (android:exported=\"false\")"
				} else if strings.Contains(match, `signature`) || strings.Contains(match, `permission="android.permission.BIND_`) {
					mitigated = true
					note = "Component protected by signature or privileged system permission"
				}
			}
		}

		// Source code mitigations: common auth annotations and guards
		filePath := filepath.Join(vp.root, c.Finding.File)
		if srcBytes, err := os.ReadFile(filePath); err == nil {
			src := string(srcBytes)
			if strings.Contains(src, "@PreAuthorize") || strings.Contains(src, "@RolesAllowed") || strings.Contains(src, "filterTouchesWhenObscured = true") {
				if strings.Contains(c.Finding.Rule, "tapjacking") && strings.Contains(src, "filterTouchesWhenObscured") {
					mitigated = true
					note = "Protected by filterTouchesWhenObscured"
				} else if strings.Contains(c.Finding.Rule, "authz") && (strings.Contains(src, "@PreAuthorize") || strings.Contains(src, "@RolesAllowed")) {
					mitigated = true
					note = "Protected by declarative authorization annotations"
				}
			}
		}

		if mitigated {
			c.Mitigated = true
			c.MitigationNote = note
			// Demote severity to Info or tag as mitigated
			c.Finding.Severity = engines.Info
			c.Finding.Message = fmt.Sprintf("[Mitigated: %s] %s", note, c.Finding.Message)
		}
		retained = append(retained, c)
	}
	return retained
}

// Stage 3: RunPoC executes the model-generated proof-of-concept test with compiler and runner feedback.
func (vp *ValidationPipeline) RunPoC(ctx context.Context, m ai.Model, runner ai.Runner, pocCmd []string, c *CandidateFinding, code string) {
	if runner == nil || len(pocCmd) == 0 {
		c.Exploitability = "unconfirmed"
		if c.Confidence >= 0.80 && !c.Mitigated {
			c.Exploitability = "likely"
		}
		return
	}

	maxAttempts := 2
	if vp.cfg != nil && vp.cfg.PoC.MaxAttempts > 0 {
		maxAttempts = vp.cfg.PoC.MaxAttempts
	}

	note := ""
	lastErr := ""
	for attempt := 0; attempt < maxAttempts; attempt++ {
		test, err := ai.ProposeTest(ctx, m, "local-only", c.Finding, code+note, "unit")
		if err != nil {
			c.Exploitability = "unconfirmed"
			return
		}

		err = ai.Reproduce(ctx, runner, vp.root, *test, pocCmd)
		if err == nil {
			// Exploit test reproduced the issue!
			c.Exploitability = "confirmed"
			c.Finding.Evidence = "PoC reproduced: " + test.Path
			return
		}

		lastErr = err.Error()

		// Differentiate compile/setup errors vs assertion/exploit failures
		isCompileError := strings.Contains(lastErr, "compile") ||
			strings.Contains(lastErr, "cannot find symbol") ||
			strings.Contains(lastErr, "syntax error") ||
			strings.Contains(lastErr, "undefined")

		if isCompileError {
			note = fmt.Sprintf("\n// previous test failed to compile: %s. Fix syntax and imports.", truncate(lastErr, 150))
		} else {
			note = "\n// previous test passed on the vulnerable code, so the exploit did not trigger. Revise the test."
		}
	}

	c.Exploitability = "unconfirmed"
	if c.Confidence >= 0.80 && !c.Mitigated {
		c.Exploitability = "likely"
	}
	c.Finding.Evidence = "PoC unconfirmed: " + truncate(lastErr, 120)
}

// Stage 4: CalibrateSeverity calibrates CVSS and caps maximum severity based on CWE baseline.
func (vp *ValidationPipeline) CalibrateSeverity(candidates []CandidateFinding) []CandidateFinding {
	if vp.cfg != nil && !vp.cfg.Validation.SeverityCalibration {
		return candidates
	}

	for i := range candidates {
		c := &candidates[i]
		// Confirmed findings maintain high severity; unconfirmed findings without evidence are capped at High
		if c.Exploitability == "unconfirmed" && c.Finding.Severity == engines.Critical {
			c.Finding.Severity = engines.High
			c.Finding.Message = "[Calibrated: unconfirmed exploitability capped at High] " + c.Finding.Message
		}
		// Calculate synthetic CVSS score
		switch c.Finding.Severity {
		case engines.Critical:
			c.CVSSScore = 9.0
		case engines.High:
			c.CVSSScore = 7.5
		case engines.Medium:
			c.CVSSScore = 5.0
		default:
			c.CVSSScore = 3.0
		}
		if c.Exploitability == "confirmed" {
			c.CVSSScore += 0.5
			if c.CVSSScore > 10.0 {
				c.CVSSScore = 10.0
			}
		}
	}
	return candidates
}

// Stage 5: Deduplicate clusters duplicate findings on the same file, line, and rule class.
func (vp *ValidationPipeline) Deduplicate(candidates []CandidateFinding) []CandidateFinding {
	if vp.cfg != nil && !vp.cfg.Validation.Dedupe {
		return candidates
	}

	seen := make(map[string]int)
	var deduped []CandidateFinding

	for _, c := range candidates {
		key := fmt.Sprintf("%s:%s:%s", c.Finding.File, c.Finding.Rule, c.Finding.Match)
		if idx, exists := seen[key]; exists {
			// If duplicate, retain the one with higher exploitability or confidence
			if (c.Exploitability == "confirmed" && deduped[idx].Exploitability != "confirmed") ||
				(c.Confidence > deduped[idx].Confidence) {
				deduped[idx] = c
			}
			continue
		}
		seen[key] = len(deduped)
		deduped = append(deduped, c)
	}
	return deduped
}

// ApplyFPMemory auto-demotes low-precision vulnerability classes using store RuleStats.
func (vp *ValidationPipeline) ApplyFPMemory(candidates []CandidateFinding) []CandidateFinding {
	if vp.store == nil {
		return candidates
	}

	demoted, err := vp.store.Demoted(0.50, 5)
	if err != nil || len(demoted) == 0 {
		return candidates
	}

	for i := range candidates {
		c := &candidates[i]
		if _, ok := demoted[c.Finding.Rule]; ok {
			c.Finding.Severity = engines.Info
			c.Finding.Message = "[FP-Memory Demoted: precision < 50% across 5+ runs] " + c.Finding.Message
		}
	}
	return candidates
}

// Process runs all 5 stages plus FP memory in sequence, outputting final sanitized findings.
func (vp *ValidationPipeline) Process(ctx context.Context, candidates []CandidateFinding, m ai.Model, runner ai.Runner, pocCmd []string) []engines.Finding {
	// Stage 1: Schema & Confidence Filter
	step1 := vp.FilterSchemaAndConfidence(candidates)

	// Stage 2: Mitigation Verification
	step2 := vp.VerifyMitigations(step1)

	// Stage 3: Dynamic PoC Execution
	for i := range step2 {
		c := &step2[i]
		code := ""
		if b, err := os.ReadFile(filepath.Join(vp.root, c.Finding.File)); err == nil {
			code = string(b)
		}
		vp.RunPoC(ctx, m, runner, pocCmd, c, code)
	}

	// Stage 4: Severity & CVSS Calibration
	step4 := vp.CalibrateSeverity(step2)

	// Stage 5: Deduplication
	step5 := vp.Deduplicate(step4)

	// FP Memory auto-demotion
	stepFinal := vp.ApplyFPMemory(step5)

	var out []engines.Finding
	for _, c := range stepFinal {
		f := c.Finding
		// Exploitability tag added to match or evidence
		if c.Exploitability != "" {
			tag := fmt.Sprintf("[exploitability:%s]", c.Exploitability)
			if !strings.Contains(f.Evidence, tag) {
				if f.Evidence != "" {
					f.Evidence = tag + " " + f.Evidence
				} else {
					f.Evidence = tag
				}
			}
		}
		out = append(out, f)
	}
	return out
}
