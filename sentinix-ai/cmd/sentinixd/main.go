// sentinixd: the Sentinix server. Webhooks, API, scheduler and dashboard in one process.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	"sentinix/pkg/sandbox"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/server"
	"sentinix/pkg/store"
	"sentinix/pkg/web"
)

var version = "1.0.0"

func main() {
	cfgPath := flag.String("config", "sentinix.json", "JSON list of git hosts; values like \"$VAR\" are read from the environment")
	addr := flag.String("addr", ":8080", "listen address")
	dsn := flag.String("db", "sqlite:sentinix.db", "postgres://… or sqlite:<path>")
	workers := flag.Int("workers", 2, "concurrent scans")
	failOn := flag.String("fail-on", "high", "fail PR status at or above this severity")
	packDir := flag.String("packs", "", "directory of signed extra rule packs")
	trust := flag.String("trust", "", "file of trusted pack public keys")
	osv := flag.String("osv", "", "OSV API URL (https://api.osv.dev or a mirror); empty disables dependency advisories")
	rescan := flag.Duration("advisory-interval", 6*time.Hour, "how often stored SBOMs are re-matched against OSV")
	integ := flag.String("integrations", "", "JSON file with Slack, Teams, webhook, Jira and Linear settings")
	huntCalls := flag.Int("hunt-max-calls", 0, "model calls per scan for the advisory llm-audit engine on AI-enabled projects (0: off)")
	aiCfg := flag.String("ai", "", "JSON list of OpenAI-compatible models for the optional AI add-on")
	sbImage := flag.String("sandbox-image", "", "container image with scanners; when set, external engines and generated tests run in it with no network")
	enterprise := flag.Bool("enterprise", false, "enterprise mode: requires sandbox for external engines")
	enforce := flag.String("enforcement", "block", "default for repos: block (failing status stops the merge) or report (comment only); projects can override")
	maxFetchBytes := flag.Int64("max-fetch-bytes", 500<<20, "maximum fetch and working-tree size in bytes")
	reqEngines := flag.String("required-engines", "secrets,packs,semgrep,gitleaks,osv-scanner", "comma-separated engines required to run on every scan; missing engines fail the PR")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		log.Printf("Sentinix AI Daemon (sentinixd) version %s\n", version)
		return
	}
	if *enterprise && *sbImage == "" {
		log.Fatal("enterprise mode requires -sandbox-image for external engines")
	}
	if *enforce != "block" && *enforce != "report" {
		log.Fatal("-enforcement must be block or report")
	}
	scheduler.MaxFetchBytes = *maxFetchBytes

	min, err := engines.ParseSeverity(*failOn)
	check(err)
	hosts, err := loadHosts(*cfgPath)
	check(err)
	pe, err := packs.NewEngine(*packDir, *trust)
	check(err)
	st, err := store.Open(*dsn)
	check(err)

	key := os.Getenv("SENTINIX_SESSION_KEY")
	if len(key) < 32 {
		log.Fatal("SENTINIX_SESSION_KEY must be at least 32 characters")
	}
	if n := scheduler.SweepStaleTemp(time.Hour); n > 0 {
		log.Printf("removed %d stale scan temp dir(s)", n)
	}
	st.StartAuditVerifier(15*time.Minute, log.Printf)
	var requiredEngines []string
	if *reqEngines != "" {
		for _, e := range strings.Split(*reqEngines, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				requiredEngines = append(requiredEngines, e)
			}
		}
	}
	app := &server.App{Static: web.Handler(), Store: st, Hosts: hosts, Packs: pe, FailOn: min, Enforcement: *enforce, Workers: *workers,
		RequiredEngines: requiredEngines,
		Sess:            auth.Sessions{Key: []byte(key), Secure: os.Getenv("SENTINIX_INSECURE_COOKIES") == ""},
		OwnerEmail: os.Getenv("SENTINIX_OWNER_EMAIL"),
		Brand:      report.Brand{Name: os.Getenv("SENTINIX_BRAND_NAME"), Accent: os.Getenv("SENTINIX_BRAND_ACCENT")}}
	if *integ != "" {
		app.Integrations, err = integrations.Load(*integ)
		check(err)
		app.OnScan = func(ctx context.Context, p store.Project, sc store.Scan, fs []store.Finding) {
			for _, e := range app.Integrations.Notify(ctx, p, sc, fs) {
				log.Printf("notify: %v", e)
			}
		}
	}
	if *aiCfg != "" {
		app.AIModels, err = loadModels(*aiCfg)
		check(err)
		app.HuntMaxCalls = *huntCalls
	}
	if *sbImage == "in-container" || (*sbImage == "" && os.Getenv("SENTINIX_CONTAINER") != "") {
		ic := sandbox.InContainer{}
		engines.Sandbox = ic.ScanCmd
		app.TestRunner = ic.Run
	} else if *sbImage != "" {
		bin := os.Getenv("SENTINIX_CONTAINER_BIN")
		if bin == "" {
			bin = "docker"
		}
		if _, err := exec.LookPath(bin); err != nil {
			log.Printf("container binary %q not found on PATH, using InContainer sandbox", bin)
			ic := sandbox.InContainer{}
			engines.Sandbox = ic.ScanCmd
			app.TestRunner = ic.Run
		} else {
			d := sandbox.Docker{Image: *sbImage, Bin: bin}
			engines.Sandbox = d.ScanCmd
			app.TestRunner = d.Run
		}
	}
	if iss := os.Getenv("OIDC_ISSUER"); iss != "" {
		app.OIDC, err = auth.NewOIDC(context.Background(), iss, os.Getenv("OIDC_CLIENT_ID"), os.Getenv("OIDC_CLIENT_SECRET"),
			os.Getenv("OIDC_REDIRECT_URL"), app.Sess, app.Login)
		check(err)
		for _, d := range strings.Split(os.Getenv("SENTINIX_ALLOWED_EMAIL_DOMAINS"), ",") {
			if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
				app.OIDC.AllowedDomains = append(app.OIDC.AllowedDomains, d)
			}
		}
		if len(app.OIDC.AllowedDomains) == 0 {
			log.Print("WARNING: SENTINIX_ALLOWED_EMAIL_DOMAINS unset: any verified email at the IdP can sign in (as developer, no project grants)")
		}
	} else {
		log.Print("OIDC_ISSUER not set: SSO login disabled")
	}

	if *osv != "" {
		app.OSV = &advisories.Client{URL: *osv}
		go func() {
			for range time.Tick(*rescan) {
				if err := app.RescanAdvisories(context.Background()); err != nil {
					log.Print(err)
				}
			}
		}()
	}
	go func() { // scheduled scans, checked once a minute
		for t := range time.Tick(time.Minute) {
			app.Tick(context.Background(), t)
		}
	}()
	log.Printf("listening on %s, %d host(s)", *addr, len(hosts))
	srv := &http.Server{
		Addr:         *addr,
		Handler:      app.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	check(srv.ListenAndServe())
}

func loadHosts(path string) (map[string]githost.Host, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfgs []githost.Config
	if err := json.Unmarshal(data, &cfgs); err != nil {
		return nil, err
	}
	env := func(v string) string {
		if strings.HasPrefix(v, "$") {
			return os.Getenv(v[1:])
		}
		return v
	}
	hosts := map[string]githost.Host{}
	for _, c := range cfgs {
		c.Secret, c.Token = env(c.Secret), env(c.Token)
		h, err := githost.New(c)
		if err != nil {
			return nil, err
		}
		hosts[c.Name] = h
	}
	return hosts, nil
}

func loadModels(path string) ([]ai.Model, error) { return ai.LoadModels(path) }

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
