package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/report"
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
	p := who(r)
	if p.Role == auth.Developer {
		var filtered []store.Project
		for _, proj := range ps {
			if a.canReadProject(r, proj.ID) {
				filtered = append(filtered, proj)
			}
		}
		ps = filtered
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
	var raw map[string]any
	if !readJSON(w, r, &raw) {
		return
	}
	user := who(r).Email

	allowedKeys := map[string]bool{
		"branch": true, "schedule": true, "tz": true, "pack_pins": true,
		"enforcement": true, "fail_on": true, "model_policy": true, "ai": true,
		"false_positive": true,
	}
	for k := range raw {
		if !allowedKeys[k] {
			http.Error(w, "unknown key: "+k, 400)
			return
		}
	}

	getString := func(key string) *string {
		if v, ok := raw[key].(string); ok {
			return &v
		}
		return nil
	}
	getBool := func(key string) *bool {
		if v, ok := raw[key].(bool); ok {
			return &v
		}
		return nil
	}

	var approvalIDs []string

	// Security fields (enforcement, fail_on, model_policy, ai, false_positive, branch, schedule, pack_pins)
	// must create an approval and change nothing until approved.
	if enf := getString("enforcement"); enf != nil && *enf != p.Enforcement {
		if *enf != "block" && *enf != "report" {
			http.Error(w, "enforcement must be block or report", 400)
			return
		}
		appID, err := a.Store.Propose("enforcement", map[string]any{"project": p.ID, "enforcement": *enf}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if fo := getString("fail_on"); fo != nil && *fo != p.FailOn {
		if _, err := engines.ParseSeverity(*fo); *fo != "" && err != nil {
			http.Error(w, "bad fail_on", 400)
			return
		}
		appID, err := a.Store.Propose("fail_on", map[string]any{"project": p.ID, "fail_on": *fo}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if mp := getString("model_policy"); mp != nil && *mp != p.ModelPolicy {
		if *mp != "local-only" && *mp != "cloud-redacted" && *mp != "cloud" {
			http.Error(w, "bad model_policy", 400)
			return
		}
		appID, err := a.Store.Propose("model_policy", map[string]any{"project": p.ID, "model_policy": *mp}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if aiVal := getBool("ai"); aiVal != nil && *aiVal != p.AI {
		appID, err := a.Store.Propose("ai", map[string]any{"project": p.ID, "ai": *aiVal}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if b := getString("branch"); b != nil && *b != p.Branch {
		appID, err := a.Store.Propose("branch", map[string]any{"project": p.ID, "branch": *b}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if s := getString("schedule"); s != nil && *s != p.Schedule {
		if *s != "" {
			if _, err := dueFn(*s, p.TZ, time.Now()); err != nil {
				fail(w, err, 400)
				return
			}
		}
		appID, err := a.Store.Propose("schedule", map[string]any{"project": p.ID, "schedule": *s}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if pins, ok := raw["pack_pins"].(map[string]any); ok {
		m := map[string]string{}
		for k, v := range pins {
			if s, ok := v.(string); ok {
				m[k] = s
			}
		}
		appID, err := a.Store.Propose("pack_pins", map[string]any{"project": p.ID, "pack_pins": m}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}
	if fp := getString("false_positive"); fp != nil {
		appID, err := a.Store.Propose("false_positive", map[string]any{"project": p.ID, "false_positive": *fp}, user)
		if err != nil {
			fail(w, err, 500)
			return
		}
		approvalIDs = append(approvalIDs, appID)
	}

	hasNonSec := false
	if t := getString("tz"); t != nil && *t != p.TZ {
		p.TZ = *t
		hasNonSec = true
	}

	if hasNonSec {
		if err := a.Store.UpdateProject(p); err != nil {
			fail(w, err, 500)
			return
		}
		a.audit_(r, "project:update", p.Name)
	}

	if len(approvalIDs) > 0 {
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, map[string]any{
			"approval_id":  approvalIDs[0],
			"approval_ids": approvalIDs,
			"status":       "approval_required",
		})
		return
	}

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
	id := r.PathValue("id")
	if !a.canReadProject(r, id) {
		http.NotFound(w, r)
		return
	}
	ss, err := a.Store.Scans(id, 50)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, ss)
}

func (a *App) surface(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.canReadProject(r, id) {
		http.NotFound(w, r)
		return
	}
	rows, err := a.Store.Surface(id, 200)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, rows)
}

func (a *App) createToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string
		ExpiresAt int64
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if _, err := a.Store.Project(id); err != nil {
		http.NotFound(w, r)
		return
	}
	tok, err := a.Store.CreateToken(id, in.Name, in.ExpiresAt)
	if err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "token:create", id)
	writeJSON(w, map[string]string{"token": tok}) // shown once
}

func (a *App) revokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tok := r.PathValue("token")
	if tok == "" {
		var in struct{ Token string }
		readJSON(w, r, &in)
		tok = in.Token
	}
	if tok == "" {
		http.Error(w, "token required", 400)
		return
	}
	if err := a.Store.RevokeToken(id, tok); err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "token:revoke", id, "active", "revoked")
	w.WriteHeader(http.StatusOK)
}

func (a *App) deleteToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tok := r.PathValue("token")
	if tok == "" {
		var in struct{ Token string }
		readJSON(w, r, &in)
		tok = in.Token
	}
	if tok == "" {
		http.Error(w, "token required", 400)
		return
	}
	if err := a.Store.DeleteToken(id, tok); err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "token:delete", id, "active", "deleted")
	w.WriteHeader(http.StatusOK)
}

// ingest takes a CI scan from the CLI. Only a token for the same project may post.
func (a *App) ingest(w http.ResponseWriter, r *http.Request) {
	p, ok := a.who(r)
	if !ok || p.Project == "" || p.Project != r.PathValue("id") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var in struct {
		Commit   string            `json:"commit"`
		Ran      []string          `json:"ran"`
		Findings []engines.Finding `json:"findings"`
	}
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(bodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	knownEngines := map[string]bool{
		"secrets": true, "semgrep": true, "gitleaks": true,
		"osv-scanner": true, "packs": true, "advisories": true,
	}

	var signedUpload bool
	if sigHeader := r.Header.Get("X-Signature"); sigHeader != "" && len(a.TrustedKeys) > 0 {
		if sigBytes, err := base64.StdEncoding.DecodeString(sigHeader); err == nil {
			for _, k := range a.TrustedKeys {
				if ed25519.Verify(k, bodyBytes, sigBytes) {
					signedUpload = true
					break
				}
			}
		}
	}

	source := "ci-unattested"
	if signedUpload {
		source = "ci-attested"
	}

	for i := range in.Findings {
		f := &in.Findings[i]
		if !signedUpload && !knownEngines[f.Engine] && !strings.HasPrefix(f.Engine, "pack:") {
			http.Error(w, fmt.Sprintf("unattested engine %q", f.Engine), http.StatusBadRequest)
			return
		}

		if f.Severity < engines.Info || f.Severity > engines.Critical {
			http.Error(w, "invalid severity", http.StatusBadRequest)
			return
		}

		if !signedUpload && f.Severity > engines.Medium {
			f.Severity = engines.Medium
		}
		f.Source = source

		// Ensure Match is hashed (no raw match on wire or stored)
		if f.Match != "" && len(f.Match) != 64 {
			h := sha256.Sum256([]byte(f.Match))
			f.Match = hex.EncodeToString(h[:])
		}

		// Truncate all free-text fields
		if len(f.Message) > 500 {
			f.Message = f.Message[:500]
		}
		if len(f.Evidence) > 200 {
			f.Evidence = f.Evidence[:200]
		}
		f.Evidence = engines.Redact(f.Evidence)
		if len(f.Fix) > 500 {
			f.Fix = f.Fix[:500]
		}
		if len(f.CWE) > 50 {
			f.CWE = f.CWE[:50]
		}
		if len(f.Rule) > 100 {
			f.Rule = f.Rule[:100]
		}
		if len(f.File) > 255 {
			f.File = f.File[:255]
		}
	}

	proj, err := a.Store.Project(p.Project)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Coverage ("ran") is never taken from the client and dummy findings must not close other findings: pass nil for ran
	sc, err := a.Store.RecordScan(store.Scan{ProjectID: proj.ID, Mode: "ci", Commit: in.Commit, Source: source}, nil, in.Findings)
	if err != nil {
		fail(w, err, 500)
		return
	}
	src := a.clientIP(r)
	a.Store.AddAudit(store.Audit{Actor: p.Email, Action: "scan:ci", Target: proj.Name, Commit: in.Commit, Source: src})
	writeJSON(w, sc)
}

func (a *App) findings(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	q := r.URL.Query()
	projID := strings.TrimSpace(q.Get("project"))
	// Developers and project tokens must always specify an explicit project
	if p.Role == auth.Developer || p.Project != "" {
		if projID == "" {
			http.Error(w, "project query parameter is required", http.StatusBadRequest)
			return
		}
	}
	if projID != "" {
		if !a.canReadProject(r, projID) {
			http.NotFound(w, r)
			return
		}
	}
	f := store.Filter{Project: projID, Status: q.Get("status"), Owner: q.Get("owner")}
	if p.Project != "" {
		f.Project = p.Project
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
	if err != nil || (who(r).Project != "" && who(r).Project != f.ProjectID) || !a.canReadProject(r, f.ProjectID) {
		http.NotFound(w, r)
		return
	}
	out := map[string]any{"finding": f}
	if v, err := a.Store.Verdict(f.ID); err == nil {
		out["verdict"] = v
	}
	writeJSON(w, out)
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
	if in.Status == "false_positive" {
		aid, err := a.Store.Propose("false_positive", map[string]any{"finding": id, "owner": in.Owner, "reason": in.Reason}, who(r).Email)
		if err != nil {
			fail(w, err, 500)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, map[string]string{"status": "approval_required", "approval": aid, "approval_id": aid})
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
		Kind    string         `json:"kind"`
		Payload map[string]any `json:"payload"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		http.Error(w, "bad json: "+err.Error(), 400)
		return
	}

	var allowedPayloadKeys map[string]bool
	switch in.Kind {
	case "disable_rule":
		allowedPayloadKeys = map[string]bool{"rule": true, "scope": true, "until": true}
	case "loosen_severity":
		allowedPayloadKeys = map[string]bool{"rule": true, "severity": true}
	case "demote_rule", "rule_demotion":
		allowedPayloadKeys = map[string]bool{"rule": true}
	case "enforcement":
		allowedPayloadKeys = map[string]bool{"project": true, "enforcement": true}
	case "fail_on":
		allowedPayloadKeys = map[string]bool{"project": true, "fail_on": true}
	case "model_policy":
		allowedPayloadKeys = map[string]bool{"project": true, "model_policy": true}
	case "ai":
		allowedPayloadKeys = map[string]bool{"project": true, "ai": true}
	case "ignore", "sentinix:ignore":
		allowedPayloadKeys = map[string]bool{"project": true, "file": true, "line": true, "rule": true, "fingerprint": true, "until": true}
	default:
		http.Error(w, "invalid approval kind", 400)
		return
	}
	for k := range in.Payload {
		if !allowedPayloadKeys[k] {
			http.Error(w, "unknown payload key: "+k, 400)
			return
		}
	}

	switch in.Kind {
	case "disable_rule":
		rule, rok := in.Payload["rule"].(string)
		if !rok || rule == "" {
			http.Error(w, "payload.rule required", 400)
			return
		}
		scope, sok := in.Payload["scope"].(string)
		untilVal, uok := in.Payload["until"]
		var until int64
		switch v := untilVal.(type) {
		case float64:
			until = int64(v)
		case int64:
			until = v
		case int:
			until = int64(v)
		case json.Number:
			until, _ = v.Int64()
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				until = t.Unix()
			} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				until = n
			}
		}
		if !sok || strings.TrimSpace(scope) == "" || !uok || until <= time.Now().Unix() {
			http.Error(w, "disable_rule requires payload.rule, payload.scope and future payload.until", 400)
			return
		}
	case "loosen_severity":
		_, rok := in.Payload["rule"].(string)
		s, sok := in.Payload["severity"].(string)
		if _, err := engines.ParseSeverity(s); !rok || !sok || err != nil {
			http.Error(w, "payload.rule and payload.severity required", 400)
			return
		}
	case "demote_rule", "rule_demotion":
		if _, ok := in.Payload["rule"].(string); !ok {
			http.Error(w, "payload.rule required", 400)
			return
		}
	case "enforcement":
		proj, pok := in.Payload["project"].(string)
		enf, eok := in.Payload["enforcement"].(string)
		if !pok || !eok || (enf != "block" && enf != "report") {
			http.Error(w, "payload.project and valid payload.enforcement (block|report) required", 400)
			return
		}
		if _, err := a.Store.Project(proj); err != nil {
			http.NotFound(w, r)
			return
		}
	case "fail_on":
		proj, pok := in.Payload["project"].(string)
		fo, fok := in.Payload["fail_on"].(string)
		if !pok || !fok {
			http.Error(w, "payload.project and payload.fail_on required", 400)
			return
		}
		if _, err := engines.ParseSeverity(fo); fo != "" && err != nil {
			http.Error(w, "invalid payload.fail_on", 400)
			return
		}
		if _, err := a.Store.Project(proj); err != nil {
			http.NotFound(w, r)
			return
		}
	case "model_policy":
		proj, pok := in.Payload["project"].(string)
		mp, mok := in.Payload["model_policy"].(string)
		if !pok || !mok || (mp != "local-only" && mp != "cloud-redacted" && mp != "cloud") {
			http.Error(w, "payload.project and valid payload.model_policy required", 400)
			return
		}
		if _, err := a.Store.Project(proj); err != nil {
			http.NotFound(w, r)
			return
		}
	case "ai":
		proj, pok := in.Payload["project"].(string)
		if !pok || proj == "" {
			http.Error(w, "payload.project required", 400)
			return
		}
		if _, err := a.Store.Project(proj); err != nil {
			http.NotFound(w, r)
			return
		}
	case "ignore", "sentinix:ignore":
		proj, pok := in.Payload["project"].(string)
		if !pok || strings.TrimSpace(proj) == "" {
			http.Error(w, "payload.project required", 400)
			return
		}
		file, fok := in.Payload["file"].(string)
		rule, rok := in.Payload["rule"].(string)
		fp, fpok := in.Payload["fingerprint"].(string)
		_, lok := in.Payload["line"]
		if !fok || file == "" || !rok || rule == "" || !fpok || fp == "" || !lok {
			http.Error(w, "payload must carry project, file, line, rule and fingerprint", 400)
			return
		}
		var until int64
		switch v := in.Payload["until"].(type) {
		case float64:
			until = int64(v)
		case int64:
			until = v
		case int:
			until = int64(v)
		case json.Number:
			until, _ = v.Int64()
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				until = t.Unix()
			} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				until = n
			}
		}
		if until <= time.Now().Unix() {
			http.Error(w, "future payload.until expiry required", 400)
			return
		}
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
	writeJSON(w, ap)
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
	u, err := a.Store.UserByEmail(in.Email)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cur := u.Role
	if (in.Role == auth.Owner || cur == auth.Owner) && who(r).Role != auth.Owner {
		http.Error(w, "only an owner can change owner roles", 403)
		return
	}
	// All role changes (including Owner role changes) require a second person's approval
	aid, err := a.Store.Propose("role_change", map[string]any{"email": in.Email, "role": in.Role, "current": cur}, who(r).Email)
	if err != nil {
		fail(w, err, 500)
		return
	}
	a.audit_(r, "role:propose:"+in.Role, in.Email, cur, in.Role)
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{
		"status":      "approval_required",
		"id":          aid,
		"approval_id": aid,
	})
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

// report renders csv, sarif, json, sbom (CycloneDX) or html (print to PDF) for one project.
func (a *App) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.canReadProject(r, id) {
		http.NotFound(w, r)
		return
	}
	p, err := a.Store.Project(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "open"
	}
	fs, err := a.Store.Findings(store.Filter{Project: p.ID, Status: status, Limit: 10000})
	if err != nil {
		fail(w, err, 500)
		return
	}
	switch r.PathValue("fmt") {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="findings.csv"`)
		report.CSV(w, fs)
	case "json":
		writeJSON(w, fs)
	case "sarif":
		w.Header().Set("Content-Type", "application/sarif+json")
		plain := make([]engines.Finding, len(fs))
		for i, f := range fs {
			plain[i] = f.Finding
		}
		report.SARIF(w, plain)
	case "sbom":
		cs, err := a.Store.Components(p.ID)
		if err != nil {
			fail(w, err, 500)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.cyclonedx+json")
		report.SBOM(w, p.Name, cs, fs)
	case "html":
		post, _ := a.Store.Posture(p.ID)
		var last *store.Scan
		if ss, _ := a.Store.Scans(p.ID, 1); len(ss) > 0 {
			last = &ss[0]
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		report.HTML(w, report.Data{Brand: a.Brand, Project: p, Scan: last, Findings: fs, Posture: post})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) rescanAdvisories(w http.ResponseWriter, r *http.Request) {
	if a.OSV == nil {
		http.Error(w, "advisory lookups are not configured", http.StatusConflict)
		return
	}
	a.audit_(r, "advisories:rescan", "all")
	go a.RescanAdvisories(context.Background())
	w.WriteHeader(http.StatusAccepted)
}

func (a *App) sbom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.canReadProject(r, id) {
		http.NotFound(w, r)
		return
	}
	cs, err := a.Store.Components(id)
	if err != nil {
		fail(w, err, 500)
		return
	}
	writeJSON(w, cs)
}

// ticket files a finding in Jira or Linear and records the key on the finding.
func (a *App) ticket(w http.ResponseWriter, r *http.Request) {
	if a.Integrations == nil {
		http.Error(w, "no integrations configured", http.StatusConflict)
		return
	}
	var in struct {
		Tracker string
		Due     string // YYYY-MM-DD, optional
	}
	if !readJSON(w, r, &in) {
		return
	}
	f, err := a.Store.Finding(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if f.Ticket != "" {
		http.Error(w, "finding already has ticket "+f.Ticket, http.StatusConflict)
		return
	}
	var due time.Time
	if in.Due != "" {
		if due, err = time.Parse("2006-01-02", in.Due); err != nil {
			http.Error(w, "due must be YYYY-MM-DD", 400)
			return
		}
	}
	proj, _ := a.Store.Project(f.ProjectID)
	key, err := a.Integrations.Ticket(r.Context(), in.Tracker, f, proj.Name, due)
	if err != nil {
		fail(w, err, http.StatusBadGateway)
		return
	}
	a.Store.SetTicket(f.ID, in.Tracker+":"+key)
	a.audit_(r, "ticket:"+in.Tracker, f.ID)
	writeJSON(w, map[string]string{"ticket": in.Tracker + ":" + key})
}

// revokeUser ends every session a user holds (deprovisioning). It does not need a second approver:
// it only removes access, and the audit log records who did it.
func (a *App) revokeUser(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email string }
	if !readJSON(w, r, &in) {
		return
	}
	us, _ := a.Store.Users()
	for _, u := range us {
		if u.Email == in.Email && u.Role == auth.Owner && who(r).Role != auth.Owner {
			http.Error(w, "only an owner can revoke an owner", 403)
			return
		}
	}
	if err := a.Store.RevokeSessions(in.Email); err != nil {
		http.NotFound(w, r)
		return
	}
	a.audit_(r, "session:revoke", in.Email)
	w.WriteHeader(http.StatusNoContent)
}
