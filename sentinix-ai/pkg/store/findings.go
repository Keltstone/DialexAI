package store

import (
	"fmt"
	"strings"

	"sentinix/pkg/engines"
)

type Finding struct {
	engines.Finding
	ID, ProjectID, Fingerprint, Status, Owner, FirstCommit, PR string
	FirstSeen, LastSeen                                        int64
	AcceptedReason                                             string
	AcceptedUntil                                              int64 // unix; accepted risks reopen after this
	Ticket                                                     string
}

type Scan struct {
	ID, ProjectID, Mode, Commit, PR string
	Started, DurationMS             int64
	New, Fixed, Unchanged           int
	Packs, Engines                  string
	GateDecision, Reason, Skipped   string
	Source                          string
}

const findCols = "id,project_id,fingerprint,rule,engine,file,line,severity,message,evidence,cwe,fix,status,owner,first_commit,first_seen,last_seen,accepted_reason,accepted_until,ticket,source,category,pr"

func scanFinding(r interface{ Scan(...any) error }) (Finding, error) {
	var f Finding
	var sev int
	err := r.Scan(&f.ID, &f.ProjectID, &f.Fingerprint, &f.Rule, &f.Engine, &f.File, &f.Line, &sev, &f.Message, &f.Evidence,
		&f.CWE, &f.Fix, &f.Status, &f.Owner, &f.FirstCommit, &f.FirstSeen, &f.LastSeen, &f.AcceptedReason, &f.AcceptedUntil, &f.Ticket, &f.Source, &f.Category, &f.PR)
	f.Severity = engines.Severity(sev)
	if f.Category == "" {
		f.Category = "security"
	}
	return f, err
}

// engineKey maps pack findings back to the "packs" engine that produced them.
func engineKey(e string) string {
	if strings.HasPrefix(e, "pack:") {
		return "packs"
	}
	return e
}

// RecordScan stores a full scan. Findings are new, unchanged, or reopened if they had been fixed;
// open findings from an engine that ran but no longer reports them become fixed.
// Engines that did not run cannot fix anything, so a missing binary never looks like a clean repo.
func (s *Store) RecordScan(sc Scan, ran []string, fs []engines.Finding) (Scan, error) {
	sc.ID, sc.Started = id(), now()
	tx, err := s.db.Begin()
	if err != nil {
		return sc, err
	}
	defer tx.Rollback()
	t := now()
	// accepted risks past their expiry come back
	if _, err := tx.Exec("update findings set status='open',updated=$2 where project_id=$1 and status='accepted' and accepted_until>0 and accepted_until<$2", sc.ProjectID, t); err != nil {
		return sc, err
	}
	seen := map[string]bool{}
	for _, f := range fs {
		fp := f.Fingerprint()
		seen[fp] = true
		src := f.Source
		if src == "" {
			src = sc.Source
		}
		cat := f.Category
		if cat == "" {
			cat = "security"
		}
		var st string
		err := tx.QueryRow("select status from findings where project_id=$1 and fingerprint=$2", sc.ProjectID, fp).Scan(&st)
		switch {
		case err != nil: // not found
			_, err = tx.Exec("insert into findings("+findCols+",updated) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'open','',$13,$14,$14,'',0,'',$15,$16,$17,$14)",
				id(), sc.ProjectID, fp, f.Rule, f.Engine, f.File, f.Line, int(f.Severity), f.Message, f.Evidence, f.CWE, f.Fix, sc.Commit, t, src, cat, sc.PR)
			if err == nil && sc.PR != "" {
				_, _ = tx.Exec("insert into finding_prs(project_id,fingerprint,pr,engine) values($1,$2,$3,$4) on conflict do nothing",
					sc.ProjectID, fp, sc.PR, f.Engine)
			}
			sc.New++
		case st == "fixed":
			if src == "ci-unattested" || sc.Source == "ci-unattested" {
				// Reopening a fixed finding from an unattested upload is refused.
				continue
			}
			_, err = tx.Exec("update findings set status='open',line=$3,last_seen=$4,updated=$4,severity=$5,source=$6,category=$7,pr=$8 where project_id=$1 and fingerprint=$2",
				sc.ProjectID, fp, f.Line, t, int(f.Severity), src, cat, sc.PR)
			if err == nil && sc.PR != "" {
				_, _ = tx.Exec("insert into finding_prs(project_id,fingerprint,pr,engine) values($1,$2,$3,$4) on conflict do nothing",
					sc.ProjectID, fp, sc.PR, f.Engine)
			}
			sc.New++
		default:
			_, err = tx.Exec("update findings set line=$3,last_seen=$4,category=$5,pr=$6 where project_id=$1 and fingerprint=$2", sc.ProjectID, fp, f.Line, t, cat, sc.PR)
			if err == nil && sc.PR != "" {
				_, _ = tx.Exec("insert into finding_prs(project_id,fingerprint,pr,engine) values($1,$2,$3,$4) on conflict do nothing",
					sc.ProjectID, fp, sc.PR, f.Engine)
			}
			sc.Unchanged++
		}
		if err != nil {
			return sc, err
		}
	}
	ranSet := map[string]bool{}
	for _, r := range ran {
		ranSet[r] = true
	}
	rows, err := tx.Query("select fingerprint,engine from findings where project_id=$1 and status='open'", sc.ProjectID)
	if err != nil {
		return sc, err
	}
	var gone []string
	for rows.Next() {
		var fp, e string
		rows.Scan(&fp, &e)
		if !seen[fp] && ranSet[engineKey(e)] {
			gone = append(gone, fp)
		}
	}
	rows.Close()
	var reallyFixed int
	for _, fp := range gone {
		if sc.PR != "" {
			_, _ = tx.Exec("delete from finding_prs where project_id=$1 and pr=$2 and fingerprint=$3", sc.ProjectID, sc.PR, fp)
			var remaining int
			var otherPR string
			_ = tx.QueryRow("select count(*), coalesce(max(pr), '') from finding_prs where project_id=$1 and fingerprint=$2", sc.ProjectID, fp).Scan(&remaining, &otherPR)
			if remaining > 0 {
				if otherPR != "" {
					_, _ = tx.Exec("update findings set pr=$1, updated=$2 where project_id=$3 and fingerprint=$4 and status='open'", otherPR, t, sc.ProjectID, fp)
				}
				continue
			}
		}
		if _, err := tx.Exec("update findings set status='fixed',updated=$3 where project_id=$1 and fingerprint=$2", sc.ProjectID, fp, t); err != nil {
			return sc, err
		}
		reallyFixed++
	}
	sc.Fixed = reallyFixed
	sc.Engines = strings.Join(ran, ",")
	_, err = tx.Exec("insert into scans(id,project_id,mode,commit_sha,pr,started,duration_ms,new_count,fixed_count,unchanged_count,packs,engines) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)",
		sc.ID, sc.ProjectID, sc.Mode, sc.Commit, sc.PR, sc.Started, sc.DurationMS, sc.New, sc.Fixed, sc.Unchanged, sc.Packs, sc.Engines)
	if err != nil {
		return sc, err
	}
	return sc, tx.Commit()
}

