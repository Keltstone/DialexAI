// Package store persists projects, scans, findings, approvals, users and the audit log.
// One SQL dialect subset runs on Postgres (production) and SQLite (dev, tests).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Store struct {
	db       *sql.DB
	auditKey []byte
	auditBad atomic.Bool
}

func (s *Store) SetAuditKey(k []byte) {
	s.auditKey = k
}

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
  accepted_reason text not null, accepted_until bigint not null, ticket text not null, source text not null default '', unique(project_id, fingerprint));
create table if not exists audit(id text primary key, seq bigint not null default 0, ts bigint not null, actor text not null, action text not null,
  target text not null, commit_sha text not null, approver text not null);
create table if not exists approvals(id text primary key, kind text not null, payload text not null, proposer text not null,
  status text not null, approver text not null, ts bigint not null);
create table if not exists users(id text primary key, email text not null unique, name text not null, role text not null, created bigint not null);
create table if not exists rule_overrides(rule text not null, disabled bigint not null, severity bigint not null, scope text not null default '*', expires_at bigint not null default 0, primary key(rule, scope));
create table if not exists surface(id text primary key, project_id text not null, commit_sha text not null, pr text not null,
  kind text not null, change text not null, file text not null, detail text not null, ts bigint not null);
create table if not exists components(project_id text not null, ecosystem text not null, name text not null,
  version text not null, file text not null);
create table if not exists verdicts(finding_id text primary key, body text not null, ts bigint not null);
create table if not exists tokens(hash text primary key, project_id text not null, name text not null, created bigint not null);
create table if not exists webhook_deliveries(id text primary key, ts bigint not null);
create table if not exists durable_queue(id text primary key, host text not null, payload text not null, status text not null, created bigint not null);
create table if not exists leader_election(lock_key text primary key, holder text not null, expires_at bigint not null);
create table if not exists skipped_file_overrides(project_id text not null default '*', file text not null, expires_at bigint not null, reason text not null default '', approver text not null default '', ts bigint not null default 0, primary key(project_id, file));
create table if not exists project_grants(project_id text not null, grantee text not null, primary key(project_id, grantee))`

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
		"alter table durable_queue add column lease_until bigint not null default 0",
		"alter table users add column revoked_before bigint not null default 0",
		"alter table audit add column seq bigint not null default 0",
		"alter table audit add column source text not null default ''",
		"alter table audit add column before_val text not null default ''",
		"alter table audit add column after_val text not null default ''",
		"alter table tokens add column expires_at bigint not null default 0",
		"alter table tokens add column revoked bigint not null default 0",
		"alter table rule_overrides add column scope text not null default '*'",
		"alter table rule_overrides add column expires_at bigint not null default 0",
		"alter table audit add column engines text not null default ''",
		"alter table audit add column pack_versions text not null default ''",
		"alter table audit add column skipped_files text not null default ''",
		"alter table audit add column mode text not null default ''",
		"alter table audit add column outcome text not null default ''",
		"alter table audit add column reason text not null default ''",
		"alter table scans add column gate_decision text not null default ''",
		"alter table scans add column reason text not null default ''",
		"alter table scans add column skipped text not null default ''",
		"alter table audit add column prev_hash text not null default ''",
		"alter table audit add column hash text not null default ''",
		"alter table findings add column source text not null default ''",
		"create table if not exists audit_counter(id int primary key, last_seq bigint not null)",
		"create unique index if not exists idx_audit_seq on audit(seq)",
	} {
		if _, err := db.Exec(q); err != nil && !strings.Contains(err.Error(), "duplicate column") && !strings.Contains(err.Error(), "already exists") {
			return nil, err
		}
	}
	db.Exec("insert into audit_counter(id, last_seq) values(1, 0) on conflict do nothing")
	st := &Store{db: db}
	if k := os.Getenv("SENTINIX_AUDIT_KEY"); k != "" {
		st.auditKey = []byte(k)
	} else if k := os.Getenv("SENTINIX_SESSION_KEY"); k != "" {
		st.auditKey = []byte(k)
	}
	return st, nil
}


func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func id() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func now() int64 { return time.Now().Unix() }

func (s *Store) TryAcquireLeader(ctx context.Context, key, holder string, ttl time.Duration) (bool, error) {
	nowUnix := time.Now().Unix()
	expiresUnix := nowUnix + int64(ttl.Seconds())

	// For Postgres, also attempt pg_try_advisory_lock
	var isPG bool
	var dummy int
	if err := s.db.QueryRowContext(ctx, "select 1 from information_schema.tables limit 1").Scan(&dummy); err == nil {
		isPG = true
	}
	if isPG {
		var acquired bool
		lockID := int64(777666555)
		if err := s.db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err == nil && !acquired {
			return false, nil
		}
	}

	// Portable table-based leader lock for both SQLite and Postgres
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var curHolder string
	var expiresAt int64
	err = tx.QueryRowContext(ctx, "select holder, expires_at from leader_election where lock_key=$1", key).Scan(&curHolder, &expiresAt)
	if err != nil { // not exists
		if _, err := tx.ExecContext(ctx, "insert into leader_election(lock_key, holder, expires_at) values($1, $2, $3)", key, holder, expiresUnix); err != nil {
			return false, nil
		}
		return tx.Commit() == nil, nil
	}
	if curHolder == holder || expiresAt <= nowUnix {
		if _, err := tx.ExecContext(ctx, "update leader_election set holder=$1, expires_at=$2 where lock_key=$3", holder, expiresUnix, key); err != nil {
			return false, nil
		}
		return tx.Commit() == nil, nil
	}
	return false, nil
}

