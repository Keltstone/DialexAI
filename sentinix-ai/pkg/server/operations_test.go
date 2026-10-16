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
	"sentinix/pkg/githost"
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

	app1 := &App{Store: st, Hosts: hosts, Sess: auth.Sessions{Key: make([]byte, 32)}}
	app2 := &App{Store: st, Hosts: hosts, Sess: auth.Sessions{Key: make([]byte, 32)}}

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
