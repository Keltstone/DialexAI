package hunt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sentinix/pkg/ai"
	"sentinix/pkg/engines"
)

// classes per entry-point kind: the fixed vulnerability vocabulary (rule id is hunt/<class>).
var classes = map[string][]string{
	"component": {"confused-deputy", "intent-redirect", "path-traversal"},
	"deeplink":  {"deeplink-host", "webview-bridge"},
	"route":     {"authz", "injection", "path-traversal"},
}

var cwe = map[string]string{
	"confused-deputy": "CWE-441", "intent-redirect": "CWE-601", "path-traversal": "CWE-22",
	"deeplink-host": "CWE-939", "webview-bridge": "CWE-749", "authz": "CWE-862", "injection": "CWE-89",
}

// prompt styles: strict avoids missing obvious bugs; broad invites creative findings. Broad runs only
// when strict found nothing.
var passes = []struct{ name, ask string }{
	{"strict", "Report only a bug that is clearly exploitable by an unprivileged attacker in a realistic state. "},
	{"broad", "Think creatively about unusual attacker-controlled inputs and chained weaknesses. "},
}

const maxSlice = 6000

// Engine audits entry points with the first model the policy allows. Truncated is set when MaxCalls
// stopped the run early, so callers can report a coverage gap instead of a clean result.
type Engine struct {
	Models    []ai.Model
	Policy    string
	MaxCalls  int
	Truncated bool
	// Optional proof of concept: with Run and PoCCmd set, the model writes a test for each finding and it
	// runs on the current tree. Reproduced -> evidence says so; otherwise the finding stays, marked unconfirmed.
	Run    ai.Runner
	PoCCmd []string
	// Packs add classes (and prompt hints) from signed hunt packs; see LoadPack.
	Packs []*Pack
	// Variant is a known vulnerability description (advisory text). When set, every entry point is also
	// checked for variants of that bug (rule hunt/variant). Treated as untrusted data.
	Variant string
}

func (e *Engine) Name() string { return "llm-audit" }

// Gap feeds the scan's coverage gaps (scheduler.scanAll reads it after Scan). Not Skipped(): that fails PR status.
func (e *Engine) Gap() string {
	if e.Truncated {
		return fmt.Sprintf("llm-audit stopped at %d model calls", e.MaxCalls)
	}
	return ""
}

type answer struct {
	Vulnerable bool    `json:"vulnerable"`
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func (e *Engine) Scan(ctx context.Context, root string) ([]engines.Finding, error) {
	var m ai.Model
	for _, c := range e.Models {
		if ai.Allowed(e.Policy, c) {
			m = c
			break
		}
	}
	if m == nil {
		return nil, fmt.Errorf("%w: no model allowed by policy %q", engines.ErrEngineUnavailable, e.Policy)
	}
	eps, err := Entrypoints(root)
	if err != nil {
		return nil, err
	}
	var out []engines.Finding
	calls := 0
	for _, ep := range eps {
		b, err := os.ReadFile(filepath.Join(root, ep.File))
		if err != nil {
			continue
		}
		code := string(b)
		if len(code) > maxSlice {
			code = code[:maxSlice]
		}
		if e.Policy == "cloud-redacted" {
			if code, err = engines.RedactSafe(code); err != nil {
				return out, fmt.Errorf("REDACTION_UNAVAILABLE: %w", err)
			}
		}
		for _, class := range e.classesFor(ep) {
			for _, p := range passes {
				if calls >= e.MaxCalls {
					e.Truncated = true
					return out, nil
				}
				calls++
				user := fmt.Sprintf("Entry point (%s): %s\nVulnerability class: %s\n%s%s\n%s\n"+
					`Reply {"vulnerable":bool,"severity":"low|medium|high|critical","confidence":0..1,"reason":"short"}.`,
					strings.Join(ep.Kinds, "+"), ep.Name, class, p.ask, e.hint(class), ai.Untrusted(code))
				reply, err := m.Complete(ctx, ai.SysRules, user)
				if err != nil {
					continue
				}
				js, ok := ai.ExtractJSON(reply)
				var a answer
				if !ok || json.Unmarshal([]byte(js), &a) != nil {
					continue
				}
				sev, err := engines.ParseSeverity(a.Severity)
				if err != nil || a.Confidence < 0 || a.Confidence > 1 || sev < engines.Low {
					continue
				}
				if !a.Vulnerable {
					continue
				}
				f := engines.Finding{
					Rule: "hunt/" + class, Engine: e.Name(), Category: engines.CategorySecurity,
					File: ep.File, Line: 1, Severity: sev, Message: truncate(a.Reason, 300),
					CWE: e.cweOf(class), Match: ep.Name + ":" + class, Source: "llm", Level: "notice",
				}
				if e.Run != nil && len(e.PoCCmd) > 0 && calls < e.MaxCalls {
					calls++
					f.Evidence = e.proof(ctx, m, f, code, root)
				}
				out = append(out, f)
				break // strict hit: skip broad pass
			}
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// classesFor is the union of the built-in and pack classes of every kind the entry point has, without repeats.
func (e *Engine) classesFor(ep Entry) []string {
	var out []string
	seen := map[string]bool{}
	add := func(c string) {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, k := range ep.Kinds {
		for _, c := range classes[k] {
			add(c)
		}
		for _, p := range e.Packs {
			for _, c := range p.Classes[k] {
				add(c.ID)
			}
		}
	}
	if e.Variant != "" {
		add("variant")
	}
	return out
}

func (e *Engine) class(id string) (c Class, ok bool) {
	for _, p := range e.Packs {
		for _, cs := range p.Classes {
			for _, c := range cs {
				if c.ID == id {
					return c, true
				}
			}
		}
	}
	return
}

func (e *Engine) hint(id string) string {
	if id == "variant" {
		return "Look for a variant of this known vulnerability (same root cause, different code path):\n" + ai.Untrusted(truncate(e.Variant, 3000)) + "\n"
	}
	if c, ok := e.class(id); ok && c.Hint != "" {
		return "Check specifically: " + c.Hint + "\n"
	}
	return ""
}

func (e *Engine) cweOf(id string) string {
	if c, ok := e.class(id); ok && c.CWE != "" {
		return c.CWE
	}
	return cwe[id]
}

// proof returns the short evidence line: whether a model-written test reproduced the issue.
func (e *Engine) proof(ctx context.Context, m ai.Model, f engines.Finding, code, root string) string {
	t, err := ai.ProposeTest(ctx, m, e.Policy, f, code, "unit")
	if err != nil {
		return "PoC unconfirmed: no usable test"
	}
	if err := ai.Reproduce(ctx, e.Run, root, *t, e.PoCCmd); err != nil {
		return "PoC unconfirmed: " + truncate(err.Error(), 120)
	}
	return "PoC reproduced: " + t.Path
}
