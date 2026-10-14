// sentinixd: the Sentinix server. Webhooks, API, scheduler and dashboard in one process.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"sentinix/pkg/advisories"
	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/packs"
	"sentinix/pkg/server"
	"sentinix/pkg/store"
)

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
	flag.Parse()

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
	app := &server.App{Store: st, Hosts: hosts, Packs: pe, FailOn: min, Workers: *workers,
		Sess:       auth.Sessions{Key: []byte(key), Secure: os.Getenv("SENTINIX_INSECURE_COOKIES") == ""},
		OwnerEmail: os.Getenv("SENTINIX_OWNER_EMAIL")}
	if iss := os.Getenv("OIDC_ISSUER"); iss != "" {
		app.OIDC, err = auth.NewOIDC(context.Background(), iss, os.Getenv("OIDC_CLIENT_ID"), os.Getenv("OIDC_CLIENT_SECRET"),
			os.Getenv("OIDC_REDIRECT_URL"), app.Sess, app.Login)
		check(err)
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
	check(http.ListenAndServe(*addr, app.Handler()))
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

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
