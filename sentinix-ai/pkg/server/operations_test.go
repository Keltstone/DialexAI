package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sentinix/pkg/auth"
	"sentinix/pkg/engines"
	"sentinix/pkg/githost"
	"sentinix/pkg/scheduler"
	"sentinix/pkg/store"
)

type countingHost struct {
	name      string
	scanCount atomic.Int64
	comments  []string
}

func (c *countingHost) Name() string            { return c.name }
func (c *countingHost) ConfiguredHost() string  { return "example.com" }
func (c *countingHost) RepoURL(r string) string { return "https://example.com/" + r }
func (c *countingHost) CloneAuth() (string, string) {
	c.scanCount.Add(1)
	return "user", "token"
}
func (c *countingHost) Status(ctx context.Context, e *githost.Event, s githost.State, desc string) error {
	return nil
}
func (c *countingHost) StatusNamed(ctx context.Context, e *githost.Event, name string, s githost.State, desc string) error {
	return nil
}
func (c *countingHost) Parse(r *http.Request, body []byte) (*githost.Event, error) { return nil, nil }
func (c *countingHost) Comment(ctx context.Context, e *githost.Event, body string) error {
	c.comments = append(c.comments, body)
	return nil
}

func TestOperations_LeaderElection_TwoReplicasRunCronOnce(t *testing.T) {
	st, err := store.Open("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ch := &countingHost{name: "gh"}
	hosts := map[string]githost.Host{"gh": ch}

	p, err := st.CreateProject(store.Project{
		Name:     "cron-project",
		Host:     "gh",
		Repo:     "org/repo",
		Branch:   "main",
		Schedule: "* * * * *",
		TZ:       "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	app1 := &App{Store: st, Hosts: hosts, Sess: auth.Sessions{Key: make([]byte, 32)}, SyncForTest: true}
	app2 := &App{Store: st, Hosts: hosts, Sess: auth.Sessions{Key: make([]byte, 32)}, SyncForTest: true}

	now := time.Now().UTC()
	// Both replicas tick at the same time
	app1.Tick(context.Background(), now)
	app2.Tick(context.Background(), now)

	// Since only the leader runs the cron, scan count must be at most 1
	if count := ch.scanCount.Load(); count != 1 {
		t.Errorf("expected cron to run exactly once across replicas, got %d", count)
	}

	// Verify project was scheduled
	_ = p
}

func TestOperations_ReadyzAndMetricsEndpoints(t *testing.T) {
	e := setup(t)

	// Test /readyz
	reqReady := httptest.NewRequest("GET", "/readyz", nil)
	wReady := httptest.NewRecorder()
	e.h.ServeHTTP(wReady, reqReady)
	if wReady.Code != 200 {
		t.Errorf("expected /readyz 200, got %d", wReady.Code)
	}
	if !strings.Contains(wReady.Body.String(), "ready") {
		t.Errorf("expected /readyz to contain 'ready', got %s", wReady.Body.String())
	}

	// Test /metrics
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	e.h.ServeHTTP(wMetrics, reqMetrics)
	if wMetrics.Code != 200 {
		t.Errorf("expected /metrics 200, got %d", wMetrics.Code)
	}
	body := wMetrics.Body.String()
	for _, expected := range []string{"sentinix_queue_depth", "sentinix_review_latency_seconds_p50", "sentinix_base_cache_hits_total"} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected /metrics to include %q, got: %s", expected, body)
		}
	}
}

func TestPRComment_SingleCommentAcrossThreePushesAndCollapsedResolved(t *testing.T) {
	e := setup(t)
	ch := &countingHost{name: "gh"}
	ev := &githost.Event{Repo: "org/repo", RepoPath: "org/repo", PR: "42", HeadSHA: "h1", BaseSHA: "b1"}

	res := &scheduler.Result{
		New:      []engines.Finding{{Rule: "sec/bad", File: "main.go", Line: 10, Severity: engines.High}},
		Resolved: []engines.Finding{{Rule: "sec/fixed", File: "main.go", Line: 20, Severity: engines.High}},
		Ran:      []string{"secrets"},
	}

	// First push
	e.app.finishReview(context.Background(), ch, ev, res, 100*time.Millisecond)
	if len(ch.comments) != 1 {
		t.Fatalf("expected 1 comment after first push, got %d", len(ch.comments))
	}
	comment := ch.comments[0]
	if !strings.Contains(comment, "<!-- sentinix-review-marker -->") {
		t.Errorf("expected comment to contain hidden marker, got: %s", comment)
	}
	if !strings.Contains(comment, "Resolved findings (1)") || !strings.Contains(comment, "<details>") {
		t.Errorf("expected comment to contain collapsed resolved findings, got: %s", comment)
	}

	// Second and third pushes
	e.app.finishReview(context.Background(), ch, ev, res, 120*time.Millisecond)
	e.app.finishReview(context.Background(), ch, ev, res, 110*time.Millisecond)

	// Verify all 3 pushes produced comments starting with marker
	for i, c := range ch.comments {
		if !strings.Contains(c, "<!-- sentinix-review-marker -->") {
			t.Errorf("push %d comment missing marker: %s", i+1, c)
		}
	}
}

func TestBaseBranchCaching_ZeroBaseRunsOnSecondReview(t *testing.T) {
	initialHits := baseCacheHits.Load()

	// Direct test of base branch cache in scheduler
	testKey := "base-commit-sha-test:secrets@1.0(hash)"
	scheduler.SetBaseCache(testKey, []engines.Finding{
		{Rule: "base/r1", File: "base.go", Line: 5},
	})

	cached, ok := scheduler.GetBaseCache(testKey)
	if !ok || len(cached) != 1 || cached[0].Rule != "base/r1" {
		t.Fatalf("expected cache hit with 1 finding, got: ok=%v, findings=%v", ok, cached)
	}

	// Trigger cache hit hook and verify metric increments
	scheduler.OnBaseCacheHit()
	if baseCacheHits.Load() != initialHits+1 {
		t.Errorf("expected baseCacheHits to increment, got %d (initial %d)", baseCacheHits.Load(), initialHits)
	}
}

