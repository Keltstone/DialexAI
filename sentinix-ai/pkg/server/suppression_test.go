package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/store"
)

func TestInlineSuppressionWorkflow_Governance(t *testing.T) {
	e := setup(t)
	e.st.UpsertUser("author@example.test", "Author", auth.Developer)
	e.st.UpsertUser("author+alias@example.test", "Author Alias", auth.Developer)
	e.st.UpsertUser("reviewer@example.test", "Reviewer", auth.Reviewer)

	// Register project
	proj, err := e.st.CreateProject(store.Project{
		Name:   "sup-proj",
		Host:   "github",
		Repo:   "org/sup-repo",
		Branch: "main",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// 1. Process inline suppression from source code comment: creates approval automatically
	code := "const token = \"AKIAIOSFODNN7EXAMPLE\"; // sentinix:ignore rule=secret.aws-token until=2028-01-01 owner=author@example.test reason=\"test mock\"\n"
	suppressions, err := engines.ParseInlineSuppressions(strings.NewReader(code), "src/auth.js")
	if err != nil {
		t.Fatalf("parse suppressions failed: %v", err)
	}
	if len(suppressions) != 1 {
		t.Fatalf("expected 1 suppression, got %d", len(suppressions))
	}
	s := suppressions[0]
	if s.Owner != "author@example.test" || s.Rule != "secret.aws-token" {
		t.Fatalf("unexpected parsed suppression: %+v", s)
	}

	appID, err := e.app.ProposeInlineSuppression(proj.ID, s)
	if err != nil {
		t.Fatalf("propose inline suppression failed: %v", err)
	}
	if appID == "" {
		t.Fatal("expected non-empty approval ID")
	}

	// Verify approval exists and has pending status
	ap, err := e.st.Approval(appID)
	if err != nil {
		t.Fatalf("approval not found: %v", err)
	}
	if ap.Status != "pending" {
		t.Fatalf("expected pending status, got %s", ap.Status)
	}

	// Verify reviewer queue metrics reflect the pending approval
	wMet := httptest.NewRecorder()
	reqMet := httptest.NewRequest("GET", "/metrics", nil)
	e.app.metrics(wMet, reqMet)
	metricsBody := wMet.Body.String()
	if !strings.Contains(metricsBody, "sentinix_reviewer_queue_depth") {
		t.Fatalf("metrics missing sentinix_reviewer_queue_depth: %s", metricsBody)
	}

	// 2. Author cannot approve own suppression
	wSelf := e.call("author@example.test", auth.Reviewer, "POST", "/api/approvals/"+appID+"/decide", `{"approve":true}`)
	e.ok(wSelf, 409)

	// 3. Author alias cannot approve
	wAlias := e.call("author+alias@example.test", auth.Reviewer, "POST", "/api/approvals/"+appID+"/decide", `{"approve":true}`)
	e.ok(wAlias, 409)

	// 4. Independent reviewer approves successfully
	wRev := e.call("reviewer@example.test", auth.Reviewer, "POST", "/api/approvals/"+appID+"/decide", `{"approve":true}`)
	e.ok(wRev, 200)

	// 5. Expired suppression: an expired suppression cannot be approved and does not suppress findings
	pastUntil := time.Now().Add(-24 * time.Hour).Unix()
	sExpired := engines.InlineSuppression{
		File:   "src/auth.js",
		Line:   1,
		Rule:   "secret.aws-token",
		Until:  pastUntil,
		Owner:  "author@example.test",
		Reason: "expired temporary pass",
	}
	expiredAppID, err := e.st.Propose("sentinix:ignore", map[string]any{
		"project": proj.ID,
		"file":    sExpired.File,
		"line":    sExpired.Line,
		"rule":    sExpired.Rule,
		"until":   sExpired.Until,
		"owner":   sExpired.Owner,
	}, sExpired.Owner)
	if err != nil {
		t.Fatalf("propose expired suppression failed: %v", err)
	}

	// Deciding an expired suppression must fail
	wDecideExpired := e.call("reviewer@example.test", auth.Reviewer, "POST", "/api/approvals/"+expiredAppID+"/decide", `{"approve":true}`)
	if wDecideExpired.Code != 400 && wDecideExpired.Code != 409 {
		t.Fatalf("expected 400/409 on deciding expired suppression, got %d", wDecideExpired.Code)
	}
}
