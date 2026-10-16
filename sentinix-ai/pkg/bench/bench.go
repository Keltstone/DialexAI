// Package bench scores engines against a sample project with planted bugs and a clean (false-positive) set.
// Targets from the spec: precision 80% or higher, recall 70% or higher.
package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"sentinix/pkg/engines"
)

type Planted struct {
	Rule string `json:"rule"`
	File string `json:"file"`
}

type Metrics struct {
	Planted, Reported, TruePositive int
	Precision, Recall               float64
	Missed                          []Planted
	FalsePositives                  []engines.Finding
}

// Run scans dir with es and compares against dir/expected.json. Anything reported that was not planted is a false positive.
func Run(dir string, es []engines.Engine) (*Metrics, error) {
	b, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		return nil, err
	}
	var want []Planted
	if err := json.Unmarshal(b, &want); err != nil {
		return nil, err
	}
	var got []engines.Finding
	for _, e := range es {
		fs, err := e.Scan(context.Background(), dir)
		if err != nil {
			return nil, err
		}
		got = append(got, fs...)
	}
	m := &Metrics{Planted: len(want)}
	found := map[Planted]bool{}
	wantSet := map[Planted]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	for _, f := range got {
		k := Planted{f.Rule, f.File}
		m.Reported++
		if wantSet[k] {
			found[k] = true
		} else {
			m.FalsePositives = append(m.FalsePositives, f)
		}
	}
	m.TruePositive = len(found)
	for _, w := range want {
		if !found[w] {
			m.Missed = append(m.Missed, w)
		}
	}
	if m.Planted > 0 {
		m.Recall = float64(m.TruePositive) / float64(m.Planted)
	}
	// precision counts findings, so duplicates of a planted bug are not penalised but strays are
	if m.Reported > 0 {
		m.Precision = float64(m.Reported-len(m.FalsePositives)) / float64(m.Reported)
	}
	return m, nil
}

func (m *Metrics) Meets(precision, recall float64) bool {
	return m.Precision >= precision && m.Recall >= recall
}
