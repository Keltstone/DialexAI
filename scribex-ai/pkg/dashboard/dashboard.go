// Package dashboard serves cross-repo doc health with team-scoped RBAC.
//
// repos file: `name path team` per line. rbac file: `sha256(token) role team,team` per line
// (role admin sees all repos, viewer only its teams). Clients send the token as the HTTP Basic password.
package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strings"

	"scribex/pkg/docs"
)

type Repo struct{ Name, Path, Team string }
type Principal struct {
	Hash  string
	Admin bool
	Teams map[string]bool
}
type Health struct {
	Repo, Team         string
	Total, Stale, Dead int
	Drift              float64 // (stale+dead)/total
}

func lines(path string) (out [][]string, err error) {
	b, err := os.ReadFile(path)
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0][0] != '#' {
			out = append(out, f)
		}
	}
	return
}

func LoadRepos(path string) (rs []Repo, err error) {
	ls, err := lines(path)
	for _, f := range ls {
		for len(f) < 3 {
			f = append(f, "")
		}
		rs = append(rs, Repo{f[0], f[1], f[2]})
	}
	return
}

func LoadRBAC(path string) (ps []Principal, err error) {
	ls, err := lines(path)
	for _, f := range ls {
		p := Principal{Hash: f[0], Teams: map[string]bool{}}
		if len(f) > 1 {
			p.Admin = f[1] == "admin"
		}
		if len(f) > 2 {
			for _, t := range strings.Split(f[2], ",") {
				p.Teams[t] = true
			}
		}
		ps = append(ps, p)
	}
	return
}

// HashToken is what goes in the rbac file.
func HashToken(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }

func Measure(r Repo) (Health, error) {
	rep, err := docs.Check(r.Path, false)
	h := Health{Repo: r.Name, Team: r.Team, Total: rep.Total}
	for _, i := range rep.Issues {
		if i.Stale {
			h.Stale++
		} else {
			h.Dead++
		}
	}
	if h.Total > 0 {
		h.Drift = float64(h.Stale+h.Dead) / float64(h.Total)
	}
	return h, err
}

var page = template.Must(template.New("p").Funcs(template.FuncMap{"mul": func(f float64) float64 { return f * 100 }}).Parse(`<!doctype html><meta charset=utf-8><title>Scribex</title>
<style>body{font:14px system-ui;margin:2rem}td,th{padding:.3rem .8rem;text-align:left}</style>
<h1>Scribex doc health</h1><table><tr><th>Repo<th>Team<th>Anchors<th>Stale<th>Dead<th>Drift</tr>
{{range .}}<tr><td>{{.Repo}}<td>{{.Team}}<td>{{.Total}}<td>{{.Stale}}<td>{{.Dead}}<td>{{printf "%.0f%%" (mul .Drift)}}</tr>{{end}}</table>`))

func Handler(repos []Repo, ps []Principal) http.Handler {
	auth := func(r *http.Request) *Principal {
		_, tok, ok := r.BasicAuth()
		if !ok {
			return nil
		}
		h := HashToken(tok)
		for i := range ps {
			if ps[i].Hash == h {
				return &ps[i]
			}
		}
		return nil
	}
	view := func(w http.ResponseWriter, r *http.Request) []Health {
		p := auth(r)
		if p == nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="scribex"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return nil
		}
		hs := []Health{}
		for _, rp := range repos {
			if p.Admin || p.Teams[rp.Team] {
				h, _ := Measure(rp)
				hs = append(hs, h)
			}
		}
		sort.Slice(hs, func(i, j int) bool { return hs[i].Drift > hs[j].Drift })
		return hs
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if hs := view(w, r); hs != nil {
			json.NewEncoder(w).Encode(hs)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if hs := view(w, r); hs != nil {
			page.Execute(w, hs)
		}
	})
	return mux
}
