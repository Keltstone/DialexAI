package bench

import (
	"testing"

	"sentinix/pkg/engines"
	"sentinix/pkg/packs"
)

func TestBuiltinPacksMeetSpecTargets(t *testing.T) {
	ps, err := packs.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	m, err := Run("../../bench/vulnerable-kmp", []engines.Engine{packs.Engine{Packs: ps}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("planted=%d reported=%d precision=%.2f recall=%.2f", m.Planted, m.Reported, m.Precision, m.Recall)
	if !m.Meets(0.80, 0.70) {
		t.Fatalf("below target: precision=%.2f recall=%.2f missed=%v false positives=%+v", m.Precision, m.Recall, m.Missed, m.FalsePositives)
	}
}
