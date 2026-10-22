package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/store"
	"strings"
	"testing"
)

type mockHostWithStatusNamed struct {
	name          string
	comments      []string
	namedStatuses map[string]string
}

func (m *mockHostWithStatusNamed) Name() string            { return m.name }
func (m *mockHostWithStatusNamed) ConfiguredHost() string  { return "github.com" }
func (m *mockHostWithStatusNamed) CloneAuth() (string, string) { return "", "" }
func (m *mockHostWithStatusNamed) RepoURL(r string) string { return "https://github.com/" + r }
func (m *mockHostWithStatusNamed) Comment(ctx context.Context, ev *githost.Event, body string) error {
	m.comments = append(m.comments, body)
	return nil
}
func (m *mockHostWithStatusNamed) Status(ctx context.Context, ev *githost.Event, s githost.State, desc string) error {
	return m.StatusNamed(ctx, ev, "sentinix", s, desc)
}
func (m *mockHostWithStatusNamed) StatusNamed(ctx context.Context, ev *githost.Event, name string, s githost.State, desc string) error {
	if m.namedStatuses == nil {
		m.namedStatuses = make(map[string]string)
	}
	m.namedStatuses[name] = s.String() + ": " + desc
	return nil
}
func (m *mockHostWithStatusNamed) Parse(r *http.Request, body []byte) (*githost.Event, error) {
	return nil, nil
}

func TestIngest_SARIFAndJUnit(t *testing.T) {
	e := setup(t)
	p, err := e.st.CreateProject(store.Project{Name: "kmp-proj", Host: "gh", Repo: "org/repo"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := e.st.CreateToken(p.ID, "ci-token")
	if err != nil {
		t.Fatal(err)
	}

	mockH := &mockHostWithStatusNamed{name: "gh"}
	e.app.Hosts = map[string]githost.Host{"gh": mockH}

	// 1. Ingest Detekt SARIF via query params
	sarifPayload := `{
		"version": "2.1.0",
		"runs": [{
			"tool": { "driver": { "name": "detekt" } },
			"results": [{
				"ruleId": "detekt.formatting.Indentation",
				"level": "warning",
				"message": { "text": "Wrong indentation" },
				"locations": [{
					"physicalLocation": {
						"artifactLocation": { "uri": "src/Common.kt" },
						"region": { "startLine": 15 }
					}
				}]
			}]
		}]
	}`

	req := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=detekt&format=sarif&commit=c1&pr=42", strings.NewReader(sarifPayload))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("detekt ingest failed: status=%d body=%s", w.Code, w.Body)
	}

	// Verify upload record
	ups, err := e.st.IngestUploads(p.ID, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 1 {
		t.Fatalf("expected 1 upload record, got %d", len(ups))
	}
	if ups[0].Engine != "detekt" || ups[0].Category != engines.CategoryStyle || ups[0].FindingCount != 1 {
		t.Errorf("unexpected upload: %+v", ups[0])
	}

	// 2. Ingest Konsist JUnit XML via query params
	junitPayload := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.DomainRuleTest" tests="1" failures="1">
    <testcase name="domain layer has no platform dependencies" classname="com.example.DomainRuleTest">
        <failure message="Domain rule failed" type="AssertionError">
            at com.example.DomainRuleTest(Domain.kt:8)
        </failure>
    </testcase>
</testsuite>`

	req = httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=konsist&format=junit&commit=c1&pr=42", strings.NewReader(junitPayload))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("konsist ingest failed: status=%d body=%s", w.Code, w.Body)
	}

	// Verify both uploads exist
	ups, err = e.st.IngestUploads(p.ID, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(ups))
	}

	// 3. Idempotent re-upload replaces previous record
	reuploadPayload := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.DomainRuleTest" tests="2" failures="0">
    <testcase name="domain layer has no platform dependencies" classname="com.example.DomainRuleTest" />
</testsuite>`

	req = httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=konsist&format=junit&commit=c1&pr=42", strings.NewReader(reuploadPayload))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("konsist reupload failed: status=%d body=%s", w.Code, w.Body)
	}

	ups, err = e.st.IngestUploads(p.ID, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 2 {
		t.Fatalf("expected still 2 uploads after reupload, got %d", len(ups))
	}
	for _, u := range ups {
		if u.Engine == "konsist" && u.FindingCount != 0 {
			t.Errorf("expected konsist finding count 0 after reupload, got %d", u.FindingCount)
		}
	}

	// 4. Malformed XML returns 400 and records failed upload
	badPayload := `not-xml-at-all`
	req = httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=ktlint&format=junit&commit=c1&pr=42", strings.NewReader(badPayload))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for malformed payload, got %d", w.Code)
	}

	ups, err = e.st.IngestUploads(p.ID, "c1")
	if err != nil {
		t.Fatal(err)
	}
	var failedFound bool
	for _, u := range ups {
		if u.Engine == "ktlint" && u.ParseStatus == "failed" {
			failedFound = true
		}
	}
	if !failedFound {
		t.Errorf("expected failed parse status recorded in ingest_uploads for ktlint")
	}

	// 5. Path traversal attempt rejected with 400
	traversalPayload := `{
		"version": "2.1.0",
		"runs": [{
			"tool": { "driver": { "name": "detekt" } },
			"results": [{
				"ruleId": "r",
				"message": { "text": "msg" },
				"locations": [{
					"physicalLocation": {
						"artifactLocation": { "uri": "../../../../etc/passwd" },
						"region": { "startLine": 1 }
					}
				}]
			}]
		}]
	}`
	req = httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=detekt&format=sarif&commit=c1&pr=42", strings.NewReader(traversalPayload))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	// Round 13 item 5: drop the hostile result per finding instead of 400 for the whole report
	if w.Code != 200 {
		t.Fatalf("expected 200 (hostile finding dropped), got %d", w.Code)
	}
	fsAfter, _ := e.st.Findings(store.Filter{Project: p.ID, Status: "open"})
	for _, f := range fsAfter {
		if f.Rule == "r" {
			t.Fatalf("hostile finding was stored, expected dropped: %+v", f)
		}
	}

	// 6. Project token scoping: posting to another project fails with 403
	req = httptest.NewRequest("POST", "/api/projects/other-project/ingest?engine=detekt&format=sarif", strings.NewReader(sarifPayload))
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403 for mismatched project token, got %d", w.Code)
	}
}

