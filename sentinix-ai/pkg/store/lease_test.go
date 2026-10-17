package store

import (
	"os"
	"testing"
)

// Two replicas on one DB: a second Open must not steal a live job; an expired lease is reclaimed.
func TestJobLeaseTwoReplicas(t *testing.T) {
	dsn := os.Getenv("PGDSN")
	if dsn == "" {
		t.Skip("set PGDSN to a postgres:// URL")
	}
	a, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	a.db.Exec("delete from durable_queue")
	if _, err := a.EnqueueJob("h", "{}"); err != nil {
		t.Fatal(err)
	}
	id, _, _, err := a.DequeueJob()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(dsn) // replica 2 starts while replica 1 is mid-scan
	if err != nil {
		t.Fatal(err)
	}
	b.ReclaimStaleJobs()
	if _, _, _, err := b.DequeueJob(); err == nil {
		t.Fatal("replica 2 stole a live job")
	}
	a.db.Exec("update durable_queue set lease_until=1 where id=$1", id) // replica 1 killed -9: lease lapses
	b.ReclaimStaleJobs()
	got, _, _, err := b.DequeueJob()
	if err != nil || got != id {
		t.Fatalf("expired job not reclaimed: %v %q", err, got)
	}
}
