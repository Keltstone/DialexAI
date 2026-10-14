package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	writeJSON(w, map[string]string{"email": p.Email, "role": p.Role})
}

func (a *App) posture(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Posture(r.URL.Query().Get("project"))
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, p)
}

func (a *App) projects(w http.ResponseWriter, r *http.Request) {
	ps, err := a.Store.Projects()
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, ps)
}

func (a *App) createProject(w http.ResponseWriter, r *http.Request) {
	var p store.Project
	if !readJSON(w, r, &p) {
		return
	}
	if _, ok := a.Hosts[p.Host]; !ok || p.Name == "" || p.Repo == "" {
		http.Error(w, "name, repo and a configured host are required", 400)
		return
	}
	p, err := a.Store.CreateProject(p)
	if err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "project:create", p.Name)
	writeJSON(w, p)
}

func (a *App) updateProject(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Project(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Branch, Schedule, TZ, ModelPolicy *string
		AI                                *bool
		PackPins                          map[string]string
	}
	if !readJSON(w, r, &in) {
		return
	}
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&p.Branch, in.Branch)
	set(&p.Schedule, in.Schedule)
	set(&p.TZ, in.TZ)
	set(&p.ModelPolicy, in.ModelPolicy)
	if in.AI != nil {
		p.AI = *in.AI
	}
	if in.PackPins != nil {
		p.PackPins = in.PackPins
	}
	if p.Schedule != "" { // reject bad specs now, not at 2 a.m.
		if _, err := dueFn(p.Schedule, p.TZ, time.Now()); err != nil {
			fail(w, err, 400)
			return
		}
	}
	if p.ModelPolicy != "local-only" && p.ModelPolicy != "cloud-redacted" && p.ModelPolicy != "cloud" {
		http.Error(w, "bad model_policy", 400)
		return
	}
	if err := a.Store.UpdateProject(p); err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "project:update", p.Name)
	writeJSON(w, p)
}

func (a *App) scanNow(w http.ResponseWriter, r *http.Request) {
	p, err := a.Store.Project(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var in struct{ Branch string }
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) // body optional
	a.audit_(r, "scan:request", p.Name)
	go a.queueFull(r.Context(), p, in.Branch)
	w.WriteHeader(http.StatusAccepted)
}

func (a *App) scans(w http.ResponseWriter, r *http.Request) {
	ss, err := a.Store.Scans(r.PathValue("id"), 50)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, ss)
}

func (a *App) surface(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.Surface(r.PathValue("id"), 200)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, rows)
}

func (a *App) createToken(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if _, err := a.Store.Project(id); err != nil {
		http.NotFound(w, r)
		return
	}
	tok, err := a.Store.CreateToken(id, in.Name)
	if err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "token:create", id)
	writeJSON(w, map[string]string{"token": tok}) // shown once
}

// ingest takes a CI scan from the CLI. Only a token for the same project may post.
func (a *App) ingest(w http.ResponseWriter, r *http.Request) {
	p, ok := a.who(r)
	if !ok || p.Project == "" || p.Project != r.PathValue("id") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var in struct {
		Commit   string
		Ran      []string
		Findings []engines.Finding
	}
	if !readJSON(w, r, &in) {
		return
	}
	proj, err := a.Store.Project(p.Project)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sc, err := a.Store.RecordScan(store.Scan{ProjectID: proj.ID, Mode: "ci", Commit: in.Commit}, in.Ran, in.Findings)
	if err != nil {
		fail(w, err, 500)
		return
	}
	a.Store.AddAudit(store.Audit{Actor: p.Email, Action: "scan:ci", Target: proj.Name, Commit: in.Commit})
	writeJSON(w, sc)
}

func (a *App) findings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{Project: q.Get("project"), Status: q.Get("status"), Owner: q.Get("owner")}
	if p := who(r); p.Project != "" {
		f.Project = p.Project // tokens see only their own project
	}
	if s := q.Get("severity"); s != "" {
		sev, err := engines.ParseSeverity(s)
		if err != nil {
			fail(w, err, 400)
			return
		}
		f.MinSeverity = sev
	}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	fs, err := a.Store.Findings(f)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, fs)
}

func (a *App) finding(w http.ResponseWriter, r *http.Request) {
	f, err := a.Store.Finding(r.PathValue("id"))
	if err != nil || (who(r).Project != "" && who(r).Project != f.ProjectID) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, f)
}

