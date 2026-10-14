package server

import (
	"net/http"
	"sentinix/pkg/ai"

	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

// aiContext loads the finding, checks the project opted in, and fetches the code snippet around it.
func (a *App) aiContext(w http.ResponseWriter, r *http.Request) (f store.Finding, p store.Project, snippet string, ok bool) {
	f, err := a.Store.Finding(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err = a.Store.Project(f.ProjectID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if len(a.AIModels) == 0 || !p.AI {
		http.Error(w, "AI add-on is not enabled for this project", http.StatusConflict)
		return
	}
	h, found := a.Hosts[p.Host]
	if !found {
		http.Error(w, "unknown host", http.StatusConflict)
		return
	}
	user, token := h.CloneAuth()
	snippet, err = scheduler.Snippet(h.RepoURL(p.Repo), p.Branch, user, token, f.File, f.Line)
	if err != nil {
		fail(w, err, http.StatusBadGateway)
		return
	}
	return f, p, snippet, true
}

func (a *App) aiTriage(w http.ResponseWriter, r *http.Request) {
	f, p, snippet, ok := a.aiContext(w, r)
	if !ok {
		return
	}
	v, err := ai.Triage(r.Context(), a.AIModels, p.ModelPolicy, f.Finding, snippet)
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	a.Store.SaveVerdict(f.ID, v)
	a.audit_(r, "ai:triage", f.ID)
	writeJSON(w, v)
}

// aiPatch returns a draft diff; nothing is applied or committed.
func (a *App) aiPatch(w http.ResponseWriter, r *http.Request) {
	f, p, snippet, ok := a.aiContext(w, r)
	if !ok {
		return
	}
	diff, err := ai.DraftPatch(r.Context(), a.AIModels[0], p.ModelPolicy, f.Finding, snippet)
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	a.audit_(r, "ai:patch", f.ID)
	writeJSON(w, map[string]string{"patch": diff, "status": "draft, not verified"})
}

// aiTest proposes a patch and a regression test, then keeps them only if the test fails on the current code
// and passes with the patch. Model-written code runs in the sandbox and only an owner may start it.
func (a *App) aiTest(w http.ResponseWriter, r *http.Request) {
	if a.TestRunner == nil {
		http.Error(w, "test generation needs a sandbox (-sandbox-image)", http.StatusConflict)
		return
	}
	var in struct {
		Framework string
		Command   []string
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Framework == "" || len(in.Command) == 0 {
		http.Error(w, "framework and command are required", http.StatusBadRequest)
		return
	}
	f, p, snippet, ok := a.aiContext(w, r)
	if !ok {
		return
	}
	m := a.AIModels[0]
	patch, err := ai.DraftPatch(r.Context(), m, p.ModelPolicy, f.Finding, snippet)
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	test, err := ai.ProposeTest(r.Context(), m, p.ModelPolicy, f.Finding, snippet, in.Framework)
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	h := a.Hosts[p.Host]
	user, token := h.CloneAuth()
	err = scheduler.WithTree(h.RepoURL(p.Repo), p.Branch, user, token, func(dir string) error {
		return ai.Verify(r.Context(), a.TestRunner, dir, *test, patch, in.Command)
	})
	a.audit_(r, "ai:test", f.ID)
	if err != nil {
		fail(w, err, http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, map[string]any{"verified": true, "patch": patch, "test": test})
}
