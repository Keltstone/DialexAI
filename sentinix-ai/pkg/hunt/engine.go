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
	// Tools lets the model read files and grep inside the scan root before answering (at most ToolSteps calls,
	// default 3, each counted against MaxCalls). Platform limits the run to "mobile" or "backend" entry points.
	Tools     bool
	ToolSteps int
	Platform  string
	// State, when set, caches finished tasks across runs. Only, when set, runs just tasks whose
	// "file|class" contains it (single-task re-run for debugging).
	State *State
	Only  string
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
	var tl *tools
	if e.Tools {
		tl = &tools{root: root, policy: e.Policy, eps: eps}
	}
	if e.State != nil {
		defer e.State.Save()
	}
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
			for _, p := range passes {
				key := taskKey(code, ep.File, class, p.name)
				res, cached := Result{}, false
				if e.State != nil {
					res, cached = e.State.get(key)
				}
				if !cached {
					a, ok, stop := e.task(ctx, m, tl, ep, class, p.ask, code, &calls)
					if stop {
						e.Truncated = true
						return out, nil
					}
					if !ok {
						continue
					}
					res = Result{Vulnerable: a.Vulnerable, Severity: a.Severity, Reason: a.Reason}
					if res.Vulnerable && e.Run != nil && len(e.PoCCmd) > 0 && calls < e.MaxCalls {
						calls++
						sev, _ := engines.ParseSeverity(res.Severity)
						res.Evidence = e.proof(ctx, m, engines.Finding{Rule: "hunt/" + class, File: ep.File, Message: res.Reason, Severity: sev}, code, root)
					}
					if e.State != nil {
						e.State.put(key, res)
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
				break // strict hit: skip broad pass
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
			user += "\nTool result (" + r.Tool + "):\n" + ai.Untrusted(tl.run(r.toolCall)) + "\nContinue; reply with the final JSON when ready."
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
	matched := false
	for _, k := range ep.Kinds {
		if e.Platform != "" && kindPlatform[k] != e.Platform {
			continue
		}
		matched = true
		for _, c := range classes[k] {
			add(c)
		}
		for _, p := range e.Packs {
			for _, c := range p.Classes[k] {
				add(c.ID)
			}
		}
	}
	if e.Variant != "" && matched {
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
