package hunt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sentinix/pkg/ai"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

// classes per entry-point kind: the fixed vulnerability vocabulary (rule id is hunt/<class>).
var classes = map[string][]string{
	"component": {"confused-deputy", "intent-redirect", "path-traversal"},
	"deeplink":  {"deeplink-host", "webview-bridge"},
	"route":     {"authz", "injection", "path-traversal"},
}

// kindPlatform splits mixed repos: -platform mobile|backend audits only that side.
var kindPlatform = map[string]string{"component": "mobile", "deeplink": "mobile", "route": "backend"}

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
	Models     []ai.Model
	Policy     string
	MaxCalls   int
	Truncated  bool
	GapMessage string
	// Optional proof of concept: with Run and PoCCmd set, the model writes a test for each finding and it
	// runs on the current tree. Reproduced -> evidence says so; otherwise the finding stays, marked unconfirmed.
	Run    ai.Runner
	PoCCmd []string
	// Packs add classes (and prompt hints) from signed hunt packs; see LoadPack.
	Packs []*Pack
	// Variant is a known vulnerability description (advisory text). When set, every entry point is also
	// checked for variants of that bug (rule hunt/variant). Treated as untrusted data.
	Variant string
	// Tools lets the model read files and grep inside the scan root before answering (at most ToolSteps calls,
	// default 3, each counted against MaxCalls). Platform limits the run to "mobile" or "backend" entry points.
	Tools     bool
	ToolSteps int
	Platform  string
	// State, when set, caches finished tasks across runs. Only, when set, runs just tasks whose
	// "file|class" contains it (single-task re-run for debugging).
	State     *State
	Only      string
	Config    *HuntConfig
	Store     *store.Store
	ProjectID string
	RunID     string
	Tracker   *BudgetTracker
	ClassLib  *ClassLibrary
}

func (e *Engine) Name() string { return "llm-audit" }

