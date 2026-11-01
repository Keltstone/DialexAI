package dashboard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"scribex/pkg/config"
	"scribex/pkg/doctypes"
	"scribex/pkg/generate"
)

// Generate pages are opt-in (Config.EnableGenerate): they are the only part of the dashboard that
// accepts POST. Every one is admin-only, CSRF-bound to the principal, repo and action, and JS-free.

const (
	maxForm    = 64 << 10
	maxLogKeep = 32 << 10
)

// RunFunc executes an apply as a child of the server and returns its combined output.
type RunFunc func(repo Repo, args []string) (string, error)

type job struct {
	RunID   string
	Started time.Time
	Done    bool
	Err     string
	Log     string
}

type genState struct {
	key  []byte
	mu   sync.Mutex
	jobs map[string]*job // by repo name
	run  RunFunc
}

func newGenState(run RunFunc) *genState {
	k := make([]byte, 32)
	rand.Read(k)
	if run == nil {
		run = execRun
	}
	return &genState{key: k, jobs: map[string]*job{}, run: run}
}

func execRun(repo Repo, args []string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, append([]string{"generate", "-C", repo.Path}, args...)...)
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &capWriter{b: &out}, &capWriter{b: &out}
	err = cmd.Run()
	return out.String(), err
}

type capWriter struct{ b *strings.Builder }

func (c *capWriter) Write(p []byte) (int, error) {
	if c.b.Len() < maxLogKeep {
		c.b.Write(p)
	}
	return len(p), nil
}

func (g *genState) token(p *Principal, repo, action string) string {
	m := hmac.New(sha256.New, g.key)
	m.Write([]byte(p.Hash + "|" + repo + "|" + action))
	return hex.EncodeToString(m.Sum(nil))
}

func (g *genState) valid(p *Principal, repo, action, tok string) bool {
	return hmac.Equal([]byte(g.token(p, repo, action)), []byte(tok))
}

func (s *Server) repoByName(n string) (Repo, bool) {
	for _, r := range s.repos {
		if r.Name == n && !r.Unavailable {
			return r, true
		}
	}
	return Repo{}, false
}

func sameOrigin(r *http.Request) bool {
	if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || u.Host != r.Host {
			return false
		}
	}
	return true
}

func page(s *Server, w http.ResponseWriter, title, body string, refresh bool) {
	s.setSecurityHeaders(w)
	w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'none'; style-src '%s'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'", s.styleHash))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	meta := ""
	if refresh {
		meta = `<meta http-equiv="refresh" content="3">`
	}
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">%s<title>%s</title><style>%s</style></head><body><p><a href="/">Health</a> | <a href="/ai">AI settings</a> | <a href="/doctypes">Doc types</a> | <a href="/generate">Generate docs</a></p>%s</body></html>`,
		meta, html.EscapeString(title), styleCSS, body)
}

func esc(s string) string { return html.EscapeString(s) }

