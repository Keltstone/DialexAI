package store

import (
	"encoding/json"
	"sentinix/pkg/advisories"
	"sentinix/pkg/diffmap"
	"sentinix/pkg/engines"
)

// RecordPRScan stores a PR/MR scan run. PR scans never change project finding state.
func (s *Store) RecordPRScan(sc Scan) error {
	sc.ID, sc.Started = id(), now()
	_, err := s.db.Exec("insert into scans(id,project_id,mode,commit_sha,pr,started,duration_ms,new_count,fixed_count,unchanged_count,packs,engines) values($1,$2,$3,$4,$5,$6,$7,$8,0,0,$9,$10)",
		sc.ID, sc.ProjectID, sc.Mode, sc.Commit, sc.PR, sc.Started, sc.DurationMS, sc.New, sc.Packs, sc.Engines)
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

// Rule overrides are global. Loosening one is applied only after an approval.
func (s *Store) SetRuleOverride(rule string, disabled bool, sev int) error {
	d := 0
	if disabled {
		d = 1
	}
	_, err := s.db.Exec("insert into rule_overrides(rule,disabled,severity) values($1,$2,$3) on conflict(rule) do update set disabled=excluded.disabled, severity=excluded.severity", rule, d, sev)
	return err
}

// RuleOverrides returns disabled rules and severity overrides (-1 means none).
func (s *Store) RuleOverrides() (map[string]bool, map[string]engines.Severity, error) {
	rows, err := s.db.Query("select rule,disabled,severity from rule_overrides")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	dis, sev := map[string]bool{}, map[string]engines.Severity{}
	for rows.Next() {
		var r string
		var d, v int
		if err := rows.Scan(&r, &d, &v); err != nil {
			return nil, nil, err
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