// Gap feeds the scan's coverage gaps (scheduler.scanAll reads it after Scan). Not Skipped(): that fails PR status.
func (e *Engine) Gap() string {
	if e.Truncated {
		if e.GapMessage != "" {
			return "llm-audit stopped: " + e.GapMessage
		}
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
	// Active passes from config or default strict+broad
	activePasses := passes
	if e.Config != nil && len(e.Config.Passes) > 0 {
		activePasses = nil
		for _, p := range e.Config.Passes {
			activePasses = append(activePasses, struct{ name, ask string }{name: p.Name, ask: p.Prompt})
		}
	}

	// Dry run mode: planning preview only without calling any models
	if e.Config != nil && e.Config.DryRun {
		eps, err := EntrypointsWithConfig(root, e.Config)
		if err != nil {
			return nil, err
		}
		totalPlanned := 0
		for _, ep := range eps {
			totalPlanned += len(e.classesFor(ep)) * len(activePasses)
		}
		fmt.Printf("[DRY-RUN] Planned hunt audit: %d entry points, %d planned task passes.\n", len(eps), totalPlanned)
		fmt.Printf("[DRY-RUN] Budget limits: max_calls=%d, max_tokens=%d, max_cost_usd=$%.2f, policy=%s\n", e.MaxCalls, e.Config.MaxTokens, e.Config.MaxCostUSD, e.Policy)
		return nil, nil
	}

	var roles map[string]string
	if e.Config != nil {
		roles = e.Config.Roles
	}
	m, err := SelectModelForRole(e.Models, e.Policy, roles, "classify")
	if err != nil {
		return nil, err
	}

	// Budget tracker
	if e.Tracker == nil {
		if e.Config != nil {
			e.Tracker = NewBudgetTracker(*e.Config)
		} else {
			e.Tracker = NewBudgetTracker(HuntConfig{MaxCalls: e.MaxCalls, Policy: e.Policy})
		}
	}

	// Wrap model with concurrency throttling, retries, and token accounting
	relModel := NewReliableModel(m, e.Tracker, 5, 2*time.Minute)

	eps, err := EntrypointsWithConfig(root, e.Config)
	if err != nil {
		return nil, err
	}

	var out []engines.Finding
	calls := 0
	var tl *tools
	if e.Tools || (e.Config != nil && e.Config.Tools.Enabled) {
		tl = &tools{
			root:   root,
			policy: e.Policy,
			eps:    eps,
			cfg:    e.Config,
			store:  e.Store,
			runID:  e.RunID,
		}
	}
	if e.State != nil {
		defer e.State.Save()
	}

	// Database Run registration if store is configured
	if e.Store != nil && e.RunID == "" {
		e.RunID = fmt.Sprintf("run-%d", time.Now().UnixNano())
		_ = e.Store.CreateHuntRun(ctx, store.HuntRun{
			ID:        e.RunID,
			ProjectID: e.ProjectID,
			Mode:      "hunt",
			Status:    "running",
		})
	}

	start := time.Now()
	defer func() {
		if e.Store != nil && e.RunID != "" {
			callsCount, tokensCount, cost, _ := e.Tracker.Stats()
			status := "completed"
			if e.Truncated {
				status = "truncated"
			}
			_ = e.Store.UpdateHuntRun(ctx, store.HuntRun{
				ID:            e.RunID,
				ProjectID:     e.ProjectID,
				Status:        status,
				DurationMs:    time.Since(start).Milliseconds(),
				Calls:         callsCount,
				Tokens:        tokensCount,
				CostUSD:       cost,
				FindingsCount: len(out),
				Gaps:          e.Gap(),
			})
		}
	}()

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
			if e.Only != "" && !strings.Contains(ep.File+"|"+class, e.Only) {
				continue
			}
			for _, p := range activePasses {
				key := taskKey(code, ep.File, class, p.name)
				res, cached := Result{}, false

				// Check DB store cache first if available
				if e.Store != nil {
					if taskRec, ok, _ := e.Store.GetCachedHuntTask(ctx, key); ok && taskRec.Status == "success" {
						_ = json.Unmarshal([]byte(taskRec.ResultJSON), &res)
						cached = true
					}
				}
				// Offline fallback cache
				if !cached && e.State != nil {
					res, cached = e.State.get(key)
				}

				if !cached {
					// Check budget before task execution
					if ex, reason := e.Tracker.Exhausted(); ex {
						e.Truncated = true
						e.GapMessage = reason
						return out, nil
					}

					a, ok, stop := e.task(ctx, relModel, tl, ep, class, p.ask, code, &calls)
					if stop {
						e.Truncated = true
						if ex, reason := e.Tracker.Exhausted(); ex {
							e.GapMessage = reason
						}
						return out, nil
					}
					if !ok {
						continue
					}
					res = Result{Vulnerable: a.Vulnerable, Severity: a.Severity, Reason: a.Reason}
					if res.Vulnerable && e.Run != nil && len(e.PoCCmd) > 0 && calls < e.MaxCalls {
						calls++
						sev, _ := engines.ParseSeverity(res.Severity)
						res.Evidence = e.proof(ctx, relModel, engines.Finding{Rule: "hunt/" + class, File: ep.File, Message: res.Reason, Severity: sev}, code, root)
					}
					if e.State != nil {
						e.State.put(key, res)
					}
					if e.Store != nil {
						resJSON, _ := json.Marshal(res)
						_ = e.Store.SaveHuntTask(ctx, store.HuntTaskRecord{
							RunID:      e.RunID,
							TaskKey:    key,
							Status:     "success",
							File:       ep.File,
							Class:      class,
							Pass:       p.name,
							Model:      relModel.Name(),
							ResultJSON: string(resJSON),
						})
					}
				}
				sev, _ := engines.ParseSeverity(res.Severity)
				if !res.Vulnerable {
					continue
				}
				out = append(out, engines.Finding{
					Rule: "hunt/" + class, Engine: e.Name(), Category: engines.CategorySecurity,
					File: ep.File, Line: 1, Severity: sev, Message: truncate(res.Reason, 300), Evidence: res.Evidence,
					CWE: e.cweOf(class), Match: ep.Name + ":" + class, Source: "llm", Level: "notice",
				})
				stopOnHit := true
				if e.Config != nil && len(e.Config.Passes) > 0 {
					stopOnHit = e.Config.StopOnHit
				}
				if stopOnHit {
					break // strict hit: skip broad pass
				}
			}
		}
	}
	return out, nil
}

// reply is a final answer or, when Tool is set, a tool request.
type reply struct {
	answer
	toolCall
}