// LastScan is the previous run of a project, for run-to-run comparison.
func (s *Store) Scans(project string, limit int) ([]Scan, error) {
	rows, err := s.db.Query("select id,project_id,mode,commit_sha,pr,started,duration_ms,new_count,fixed_count,unchanged_count,packs,engines from scans where project_id=$1 order by started desc, id limit $2", project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Scan
	for rows.Next() {
		var x Scan
		if err := rows.Scan(&x.ID, &x.ProjectID, &x.Mode, &x.Commit, &x.PR, &x.Started, &x.DurationMS, &x.New, &x.Fixed, &x.Unchanged, &x.Packs, &x.Engines); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

type Filter struct {
	Project, Status, Owner, Category string
	MinSeverity                      engines.Severity
	Limit                            int
}

func (s *Store) Findings(f Filter) ([]Finding, error) {
	q, args := "select "+findCols+" from findings where severity>=$1", []any{int(f.MinSeverity)}
	for col, v := range map[string]string{"project_id": f.Project, "status": f.Status, "owner": f.Owner, "category": f.Category} {
		if v != "" {
			args = append(args, v)
			q += fmt.Sprintf(" and %s=$%d", col, len(args))
		}
	}
	if f.Limit <= 0 {
		f.Limit = 500
	}
	args = append(args, f.Limit)
	rows, err := s.db.Query(q+fmt.Sprintf(" order by severity desc, last_seen desc, id limit $%d", len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		x, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) Finding(id string) (Finding, error) {
	return scanFinding(s.db.QueryRow("select "+findCols+" from findings where id=$1", id))
}

// Triage sets status (open, fixed, accepted, false_positive) and owner.
// Accepting a risk needs a reason and an expiry, so exceptions get revisited.
func (s *Store) Triage(id, status, owner, reason string, until int64) error {
	switch status {
	case "open", "false_positive":
	case "accepted":
		if reason == "" || until <= now() {
			return fmt.Errorf("accepted risk needs a reason and a future expiry")
		}
	default:
		return fmt.Errorf("bad status %q", status)
	}
	_, err := s.db.Exec("update findings set status=$2,owner=$3,accepted_reason=$4,accepted_until=$5,updated=$6 where id=$1", id, status, owner, reason, until, now())
	return err
}

func (s *Store) SetTicket(id, ticket string) error {
	_, err := s.db.Exec("update findings set ticket=$2 where id=$1", id, ticket)
	return err
}

type RuleStat struct {
	Rule             string
	Total, Dismissed int
	Precision        float64
}

// RuleStats covers findings last touched in the past 90 days: dismissed = marked false positive.
func (s *Store) RuleStats() ([]RuleStat, error) {
	rows, err := s.db.Query("select rule, count(*), sum(case when status='false_positive' then 1 else 0 end) from findings where updated>=$1 group by rule order by rule", now()-90*86400)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuleStat
	for rows.Next() {
		var r RuleStat
		if err := rows.Scan(&r.Rule, &r.Total, &r.Dismissed); err != nil {
			return nil, err
		}
		r.Precision = float64(r.Total-r.Dismissed) / float64(r.Total)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Demoted lists rules below the precision floor (with enough samples) to be shown as informational.
// They are flagged, never deleted.
func (s *Store) Demoted(floor float64, minSamples int) (map[string]engines.Severity, error) {
	stats, err := s.RuleStats()
	m := map[string]engines.Severity{}
	for _, r := range stats {
		if r.Total >= minSamples && r.Precision < floor {
			m[r.Rule] = engines.Info
		}
	}
	return m, err
}
