package server

import (
	"encoding/hex"
	"crypto/sha256"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/engines"
	"sentinix/pkg/hunt"
	"sentinix/pkg/githost"
	"sentinix/pkg/packs"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

var (
	dueFn          = scheduler.Due
	seenDeliveries sync.Map
)

func deliveryID(r *http.Request) string {
	for _, h := range []string{"X-GitHub-Delivery", "X-Gitlab-Event-UUID", "X-Request-UUID", "X-Delivery-ID", "X-Delivery-Id"} {
		if id := r.Header.Get(h); id != "" {
			return id
		}
	}
	return ""
}

func (a *App) engine(project ...string) ([]engines.Engine, error) {
	pe := a.Packs
	proj := ""
	if len(project) > 0 {
		proj = project[0]
	}
	if proj != "" && a.Store != nil {
		if p, pErr := a.Store.Project(proj); pErr == nil && len(p.PackPins) > 0 {
			var filteredPacks []*packs.Pack
			for _, pk := range pe.Packs {
				if pinnedVer, ok := p.PackPins[pk.Name]; ok {
					if pk.Version == pinnedVer {
						filteredPacks = append(filteredPacks, pk)
					}
				} else {
					filteredPacks = append(filteredPacks, pk)
				}
			}
			pe.Packs = filteredPacks
		}
	}
	dis, sev, err := a.Store.RuleOverridesFor(proj)
	if err != nil {
		return nil, err
	}
	approvedIgnores := map[string]int64{}
	nowUnix := time.Now().Unix()
	if apps, err := a.Store.Approvals("approved"); err == nil {
		for _, ap := range apps {
			if ap.Kind == "demote_rule" || ap.Kind == "rule_demotion" {
				var p struct{ Rule string }
				if json.Unmarshal(ap.Payload, &p) == nil && p.Rule != "" {
					if _, ok := sev[p.Rule]; !ok {
						sev[p.Rule] = engines.Info
					}
				}
			}
			if ap.Kind == "ignore" || ap.Kind == "sentinix:ignore" {
				var ign struct {
					Project     string `json:"project"`
					File        string `json:"file"`
					Line        int    `json:"line"`
					Rule        string `json:"rule"`
					Fingerprint string `json:"fingerprint"`
					Until       int64  `json:"until"`
				}
				if json.Unmarshal(ap.Payload, &ign) == nil {
					if proj != "" && ign.Project != "" && ign.Project != proj && ign.Project != "*" {
						continue
					}
					if ign.File != "" && ign.Line > 0 && ign.Until > nowUnix && ign.Fingerprint != "" {
						rule := ign.Rule
						if rule == "" {
							rule = "*"
						}
						key := filepath.ToSlash(ign.File) + ":" + strconv.Itoa(ign.Line) + ":" + rule + ":" + ign.Fingerprint
						approvedIgnores[key] = ign.Until
						approvedIgnores[filepath.ToSlash(ign.File)+":"+strconv.Itoa(ign.Line)+":*:"+ign.Fingerprint] = ign.Until
					}
				}
			}
		}
	}
	pe.Disabled, pe.Override = dis, sev
	sec := engines.Secrets{ApprovedIgnores: approvedIgnores}
	out := []engines.Engine{pe, sec}
	if a.OSV != nil {
		out = append(out, advisories.Engine{Client: *a.OSV})
	}
	if a.HuntMaxCalls > 0 && proj != "" && len(a.AIModels) > 0 {
		if p, err := a.Store.Project(proj); err == nil && p.AI {
			out = append(out, &hunt.Engine{Models: a.AIModels, Policy: p.ModelPolicy, MaxCalls: a.HuntMaxCalls})
		}
	}
	return out, nil
}

const MaxWebhookBodyBytes = 5 << 20 // 5 MiB bounded pre-auth read

func (a *App) webhook(w http.ResponseWriter, r *http.Request) {
	h, ok := a.Hosts[r.PathValue("name")]
	if !ok {
		http.NotFound(w, r)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxWebhookBodyBytes))
	if err != nil {
		http.Error(w, "bad body", 400)
		return
	}

	// Semaphore limits concurrent parsing if configured, and is released immediately after parsing
	if a.webhookSem != nil {
		select {
		case a.webhookSem <- struct{}{}:
			defer func() { <-a.webhookSem }()
		default:
			// If semaphore is saturated, do not return 429; proceed to durable queue
		}
	}

	// Verify signature before touching dedupe table or semaphore
	ev, err := h.Parse(r, body)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	if ev == nil {
		w.WriteHeader(204)
		return
	}

	// Dedupe check: verify signature before touching dedupe table; bound and persist dedupe set
	delID := deliveryID(r)
	// The delivery-id header is unsigned, so a captured signed body can be replayed under fresh ids.
	// Also dedupe on the signed content itself (same 24h window).
	sum := sha256.Sum256(append([]byte(h.Name()+"\x00"), body...))
	bodyID := "body:" + hex.EncodeToString(sum[:16])
	for _, id := range []string{delID, bodyID} {
		if id == "" {
			continue
		}
		if seen, _ := a.Store.HasDelivery(id, 24*3600); seen {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	b, err := json.Marshal(ev)
	if err != nil {
		http.Error(w, "bad event", http.StatusBadRequest)
		return
	}

	// Enqueue first, then record the delivery ID
	if _, err := a.Store.EnqueueJob(h.Name(), string(b)); err != nil {
		http.Error(w, "queue error", http.StatusInternalServerError)
		return
	}
	if delID != "" {
		_ = a.Store.RecordDelivery(delID, 24*3600, 10000)
	}
	_ = a.Store.RecordDelivery(bodyID, 24*3600, 10000)

	w.WriteHeader(http.StatusAccepted)
}

func (a *App) review(ctx context.Context, h githost.Host, ev *githost.Event) {
	a.sem <- struct{}{}
	defer func() { <-a.sem }()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	log.Printf("%s: review %s#%s", h.Name(), ev.Repo, ev.PR)
	h.Status(ctx, ev, githost.Pending, "scanning")
	user, token := h.CloneAuth()
	projID := ""
	if p, pErr := a.Store.ProjectByRepo(h.Name(), ev.RepoPath); pErr == nil {
		projID = p.ID
	}
	eng, err := a.engine(projID)
	if err == nil {
		start := time.Now()
		var res *scheduler.Result
		if res, err = scheduler.Review(ctx, ev, user, token, eng...); err == nil {
			a.finishReview(ctx, h, ev, res, time.Since(start))
			return
		}
	}
	log.Printf("%s: %v", h.Name(), err)
	var proj *store.Project
	if p, pErr := a.Store.ProjectByRepo(h.Name(), ev.RepoPath); pErr == nil {
		proj = &p
	}
	blocking, _ := a.gate(proj)
	if blocking {
		h.Status(ctx, ev, githost.Failure, "scan error") // errors fail closed
	} else {
		h.Status(ctx, ev, githost.Neutral, "scan error (report only)")
	}
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

func FormatReviewComment(blocking bool, threshold engines.Severity, allNew, resolved []engines.Finding, ran, skipped []string, uploads []store.IngestUpload, expectedTools []string) string {
	var buf bytes.Buffer
	buf.WriteString("<!-- sentinix-review-marker -->\n")
	buf.WriteString("### Sentinix\n")
	if blocking {
		fmt.Fprintf(&buf, "Merge is blocked by new findings at **%s** or above.\n\n", threshold)
	} else {
		buf.WriteString("Report only: these findings do not block the merge.\n\n")
	}

	grouped := policy.GroupByCategory(allNew)

	// 1. Security section (open by default)
	secFindings := grouped[engines.CategorySecurity]
	if len(secFindings) > 0 {
		fmt.Fprintf(&buf, "#### Security (%d)\n```\n", len(secFindings))
		shownSec := secFindings
		if len(shownSec) > 50 {
			shownSec = shownSec[:50]
		}
		for _, f := range shownSec {
			fmt.Fprintf(&buf, "[%s] %s %s:%d %s\n", f.Severity, report.SanitizeField(f.Rule), report.SanitizeField(f.File), f.Line, report.SanitizeField(f.Message))
		}
		buf.WriteString("```\n")
		if len(secFindings) > 50 {
			fmt.Fprintf(&buf, "%d more not shown.\n", len(secFindings)-50)
		}
		buf.WriteString("\n")
	}

	// 2. Non-security sections (collapsed by default)
	nonSecCats := []string{engines.CategoryArchitecture, engines.CategoryAccessibility, engines.CategoryStyle, engines.CategoryQuality}
	for _, cat := range nonSecCats {
		catFindings := grouped[cat]
		if len(catFindings) > 0 {
			title := strings.Title(cat)
			fmt.Fprintf(&buf, "<details><summary>%s (%d)</summary>\n\n```\n", title, len(catFindings))
			shown := catFindings
			if len(shown) > 50 {
				shown = shown[:50]
			}
			for _, f := range shown {
				fmt.Fprintf(&buf, "[%s] %s %s:%d %s\n", f.Severity, report.SanitizeField(f.Rule), report.SanitizeField(f.File), f.Line, report.SanitizeField(f.Message))
			}
			buf.WriteString("```\n")
			if len(catFindings) > 50 {
				fmt.Fprintf(&buf, "%d more not shown.\n", len(catFindings)-50)
			}
			buf.WriteString("</details>\n\n")
		}
	}

	if len(allNew) == 0 {
		buf.WriteString("No new findings introduced by this change.\n\n")
	}

	if len(resolved) > 0 {
		fmt.Fprintf(&buf, "<details><summary>Resolved findings (%d)</summary>\n\n", len(resolved))
		for _, f := range resolved {
			fmt.Fprintf(&buf, "- ~%s~ in `%s:%d`\n", report.SanitizeField(f.Rule), report.SanitizeField(f.File), f.Line)
		}
		buf.WriteString("</details>\n\n")
	}

	// 3. Coverage footer
	buf.WriteString("<details><summary>Coverage</summary>\n\n")
	if len(ran) > 0 {
		var sanRan []string
		for _, r := range ran {
			sanRan = append(sanRan, report.SanitizeField(r))
		}
		fmt.Fprintf(&buf, "Engines run: %s.\n", strings.Join(sanRan, ", "))
	}
	if len(skipped) > 0 {
		var sanSkipped []string
		for _, s := range skipped {
			sanSkipped = append(sanSkipped, report.SanitizeField(s))
		}
		fmt.Fprintf(&buf, "Skipped files: %s.\n", strings.Join(sanSkipped, ", "))
	}
	buf.WriteString("Tool uploads:\n")

	if len(expectedTools) == 0 {
		expectedTools = []string{"detekt", "konsist", "ktlint", "lint"}
	}

	upMap := make(map[string]store.IngestUpload)
	for _, u := range uploads {
		upMap[strings.ToLower(u.Engine)] = u
	}

	seenTools := make(map[string]bool)
	for _, tool := range expectedTools {
		toolLower := strings.ToLower(tool)
		seenTools[toolLower] = true
		sanTool := report.SanitizeField(tool)
		if u, ok := upMap[toolLower]; ok {
			if u.ParseStatus == "success" {
				if u.Attested {
					tStr := time.Unix(u.ReceivedAt, 0).UTC().Format("15:04:05 UTC")
					fmt.Fprintf(&buf, "- %s: received (%s)\n", sanTool, tStr)
				} else {
					fmt.Fprintf(&buf, "- %s: uploaded by CI (unverified)\n", sanTool)
				}
			} else {
				fmt.Fprintf(&buf, "- %s: failed to parse\n", sanTool)
			}
		} else {
			fmt.Fprintf(&buf, "- %s: not received\n", sanTool)
		}
	}

	for _, u := range uploads {
		uLower := strings.ToLower(u.Engine)
		if !seenTools[uLower] {
			sanEngine := report.SanitizeField(u.Engine)
			if u.ParseStatus == "success" {
				if u.Attested {
					tStr := time.Unix(u.ReceivedAt, 0).UTC().Format("15:04:05 UTC")
					fmt.Fprintf(&buf, "- %s: received (%s)\n", sanEngine, tStr)
				} else {
					fmt.Fprintf(&buf, "- %s: uploaded by CI (unverified)\n", sanEngine)
				}
			} else {
				fmt.Fprintf(&buf, "- %s: failed to parse\n", sanEngine)
			}
		}
	}

	buf.WriteString("\nA missing upload is not covered and never reads as clean.\n")
	buf.WriteString("</details>\n")

	return buf.String()
}

// FormatFallbackComment produces a comment when no review scan result exists yet.
// It omits the blocked/clean header, says "scan pending", and renders coverage.
func FormatFallbackComment(uploads []store.IngestUpload, expectedTools []string) string {
	var buf bytes.Buffer
	buf.WriteString("<!-- sentinix-review-marker -->\n")
	buf.WriteString("### Sentinix\n\n")
	buf.WriteString("scan pending\n\n")

	// Coverage footer
	buf.WriteString("<details><summary>Coverage</summary>\n\n")
	buf.WriteString("Tool uploads:\n")

	if len(expectedTools) == 0 {
		expectedTools = []string{"detekt", "konsist", "ktlint", "lint"}
	}

	upMap := make(map[string]store.IngestUpload)
	for _, u := range uploads {
		upMap[strings.ToLower(u.Engine)] = u
	}

	seenTools := make(map[string]bool)
	for _, tool := range expectedTools {
		toolLower := strings.ToLower(tool)
		seenTools[toolLower] = true
		sanTool := report.SanitizeField(tool)
		if u, ok := upMap[toolLower]; ok {
			if u.ParseStatus == "success" {
				if u.Attested {
					tStr := time.Unix(u.ReceivedAt, 0).UTC().Format("15:04:05 UTC")
					fmt.Fprintf(&buf, "- %s: received (%s)\n", sanTool, tStr)
				} else {
					fmt.Fprintf(&buf, "- %s: uploaded by CI (unverified)\n", sanTool)
				}
			} else {
				fmt.Fprintf(&buf, "- %s: failed to parse\n", sanTool)
			}
		} else {
			fmt.Fprintf(&buf, "- %s: not received\n", sanTool)
		}
	}

	for _, u := range uploads {
		uLower := strings.ToLower(u.Engine)
		if !seenTools[uLower] {
			sanEngine := report.SanitizeField(u.Engine)
			if u.ParseStatus == "success" {
				if u.Attested {
					tStr := time.Unix(u.ReceivedAt, 0).UTC().Format("15:04:05 UTC")
					fmt.Fprintf(&buf, "- %s: received (%s)\n", sanEngine, tStr)
				} else {
					fmt.Fprintf(&buf, "- %s: uploaded by CI (unverified)\n", sanEngine)
				}
			} else {
				fmt.Fprintf(&buf, "- %s: failed to parse\n", sanEngine)
			}
		}
	}

	buf.WriteString("\nA missing upload is not covered and never reads as clean.\n")
	buf.WriteString("</details>\n")

	return buf.String()
}

func (a *App) finishReview(ctx context.Context, h githost.Host, ev *githost.Event, res *scheduler.Result, dur time.Duration) {
	var proj *store.Project
	if p, err := a.Store.ProjectByRepo(h.Name(), ev.RepoPath); err == nil {
		proj = &p
	}
	blocking, threshold := a.gate(proj)
	if proj != nil && a.Store != nil && ev != nil && ev.PR != "" && len(res.Ran) > 0 {
		_ = a.Store.SavePRReviewResult(proj.ID, ev.PR, res.Commit, res.Ran, res.Skipped, res.New, res.Resolved)
	}

	var uploads []store.IngestUpload
	if proj != nil && a.Store != nil {
		uploads, _ = a.Store.IngestUploads(proj.ID, res.Commit)
	}

	allNew := append([]engines.Finding{}, res.New...)
	seenFp := make(map[string]bool)
	for _, f := range allNew {
		seenFp[f.Fingerprint()] = true
	}

	for _, u := range uploads {
		if u.ParseStatus == "success" && u.FindingsJSON != "" {
			var upFs []engines.Finding
			if json.Unmarshal([]byte(u.FindingsJSON), &upFs) == nil {
				for _, f := range upFs {
					fp := f.Fingerprint()
					if !seenFp[fp] {
						seenFp[fp] = true
						allNew = append(allNew, f)
					}
				}
			}
		}
	}

	if proj != nil && a.Store != nil {
		if rulePols, err := a.Store.RulePolicies(proj.ID); err == nil && len(rulePols) > 0 {
			evFindings := policy.EvaluateFindings(allNew, &policy.ProjectPolicy{Rules: rulePols})
			allNew = evFindings.Active()
		}
	}
	grouped := policy.GroupByCategory(allNew)
	expectedTools := []string{"detekt", "konsist", "ktlint", "lint"}

	commentBody := FormatReviewComment(blocking, threshold, allNew, res.Resolved, res.Ran, res.Skipped, uploads, expectedTools)
	RecordReviewLatency(dur)
	if err := h.Comment(ctx, ev, commentBody); err != nil {
		log.Printf("%s: comment: %v", h.Name(), err)
	}

	// 1. Overall security status check ("sentinix")
	secFindings := grouped[engines.CategorySecurity]
	state, desc := githost.Success, "no new findings at gate"
	reason := desc
	switch {
	case !blocking:
		desc = fmt.Sprintf("%d new finding(s), report only", len(secFindings))
		reason = desc
	case policy.Fails(secFindings, threshold):
		state, desc = githost.Failure, fmt.Sprintf("%d new finding(s)", len(secFindings))
		reason = desc
	case len(res.Skipped) > 0:
		state, desc = githost.Failure, fmt.Sprintf("skipped changed file(s): %s", strings.Join(res.Skipped, ", "))
		reason = desc
	default:
		if len(res.Ran) > 0 {
			for _, reqName := range a.RequiredEngines {
				found := false
				for _, ran := range res.Ran {
					if ran == reqName {
						found = true
						break
					}
				}
				if !found {
					state, desc = githost.Failure, fmt.Sprintf("missing required engine: %s", reqName)
					reason = desc
					break
				}
			}
		}
	}
	if err := h.StatusNamed(ctx, ev, "sentinix", state, desc); err != nil {
		log.Printf("%s: status: %v", h.Name(), err)
	}

	// 2. Per-category status checks
	pols := store.DefaultCategoryPolicies()
	if proj != nil && a.Store != nil {
		if cp, err := a.Store.CategoryPolicies(proj.ID); err == nil {
			pols = cp
		}
	}

	for _, cat := range engines.ValidCategories {
		pol := pols[cat]
		if pol.Mode == "off" {
			continue
		}
		catFindings := grouped[cat]
		thresh, _ := engines.ParseSeverity(pol.Threshold)
		catState, catDesc := githost.Success, fmt.Sprintf("no new %s findings at gate", cat)
		if pol.Mode == "block" {
			if policy.CategoryFails(catFindings, cat, policy.CategoryPolicy{Mode: pol.Mode, Threshold: thresh}) {
				catState, catDesc = githost.Failure, fmt.Sprintf("%d new %s finding(s)", len(catFindings), cat)
			}
		} else { // comment
			if len(catFindings) > 0 {
				catDesc = fmt.Sprintf("%d new %s finding(s), comment only", len(catFindings), cat)
			} else {
				catDesc = fmt.Sprintf("no new %s findings", cat)
			}
		}
		_ = h.StatusNamed(ctx, ev, "sentinix/"+cat, catState, catDesc)
	}
	modeStr := "report"
	if blocking {
		modeStr = "block"
	}
	outcome := "success"
	if state == githost.Failure {
		outcome = "failure"
	}
	projID := ""
	if proj != nil {
		projID = proj.ID
	}
	if a.Store != nil {
		auditPayload, _ := json.Marshal(map[string]any{
			"commit":        res.Commit,
			"engines":       strings.Join(res.Ran, ","),
			"pack_versions": packVersions(a.Packs),
			"skipped_files": res.Skipped,
			"mode":          modeStr,
			"outcome":       outcome,
			"reason":        reason,
		})
		a.Store.AddAudit(store.Audit{
			Actor:        "sentinix",
			Action:       "pr_scan_gate",
			Target:       projID,
			Commit:       res.Commit,
			Source:       modeStr,
			After:        string(auditPayload),
			Engines:      strings.Join(res.Ran, ","),
			PackVersions: packVersions(a.Packs),
			SkippedFiles: strings.Join(res.Skipped, ", "),
			Mode:         modeStr,
			Outcome:      outcome,
			Reason:       reason,
		})
	}
	if proj != nil {
		sc := store.Scan{
			ProjectID:    proj.ID,
			Mode:         "pr",
			Commit:       res.Commit,
			PR:           ev.PR,
			DurationMS:   dur.Milliseconds(),
			New:          len(res.New),
			Packs:        packVersions(a.Packs),
			Engines:      strings.Join(res.Ran, ","),
			GateDecision: state.String(),
			Reason:       reason,
			Skipped:      strings.Join(res.Skipped, ", "),
		}
		a.Store.RecordPRScan(sc)
		a.Store.AddSurface(proj.ID, res.Commit, ev.PR, res.Surface)
		for _, s := range res.Suppressions {
			a.ProposeInlineSuppression(proj.ID, s)
		}
	}
}

// RefreshPRReview updates the PR comment and commit statuses using the latest uploads.
func (a *App) RefreshPRReview(ctx context.Context, proj *store.Project, commit, pr string) error {
	if proj == nil {
		return nil
	}
	h, ok := a.Hosts[proj.Host]
	if !ok {
		return nil
	}
	ev := &githost.Event{
		RepoPath:       proj.Repo,
		Repo:           proj.Repo,
		PR:             pr,
		HeadSHA:        commit,
		ConfiguredHost: h.ConfiguredHost(),
	}

	ran, skipped, newFs, resolvedFs, found, err := a.Store.GetPRReviewResult(proj.ID, pr, commit)
	if err != nil || !found {
		// If none exists, post only the coverage footer and leave existing status untouched
		uploads, _ := a.Store.IngestUploads(proj.ID, commit)
		expectedTools := []string{"detekt", "konsist", "ktlint", "lint"}

		grouped := make(map[string][]engines.Finding)
		for _, u := range uploads {
			if u.ParseStatus == "success" && u.FindingsJSON != "" {
				var upFs []engines.Finding
				if json.Unmarshal([]byte(u.FindingsJSON), &upFs) == nil {
					for _, f := range upFs {
						cat := f.Category
						if cat == "" {
							cat = engines.CategorySecurity
						}
						grouped[cat] = append(grouped[cat], f)
					}
				}
			}
		}

		commentBody := FormatFallbackComment(uploads, expectedTools)
		_ = h.Comment(ctx, ev, commentBody)

		// Post per-category statuses for non-security categories
		pols := store.DefaultCategoryPolicies()
		if cp, err := a.Store.CategoryPolicies(proj.ID); err == nil {
			pols = cp
		}
		for _, cat := range engines.ValidCategories {
			if cat == engines.CategorySecurity {
				continue // leave existing security status untouched
			}
			pol := pols[cat]
			if pol.Mode == "off" {
				continue
			}
			catFindings := grouped[cat]
			thresh, _ := engines.ParseSeverity(pol.Threshold)
			catState, catDesc := githost.Success, fmt.Sprintf("no new %s findings", cat)
			if pol.Mode == "block" {
				if policy.CategoryFails(catFindings, cat, policy.CategoryPolicy{Mode: pol.Mode, Threshold: thresh}) {
					catState, catDesc = githost.Failure, fmt.Sprintf("%d new %s finding(s)", len(catFindings), cat)
				}
			} else {
				if len(catFindings) > 0 {
					catDesc = fmt.Sprintf("%d new %s finding(s), comment only", len(catFindings), cat)
				}
			}
			_ = h.StatusNamed(ctx, ev, "sentinix/"+cat, catState, catDesc)
		}
		return nil
	}

	res := &scheduler.Result{
		Commit:   commit,
		Ran:      ran,
		Skipped:  skipped,
		New:      newFs,
		Resolved: resolvedFs,
	}
	a.finishReview(ctx, h, ev, res, 0)
	return nil
}

// queueFull runs a full scan of a project's branch (or the given one) and records it.
func (a *App) queueFull(ctx context.Context, p store.Project, branch string) {
	if a.sem == nil {
		workers := a.Workers
		if workers <= 0 {
			workers = 2
		}
		a.sem = make(chan struct{}, workers)
	}
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
	eng, err := a.engine(p.ID)
	if err != nil {
		log.Printf("project %s: %v", p.Name, err)
		return
	}
	res, dur, err := scheduler.Full(ctx, h.RepoURL(p.Repo), branch, user, token, h.ConfiguredHost(), eng...)
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
