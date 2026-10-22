package store

import (
	"path/filepath"
	"sentinix/pkg/engines"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestStore_CategoryPolicyBackwardCompatibility(t *testing.T) {
	st := newTestStore(t)

	// Create a project with legacy Enforcement="block" and FailOn="high"
	p, err := st.CreateProject(Project{
		Name:        "kmp-app",
		Host:        "gh",
		Repo:        "org/kmp-app",
		Enforcement: "block",
		FailOn:      "high",
	})
	if err != nil {
		t.Fatal(err)
	}

	pols, err := st.CategoryPolicies(p.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Security row must reflect Enforcement and FailOn
	secPol, ok := pols[engines.CategorySecurity]
	if !ok {
		t.Fatalf("missing security policy")
	}
	if secPol.Mode != "block" || secPol.Threshold != "high" {
		t.Errorf("expected security mode=block, threshold=high; got mode=%s, threshold=%s", secPol.Mode, secPol.Threshold)
	}

	// Other categories must have defaults
	archPol := pols[engines.CategoryArchitecture]
	if archPol.Mode != "comment" || archPol.Threshold != "medium" {
		t.Errorf("expected architecture mode=comment, threshold=medium; got mode=%s, threshold=%s", archPol.Mode, archPol.Threshold)
	}

	// Update category policy for security and verify projects table is updated in sync
	if err := st.SetCategoryPolicy(p.ID, engines.CategorySecurity, "comment", "medium"); err != nil {
		t.Fatal(err)
	}
	updatedProj, err := st.Project(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedProj.Enforcement != "report" || updatedProj.FailOn != "medium" {
		t.Errorf("expected project enforcement=report, fail_on=medium; got enforcement=%s, fail_on=%s", updatedProj.Enforcement, updatedProj.FailOn)
	}
}

func TestStore_IngestUploadsIdempotence(t *testing.T) {
	st := newTestStore(t)

	p, err := st.CreateProject(Project{Name: "app", Host: "gh", Repo: "o/r"})
	if err != nil {
		t.Fatal(err)
	}

	commit := "abc1234"
	engine := "detekt"

	// 1. Initial upload
	up1 := IngestUpload{
		ProjectID:    p.ID,
		Commit:       commit,
		Engine:       engine,
		Format:       "sarif",
		Category:     engines.CategoryStyle,
		ParseStatus:  "success",
		FindingCount: 5,
		FindingsJSON: `[{"rule":"r1"}]`,
	}
	if err := st.RecordIngestUpload(up1); err != nil {
		t.Fatal(err)
	}

	ups, err := st.IngestUploads(p.ID, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(ups))
	}
	if ups[0].FindingCount != 5 {
		t.Errorf("expected finding count 5, got %d", ups[0].FindingCount)
	}

	// 2. Re-upload for same (project, commit, engine) - must replace
	up2 := IngestUpload{
		ProjectID:    p.ID,
		Commit:       commit,
		Engine:       engine,
		Format:       "sarif",
		Category:     engines.CategoryStyle,
		ParseStatus:  "success",
		FindingCount: 2,
		FindingsJSON: `[{"rule":"r2"}]`,
	}
	if err := st.RecordIngestUpload(up2); err != nil {
		t.Fatal(err)
	}

	upsAfter, err := st.IngestUploads(p.ID, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(upsAfter) != 1 {
		t.Fatalf("expected still 1 upload after replacement, got %d", len(upsAfter))
	}
	if upsAfter[0].FindingCount != 2 {
		t.Errorf("expected finding count 2 after replacement, got %d", upsAfter[0].FindingCount)
	}

	// 3. Deletion removes the record (simulates missing upload)
	if err := st.DeleteIngestUpload(p.ID, commit, engine); err != nil {
		t.Fatal(err)
	}
	upsDeleted, err := st.IngestUploads(p.ID, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(upsDeleted) != 0 {
		t.Errorf("expected 0 uploads after deletion, got %d", len(upsDeleted))
	}
}

func TestStore_FindingsCategoryColumn(t *testing.T) {
	st := newTestStore(t)
	p, _ := st.CreateProject(Project{Name: "app", Host: "gh", Repo: "o/r"})

	finding := engines.Finding{
		Rule:     "arch-rule",
		Engine:   "konsist",
		Category: engines.CategoryArchitecture,
		File:     "App.kt",
		Line:     1,
		Severity: engines.High,
		Message:  "Architecture violation",
		Evidence: "import android.*",
	}

	_, err := st.RecordScan(Scan{ProjectID: p.ID, Mode: "ci", Commit: "c1"}, nil, []engines.Finding{finding})
	if err != nil {
		t.Fatal(err)
	}

	fs, err := st.Findings(Filter{Project: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	if fs[0].Category != engines.CategoryArchitecture {
		t.Errorf("expected finding category architecture, got %q", fs[0].Category)
	}
}
