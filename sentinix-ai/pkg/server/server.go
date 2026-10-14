// Package server is the HTTP API behind the dashboard, the CLI and CI: auth, projects, findings,
// approvals, webhooks and the scan job runner.
package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/ai"
	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/integrations"
	"sentinix/pkg/packs"
	"sentinix/pkg/report"
	"sentinix/pkg/store"
)

type App struct {
	Store        *store.Store
	Hosts        map[string]githost.Host
	Sess         auth.Sessions
	OIDC         *auth.OIDC // nil: SSO not configured, only API tokens and existing sessions work
	Packs        packs.Engine
	OSV          *advisories.Client // nil: no dependency advisory lookups (default; nothing leaves the network)
	FailOn       engines.Severity
	OwnerEmail   string       // this identity gets the Owner role on first login
	Static       http.Handler // dashboard
	Workers      int
	AIModels     []ai.Model        // empty: AI add-on unavailable
	TestRunner   ai.Runner         // sandboxed runner for model-written tests; nil disables test generation
	Integrations *integrations.Set // nil: no chat, webhooks or tickets
	Brand        report.Brand      // white-label name and accent for HTML reports
	// OnScan is called after a recorded full scan or reviewed PR (integrations hook).
	OnScan func(ctx context.Context, p store.Project, sc store.Scan, newFindings []store.Finding)

	sem chan struct{}
}

type principal struct {
	Email, Role, Project string // Project is set for API tokens, which have no Role
}

type ctxKey struct{}

func (a *App) Handler() http.Handler {
	if a.Workers < 1 {
		a.Workers = 2
	}
	a.sem = make(chan struct{}, a.Workers)
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })
	m.HandleFunc("POST /webhook/{name}", a.webhook)
	if a.OIDC != nil {
		m.HandleFunc("GET /auth/login", a.OIDC.Login)
		m.HandleFunc("GET /auth/callback", a.OIDC.Callback)
	}
	m.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, _ *http.Request) { a.Sess.Clear(w) })

	api := func(perm string, h http.HandlerFunc) http.HandlerFunc { return a.guard(perm, h) }
	m.HandleFunc("GET /api/me", api("read", a.me))
	m.HandleFunc("GET /api/posture", api("read", a.posture))
	m.HandleFunc("GET /api/projects", api("read", a.projects))
	m.HandleFunc("POST /api/projects", api("manage", a.createProject))
	m.HandleFunc("PATCH /api/projects/{id}", api("manage", a.updateProject))
	m.HandleFunc("POST /api/projects/{id}/scan", api("scan", a.scanNow))
	m.HandleFunc("GET /api/projects/{id}/scans", api("read", a.scans))
	m.HandleFunc("GET /api/projects/{id}/surface", api("read", a.surface))
	m.HandleFunc("POST /api/advisories/rescan", api("scan", a.rescanAdvisories))
	m.HandleFunc("GET /api/projects/{id}/sbom", api("read", a.sbom))
	m.HandleFunc("POST /api/projects/{id}/tokens", api("manage", a.createToken))
	m.HandleFunc("POST /api/projects/{id}/ingest", a.ingest) // API token only
	m.HandleFunc("GET /api/findings", api("read", a.findings))
	m.HandleFunc("GET /api/findings/{id}", api("read", a.finding))
	m.HandleFunc("POST /api/findings/{id}/triage", api("triage", a.triage))
	m.HandleFunc("POST /api/findings/{id}/ticket", api("triage", a.ticket))
	m.HandleFunc("POST /api/findings/{id}/ai/triage", api("triage", a.aiTriage))
	m.HandleFunc("POST /api/findings/{id}/ai/patch", api("triage", a.aiPatch))
	m.HandleFunc("POST /api/findings/{id}/ai/test", api("owner", a.aiTest))
	m.HandleFunc("GET /api/approvals", api("read", a.approvals))
	m.HandleFunc("POST /api/approvals", api("triage", a.propose))
	m.HandleFunc("POST /api/approvals/{id}/decide", api("approve", a.decide))
	m.HandleFunc("GET /api/audit", api("audit", a.audit))
	m.HandleFunc("GET /api/users", api("manage", a.users))
	m.HandleFunc("PUT /api/users/role", api("manage", a.setRole))
	m.HandleFunc("GET /api/packs", api("read", a.packList))
	m.HandleFunc("GET /api/projects/{id}/report/{fmt}", api("read", a.report))
	if a.Static != nil {
		m.Handle("/", a.Static)
	}
	return m
}

// guard authenticates (session cookie or Bearer token), checks the permission, and stores the principal.
// Mutating cookie requests must carry X-Requested-With, which a cross-site form cannot set (CSRF).
func (a *App) guard(perm string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.who(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if p.Project != "" { // tokens only reach token routes, not guarded ones, except read of their project
			if perm != "read" || (r.PathValue("id") != p.Project && !strings.HasPrefix(r.URL.Path, "/api/findings")) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		} else if !auth.Can(p.Role, perm) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != "GET" && p.Project == "" && r.Header.Get("X-Requested-With") != "sentinix" {
			http.Error(w, "missing X-Requested-With", http.StatusForbidden)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	}
}

func (a *App) who(r *http.Request) (principal, bool) {
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		proj, ok := a.Store.TokenProject(tok)
		return principal{Project: proj, Email: "token:" + proj}, ok
	}
	email, ok := a.Sess.Email(r)
	if !ok {
		return principal{}, false
	}
	us, err := a.Store.Users()
	if err != nil {
		return principal{}, false
	}
	for _, u := range us {
		if u.Email == email {
			return principal{Email: email, Role: u.Role}, true
		}
	}
	return principal{}, false
}

func who(r *http.Request) principal { return r.Context().Value(ctxKey{}).(principal) }

// Login is the OIDC OnLogin hook: new users start as developers; the configured owner email becomes owner.
func (a *App) Login(email, name string) error {
	role := auth.Developer
	if a.OwnerEmail != "" && strings.EqualFold(email, a.OwnerEmail) {
		role = auth.Owner
	}
	_, err := a.Store.UpsertUser(email, name, role)
	return err
}

func (a *App) audit_(r *http.Request, action, target string) {
	if err := a.Store.AddAudit(store.Audit{Actor: who(r).Email, Action: action, Target: target}); err != nil {
		log.Printf("audit: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(v); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return false
	}
	return true
}

func fail(w http.ResponseWriter, err error, code int) {
	http.Error(w, err.Error(), code)
}

// Tick runs scheduled full scans for projects whose cron fires in the current minute. Call once a minute.
func (a *App) Tick(ctx context.Context, now time.Time) {
	ps, err := a.Store.Projects()
	if err != nil {
		log.Printf("tick: %v", err)
		return
	}
	for _, p := range ps {
		if p.Schedule == "" {
			continue
		}
		due, err := dueFn(p.Schedule, p.TZ, now)
		if err != nil {
			log.Printf("project %s: schedule: %v", p.Name, err)
			continue
		}
		if due {
			go a.queueFull(ctx, p, "")
		}
	}
}
