package hunt

import (
	"context"
	"crypto/ed25519"
	"errors"
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
