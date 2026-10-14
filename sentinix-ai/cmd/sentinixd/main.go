// sentinixd: webhook server. One endpoint per configured git host: POST /webhook/<name>.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/policy"
	"sentinix/pkg/report"
	"sentinix/pkg/scheduler"
)

func main() {
	cfgPath := flag.String("config", "sentinix.json", "JSON list of git hosts; values like \"$VAR\" are read from the environment")
	addr := flag.String("addr", ":8080", "listen address")
	workers := flag.Int("workers", 2, "concurrent reviews")
	failOn := flag.String("fail-on", "high", "fail the status at or above this severity")
	flag.Parse()

	min, err := engines.ParseSeverity(*failOn)
	check(err)
	hosts, err := load(*cfgPath)
	check(err)
	sem := make(chan struct{}, *workers)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })
	mux.HandleFunc("POST /webhook/{name}", func(w http.ResponseWriter, r *http.Request) {
		h, ok := hosts[r.PathValue("name")]
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
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			review(ctx, h, ev, min)
		}()
	})
	log.Printf("listening on %s, %d host(s)", *addr, len(hosts))
	check(http.ListenAndServe(*addr, mux))
}

func review(ctx context.Context, h githost.Host, ev *githost.Event, min engines.Severity) {
	log.Printf("%s: review %s#%s", h.Name(), ev.Repo, ev.PR)
	h.Status(ctx, ev, githost.Pending, "scanning")
	user, token := h.CloneAuth()
	res, err := scheduler.Review(ev, user, token)
	if err != nil {
		log.Printf("%s: %v", h.Name(), err)
		h.Status(ctx, ev, githost.Failure, "scan error") // errors fail closed
		return
	}
	shown := res.New
	if len(shown) > 50 {
		shown = shown[:50]
	}
	var buf bytes.Buffer
	buf.WriteString("### Sentinix\n```\n")
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
	if policy.Fails(res.New, min) {
		state, desc = githost.Failure, fmt.Sprintf("%d new finding(s)", len(res.New))
	}
	if err := h.Status(ctx, ev, state, desc); err != nil {
		log.Printf("%s: status: %v", h.Name(), err)
	}
}

func load(path string) (map[string]githost.Host, error) {
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
