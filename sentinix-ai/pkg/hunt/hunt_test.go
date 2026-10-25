package hunt

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"sentinix/pkg/ai"
	"sentinix/pkg/bench"
	"sentinix/pkg/engines"
	"sentinix/pkg/packs"
)

const benchDir = "../../bench/hunt-android"

// fake flags a bug only for the planted (entry, class) pairs, so precision/recall are exact.
type fake struct {
	local bool
	calls *int
	seen  *string
}

func (f fake) Name() string { return "fake" }
func (f fake) Local() bool  { return f.local }
func (f fake) Complete(_ context.Context, _, user string) (string, error) {
	if f.calls != nil {
		*f.calls++
	}
	if f.seen != nil {
		*f.seen = user
	}
	yes := (strings.Contains(user, "MapActivity") && strings.Contains(user, "class: confused-deputy")) ||
		(strings.Contains(user, "Entry point (component+deeplink): DeepLinkActivity") && strings.Contains(user, "class: deeplink-host"))
	if yes {
		return `{"vulnerable":true,"severity":"high","confidence":0.8,"reason":"planted"}`, nil
	}
	return `{"vulnerable":false,"severity":"low","confidence":0.5,"reason":"fine"}`, nil
}

func TestEntrypoints(t *testing.T) {
	eps, err := Entrypoints(benchDir)
	if err != nil || len(eps) != 3 {
		t.Fatalf("%v %v", eps, err)
	}
}

