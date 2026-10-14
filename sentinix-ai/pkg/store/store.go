// Package store persists projects, scans, findings, approvals, users and the audit log.
// One SQL dialect subset runs on Postgres (production) and SQLite (dev, tests).
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
create table if not exists projects(id text primary key, name text not null, host text not null, repo text not null,
  branch text not null, schedule text not null, tz text not null, pack_pins text not null, ai bigint not null,
  model_policy text not null, created bigint not null);
create table if not exists scans(id text primary key, project_id text not null, mode text not null, commit_sha text not null,
  pr text not null, started bigint not null, duration_ms bigint not null, new_count bigint not null, fixed_count bigint not null,
  unchanged_count bigint not null, packs text not null, engines text not null);
create table if not exists findings(id text primary key, project_id text not null, fingerprint text not null, rule text not null,
  engine text not null, file text not null, line bigint not null, severity bigint not null, message text not null,
  evidence text not null, cwe text not null, fix text not null, status text not null, owner text not null,
  first_commit text not null, first_seen bigint not null, last_seen bigint not null, updated bigint not null,
  accepted_reason text not null, accepted_until bigint not null, ticket text not null, unique(project_id, fingerprint));
create table if not exists audit(id text primary key, ts bigint not null, actor text not null, action text not null,
  target text not null, commit_sha text not null, approver text not null);
create table if not exists approvals(id text primary key, kind text not null, payload text not null, proposer text not null,
  status text not null, approver text not null, ts bigint not null);
create table if not exists users(id text primary key, email text not null unique, name text not null, role text not null, created bigint not null);
create table if not exists rule_overrides(rule text primary key, disabled bigint not null, severity bigint not null);
create table if not exists surface(id text primary key, project_id text not null, commit_sha text not null, pr text not null,
  kind text not null, change text not null, file text not null, detail text not null, ts bigint not null);
create table if not exists components(project_id text not null, ecosystem text not null, name text not null,
  version text not null, file text not null);
create table if not exists verdicts(finding_id text primary key, body text not null, ts bigint not null);
create table if not exists tokens(hash text primary key, project_id text not null, name text not null, created bigint not null)`

// Open takes "postgres://…" or "sqlite:<path>" (":memory:" works for tests).
func Open(dsn string) (*Store, error) {
	driver, name := "sqlite", strings.TrimPrefix(dsn, "sqlite:")
	if strings.HasPrefix(dsn, "postgres") {
		driver, name = "pgx", dsn
	}
	db, err := sql.Open(driver, name)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1) // SQLite: one writer, and :memory: is per-connection
	}
	for _, q := range strings.Split(schema, ";") {
		if _, err := db.Exec(q); err != nil {
			return nil, err
		}
	}
	// columns added after the first release; an "already exists" error means the database is current
	for _, q := range []string{
		"alter table projects add column enforcement text not null default 'block'",
		"alter table projects add column fail_on text not null default ''",
	} {
		if _, err := db.Exec(q); err != nil && !strings.Contains(err.Error(), "duplicate column") && !strings.Contains(err.Error(), "already exists") {
			return nil, err
		}
	}
	return &Store{db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func id() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func now() int64 { return time.Now().Unix() }
