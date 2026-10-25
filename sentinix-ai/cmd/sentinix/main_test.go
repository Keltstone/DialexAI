package main

import (
	"context"
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
	"sentinix/pkg/policy"
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

func TestCustomRulesNewAndTest(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "team-rules.json")

	// 1. Generate new rule pack template
	packCmd([]string{"new", "team-rules", outFile})

	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("expected rule template to be created at %s, got error: %v", outFile, err)
	}

	// 2. Test the rule pack template
	packCmd([]string{"test", outFile})
}

func TestHookInstallAndUninstall(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	// Create fake .git directory
	os.MkdirAll(".git", 0o755)

	// 1. Install hook
	hookCmd([]string{"install"})
	hookPath := filepath.Join(".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("expected hook to be installed at %s, got error: %v", hookPath, err)
	}

	// 2. Uninstall hook
	hookCmd([]string{"uninstall"})
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected hook to be deleted after uninstall")
	}
}

func TestPolicyInitAndShow(t *testing.T) {
	tmpDir := t.TempDir()
	policyFile := filepath.Join(tmpDir, "policy.json")

	// 1. Initialize policy
	policyCmd([]string{"init", policyFile})

	if _, err := os.Stat(policyFile); err != nil {
		t.Fatalf("expected policy to exist at %s: %v", policyFile, err)
	}

	// 2. Show policy
	policyCmd([]string{"show", policyFile})

	// 3. Verify content
	p, err := policy.LoadProjectPolicy(policyFile)
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultLevel != policy.LevelWarn {
		t.Errorf("expected default level warn, got %s", p.DefaultLevel)
	}
	if p.Rules["go/insecure-tls-skip-verify"] != policy.LevelBlock {
		t.Errorf("expected go/insecure-tls-skip-verify to be block, got %s", p.Rules["go/insecure-tls-skip-verify"])
	}
}

func TestHuntConfigCommands(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, ".sentinix", "hunt.json")

	// 1. init
	huntCmd([]string{"config", "init", confPath})
	if _, err := os.Stat(confPath); err != nil {
		t.Fatalf("expected config at %s: %v", confPath, err)
	}

	// 2. show
	huntCmd([]string{"config", "show", tmpDir})
	huntCmd([]string{"config", "show", "--explain", tmpDir})

	// 3. validate
	huntCmd([]string{"config", "validate", confPath})
}

func TestHuntRunsAndShowCommands(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.Open("sqlite:" + dbPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	runID := "run-test-123"
	if err := st.CreateHuntRun(ctx, store.HuntRun{
		ID:            runID,
		ProjectID:     "proj-demo",
		Status:        "completed",
		Calls:         15,
		Tokens:        4200,
		CostUSD:       0.12,
		FindingsCount: 2,
	}); err != nil {
		t.Fatal(err)
	}
	_ = st.RecordHuntToolCall(ctx, store.HuntToolCallRecord{
		RunID:         runID,
		TaskID:        "task-1",
		Tool:          "read_file",
		ArgsHash:      "abcd1234",
		BytesReturned: 500,
	})
	st.Close()

	// 1. hunt runs
	huntCmd([]string{"runs", "--db", "sqlite:" + dbPath, "--project", "proj-demo"})

	// 2. hunt show
	huntCmd([]string{"show", runID, "--db", "sqlite:" + dbPath})
}

func TestHuntBundleSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	bundleFile := filepath.Join(tmpDir, "community-pack.json")

	// 1. bundle new
	huntCmd([]string{"bundle", "new", bundleFile})
	if _, err := os.Stat(bundleFile); err != nil {
		t.Fatalf("bundle new did not create file: %v", err)
	}

	// 2. bundle test (unsigned)
	huntCmd([]string{"bundle", "test", bundleFile})

	// 3. keygen for signing
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(tmpDir, "author.key")
	pubFile := filepath.Join(tmpDir, "trusted.pub")
	if err := os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pubFile, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. bundle sign
	// Capture signed output by redirecting stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	huntCmd([]string{"bundle", "sign", bundleFile, "--key", keyFile})

	w.Close()
	os.Stdout = oldStdout

	var signedBuf strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			signedBuf.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	r.Close()

	signedFile := filepath.Join(tmpDir, "community-pack.signed.json")
	if err := os.WriteFile(signedFile, []byte(signedBuf.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. bundle test (signed)
	huntCmd([]string{"bundle", "test", signedFile})

	// 6. bundle verify
	huntCmd([]string{"bundle", "verify", signedFile, "--trust", pubFile})
}
