package store

import (
	"database/sql"
	"encoding/json"
	"sentinix/pkg/advisories"
	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
)

// RecordPRScan stores a PR/MR scan run. PR scans never change project finding state.
func (s *Store) RecordPRScan(sc Scan) error {
	sc.ID, sc.Started = id(), now()
	_, err := s.db.Exec("insert into scans(id,project_id,mode,commit_sha,pr,started,duration_ms,new_count,fixed_count,unchanged_count,packs,engines,gate_decision,reason,skipped) values($1,$2,$3,$4,$5,$6,$7,$8,0,0,$9,$10,$11,$12,$13)",
		sc.ID, sc.ProjectID, sc.Mode, sc.Commit, sc.PR, sc.Started, sc.DurationMS, sc.New, sc.Packs, sc.Engines, sc.GateDecision, sc.Reason, sc.Skipped)
	return err
}

func (s *Store) AddSurface(project, commit, pr string, items []diffmap.Item) error {
	for _, i := range items {
		if _, err := s.db.Exec("insert into surface(id,project_id,commit_sha,pr,kind,change,file,detail,ts) values($1,$2,$3,$4,$5,$6,$7,$8,$9)",
			id(), project, commit, pr, i.Type, i.Change, i.File, i.Detail, now()); err != nil {
			return err
		}
	}
	return nil
}

type SurfaceRow struct {
	diffmap.Item
	Commit, PR string
	TS         int64
}

func (s *Store) Surface(project string, limit int) ([]SurfaceRow, error) {
	rows, err := s.db.Query("select kind,change,file,detail,commit_sha,pr,ts from surface where project_id=$1 order by ts desc, id limit $2", project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SurfaceRow
	for rows.Next() {
		var r SurfaceRow
		if err := rows.Scan(&r.Type, &r.Change, &r.File, &r.Detail, &r.Commit, &r.PR, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetRuleOverrideScoped sets a rule override with a project scope and expiry timestamp.
func (s *Store) SetRuleOverrideScoped(rule string, disabled bool, sev int, scope string, expiresAt int64) error {
	d := 0
	if disabled {
		d = 1
	}
	if scope == "" {
		scope = "*"
	}
	_, err := s.db.Exec("insert into rule_overrides(rule,disabled,severity,scope,expires_at) values($1,$2,$3,$4,$5) on conflict(rule, scope) do update set disabled=excluded.disabled, severity=excluded.severity, expires_at=excluded.expires_at", rule, d, sev, scope, expiresAt)
	return err
}

// Rule overrides are global. Loosening one is applied only after an approval.
func (s *Store) SetRuleOverride(rule string, disabled bool, sev int) error {
	return s.SetRuleOverrideScoped(rule, disabled, sev, "*", 0)
}

// RuleOverrides returns disabled rules and severity overrides (-1 means none).
func (s *Store) RuleOverrides() (map[string]bool, map[string]engines.Severity, error) {
	return s.RuleOverridesFor("")
}

// RuleOverridesFor returns active disabled rules and severity overrides for a project.
func (s *Store) RuleOverridesFor(project string) (map[string]bool, map[string]engines.Severity, error) {
	var altProject string
	if project != "" {
		var id, name string
		if err := s.db.QueryRow("select id, name from projects where id=$1 or name=$1", project).Scan(&id, &name); err == nil {
			if project == id {
				altProject = name
			} else {
				altProject = id
			}
		}
	}
	rows, err := s.db.Query("select rule,disabled,severity,scope,expires_at from rule_overrides")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	dis, sev := map[string]bool{}, map[string]engines.Severity{}
	currentTime := now()
	for rows.Next() {
		var r, sc string
		var d, v int
		var exp int64
		if err := rows.Scan(&r, &d, &v, &sc, &exp); err != nil {
			return nil, nil, err
		}
		if exp > 0 && currentTime > exp {
			continue
		}
		if project == "" {
			if sc != "*" {
				continue
			}
		} else {
			if sc != "*" && sc != project && (altProject == "" || sc != altProject) {
				continue
			}
		}
		if d == 1 {
			dis[r] = true
		}
		if v >= 0 {
			sev[r] = engines.Severity(v)
		}
	}
	return dis, sev, rows.Err()
}

type Posture struct {
	Open       [5]int      `json:"open"` // by severity, info..critical
	Weekly     []WeekPoint `json:"weekly"`
	AvgFixDays float64     `json:"avg_fix_days"`
	Oldest     []Finding   `json:"oldest_criticals"`
}

type WeekPoint struct {
	Week       int64 `json:"week"` // unix of week start
	New, Fixed int
}

// Posture summarises one project, or all when project is "".
func (s *Store) Posture(project string) (Posture, error) {
	var p Posture
	where, args := "", []any{}
	if project != "" {
		where, args = " and project_id=$1", []any{project}
	}
	rows, err := s.db.Query("select severity,count(*) from findings where status='open'"+where+" group by severity", args...)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var sev, n int
		rows.Scan(&sev, &n)
		if sev >= 0 && sev < 5 {
			p.Open[sev] = n
		}
	}
	rows.Close()

	wk := "select (started/604800)*604800, sum(new_count), sum(fixed_count) from scans where mode in ('full','ci')"
	if project != "" {
		wk += " and project_id=$1"
	}
	rows, err = s.db.Query(wk+" group by 1 order by 1 desc limit 8", args...)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var w WeekPoint
		rows.Scan(&w.Week, &w.New, &w.Fixed)
		p.Weekly = append(p.Weekly, w)
	}
	rows.Close()

	var avg *float64
	s.db.QueryRow("select avg(updated-first_seen) from findings where status='fixed'"+where, args...).Scan(&avg)
	if avg != nil {
		p.AvgFixDays = *avg / 86400
	}

	q := "select " + findCols + " from findings where status='open' and severity=4" + where + " order by first_seen limit 5"
	rows, err = s.db.Query(q, args...)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return p, err
		}
		p.Oldest = append(p.Oldest, f)
	}
	return p, rows.Err()
}