func TestReviewCommentAndCoverageFooter(t *testing.T) {
	uploads := []store.IngestUpload{
		{Engine: "detekt", ParseStatus: "success", FindingCount: 1, ReceivedAt: 1700000000},
		{Engine: "ktlint", ParseStatus: "failed", ErrorMessage: "syntax error"},
	}
	expectedTools := []string{"detekt", "ktlint", "konsist", "lint"}

	findings := []engines.Finding{
		{Rule: "sec-leak", Category: engines.CategorySecurity, Severity: engines.High, Message: "AWS Secret", File: "secrets.go", Line: 10},
		{Rule: "detekt.style", Category: engines.CategoryStyle, Severity: engines.Low, Message: "Indentation", File: "Style.kt", Line: 20},
	}

	comment := FormatReviewComment(true, engines.High, findings, nil, []string{"secrets"}, nil, uploads, expectedTools)

	// Verify grouping: security is open, style is collapsed in <details>
	if !strings.Contains(comment, "Security (1)") {
		t.Errorf("expected Security heading in comment")
	}
	if !strings.Contains(comment, "<details><summary>Style (1)</summary>") {
		t.Errorf("expected collapsed Style section in comment")
	}

	// Verify coverage footer
	if !strings.Contains(comment, "- detekt: uploaded by CI (unverified)") {
		t.Errorf("expected detekt uploaded by CI (unverified) in coverage footer")
	}
	if !strings.Contains(comment, "- ktlint: failed to parse") {
		t.Errorf("expected ktlint failed to parse in coverage footer")
	}
	if !strings.Contains(comment, "- konsist: not received") {
		t.Errorf("expected konsist not received in coverage footer")
	}
	if !strings.Contains(comment, "- lint: not received") {
		t.Errorf("expected lint not received in coverage footer")
	}
	if !strings.Contains(comment, "A missing upload is not covered and never reads as clean.") {
		t.Errorf("expected explicit warning about missing uploads in footer")
	}
}