func TestBenchMeetsTargets(t *testing.T) {
	e := &Engine{Models: []ai.Model{fake{local: true}}, Policy: "local-only", MaxCalls: 100}
	m, err := bench.Run(benchDir, []engines.Engine{e})
	if err != nil || !m.Meets(0.80, 0.70) || len(m.FalsePositives) != 0 {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestBudgetAndPolicy(t *testing.T) {
	calls := 0
	e := &Engine{Models: []ai.Model{fake{local: true, calls: &calls}}, Policy: "local-only", MaxCalls: 2}
	if _, err := e.Scan(context.Background(), benchDir); err != nil || calls != 2 || !e.Truncated || e.Gap() == "" {
		t.Fatalf("calls=%d trunc=%v %v", calls, e.Truncated, err)
	}
	cloud := &Engine{Models: []ai.Model{fake{local: false}}, Policy: "local-only", MaxCalls: 9}
	if _, err := cloud.Scan(context.Background(), benchDir); err == nil {
		t.Fatal("cloud model used under local-only")
	}
}

func TestCodeFramedAsData(t *testing.T) {
	var seen string
	e := &Engine{Models: []ai.Model{fake{local: true, seen: &seen}}, Policy: "local-only", MaxCalls: 1}
	e.Scan(context.Background(), benchDir)
	if !strings.Contains(seen, "UNTRUSTED CODE") {
		t.Fatal(seen)
	}
}

// pocModel flags MapActivity, then answers the PoC request with a test file.
type pocModel struct{ fake }

func (p pocModel) Complete(ctx context.Context, sys, user string) (string, error) {
	if strings.Contains(user, "Write one") {
		return `{"path":"poc_test.sh","content":"exit 1"}`, nil
	}
	return p.fake.Complete(ctx, sys, user)
}

func TestPoCReproducedAndUnconfirmed(t *testing.T) {
	run := func(code error) ai.Runner { return func(context.Context, string, []string) error { return code } }
	for _, c := range []struct {
		err  error
		want string
	}{{errTest, "PoC reproduced"}, {nil, "PoC unconfirmed"}} {
		e := &Engine{Models: []ai.Model{pocModel{fake{local: true}}}, Policy: "local-only", MaxCalls: 100, Run: run(c.err), PoCCmd: []string{"sh"}}
		fs, err := e.Scan(context.Background(), benchDir)
		if err != nil || len(fs) != 2 || !strings.HasPrefix(fs[0].Evidence, c.want) {
			t.Fatalf("%+v %v", fs, err)
		}
	}
}

var errTest = errors.New("test failed: exploit works")

func TestSignedPackAddsClass(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	body := `{"name":"x","version":"1","classes":{"component":[{"id":"tapjacking","cwe":"CWE-1021","hint":"overlay on exported activity"}]}}`
	signed, err := packs.Sign([]byte(body), priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPack(signed, nil); err == nil {
		t.Fatal("unsigned-by-trusted pack loaded")
	}
	p, err := LoadPack(signed, []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatal(err)
	}
	e := &Engine{Models: []ai.Model{fake{local: true}}, Policy: "local-only", MaxCalls: 100, Packs: []*Pack{p}}
	if got := e.classesFor(Entry{Kinds: []string{"component"}}); got[len(got)-1] != "tapjacking" || e.cweOf("tapjacking") != "CWE-1021" {
		t.Fatal(got)
	}
	if !strings.Contains(e.hint("tapjacking"), "overlay on exported activity") || e.hint("authz") != "" {
		t.Fatal("hint not scoped to its class")
	}
	bad, _ := packs.Sign([]byte(`{"name":"x","version":"1","classes":{"nope":[{"id":"a-b"}]}}`), priv)
	if _, err := LoadPack(bad, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestVariantClassCarriesAdvisoryAsData(t *testing.T) {
	e := &Engine{Variant: "tile url overwritten via exported intent"}
	got := e.classesFor(Entry{Kinds: []string{"route"}})
	if got[len(got)-1] != "variant" {
		t.Fatal(got)
	}
	h := e.hint("variant")
	if !strings.Contains(h, "UNTRUSTED CODE") || !strings.Contains(h, "tile url") {
		t.Fatal(h)
	}
}

// toolModel asks to read MapActivity.kt first, then answers from what it saw.
type toolModel struct{ users *[]string }

func (m toolModel) Name() string { return "tool" }
func (m toolModel) Local() bool  { return true }
func (m toolModel) Complete(_ context.Context, _, user string) (string, error) {
	*m.users = append(*m.users, user)
	if !strings.Contains(user, "Tool result (read_file)") {
		return `{"tool":"read_file","path":"app/src/main/kotlin/MapActivity.kt","start":1,"end":5}`, nil
	}
	if strings.Contains(user, `getStringExtra("tile_url")`) {
		return `{"vulnerable":true,"severity":"high","confidence":0.9,"reason":"extra written to prefs"}`, nil
	}
	return `{"vulnerable":false}`, nil
}

func TestToolLoopReadsInsideRootOnly(t *testing.T) {
	var users []string
	e := &Engine{Models: []ai.Model{toolModel{&users}}, Policy: "local-only", MaxCalls: 500, Tools: true, Only: "MapActivity.kt|confused-deputy"}
	fs, err := e.Scan(context.Background(), benchDir)
	if err != nil || len(fs) != 1 || fs[0].Rule != "hunt/confused-deputy" {
		t.Fatalf("%+v %v", fs, err)
	}
	tl := tools{root: benchDir}
	for _, p := range []string{"../../go.mod", "/etc/passwd"} {
		if out := tl.readFile(p, 1, 5); !strings.HasPrefix(out, "error") {
			t.Fatalf("escaped root via %q: %s", p, out)
		}
	}
	if g := tl.grep("endsWith"); !strings.Contains(g, "DeepLinkActivity.kt:4") {
		t.Fatal(g)
	}
	if tl.grep("(") != "error: bad pattern" {
		t.Fatal("bad regex accepted")
	}
}

func TestStateSkipsFinishedTasksAndPlatformFilters(t *testing.T) {
	st, _ := LoadState(filepath.Join(t.TempDir(), "s.json"))
	calls := 0
	mk := func() *Engine {
		return &Engine{Models: []ai.Model{fake{local: true, calls: &calls}}, Policy: "local-only", MaxCalls: 500, State: st}
	}
	first, _ := mk().Scan(context.Background(), benchDir)
	n := calls
	second, _ := mk().Scan(context.Background(), benchDir)
	if calls != n || len(first) != len(second) || n == 0 {
		t.Fatalf("resume re-called the model: %d -> %d", n, calls)
	}
	again, err := LoadState(st.path)
	if err != nil || len(again.M) == 0 {
		t.Fatal("state not persisted")
	}
	e := &Engine{Platform: "backend"}
	if cs := e.classesFor(Entry{Kinds: []string{"component", "deeplink"}}); len(cs) != 0 {
		t.Fatalf("mobile entry audited as backend: %v", cs)
	}
}

func TestFetchGHSA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/advisories/GHSA-c944-cv5f-hpvr" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"summary":"tile url overwrite","description":"exported intent","cwes":[{"cwe_id":"CWE-441"}]}`))
	}))
	defer srv.Close()
	got, err := FetchGHSA(context.Background(), "GHSA-c944-cv5f-hpvr", srv.URL)
	if err != nil || !strings.Contains(got, "tile url overwrite") || !strings.Contains(got, "CWE-441") {
		t.Fatal(got, err)
	}
	if _, err := FetchGHSA(context.Background(), "../etc/passwd", srv.URL); err == nil {
		t.Fatal("bad id accepted")
	}
}
