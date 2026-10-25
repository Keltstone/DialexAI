package hunt

import (
	"context"
	"strings"
	"testing"

	"sentinix/pkg/ai"
	"sentinix/pkg/bench"
	"sentinix/pkg/engines"
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
