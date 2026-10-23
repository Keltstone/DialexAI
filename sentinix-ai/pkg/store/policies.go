package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sentinix/pkg/engines"
)

type CategoryPolicy struct {
	Mode      string `json:"mode"`      // "block" | "comment" | "off"
	Threshold string `json:"threshold"` // "info" | "low" | "medium" | "high" | "critical"
}

// DefaultCategoryPolicies returns the baseline defaults specified in the review orchestrator spec (section 7).
func DefaultCategoryPolicies() map[string]CategoryPolicy {
	return map[string]CategoryPolicy{
		engines.CategorySecurity:      {Mode: "block", Threshold: "high"},
		engines.CategoryArchitecture:  {Mode: "comment", Threshold: "medium"},
		engines.CategoryAccessibility: {Mode: "comment", Threshold: "medium"},
		engines.CategoryStyle:         {Mode: "comment", Threshold: "low"},
		engines.CategoryQuality:       {Mode: "comment", Threshold: "medium"},
	}
}

// CategoryPolicies returns the effective policies per category for a project,
// combining defaults, existing project Enforcement/FailOn (for backward compatibility),
// and any stored per-category overrides.
func (s *Store) CategoryPolicies(projectID string) (map[string]CategoryPolicy, error) {
	p, err := s.Project(projectID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	res := DefaultCategoryPolicies()

	// If project exists, sync security row with project's Enforcement and FailOn
	if p.ID != "" {
		sec := res[engines.CategorySecurity]
		if p.Enforcement == "report" {
			sec.Mode = "comment"
		} else if p.Enforcement == "block" {
			sec.Mode = "block"
		}
		if p.FailOn != "" {
			sec.Threshold = p.FailOn
		}
		res[engines.CategorySecurity] = sec
	}

	rows, err := s.db.Query("select category, mode, threshold from project_policies where project_id=$1", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var cat, mode, thresh string
		if err := rows.Scan(&cat, &mode, &thresh); err != nil {
			return nil, err
		}
		res[cat] = CategoryPolicy{Mode: mode, Threshold: thresh}
	}

	return res, rows.Err()
}

// CategoryPolicy returns the effective policy for a single category of a project.
func (s *Store) CategoryPolicy(projectID, category string) (CategoryPolicy, error) {
	pols, err := s.CategoryPolicies(projectID)
	if err != nil {
		return CategoryPolicy{}, err
	}
	if p, ok := pols[category]; ok {
		return p, nil
	}
	// Fallback to default
	defs := DefaultCategoryPolicies()
	if d, ok := defs[category]; ok {
		return d, nil
	}
	return CategoryPolicy{Mode: "comment", Threshold: "medium"}, nil
}

// SetCategoryPolicy updates a category policy and maintains synchronization
// with projects.enforcement and projects.fail_on for the security category.
func (s *Store) SetCategoryPolicy(projectID, category, mode, threshold string) error {
	if !engines.IsValidCategory(category) {
		return fmt.Errorf("invalid category: %s", category)
	}
	if mode != "block" && mode != "comment" && mode != "off" {
		return fmt.Errorf("invalid mode: %s", mode)
	}
	if _, err := engines.ParseSeverity(threshold); err != nil {
		return fmt.Errorf("invalid threshold %q: %w", threshold, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("delete from project_policies where project_id=$1 and category=$2", projectID, category)
	if err != nil {
		return err
	}
	_, err = tx.Exec("insert into project_policies(project_id, category, mode, threshold) values($1, $2, $3, $4)",
		projectID, category, mode, threshold)
	if err != nil {
		return err
	}

	if category == engines.CategorySecurity {
		enf := "block"
		if mode == "comment" || mode == "off" {
			enf = "report"
		}
		_, err = tx.Exec("update projects set enforcement=$1, fail_on=$2 where id=$3", enf, threshold, projectID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

type IngestUpload struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Commit       string `json:"commit"`
	PR           string `json:"pr"`
	Engine       string `json:"engine"`
	Category     string `json:"category"`
	Format       string `json:"format"`
	ReceivedAt   int64  `json:"received_at"`
	ParseStatus  string `json:"parse_status"` // "success" | "failed"
	FindingCount int    `json:"finding_count"`
	FindingsJSON string `json:"findings_json,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Attested     bool   `json:"attested"`
}

// RecordIngestUpload idempotently records or replaces an upload for (project, commit, engine).
func (s *Store) RecordIngestUpload(up IngestUpload) error {
	if up.ID == "" {
		up.ID = id()
	}
	if up.ReceivedAt == 0 {
		up.ReceivedAt = now()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("delete from ingest_uploads where project_id=$1 and commit_sha=$2 and engine=$3",
		up.ProjectID, up.Commit, up.Engine)
	if err != nil {
		return err
	}

	attestedInt := 0
	if up.Attested {
		attestedInt = 1
	}

	_, err = tx.Exec("insert into ingest_uploads(id, project_id, commit_sha, pr, engine, category, format, received_at, parse_status, finding_count, findings_json, error_message, attested) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)",
		up.ID, up.ProjectID, up.Commit, up.PR, up.Engine, up.Category, up.Format, up.ReceivedAt, up.ParseStatus, up.FindingCount, up.FindingsJSON, up.ErrorMessage, attestedInt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// IngestUploads returns all recorded ingest uploads for a commit in a project.
func (s *Store) IngestUploads(projectID, commitSHA string) ([]IngestUpload, error) {
	rows, err := s.db.Query("select id, project_id, commit_sha, pr, engine, category, format, received_at, parse_status, finding_count, findings_json, error_message, attested from ingest_uploads where project_id=$1 and commit_sha=$2 order by engine",
		projectID, commitSHA)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IngestUpload
	for rows.Next() {
		var u IngestUpload
		var att int
		if err := rows.Scan(&u.ID, &u.ProjectID, &u.Commit, &u.PR, &u.Engine, &u.Category, &u.Format, &u.ReceivedAt, &u.ParseStatus, &u.FindingCount, &u.FindingsJSON, &u.ErrorMessage, &att); err != nil {
			return nil, err
		}
		u.Attested = att == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteIngestUpload removes an ingest upload record (used to simulate removed uploads or cleanup).
func (s *Store) DeleteIngestUpload(projectID, commitSHA, engine string) error {
	_, err := s.db.Exec("delete from ingest_uploads where project_id=$1 and commit_sha=$2 and engine=$3",
		projectID, commitSHA, engine)
	return err
}

// RecordIngestSignature checks and records a signature in the replay cache.
// Returns an error if the signature has already been used (replay detected).
func (s *Store) RecordIngestSignature(sig string, projectID string, ts int64) error {
	sigHash := sha256.Sum256([]byte(sig))
	sigHex := hex.EncodeToString(sigHash[:])
	currentTime := now()

	// Clean up signatures older than 1800 seconds (30 minutes)
	cutoff := currentTime - 1800
	_, _ = s.db.Exec("delete from ingest_signatures where created_at < $1", cutoff)

	_, err := s.db.Exec("insert into ingest_signatures(sig_hash, project_id, ts, created_at) values($1, $2, $3, $4)",
		sigHex, projectID, ts, currentTime)
	return err
}


// SavePRReviewResult persists the outcome of a full PR review scan for rebuilding on upload.
func (s *Store) SavePRReviewResult(projectID, pr, commit string, ran, skipped []string, newFs, resolvedFs []engines.Finding) error {
	newJSON, _ := json.Marshal(newFs)
	resolvedJSON, _ := json.Marshal(resolvedFs)
	ranStr := strings.Join(ran, ",")
	skippedStr := strings.Join(skipped, ",")
	t := now()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, _ = tx.Exec("delete from pr_review_results where project_id=$1 and pr=$2 and commit_sha=$3", projectID, pr, commit)
	_, err = tx.Exec("insert into pr_review_results(project_id, pr, commit_sha, ran, skipped, new_findings_json, resolved_findings_json, updated_at) values($1, $2, $3, $4, $5, $6, $7, $8)",
		projectID, pr, commit, ranStr, skippedStr, string(newJSON), string(resolvedJSON), t)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetPRReviewResult retrieves the persisted PR review result for (project, pr, commit).
func (s *Store) GetPRReviewResult(projectID, pr, commit string) (ran, skipped []string, newFs, resolvedFs []engines.Finding, found bool, err error) {
	var ranStr, skippedStr, newJSON, resolvedJSON string
	var updatedAt int64
	err = s.db.QueryRow("select ran, skipped, new_findings_json, resolved_findings_json, updated_at from pr_review_results where project_id=$1 and pr=$2 and commit_sha=$3",
		projectID, pr, commit).Scan(&ranStr, &skippedStr, &newJSON, &resolvedJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	if ranStr != "" {
		ran = strings.Split(ranStr, ",")
	}
	if skippedStr != "" {
		skipped = strings.Split(skippedStr, ",")
	}
	_ = json.Unmarshal([]byte(newJSON), &newFs)
	_ = json.Unmarshal([]byte(resolvedJSON), &resolvedFs)
	return ran, skipped, newFs, resolvedFs, true, nil
}

// isServerOwnedEngine returns true if the engine is managed by the server.
func isServerOwnedEngine(e string) bool {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case "secrets", "gitleaks", "semgrep", "osv-scanner", "packs", "advisories":
		return true
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(e)), "pack:") {
		return true
	}
	return false
}

// CloseEngineFindings marks open findings from this engine that were not seen in current upload as fixed.
// Lifecycle is strictly scoped:
// - Server-owned engines (secrets, gitleaks, semgrep, osv-scanner, packs, advisories) can NEVER be closed by ingest.
// - Ingest uploads can only close findings whose source matches the same ingest origin (e.g. ci-unattested or ci-attested).
// - Scoped by PR via finding_prs link table: closing PR2 never closes a finding PR1 still reports;
//   findings are only marked fixed when no other PR/branch still reports them.
func (s *Store) CloseEngineFindings(projectID, engine, pr, source string, currentFingerprints []string, uploadTs int64) error {
	if isServerOwnedEngine(engine) || source == "" {
		return nil
	}
	t := now()
	seen := make(map[string]bool, len(currentFingerprints))
	for _, fp := range currentFingerprints {
		seen[fp] = true
		if pr != "" {
			_, _ = s.db.Exec("insert into finding_prs(project_id,fingerprint,pr,engine) values($1,$2,$3,$4) on conflict do nothing",
				projectID, fp, pr, engine)
		}
	}

	var rows *sql.Rows
	var err error
	if pr != "" {
		if uploadTs > 0 {
			rows, err = s.db.Query(`select distinct fp.fingerprint from finding_prs fp 
				join findings f on f.project_id=fp.project_id and f.fingerprint=fp.fingerprint 
				where fp.project_id=$1 and fp.engine=$2 and fp.pr=$3 and f.source=$4 and f.status='open' and f.last_seen<=$5
				union 
				select fingerprint from findings where project_id=$1 and engine=$2 and pr=$3 and source=$4 and status='open' and last_seen<=$5`,
				projectID, engine, pr, source, uploadTs)
		} else {
			rows, err = s.db.Query(`select distinct fp.fingerprint from finding_prs fp 
				join findings f on f.project_id=fp.project_id and f.fingerprint=fp.fingerprint 
				where fp.project_id=$1 and fp.engine=$2 and fp.pr=$3 and f.source=$4 and f.status='open'
				union 
				select fingerprint from findings where project_id=$1 and engine=$2 and pr=$3 and source=$4 and status='open'`,
				projectID, engine, pr, source)
		}
	} else {
		if uploadTs > 0 {
			rows, err = s.db.Query("select fingerprint from findings where project_id=$1 and engine=$2 and pr='' and source=$3 and status='open' and last_seen<=$4",
				projectID, engine, source, uploadTs)
		} else {
			rows, err = s.db.Query("select fingerprint from findings where project_id=$1 and engine=$2 and pr='' and source=$3 and status='open'",
				projectID, engine, source)
		}
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	var toClose []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err == nil {
			if !seen[fp] {
				toClose = append(toClose, fp)
			}
		}
	}
	for _, fp := range toClose {
		if pr != "" {
			_, _ = s.db.Exec("delete from finding_prs where project_id=$1 and engine=$2 and pr=$3 and fingerprint=$4",
				projectID, engine, pr, fp)
			var remaining int
			var otherPR string
			_ = s.db.QueryRow("select count(*), coalesce(max(pr), '') from finding_prs where project_id=$1 and fingerprint=$2",
				projectID, fp).Scan(&remaining, &otherPR)
			if remaining > 0 {
				if otherPR != "" {
					_, _ = s.db.Exec("update findings set pr=$1, updated=$2 where project_id=$3 and fingerprint=$4 and status='open'",
						otherPR, t, projectID, fp)
				}
				continue
			}
		}
		if _, err := s.db.Exec("update findings set status='fixed', updated=$1 where project_id=$2 and fingerprint=$3 and source=$4", t, projectID, fp, source); err != nil {
			return err
		}
	}
	return nil
}
