package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

var dueFn = scheduler.Due

// engine returns the pack engine (approved overrides and noisy-rule demotion applied), plus the advisory engine when OSV is on.
func (a *App) engine() ([]engines.Engine, error) {
	pe := a.Packs
	dis, sev, err := a.Store.RuleOverrides()
	if err != nil {
		return nil, err
	}
	demoted, err := a.Store.Demoted(0.5, 10)
	if err != nil {
		return nil, err
	}
	for r, s := range demoted {
		if _, ok := sev[r]; !ok {
			sev[r] = s
		}
	}
	pe.Disabled, pe.Override = dis, sev
	out := []engines.Engine{pe}
	if a.OSV != nil {
		out = append(out, advisories.Engine{Client: *a.OSV})
	}
	return out, nil
}

func (a *App) webhook(w http.ResponseWriter, r *http.Request) {
	h, ok := a.Hosts[r.PathValue("name")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
	if err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	ev, err := h.Parse(r, body)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	if ev == nil {
		w.WriteHeader(204)
		return
	}
	w.WriteHeader(202)
	go a.review(context.Background(), h, ev)
}

func (a *App) review(ctx context.Context, h githost.Host, ev *githost.Event) {
	a.sem <- struct{}{}
	defer func() { <-a.sem }()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	log.Printf("%s: review %s#%s", h.Name(), ev.Repo, ev.PR)
	h.Status(ctx, ev, githost.Pending, "scanning")
	user, token := h.CloneAuth()
	eng, err := a.engine()
	if err == nil {
		start := time.Now()
		var res *scheduler.Result
		if res, err = scheduler.Review(ev, user, token, eng...); err == nil {
			a.finishReview(ctx, h, ev, res, time.Since(start))
			return
		}
	}
	log.Printf("%s: %v", h.Name(), err)
	h.Status(ctx, ev, githost.Failure, "scan error") // errors fail closed
}

// gate returns whether a failing status may block the merge, and the severity at which it fails.
// A registered project's own settings win; anything else falls back to the server defaults.
func (a *App) gate(p *store.Project) (blocking bool, threshold engines.Severity) {
	blocking, threshold = a.Enforcement != "report", a.FailOn
	if p == nil {
		return
	}
	blocking = p.Enforcement != "report"
	if s, err := engines.ParseSeverity(p.FailOn); err == nil && p.FailOn != "" {
		threshold = s
	}
	return
}

func (a *App) finishReview(ctx context.Context, h githost.Host, ev *githost.Event, res *scheduler.Result, dur time.Duration) {
	var proj *store.Project
	if p, err := a.Store.ProjectByRepo(h.Name(), ev.RepoPath); err == nil {
		proj = &p
	}
	blocking, threshold := a.gate(proj)

	shown := res.New
	if len(shown) > 50 {
		shown = shown[:50]
	}
	var buf bytes.Buffer
	buf.WriteString("### Sentinix\n")
	if blocking {
		fmt.Fprintf(&buf, "Merge is blocked by new findings at **%s** or above.\n", threshold)
	} else {
		buf.WriteString("Report only: these findings do not block the merge.\n")
	}
	buf.WriteString("```\n")
	report.Text(&buf, shown, res.Ran)
	buf.WriteString("```\n")
	if len(res.New) > 50 {
		fmt.Fprintf(&buf, "%d more not shown.\n", len(res.New)-50)
	}
	// ponytail: posts a new comment per push; add find-and-update when it gets noisy.
	if err := h.Comment(ctx, ev, buf.String()); err != nil {
		log.Printf("%s: comment: %v", h.Name(), err)
	}
	state, desc := githost.Success, "no new findings at gate"
	switch {
	case !blocking:
		desc = fmt.Sprintf("%d new finding(s), report only", len(res.New))
	case policy.Fails(res.New, threshold):
		state, desc = githost.Failure, fmt.Sprintf("%d new finding(s)", len(res.New))
	}
	if err := h.Status(ctx, ev, state, desc); err != nil {
		log.Printf("%s: status: %v", h.Name(), err)
	}
	if proj != nil {
		sc := store.Scan{ProjectID: proj.ID, Mode: "pr", Commit: res.Commit, PR: ev.PR, DurationMS: dur.Milliseconds(), New: len(res.New)}
		a.Store.RecordPRScan(sc)
		a.Store.AddSurface(proj.ID, res.Commit, ev.PR, res.Surface)
	}
}

// queueFull runs a full scan of a project's branch (or the given one) and records it.
func (a *App) queueFull(ctx context.Context, p store.Project, branch string) {
	a.sem <- struct{}{}
	defer func() { <-a.sem }()
	if branch == "" {
		branch = p.Branch
	}
	h, ok := a.Hosts[p.Host]
	if !ok {
		log.Printf("project %s: unknown host %q", p.Name, p.Host)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	user, token := h.CloneAuth()
	eng, err := a.engine()
	if err != nil {
		log.Printf("project %s: %v", p.Name, err)
		return
	}
	res, dur, err := scheduler.Full(h.RepoURL(p.Repo), branch, user, token, eng...)
	if err != nil {
		log.Printf("project %s: full scan: %v", p.Name, err)
		return
	}
	a.record(ctx, p, "full", res, dur)
}

func (a *App) record(ctx context.Context, p store.Project, mode string, res *scheduler.Result, dur time.Duration) (store.Scan, error) {
	sc, err := a.Store.RecordScan(store.Scan{ProjectID: p.ID, Mode: mode, Commit: res.Commit, DurationMS: dur.Milliseconds(),
		Packs: packVersions(a.Packs)}, res.Ran, res.New)
	if err != nil {
		log.Printf("project %s: record: %v", p.Name, err)
		return sc, err
	}
	if mode == "full" {
		if err := a.Store.ReplaceComponents(p.ID, res.Components); err != nil {
			log.Printf("project %s: sbom: %v", p.Name, err)
		}
	}
	a.Store.AddAudit(store.Audit{Actor: "system", Action: "scan:" + mode, Target: p.Name, Commit: res.Commit})
	if a.OnScan != nil {
		nf, _ := a.Store.NewSince(p.ID, sc.Started)
		a.OnScan(ctx, p, sc, nf)
	}
	return sc, nil
}

func packVersions(pe interface{ Versions() string }) string { return pe.Versions() }

// RescanAdvisories re-matches every stored SBOM against OSV. No code is fetched or scanned.
// Only advisory findings change: the store fixes advisories that disappeared and reopens returning ones.
func (a *App) RescanAdvisories(ctx context.Context) error {
	if a.OSV == nil {
		return fmt.Errorf("advisory lookups are not configured")
	}
	ps, err := a.Store.Projects()
	if err != nil {
		return err
	}
	for _, p := range ps {
		cs, err := a.Store.Components(p.ID)
		if err != nil || len(cs) == 0 {
			continue
		}
		start := time.Now()
		fs, err := a.OSV.Match(ctx, cs)
		if err != nil {
			log.Printf("project %s: advisories: %v", p.Name, err)
			continue
		}
		sc, err := a.Store.RecordScan(store.Scan{ProjectID: p.ID, Mode: "advisory", DurationMS: time.Since(start).Milliseconds()}, []string{"advisories"}, fs)
		if err != nil {
			log.Printf("project %s: advisories: %v", p.Name, err)
			continue
		}
		if a.OnScan != nil && sc.New > 0 {
			nf, _ := a.Store.NewSince(p.ID, sc.Started)
			a.OnScan(ctx, p, sc, nf)
		}
	}
	return nil
}