func TestPolicyEndpointsAndSafeguards(t *testing.T) {
	e := setup(t)
	p, err := e.st.CreateProject(store.Project{Name: "policy-test", Host: "gh", Repo: "o/r", Enforcement: "block", FailOn: "high"})
	if err != nil {
		t.Fatal(err)
	}

	// 1. GET /api/projects/{id}/policy returns effective policies
	w := e.call("admin@example.test", auth.Admin, "GET", "/api/projects/"+p.ID+"/policy", "")
	if w.Code != 200 {
		t.Fatalf("GET /policy failed: code=%d body=%s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"security"`) || !strings.Contains(w.Body.String(), `"architecture"`) {
		t.Errorf("unexpected policy body: %s", w.Body.String())
	}

	// 2. Tightening: architecture comment -> block applies immediately without approval
	tightenPayload := `{"architecture": {"mode": "block", "threshold": "medium"}}`
	w = e.call("admin@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID+"/policy", tightenPayload)
	if w.Code != 200 {
		t.Fatalf("tightening policy failed: code=%d body=%s", w.Code, w.Body)
	}

	pol, err := e.st.CategoryPolicy(p.ID, engines.CategoryArchitecture)
	if err != nil {
		t.Fatal(err)
	}
	if pol.Mode != "block" {
		t.Errorf("expected architecture mode to be block, got %s", pol.Mode)
	}

	// 3. Loosening: architecture block -> comment requires approval
	loosenPayload := `{"architecture": {"mode": "comment", "threshold": "medium"}}`
	w = e.call("admin@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID+"/policy", loosenPayload)
	if w.Code != 202 {
		t.Fatalf("expected 202 Accepted for loosening policy, got %d (body=%s)", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "approval_required") {
		t.Errorf("expected approval_required in response, got %s", w.Body.String())
	}

	// 4. Turning off: architecture mode -> off requires approval
	offPayload := `{"architecture": {"mode": "off", "threshold": "medium"}}`
	w = e.call("admin@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID+"/policy", offPayload)
	if w.Code != 202 {
		t.Fatalf("expected 202 Accepted for turning check off, got %d", w.Code)
	}

	// 5. Bogus threshold/mode/category returns 400 Bad Request
	bogusThresh := `{"architecture": {"mode": "block", "threshold": "bogus"}}`
	w = e.call("admin@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID+"/policy", bogusThresh)
	if w.Code != 400 {
		t.Fatalf("expected 400 Bad Request for bogus threshold, got %d (body=%s)", w.Code, w.Body)
	}

	bogusMode := `{"architecture": {"mode": "bogus", "threshold": "high"}}`
	w = e.call("admin@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID+"/policy", bogusMode)
	if w.Code != 400 {
		t.Fatalf("expected 400 Bad Request for bogus mode, got %d (body=%s)", w.Code, w.Body)
	}

	bogusCat := `{"bogus_cat": {"mode": "block", "threshold": "high"}}`
	w = e.call("admin@example.test", auth.Admin, "PATCH", "/api/projects/"+p.ID+"/policy", bogusCat)
	if w.Code != 400 {
		t.Fatalf("expected 400 Bad Request for bogus category, got %d (body=%s)", w.Code, w.Body)
	}
}

func TestIngest_SignedUploadClosesOwnEngineFindings(t *testing.T) {
	e := setup(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	e.app.TrustedKeys = []ed25519.PublicKey{pub}

	p, err := e.st.CreateProject(store.Project{Name: "signed-close-test", Host: "gh", Repo: "o/r", Enforcement: "block", FailOn: "high"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := e.st.CreateToken(p.ID, "ci-token")
	if err != nil {
		t.Fatal(err)
	}

	h := &mockHostWithStatusNamed{name: "gh"}
	e.app.Hosts = map[string]githost.Host{"gh": h}

	signedPost := func(q, body string) (int, string) {
		u, _ := url.Parse("?" + q)
		engine := u.Query().Get("engine")
		format := u.Query().Get("format")
		commit := u.Query().Get("commit")
		pr := u.Query().Get("pr")
		h := sha256.Sum256([]byte(body))
		msg := fmt.Sprintf("%s|%s|%s|%s|%s|%x", p.ID, commit, pr, engine, format, h)
		sig := ed25519.Sign(priv, []byte(msg))
		sigB64 := base64.StdEncoding.EncodeToString(sig)
		req := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?"+q, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-Signature", sigB64)
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}

	sarifFmt := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"detekt"}},"results":[%s]}]}`
	one := `{"ruleId":"detekt.MagicNumber","level":"warning","message":{"text":"avoid magic numbers"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/Num.kt"},"region":{"startLine":42}}}]}`

	// 1. Initial signed upload with a finding on PR 11
	code, body := signedPost("engine=detekt&format=sarif&commit=c1&pr=11", strings.Replace(sarifFmt, "%s", one, 1))
	if code != 200 {
		t.Fatalf("initial signed upload failed: code=%d body=%s", code, body)
	}

	fs, err := e.st.Findings(store.Filter{Project: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 || fs[0].Status != "open" {
		t.Fatalf("expected 1 open finding, got %+v", fs)
	}
	if fs[0].Source != "ci-attested" {
		t.Errorf("expected source ci-attested, got %s", fs[0].Source)
	}

	// 2. An unrelated unsigned upload on another PR (PR 12) must NOT close PR 11's finding
	req := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest?engine=detekt&format=sarif&commit=c2&pr=12", strings.NewReader(strings.Replace(sarifFmt, "%s", "", 1)))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("unsigned upload on pr 12 failed: code=%d body=%s", w.Code, w.Body)
	}

	fs, _ = e.st.Findings(store.Filter{Project: p.ID})
	if len(fs) != 1 || fs[0].Status != "open" {
		t.Fatalf("expected PR 11 finding to remain open after PR 12 upload, got %+v", fs)
	}

	// 3. Clean signed upload on PR 11 closes the finding
	code, body = signedPost("engine=detekt&format=sarif&commit=c3&pr=11", strings.Replace(sarifFmt, "%s", "", 1))
	if code != 200 {
		t.Fatalf("clean signed upload failed: code=%d body=%s", code, body)
	}

	fs, _ = e.st.Findings(store.Filter{Project: p.ID})
	if len(fs) != 1 || fs[0].Status != "fixed" {
		t.Fatalf("expected finding to be fixed by clean signed upload, got %+v", fs)
	}
}

// TestPackFindingIngest verifies that CLI-style pack findings are accepted through /ingest.
func TestPackFindingIngest(t *testing.T) {
	e := setup(t)
	p, err := e.st.CreateProject(store.Project{Name: "pack-test", Host: "gh", Repo: "o/pack", Enforcement: "block", FailOn: "high"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := e.st.CreateToken(p.ID, "ci-token")
	if err != nil {
		t.Fatal(err)
	}

	packBody := `{"commit":"c-pack","findings":[{"rule":"android.exported","engine":"pack:android@1.0.0","file":"app/AndroidManifest.xml","line":3,"severity":2,"message":"exported component"}]}`
	req := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/ingest", strings.NewReader(packBody))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("pack ingest failed: status=%d body=%s", w.Code, w.Body.String())
	}

	fs, err := e.st.Findings(store.Filter{Project: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	if fs[0].Engine != "pack:android@1.0.0" {
		t.Errorf("expected engine pack:android@1.0.0, got %s", fs[0].Engine)
	}
}

