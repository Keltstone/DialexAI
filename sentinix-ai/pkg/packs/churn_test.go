package packs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fpsForTest(t *testing.T, root string) map[string]string {
	e, err := NewEngine("", "")
	if err != nil {
		t.Fatal(err)
	}
	fl, err := e.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, f := range fl {
		m[f.Fingerprint()] = f.Rule + " " + f.File
	}
	return m
}

func cpForTest(t *testing.T, src string, tf func(rel, s string) (string, string)) string {
	d := t.TempDir()
	filepath.WalkDir(src, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(src, p)
		nr, ns := tf(rel, string(b))
		os.MkdirAll(filepath.Dir(filepath.Join(d, nr)), 0o755)
		os.WriteFile(filepath.Join(d, nr), []byte(ns), 0o644)
		return nil
	})
	return d
}

func TestPacksChurnRetention(t *testing.T) {
	src, _ := filepath.Abs("../../bench/vulnerable-kmp")
	base := fpsForTest(t, src)
	if len(base) == 0 {
		t.Fatal("expected baseline findings > 0")
	}

	cases := map[string]struct {
		tf func(string, string) (string, string)
		minRetention float64
	}{
		"identity (control)": {
			tf: func(r, s string) (string, string) { return r, s },
			minRetention: 0.95,
		},
		"insert 3 lines above (line move)": {
			tf: func(r, s string) (string, string) { return r, "// a\n// b\n// c\n" + s },
			minRetention: 0.95,
		},
		"CRLF": {
			tf: func(r, s string) (string, string) { return r, strings.ReplaceAll(s, "\n", "\r\n") },
			minRetention: 0.95,
		},
		"tab->4sp": {
			tf: func(r, s string) (string, string) { return r, strings.ReplaceAll(s, "\t", "    ") },
			minRetention: 0.95,
		},
		"indent +2sp every line": {
			tf: func(r, s string) (string, string) { return r, "  " + strings.ReplaceAll(s, "\n", "\n  ") },
			minRetention: 0.95,
		},
		"trailing comment on every line": {
			tf: func(r, s string) (string, string) { return r, strings.ReplaceAll(s, "\n", " // x\n") },
			minRetention: 0.95,
		},
		"rename every file (dir move)": {
			tf: func(r, s string) (string, string) { return "moved/" + r, s },
			minRetention: 0.95,
		},
		"space around = / ( normalised": {
			tf: func(r, s string) (string, string) { return r, strings.ReplaceAll(s, "(", "( ") },
			minRetention: 0.95,
		},
	}

	for n, tc := range cases {
		got := fpsForTest(t, cpForTest(t, src, tc.tf))
		same := 0
		for k := range base {
			if _, ok := got[k]; ok {
				same++
			}
		}
		retention := float64(same) / float64(len(base))
		t.Logf("%-34s retained %d/%d (%.0f%%) churn(new ids)=%d", n, same, len(base), 100*retention, len(got)-same)
		if retention < tc.minRetention {
			t.Errorf("%s: retention %.2f < required %.2f", n, retention, tc.minRetention)
		}
	}
}
