package store

import (
	"testing"
	"time"

	"sentinix/pkg/engines"
)

func open(t *testing.T) *Store {
	s, err := Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func f(rule, ev string) engines.Finding {
	return engines.Finding{Rule: rule, Engine: "secrets", File: "a.kt", Line: 1, Severity: engines.High, Evidence: ev}
}

func TestScanLifecycle(t *testing.T) {
	s := open(t)
	p, _ := s.CreateProject(Project{Name: "p", Host: "gh", Repo: "o/r"})
	sc := Scan{ProjectID: p.ID, Mode: "full", Commit: "c1"}
	r1, err := s.RecordScan(sc, []string{"secrets"}, []engines.Finding{f("r", "a"), f("r", "b")})
	if err != nil || r1.New != 2 {
		t.Fatalf("%+v %v", r1, err)
	}
	// "a" gone -> fixed; an engine that did not run fixes nothing
	r2, _ := s.RecordScan(sc, []string{"secrets"}, []engines.Finding{f("r", "b")})
	if r2.Fixed != 1 || r2.Unchanged != 1 {
		t.Fatalf("%+v", r2)
	}
	r3, _ := s.RecordScan(sc, []string{"semgrep"}, nil)
	if r3.Fixed != 0 {
		t.Fatalf("absent engine fixed findings: %+v", r3)
	}
	// "a" returns -> reopened and counted new
	r4, _ := s.RecordScan(sc, []string{"secrets"}, []engines.Finding{f("r", "a"), f("r", "b")})
	if r4.New != 1 {
		t.Fatalf("%+v", r4)
	}
}

func TestAcceptedRiskNeedsReasonAndExpires(t *testing.T) {
	s := open(t)
	p, _ := s.CreateProject(Project{Name: "p", Host: "gh", Repo: "o/r"})
	s.RecordScan(Scan{ProjectID: p.ID}, []string{"secrets"}, []engines.Finding{f("r", "a")})
	fs, _ := s.Findings(Filter{Project: p.ID})
	if s.Triage(fs[0].ID, "accepted", "", "", time.Now().Add(time.Hour).Unix()) == nil {
		t.Fatal("accepted without reason")
	}
	if err := s.Triage(fs[0].ID, "accepted", "bob", "legacy", time.Now().Add(-time.Hour).Unix()); err == nil {
		t.Fatal("accepted with past expiry")
	}
}

func TestSeparationOfDuties(t *testing.T) {
	s := open(t)
	i, _ := s.Propose("disable_rule", map[string]string{"rule": "x"}, "alice")
	if _, err := s.Decide(i, "alice", true); err == nil {
		t.Fatal("self approval allowed")
	}
	a, err := s.Decide(i, "bob", true)
	if err != nil || a.Status != "approved" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := s.Decide(i, "carol", true); err == nil {
		t.Fatal("decided twice")
	}
}

func TestTokensAndDemotion(t *testing.T) {
	s := open(t)
	p, _ := s.CreateProject(Project{Name: "p", Host: "gh", Repo: "o/r"})
	tok, _ := s.CreateToken(p.ID, "ci")
	if got, ok := s.TokenProject(tok); !ok || got != p.ID {
		t.Fatal("token lookup")
	}
	if _, ok := s.TokenProject("nope"); ok {
		t.Fatal("bad token accepted")
	}
	var fs []engines.Finding
	for _, e := range []string{"1", "2", "3", "4"} {
		fs = append(fs, f("noisy", e))
	}
	s.RecordScan(Scan{ProjectID: p.ID}, []string{"secrets"}, fs)
	all, _ := s.Findings(Filter{Project: p.ID})
	for _, x := range all[:3] {
		s.Triage(x.ID, "false_positive", "", "", 0)
	}
	d, _ := s.Demoted(0.5, 4)
	if d["noisy"] != engines.Info {
		t.Fatalf("not demoted: %v", d)
	}
}

func TestOpenUpgradesAnOldProjectsTable(t *testing.T) {
	path := t.TempDir() + "/old.db"
	s, err := Open("sqlite:" + path)
	if err != nil {
		t.Fatal(err)
	}
	// simulate a database from before enforcement existed
	s.db.Exec("alter table projects drop column enforcement")
	s.db.Exec("alter table projects drop column fail_on")
	s.Close()
	s, err = Open("sqlite:" + path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(Project{Name: "p", Host: "h", Repo: "r"})
	if err != nil || p.Enforcement != "block" {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := Open("sqlite:" + path); err != nil { // second open: columns already there
		t.Fatal(err)
	}
}