// triage: assigning, reopening and false-positive are immediate; accepting a risk needs a second person.
func (a *App) triage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status, Owner, Reason string
		Until                 int64
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	f, err := a.Store.Finding(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if in.Status == "accepted" {
		if in.Reason == "" || in.Until <= time.Now().Unix() {
			http.Error(w, "accepted risk needs a reason and a future expiry", 400)
			return
		}
		aid, err := a.Store.Propose("accept_risk", map[string]any{"finding": id, "owner": in.Owner, "reason": in.Reason, "until": in.Until}, who(r).Email)
		if err != nil {
			fail(w, err, 500)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, map[string]string{"approval": aid})
		return
	}
	if in.Status == "" {
		in.Status = f.Status
	}
	if err := a.Store.Triage(id, in.Status, in.Owner, f.AcceptedReason, f.AcceptedUntil); err != nil {
		fail(w, err, 400)
		return
	}
	a.audit_(r, "triage:"+in.Status, id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) approvals(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	if st == "" {
		st = "pending"
	}
	as, err := a.Store.Approvals(st)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, as)
}

func (a *App) propose(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind    string
		Payload map[string]any
	}
	if !readJSON(w, r, &in) {
		return
	}
	switch in.Kind {
	case "disable_rule":
		if _, ok := in.Payload["rule"].(string); !ok {
			http.Error(w, "payload.rule required", 400)
			return
		}
	case "loosen_severity":
		_, rok := in.Payload["rule"].(string)
		s, sok := in.Payload["severity"].(string)
		if _, err := engines.ParseSeverity(s); !rok || !sok || err != nil {
			http.Error(w, "payload.rule and payload.severity required", 400)
			return
		}
	default:
		http.Error(w, "kind must be disable_rule or loosen_severity", 400)
		return
	}
	id, err := a.Store.Propose(in.Kind, in.Payload, who(r).Email)
	if err != nil {
		fail(w, err, 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"approval": id})
}

func (a *App) decide(w http.ResponseWriter, r *http.Request) {
	var in struct{ Approve bool }
	if !readJSON(w, r, &in) {
		return
	}
	ap, err := a.Store.Decide(r.PathValue("id"), who(r).Email, in.Approve)
	if err != nil {
		fail(w, err, 409)
		return
	}
	if ap.Status == "approved" {
		if err := a.apply(ap); err != nil {
			fail(w, err, 500)
			return
		}
	}
	writeJSON(w, ap)
}

// apply performs an approved change. Kinds are validated when proposed.
func (a *App) apply(ap store.Approval) error {
	var p struct {
		Finding, Owner, Reason, Rule, Severity string
		Until                                  int64
	}
	if err := json.Unmarshal(ap.Payload, &p); err != nil {
		return err
	}
	switch ap.Kind {
	case "accept_risk":
		return a.Store.Triage(p.Finding, "accepted", p.Owner, p.Reason, p.Until)
	case "disable_rule":
		return a.Store.SetRuleOverride(p.Rule, true, -1)
	case "loosen_severity":
		s, _ := engines.ParseSeverity(p.Severity)
		return a.Store.SetRuleOverride(p.Rule, false, int(s))
	}
	return fmt.Errorf("unknown approval kind %q", ap.Kind)
}

func (a *App) audit(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n < 1 || n > 1000 {
		n = 200
	}
	log, err := a.Store.AuditLog(n)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, log)
}

func (a *App) users(w http.ResponseWriter, r *http.Request) {
	us, err := a.Store.Users()
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, us)
}

// setRole: admins manage users, but only an owner may grant or remove the owner role.
func (a *App) setRole(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Role string }
	if !readJSON(w, r, &in) {
		return
	}
	if !auth.ValidRole(in.Role) {
		http.Error(w, "bad role", 400)
		return
	}
	cur := ""
	us, _ := a.Store.Users()
	for _, u := range us {
		if u.Email == in.Email {
			cur = u.Role
		}
	}
	if cur == "" {
		http.NotFound(w, r)
		return
	}
	if (in.Role == auth.Owner || cur == auth.Owner) && who(r).Role != auth.Owner {
		http.Error(w, "only an owner can change owner roles", 403)
		return
	}
	if err := a.Store.SetRole(in.Email, in.Role); err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "role:"+in.Role, in.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) packList(w http.ResponseWriter, r *http.Request) {
	type packInfo struct {
		Name, Version string
		Stacks        []string
		Rules         int
	}
	var ps []packInfo
	for _, p := range a.Packs.Packs {
		ps = append(ps, packInfo{p.Name, p.Version, p.Stacks, len(p.Rules)})
	}
	stats, _ := a.Store.RuleStats()
	dis, _, _ := a.Store.RuleOverrides()
	writeJSON(w, map[string]any{"packs": ps, "rule_stats": stats, "disabled": dis})
}

// report is added with the reporting formats.
func (a *App) report(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