func (s *Server) registerGenerate(mux *http.ServeMux, authenticate func(http.ResponseWriter, *http.Request) (*Principal, string)) {
	g := s.gen
	// admin resolves the principal and the repo for every generate route.
	admin := func(w http.ResponseWriter, r *http.Request) (*Principal, Repo, bool) {
		p, _ := authenticate(w, r)
		if p == nil {
			return nil, Repo{}, false
		}
		if !p.Admin {
			s.sendError(w, r, http.StatusForbidden, "forbidden: admin only")
			return nil, Repo{}, false
		}
		name := r.URL.Query().Get("repo")
		if r.Method == http.MethodPost {
			if !sameOrigin(r) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				s.sendError(w, r, http.StatusForbidden, "forbidden: cross-site or bad content type")
				return nil, Repo{}, false
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxForm)
			if err := r.ParseForm(); err != nil {
				s.sendError(w, r, http.StatusBadRequest, "bad form")
				return nil, Repo{}, false
			}
			name = r.PostForm.Get("repo")
		}
		if name == "" && len(s.repos) == 1 {
			name = s.repos[0].Name
		}
		repo, ok := s.repoByName(name)
		if !ok {
			if r.Method == http.MethodPost {
				s.sendError(w, r, http.StatusNotFound, "unknown repo")
				return nil, Repo{}, false
			}
			s.renderRepoPicker(w, r)
			return nil, Repo{}, false
		}
		if r.Method == http.MethodPost && !g.valid(p, repo.Name, r.URL.Path, r.PostForm.Get("csrf")) {
			s.sendError(w, r, http.StatusForbidden, "forbidden: bad CSRF token")
			return nil, Repo{}, false
		}
		return p, repo, true
	}
	form := func(p *Principal, repo Repo, action string) string {
		return fmt.Sprintf(`<form method="post" action="%s"><input type="hidden" name="repo" value="%s"><input type="hidden" name="csrf" value="%s">`, esc(action), esc(repo.Name), g.token(p, repo.Name, action))
	}

	mux.HandleFunc("/ai", func(w http.ResponseWriter, r *http.Request) {
		p, repo, ok := admin(w, r)
		if !ok {
			return
		}
		if r.Method == http.MethodPost {
			kv := map[string]string{}
			for _, k := range []string{"provider", "model", "cli_command", "max_calls", "max_tokens", "review", "review_mode", "branch_prefix"} {
				kv["ai."+k] = strings.TrimSpace(r.PostForm.Get(k))
			}
			if err := config.SetMany(repo.Path, kv); err != nil {
				page(s, w, "AI settings", `<p class="error">`+esc(err.Error())+"</p>", false)
				return
			}
			http.Redirect(w, r, "/ai?repo="+url.QueryEscape(repo.Name)+"&saved=1", http.StatusSeeOther)
			return
		}
		c, err := config.Load(repo.Path)
		if err != nil {
			s.sendError(w, r, http.StatusInternalServerError, "cannot read config")
			return
		}
		a := c.AI
		sel := func(name, cur string, opts ...string) string {
			var b strings.Builder
			fmt.Fprintf(&b, `<select name="%s">`, name)
			for _, o := range opts {
				m := ""
				if o == cur {
					m = " selected"
				}
				fmt.Fprintf(&b, `<option%s>%s</option>`, m, esc(o))
			}
			return b.String() + "</select>"
		}
		in := func(name, val string) string {
			return fmt.Sprintf(`<input name="%s" value="%s" maxlength="100">`, name, esc(val))
		}
		var b strings.Builder
		b.WriteString("<h1>AI settings: " + esc(repo.Name) + "</h1>")
		if r.URL.Query().Get("saved") == "1" {
			b.WriteString("<p>Saved.</p>")
		}
		b.WriteString("<p>Non-secret keys only. API keys stay in the environment or CI secrets; they are never shown here.</p>")
		b.WriteString(form(p, repo, "/ai") + "<table>")
		row := func(l, c string) { b.WriteString("<tr><td>" + l + "</td><td>" + c + "</td></tr>") }
		row("Backend", sel("provider", a.Provider, "cli", "ollama", "anthropic", "openai"))
		row("Vendor CLI (backend=cli)", sel("cli_command", a.CLICommand, config.CLINames...))
		row("Model (empty = CLI default)", in("model", a.Model))
		row("Max model calls", in("max_calls", strconv.Itoa(a.MaxCalls)))
		row("Max tokens per reply", in("max_tokens", strconv.Itoa(a.MaxTokens)))
		row("Socratix review of derived docs", sel("review", a.Review, "off", "socratix"))
		row("Review mode", sel("review_mode", a.ReviewMode, "premortem", "redteam", "tenthman"))
		row("Branch prefix", in("branch_prefix", a.BranchPrefix))
		b.WriteString(`</table><button type="submit">Save</button></form>`)
		b.WriteString("<p>CLI backends run under the login of the user running <code>scribex serve</code>.</p>")
		page(s, w, "AI settings", b.String(), false)
	})

	mux.HandleFunc("/doctypes", func(w http.ResponseWriter, r *http.Request) {
		p, repo, ok := admin(w, r)
		if !ok {
			return
		}
		cat, err := doctypes.Load(repo.Path)
		if err != nil {
			s.sendError(w, r, http.StatusInternalServerError, "cannot read doctypes: "+err.Error())
			return
		}
		if r.Method == http.MethodPost {
			f := cat.File
			for _, t := range cat.Types {
				id := t.ID
				if r.PostForm.Get("seen_"+id) == "" {
					continue
				}
				en := r.PostForm.Get("en_"+id) == "on"
				inc, exc := splitCSV(r.PostForm.Get("inc_"+id)), splitCSV(r.PostForm.Get("exc_"+id))
				tpl, rev := strings.TrimSpace(r.PostForm.Get("tpl_"+id)), r.PostForm.Get("rev_"+id)
				if rev == "off" {
					rev = ""
				}
				e := entryFor(&f, id)
				e.Enabled, e.Include, e.Exclude, e.Template, e.Review = &en, &inc, &exc, &tpl, &rev
			}
			nc, probs := doctypes.Build(f)
			probs = append(probs, doctypes.Validate(repo.Path, nc)...)
			if len(probs) > 0 {
				page(s, w, "Doc types", `<p class="error">`+esc(strings.Join(probs, "; "))+"</p>", false)
				return
			}
			if err := doctypes.Save(repo.Path, f); err != nil {
				s.sendError(w, r, http.StatusInternalServerError, "save failed")
				return
			}
			http.Redirect(w, r, "/doctypes?repo="+url.QueryEscape(repo.Name)+"&saved=1", http.StatusSeeOther)
			return
		}
		var b strings.Builder
		b.WriteString("<h1>Doc types: " + esc(repo.Name) + "</h1>")
		if r.URL.Query().Get("saved") == "1" {
			b.WriteString("<p>Saved to .scribex/doctypes.json.</p>")
		}
		b.WriteString(form(p, repo, "/doctypes") + "<table><tr><th>On</th><th>Type</th><th>Grounding</th><th>Include globs</th><th>Exclude globs</th><th>Template</th><th>Review</th></tr>")
		for _, t := range cat.Types {
			chk := ""
			if t.Enabled {
				chk = " checked"
			}
			rev := "off"
			if t.Review != "" {
				rev = t.Review
			}
			ro := ""
			if t.Grounding == "symbol" {
				ro = " (drift-gated)"
			} else if t.Grounding == "derived" {
				ro = " (AI draft)"
			}
			fmt.Fprintf(&b, `<tr><td><input type="hidden" name="seen_%[1]s" value="1"><input type="checkbox" name="en_%[1]s"%[2]s></td><td>%[3]s<br><small>%[1]s &rarr; %[4]s</small></td><td>%[5]s%[6]s</td><td><input name="inc_%[1]s" value="%[7]s"></td><td><input name="exc_%[1]s" value="%[8]s"></td><td><input name="tpl_%[1]s" value="%[9]s"></td><td>`,
				esc(t.ID), chk, esc(t.Title), esc(t.Output), esc(t.Grounding), ro, esc(strings.Join(t.Include, ",")), esc(strings.Join(t.Exclude, ",")), esc(t.Template))
			fmt.Fprintf(&b, `<select name="rev_%s">`, esc(t.ID))
			for _, o := range []string{"off", "socratix"} {
				m := ""
				if o == rev {
					m = " selected"
				}
				fmt.Fprintf(&b, "<option%s>%s</option>", m, o)
			}
			b.WriteString("</select></td></tr>")
		}
		b.WriteString(`</table><button type="submit">Save</button></form>`)
		page(s, w, "Doc types", b.String(), false)
	})

	mux.HandleFunc("/generate", func(w http.ResponseWriter, r *http.Request) {
		p, repo, ok := admin(w, r)
		if !ok {
			return
		}
		cat, err := doctypes.Load(repo.Path)
		if err != nil {
			s.sendError(w, r, http.StatusInternalServerError, "cannot read doctypes")
			return
		}
		var b strings.Builder
		b.WriteString("<h1>Generate docs: " + esc(repo.Name) + "</h1><p>Step 1 writes only a plan (no model call). Step 2, on the plan page, writes docs.</p>")
		b.WriteString(form(p, repo, "/generate/plan") + "<p>Doc types:</p>")
		for _, t := range cat.Types {
			chk := ""
			if t.Enabled {
				chk = " checked"
			}
			fmt.Fprintf(&b, `<p><label><input type="checkbox" name="type" value="%s"%s> %s <small>(%s)</small></label></p>`, esc(t.ID), chk, esc(t.Title), esc(t.Grounding))
		}
		b.WriteString(`<p>Include globs <input name="include"> Exclude globs <input name="exclude"></p><button type="submit">Create plan</button></form><h2>Runs</h2>`)
		for _, id := range listRuns(repo.Path) {
			fmt.Fprintf(&b, `<p><a href="/generate/run?repo=%s&amp;id=%s">%s</a></p>`, url.QueryEscape(repo.Name), url.QueryEscape(id), esc(id))
		}
		page(s, w, "Generate docs", b.String(), false)
	})

	mux.HandleFunc("/generate/plan", func(w http.ResponseWriter, r *http.Request) {
		_, repo, ok := admin(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodPost {
			s.sendError(w, r, http.StatusMethodNotAllowed, "POST only")
			return
		}
		cat, err := doctypes.Load(repo.Path)
		if err != nil {
			s.sendError(w, r, http.StatusInternalServerError, "cannot read doctypes")
			return
		}
		c, err := config.Load(repo.Path)
		if err != nil {
			s.sendError(w, r, http.StatusInternalServerError, "cannot read config")
			return
		}
		var types []string
		for _, t := range r.PostForm["type"] {
			if _, ok := cat.Get(t); ok {
				types = append(types, t)
			}
		}
		pl, err := generate.MakePlan(repo.Path, cat, generate.Options{Root: repo.Path, Types: types, Include: splitCSV(r.PostForm.Get("include")), Exclude: splitCSV(r.PostForm.Get("exclude")), AI: c.AI})
		if err == nil {
			err = generate.Save(repo.Path, pl)
		}
		if err != nil {
			page(s, w, "Generate docs", `<p class="error">`+esc(err.Error())+"</p>", false)
			return
		}
		http.Redirect(w, r, "/generate/run?repo="+url.QueryEscape(repo.Name)+"&id="+url.QueryEscape(pl.RunID), http.StatusSeeOther)
	})

	mux.HandleFunc("/generate/run", func(w http.ResponseWriter, r *http.Request) {
		p, repo, ok := admin(w, r)
		if !ok {
			return
		}
		id := r.URL.Query().Get("id")
		pl, err := generate.LoadPlan(repo.Path, id)
		if err != nil {
			s.sendError(w, r, http.StatusNotFound, "no such run")
			return
		}
		g.mu.Lock()
		j := g.jobs[repo.Name]
		var jc job
		if j != nil && j.RunID == id {
			jc = *j
		}
		g.mu.Unlock()
		running := jc.RunID != "" && !jc.Done
		var b strings.Builder
		fmt.Fprintf(&b, "<h1>Run %s</h1><pre>%s</pre>", esc(id), esc(pl.Markdown()))
		b.WriteString("<h2>Status</h2><ul>")
		for _, it := range pl.Items {
			fmt.Fprintf(&b, "<li>%s / %s: %s</li>", esc(it.Type), esc(it.Unit), esc(it.Status))
		}
		b.WriteString("</ul>")
		switch {
		case running:
			b.WriteString("<p>Running... this page refreshes.</p>")
		case jc.RunID != "":
			fmt.Fprintf(&b, "<p>Finished. Error: %s</p><pre>%s</pre>", esc(jc.Err), esc(jc.Log))
		}
		if !running {
			b.WriteString(form(p, repo, "/generate/apply") + fmt.Sprintf(`<input type="hidden" name="id" value="%s">`, esc(id)))
			if !pl.Local {
				b.WriteString(`<p><label><input type="checkbox" name="confirm" value="yes"> I confirm the ` + strconv.Itoa(len(pl.WouldSend)) + ` file(s) listed above may be sent to ` + esc(pl.Backend) + `.</label></p>`)
			}
			b.WriteString(`<p><label><input type="checkbox" name="branch" value="yes"> Commit result to a new branch</label></p><button type="submit">Apply</button></form>`)
		}
		page(s, w, "Run", b.String(), running)
	})

	mux.HandleFunc("/generate/apply", func(w http.ResponseWriter, r *http.Request) {
		_, repo, ok := admin(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodPost {
			s.sendError(w, r, http.StatusMethodNotAllowed, "POST only")
			return
		}
		id := r.PostForm.Get("id")
		pl, err := generate.LoadPlan(repo.Path, id)
		if err != nil {
			s.sendError(w, r, http.StatusNotFound, "no such run")
			return
		}
		if !pl.Local && r.PostForm.Get("confirm") != "yes" {
			s.sendError(w, r, http.StatusBadRequest, "confirm that the listed files may leave this machine")
			return
		}
		args := []string{"-resume", id, "-apply"}
		if !pl.Local {
			args = append(args, "-confirm-send")
		}
		if r.PostForm.Get("branch") == "yes" {
			args = append(args, "-branch")
		}
		g.mu.Lock()
		if j := g.jobs[repo.Name]; j != nil && !j.Done {
			g.mu.Unlock()
			s.sendError(w, r, http.StatusConflict, "a job is already running for this repo")
			return
		}
		j := &job{RunID: id, Started: s.config.Now()}
		g.jobs[repo.Name] = j
		g.mu.Unlock()
		go func() {
			out, err := g.run(repo, args)
			g.mu.Lock()
			j.Log, j.Done = out, true
			if err != nil {
				j.Err = err.Error()
			}
			g.mu.Unlock()
		}()
		http.Redirect(w, r, "/generate/run?repo="+url.QueryEscape(repo.Name)+"&id="+url.QueryEscape(id), http.StatusSeeOther)
	})

	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		p, _ := authenticate(w, r)
		if p == nil {
			return
		}
		if !p.Admin {
			s.sendError(w, r, http.StatusForbidden, "forbidden: admin only")
			return
		}
		repo, ok := s.repoByName(r.URL.Query().Get("repo"))
		id := strings.TrimPrefix(r.URL.Path, "/api/runs/")
		pl, err := generate.LoadPlan(repo.Path, id)
		if !ok || err != nil {
			s.sendError(w, r, http.StatusNotFound, "no such run")
			return
		}
		counts := map[string]int{}
		for _, it := range pl.Items {
			counts[it.Status]++
		}
		g.mu.Lock()
		state := "idle"
		if j := g.jobs[repo.Name]; j != nil && j.RunID == id {
			state = "running"
			if j.Done {
				state = "finished"
			}
		}
		g.mu.Unlock()
		s.setSecurityHeaders(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{"run_id": id, "job": state, "items": counts})
	})
}

func (s *Server) renderRepoPicker(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("<h1>Choose a repository</h1>")
	for _, rp := range s.repos {
		if !rp.Unavailable {
			fmt.Fprintf(&b, `<p><a href="%s?repo=%s">%s</a></p>`, esc(r.URL.Path), url.QueryEscape(rp.Name), esc(rp.Name))
		}
	}
	page(s, w, "Choose repository", b.String(), false)
}

func splitCSV(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func entryFor(f *doctypes.File, id string) *doctypes.Entry {
	for i := range f.Types {
		if f.Types[i].ID == id {
			return &f.Types[i]
		}
	}
	f.Types = append(f.Types, doctypes.Entry{ID: id})
	return &f.Types[len(f.Types)-1]
}

func listRuns(root string) []string {
	ents, err := os.ReadDir(filepath.Join(root, ".scribex", "runs"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && generate.ValidRunID(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