// task runs one fresh-context check. It returns the validated answer, whether one was usable, and stop when
// the call budget ran out.
func (e *Engine) task(ctx context.Context, m ai.Model, tl *tools, ep Entry, class, ask, code string, calls *int) (a answer, ok, stop bool) {
	tools := ""
	if tl != nil {
		tools = toolHelp
	}
	user := fmt.Sprintf("Entry point (%s): %s\nVulnerability class: %s\n%s%s%s\n%s\n"+
		`Reply {"vulnerable":bool,"severity":"low|medium|high|critical","confidence":0..1,"reason":"short"}.`,
		strings.Join(ep.Kinds, "+"), ep.Name, class, ask, e.hint(class), tools, ai.Untrusted(code))
	steps := e.ToolSteps
	if steps == 0 && e.Config != nil && e.Config.Tools.StepCap > 0 {
		steps = e.Config.Tools.StepCap
	}
	if steps == 0 {
		steps = 3
	}
	for step := 0; ; step++ {
		if *calls >= e.MaxCalls {
			return a, false, true
		}
		*calls++
		out, err := m.Complete(ctx, ai.SysRules, user)
		if err != nil {
			return a, false, false
		}
		js, found := ai.ExtractJSON(out)
		var r reply
		if !found || json.Unmarshal([]byte(js), &r) != nil {
			return a, false, false
		}
		if r.Tool != "" {
			if tl == nil || step >= steps {
				return a, false, false
			}
			user += "\nTool result (" + r.Tool + "):\n" + ai.Untrusted(tl.run(ctx, r.toolCall)) + "\nContinue; reply with the final JSON when ready."
			continue
		}
		if !r.Vulnerable { // severity only matters for a claimed bug
			return r.answer, true, false
		}
		sev, err := engines.ParseSeverity(r.Severity)
		if err != nil || r.Confidence < 0 || r.Confidence > 1 || sev < engines.Low {
			return a, false, false
		}
		return r.answer, true, false
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// classesFor returns applicable vulnerability class IDs for the entry point.
func (e *Engine) classesFor(ep Entry) []string {
	if e.ClassLib == nil {
		var custom []ClassConfig
		if e.Config != nil {
			custom = e.Config.Classes
		}
		e.ClassLib, _ = NewClassLibrary(custom, e.Packs)
	}
	out := e.ClassLib.ClassesFor(ep, e.Platform, e.Packs)
	matched := false
	for _, k := range ep.Kinds {
		if e.Platform != "" && kindPlatform[k] != "" && kindPlatform[k] != e.Platform {
			continue
		}
		matched = true
		break
	}
	if e.Variant != "" && matched {
		out = append(out, "variant")
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
	if e.ClassLib != nil {
		if c, ok := e.ClassLib.GetClass(id); ok && c.Hint != "" {
			return "Check specifically: " + c.Hint + "\n"
		}
	}
	if c, ok := e.class(id); ok && c.Hint != "" {
		return "Check specifically: " + c.Hint + "\n"
	}
	return ""
}

func (e *Engine) cweOf(id string) string {
	if e.ClassLib != nil {
		if c, ok := e.ClassLib.GetClass(id); ok && c.CWE != "" {
			return c.CWE
		}
	}
	if c, ok := e.class(id); ok && c.CWE != "" {
		return c.CWE
	}
	return cwe[id]
}

// proof returns the short evidence line: whether a model-written test reproduced the issue. A test that passes
// on the current code gets one revision with that feedback (the model's "debugger"). ponytail: the runner reports
// only pass/fail, so a test that fails to compile counts as "reproduced"; richer feedback needs runner output.
func (e *Engine) proof(ctx context.Context, m ai.Model, f engines.Finding, code, root string) string {
	note := ""
	last := ""
	for attempt := 0; attempt < 2; attempt++ {
		t, err := ai.ProposeTest(ctx, m, e.Policy, f, code+note, "unit")
		if err != nil {
			return "PoC unconfirmed: no usable test"
		}
		if err = ai.Reproduce(ctx, e.Run, root, *t, e.PoCCmd); err == nil {
			return "PoC reproduced: " + t.Path
		}
		last = err.Error()
		note = "\n// previous test passed on the vulnerable code, so the exploit did not trigger. Revise the test."
	}
	return "PoC unconfirmed: " + truncate(last, 120)
}