// NewSince lists findings first seen at or after ts (new in the scan that started then).
func (s *Store) NewSince(project string, ts int64) ([]Finding, error) {
	rows, err := s.db.Query("select "+findCols+" from findings where project_id=$1 and first_seen>=$2 and status='open' order by severity desc, id", project, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ReplaceComponents stores the project's current SBOM from its latest full scan.
func (s *Store) ReplaceComponents(project string, cs []advisories.Component) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("delete from components where project_id=$1", project); err != nil {
		return err
	}
	for _, c := range cs {
		if _, err := tx.Exec("insert into components(project_id,ecosystem,name,version,file) values($1,$2,$3,$4,$5)", project, c.Ecosystem, c.Name, c.Version, c.File); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Components(project string) ([]advisories.Component, error) {
	rows, err := s.db.Query("select ecosystem,name,version,file from components where project_id=$1 order by ecosystem,name,version", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []advisories.Component
	for rows.Next() {
		var c advisories.Component
		if err := rows.Scan(&c.Ecosystem, &c.Name, &c.Version, &c.File); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveVerdict keeps the latest AI verdict for a finding as JSON. Verdicts are advisory.
func (s *Store) SaveVerdict(findingID string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("insert into verdicts(finding_id,body,ts) values($1,$2,$3) on conflict(finding_id) do update set body=excluded.body, ts=excluded.ts", findingID, string(b), now())
	return err
}

func (s *Store) Verdict(findingID string) (json.RawMessage, error) {
	var b string
	err := s.db.QueryRow("select body from verdicts where finding_id=$1", findingID).Scan(&b)
	return json.RawMessage(b), err
}

// CheckAndRecordDelivery checks if delivery id was already seen within ttlSeconds, persists it, and bounds the set (TTL + size).
func (s *Store) CheckAndRecordDelivery(id string, ttlSeconds int64, maxSize int) (bool, error) {
	currentTime := now()
	// Prune expired
	if ttlSeconds > 0 {
		s.db.Exec("delete from webhook_deliveries where ts < $1", currentTime-ttlSeconds)
	}
	// Check if already seen
	var exists int
	err := s.db.QueryRow("select 1 from webhook_deliveries where id=$1", id).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	// Bound by size
	if maxSize > 0 {
		var count int
		if err := s.db.QueryRow("select count(*) from webhook_deliveries").Scan(&count); err == nil && count >= maxSize {
			s.db.Exec("delete from webhook_deliveries where id in (select id from webhook_deliveries order by ts asc limit 100)")
		}
	}
	// Persist
	_, err = s.db.Exec("insert into webhook_deliveries(id, ts) values($1, $2)", id, currentTime)
	return false, err
}

func (s *Store) EnqueueJob(host string, payload string) (string, error) {
	jobID := id()
	_, err := s.db.Exec("insert into durable_queue(id, host, payload, status, created) values($1, $2, $3, 'pending', $4)",
		jobID, host, payload, now())
	return jobID, err
}

func (s *Store) DequeueJob() (string, string, string, error) {
	var jobID, host, payload string

	// 1. Try atomic Postgres UPDATE ... RETURNING with FOR UPDATE SKIP LOCKED
	qPostgres := `
		update durable_queue
		set status = 'running'
		where id = (
			select id
			from durable_queue
			where status = 'pending'
			order by created asc
			for update skip locked
			limit 1
		)
		returning id, host, payload
	`
	err := s.db.QueryRow(qPostgres).Scan(&jobID, &host, &payload)
	if err == nil {
		return jobID, host, payload, nil
	}

	// 2. Fallback for SQLite: use an immediate transaction
	tx, txErr := s.db.Begin()
	if txErr != nil {
		return "", "", "", txErr
	}
	defer tx.Rollback()

	err = tx.QueryRow("select id, host, payload from durable_queue where status='pending' order by created asc limit 1").Scan(&jobID, &host, &payload)
	if err != nil {
		return "", "", "", err
	}
	res, err := tx.Exec("update durable_queue set status='running' where id=$1 and status='pending'", jobID)
	if err != nil {
		return "", "", "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", "", "", sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return "", "", "", err
	}
	return jobID, host, payload, nil
}

func (s *Store) FinishJob(jobID string) error {
	_, err := s.db.Exec("delete from durable_queue where id=$1", jobID)
	return err
}

func (s *Store) ReclaimStaleJobs() error {
	_, err := s.db.Exec("update durable_queue set status='pending' where status='running'")
	return err
}

func (s *Store) GrantProjectAccess(projectID, grantee string) error {
	_, err := s.db.Exec("insert into project_grants(project_id, grantee) values($1, $2) on conflict do nothing", projectID, grantee)
	return err
}

func (s *Store) RevokeProjectAccess(projectID, grantee string) error {
	_, err := s.db.Exec("delete from project_grants where project_id=$1 and grantee=$2", projectID, grantee)
	return err
}

func (s *Store) HasProjectAccess(email string, groups []string, projectID string) (bool, error) {
	var count int
	query := "select count(*) from project_grants where project_id=$1 and grantee=$2"
	if err := s.db.QueryRow(query, projectID, email).Scan(&count); err == nil && count > 0 {
		return true, nil
	}
	for _, g := range groups {
		if g == "" {
			continue
		}
		if err := s.db.QueryRow(query, projectID, g).Scan(&count); err == nil && count > 0 {
			return true, nil
		}
	}
	return false, nil
}
