package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestSentinixSignCmd(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privB64 := base64.StdEncoding.EncodeToString(priv)

	tmpDir := t.TempDir()
	reportFile := filepath.Join(tmpDir, "report.sarif")
	content := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"detekt"}}}]}`
	if err := os.WriteFile(reportFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	p, err := st.CreateProject(store.Project{Name: "sign-proj", Host: "gh", Repo: "o/s", Enforcement: "block", FailOn: "high"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken(p.ID, "tok")
	if err != nil {
		t.Fatal(err)
	}

	app := &server.App{Store: st, TrustedKeys: []ed25519.PublicKey{pub}}
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	// Capture stdout from signCmd
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	now := time.Now().Unix()
	signCmd([]string{
		"-key", privB64,
		"-project", p.ID,
		"-commit", "c-sign",
		"-pr", "42",
		"-engine", "detekt",
		"-format", "sarif",
		"-timestamp", fmt.Sprintf("%d", now),
		reportFile,
	})

	w.Close()
	os.Stdout = oldStdout

	outBytes, _ := os.ReadFile(reportFile)
	_ = outBytes
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var sigHeader, tsHeader string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "X-Signature: ") {
			sigHeader = strings.TrimPrefix(line, "X-Signature: ")
		}
		if strings.HasPrefix(line, "X-Signature-Timestamp: ") {
			tsHeader = strings.TrimPrefix(line, "X-Signature-Timestamp: ")
		}
	}

	if sigHeader == "" || tsHeader == "" {
		t.Fatalf("expected X-Signature and X-Signature-Timestamp in output, got: %q", output)
	}

	// Now upload to server using the generated signature and timestamp
	req := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=detekt&format=sarif&commit=c-sign&pr=42", strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Signature", sigHeader)
	req.Header.Set("X-Signature-Timestamp", tsHeader)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("ingest with signed output failed: code=%d body=%s", rec.Code, rec.Body.String())
	}

	ups, err := st.IngestUploads(p.ID, "c-sign")
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 1 || !ups[0].Attested {
		t.Fatalf("expected upload to be attested via sentinix sign output, got %+v", ups)
	}
}

