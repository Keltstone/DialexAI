package main

import (
	"net/http/httptest"
	"testing"

	"sentinix/pkg/engines"
	"sentinix/pkg/server"
	"sentinix/pkg/store"
)

func TestScanServerUpload_PackFinding(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p, err := st.CreateProject(store.Project{Name: "pack-proj", Host: "gh", Repo: "o/r", Enforcement: "block", FailOn: "high"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(p.ID, "cli-token")
	if err != nil {
		t.Fatal(err)
	}

	app := &server.App{Store: st}
	handler := app.Handler()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	packFinding := engines.Finding{
		Rule:     "android.exported",
		Engine:   "pack:android",
		File:     "app/src/main/AndroidManifest.xml",
		Line:     12,
		Severity: engines.Medium,
		Message:  "exported activity without permission",
		Category: "security",
	}

	err = upload(ts.URL, p.ID, tok, []string{"pack:android"}, []engines.Finding{packFinding})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	fs, err := st.Findings(store.Filter{Project: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding stored, got %d", len(fs))
	}
	if fs[0].Engine != "pack:android" {
		t.Errorf("expected engine pack:android, got %q", fs[0].Engine)
	}
}
