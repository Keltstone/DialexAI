package store

import (
	"unicode"

	"golang.org/x/text/unicode/norm"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sentinix/pkg/engines"
)

type Audit struct {
	ID, Actor, Action, Target, Commit, Approver string
	Source, Before, After                       string
	Engines, PackVersions, SkippedFiles         string
	Mode, Outcome, Reason                       string
	PrevHash, Hash                              string
	TS                                          int64
	Seq                                         int64
}

func computeAuditHash(prevHash string, a Audit, key []byte) string {
	payload := fmt.Sprintf("%d|%s|%s|%d|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		a.Seq, prevHash, a.ID, a.TS, a.Actor, a.Action, a.Target, a.Commit, a.Approver,
		a.Source, a.Before, a.After, a.Engines, a.PackVersions, a.SkippedFiles,
		a.Mode, a.Outcome, a.Reason)
	if len(key) > 0 {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(payload))
		return hex.EncodeToString(mac.Sum(nil))
	}
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

func addAuditTx(tx *sql.Tx, a Audit, key []byte) error {
	var dummy any
	_ = tx.QueryRow("select pg_advisory_xact_lock(718293)").Scan(&dummy)
	_, _ = tx.Exec("update audit_counter set last_seq=last_seq where id=1")

	if a.ID == "" {
		a.ID = id()
	}
	if a.TS == 0 {
		a.TS = now()
	}
	var maxSeq int64
	var prevHash string
	row := tx.QueryRow("select seq, hash from audit order by seq desc limit 1")
	err := row.Scan(&maxSeq, &prevHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read previous audit hash: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		maxSeq = 0
		prevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	}
	a.Seq = maxSeq + 1
	a.PrevHash = prevHash
	a.Hash = computeAuditHash(prevHash, a, key)
	_, err = tx.Exec("insert into audit(id,seq,ts,actor,action,target,commit_sha,approver,source,before_val,after_val,engines,pack_versions,skipped_files,mode,outcome,reason,prev_hash,hash) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)",
		a.ID, a.Seq, a.TS, a.Actor, a.Action, a.Target, a.Commit, a.Approver, a.Source, a.Before, a.After, a.Engines, a.PackVersions, a.SkippedFiles, a.Mode, a.Outcome, a.Reason, a.PrevHash, a.Hash)
	if err != nil {
		return err
	}
	_, _ = tx.Exec("update audit_counter set last_seq=$1 where id=1", a.Seq)
	return nil
}

func (s *Store) UpgradeAuditSeqZero() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var dummy any
	_ = tx.QueryRow("select pg_advisory_xact_lock(718293)").Scan(&dummy)
	_, _ = tx.Exec("update audit_counter set last_seq=last_seq where id=1")

	rows, err := tx.Query("select id, ts, actor, action, target, commit_sha, approver, source, before_val, after_val, engines, pack_versions, skipped_files, mode, outcome, reason from audit where seq=0 order by ts asc, id asc")
	if err != nil {
		return err
	}
	defer rows.Close()

	var toUpgrade []Audit
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.TS, &a.Actor, &a.Action, &a.Target, &a.Commit, &a.Approver, &a.Source, &a.Before, &a.After, &a.Engines, &a.PackVersions, &a.SkippedFiles, &a.Mode, &a.Outcome, &a.Reason); err != nil {
			return err
		}
		toUpgrade = append(toUpgrade, a)
	}
	rows.Close()

	if len(toUpgrade) == 0 {
		return nil
	}

	var maxSeq int64
	var prevHash string
	err = tx.QueryRow("select seq, hash from audit where seq > 0 order by seq desc limit 1").Scan(&maxSeq, &prevHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		maxSeq = 0
		prevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	}

	for _, a := range toUpgrade {
		maxSeq++
		a.Seq = maxSeq
		a.PrevHash = prevHash
		a.Hash = computeAuditHash(prevHash, a, s.auditKey)
		_, err = tx.Exec("update audit set seq=$1, prev_hash=$2, hash=$3 where id=$4", a.Seq, a.PrevHash, a.Hash, a.ID)
		if err != nil {
			return err
		}
		prevHash = a.Hash
	}

	_, _ = tx.Exec("update audit_counter set last_seq=$1 where id=1", maxSeq)
	return tx.Commit()
}

func (s *Store) AddAudit(a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := addAuditTx(tx, a, s.auditKey); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AuditLog(limit int) ([]Audit, error) {
	rows, err := s.db.Query("select id,seq,ts,actor,action,target,commit_sha,approver,source,before_val,after_val,engines,pack_versions,skipped_files,mode,outcome,reason,prev_hash,hash from audit order by seq desc limit $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Audit
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.Seq, &a.TS, &a.Actor, &a.Action, &a.Target, &a.Commit, &a.Approver, &a.Source, &a.Before, &a.After, &a.Engines, &a.PackVersions, &a.SkippedFiles, &a.Mode, &a.Outcome, &a.Reason, &a.PrevHash, &a.Hash); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// StartAuditVerifier re-verifies the chain every interval; AuditChainOK feeds the metric.
func (s *Store) StartAuditVerifier(interval time.Duration, logf func(string, ...any)) {
	run := func() {
		if err := s.VerifyAuditChain(); err != nil {
			s.auditBad.Store(true)
			logf("AUDIT CHAIN VERIFICATION FAILED: %v", err)
			return
		}
		s.auditBad.Store(false)
	}
	run()
	go func() {
		for range time.Tick(interval) {
			run()
		}
	}()
}

func (s *Store) AuditChainOK() bool { return !s.auditBad.Load() }

func (s *Store) VerifyAuditChain() error {
	rows, err := s.db.Query("select id,seq,ts,actor,action,target,commit_sha,approver,source,before_val,after_val,engines,pack_versions,skipped_files,mode,outcome,reason,prev_hash,hash from audit order by seq asc")
	if err != nil {
		return err
	}
	defer rows.Close()

	expectedPrev := "0000000000000000000000000000000000000000000000000000000000000000"
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.Seq, &a.TS, &a.Actor, &a.Action, &a.Target, &a.Commit, &a.Approver, &a.Source, &a.Before, &a.After, &a.Engines, &a.PackVersions, &a.SkippedFiles, &a.Mode, &a.Outcome, &a.Reason, &a.PrevHash, &a.Hash); err != nil {
			return err
		}
		if a.PrevHash != expectedPrev {
			return fmt.Errorf("audit chain verification failed at row %s: prev_hash %q != expected %q", a.ID, a.PrevHash, expectedPrev)
		}
		expectedHash := computeAuditHash(a.PrevHash, a, s.auditKey)
		if a.Hash != expectedHash {
			return fmt.Errorf("audit chain verification failed at row %s: hash %q != expected %q (tampered)", a.ID, a.Hash, expectedHash)
		}
		expectedPrev = a.Hash
	}
	return rows.Err()
}


// ApprovalTTLSeconds: a pending approval older than this can no longer be decided (state it was proposed against has moved on).
const ApprovalTTLSeconds = 7 * 24 * 3600

// Approvals: anything that loosens a rule, removes a test or accepts a risk waits here for a second person.
type Approval struct {
	ID, Kind, Proposer, Status, Approver string
	Payload                              json.RawMessage
	TS                                   int64
}

func (s *Store) Propose(kind string, payload any, proposer string) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	i := id()
	_, err = s.db.Exec("insert into approvals(id,kind,payload,proposer,status,approver,ts) values($1,$2,$3,$4,'pending','',$5)", i, kind, string(b), proposer, now())
	if err == nil {
		err = s.AddAudit(Audit{Actor: proposer, Action: "propose:" + kind, Target: i})
	}
	return i, err
}

var confusableMap = map[rune]rune{
	// Cyrillic
	'\u0430': 'a', '\u0410': 'a', // а, А
	'\u0432': 'b', '\u0412': 'b', // в, В
	'\u0441': 'c', '\u0421': 'c', // с, С
	'\u0434': 'd', '\u0414': 'd', // д, Д
	'\u0435': 'e', '\u0415': 'e', // е, Е
	'\u0451': 'e', '\u0401': 'e', // ё, Ё
	'\u0433': 'r', '\u0413': 'r', // г, Г
	'\u04bb': 'h', '\u04ba': 'h', // һ, Һ
	'\u0456': 'i', '\u0406': 'i', // і, І
	'\u0458': 'j', '\u0408': 'j', // ј, Ј
	'\u043a': 'k', '\u041a': 'k', // к, К
	'\u043c': 'm', '\u041c': 'm', // м, М
	'\u043d': 'h', '\u041d': 'h', // н, Н
	'\u043e': 'o', '\u041e': 'o', // о, О
	'\u0440': 'p', '\u0420': 'p', // р, Р
	'\u0455': 's', '\u0405': 's', // ѕ, Ѕ
	'\u0442': 't', '\u0422': 't', // т, Т
	'\u0443': 'y', '\u0423': 'y', // у, У
	'\u0445': 'x', '\u0425': 'x', // х, Х
	'\u0501': 'd', '\u0500': 'd', // ԁ, Ԁ
	'\u051b': 'q', '\u051a': 'q', // ԛ, Ԛ
	'\u051d': 'w', '\u051c': 'w', // ԝ, Ԝ

	// Greek
	'\u03b1': 'a', '\u0391': 'a', // α, Α
	'\u03b2': 'b', '\u0392': 'b', // β, Β
	'\u03b3': 'y', '\u0393': 'y', // γ, Γ
	'\u03b4': 'd', '\u0394': 'd', // δ, Δ
	'\u03b5': 'e', '\u0395': 'e', // ε, Ε
	'\u03b6': 'z', '\u0396': 'z', // ζ, Ζ
	'\u03b7': 'n', '\u0397': 'h', // η, Η
	'\u03b9': 'i', '\u0399': 'i', // ι, Ι
	'\u03ba': 'k', '\u039a': 'k', // κ, Κ
	'\u03bd': 'v', '\u039d': 'n', // ν, Ν
	'\u03bf': 'o', '\u039f': 'o', // ο, Ο
	'\u03c1': 'p', '\u03a1': 'p', // ρ, Ρ
	'\u03c2': 's', '\u03c3': 's', '\u03a3': 's', // ς, σ, Σ
	'\u03c4': 't', '\u03a4': 't', // τ, Τ
	'\u03c5': 'u', '\u03a5': 'y', // υ, Υ
	'\u03c7': 'x', '\u03a7': 'x', // χ, Χ
	'\u03c9': 'w', '\u03a9': 'w', // ω, Ω
}

func foldHomoglyphs(s string) string {
	return strings.Map(func(r rune) rune {
		if c, ok := confusableMap[r]; ok {
			return c
		}
		return r
	}, s)
}

func canonicalEmail(e string) string {
	// NFKC folds fullwidth/compat forms; drop invisible format runes (zero-width joiners, BOM, bidi marks).
	e = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, norm.NFKC.String(e))
	e = strings.TrimSpace(strings.ToLower(e))
	e = foldHomoglyphs(e)
	user, domain, ok := strings.Cut(e, "@")
	if !ok {
		return e
	}
	if idx := strings.Index(user, "+"); idx != -1 {
		user = user[:idx]
	}
	user = strings.ReplaceAll(user, ".", "")
	return user + "@" + domain
}

func isSameActorOrAlias(a, b string) bool {
	if a == b {
		return true
	}
	ca, cb := canonicalEmail(a), canonicalEmail(b)
	return ca != "" && ca == cb
}

// Decide enforces separation of duties: the proposer cannot decide their own proposal.
// apply runs in the same transaction as Decide.
func (s *Store) Decide(approvalID, approver string, approve bool) (Approval, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Approval{}, err
	}
	defer tx.Rollback()

	var a Approval
	var p string
	err = tx.QueryRow("select id,kind,payload,proposer,status,approver,ts from approvals where id=$1", approvalID).Scan(&a.ID, &a.Kind, &p, &a.Proposer, &a.Status, &a.Approver, &a.TS)
	if err != nil {
		return a, err
	}
	a.Payload = json.RawMessage(p)

	if a.Status != "pending" {
		return a, fmt.Errorf("already %s", a.Status)
	}
	if a.Proposer == approver || isSameActorOrAlias(a.Proposer, approver) {
		return a, fmt.Errorf("proposer cannot approve their own change")
	}
	if now()-a.TS > ApprovalTTLSeconds {
		return a, fmt.Errorf("approval expired; propose again")
	}
	if a.Kind == "role_change" || a.Kind == "role" {
		var rolePayload struct{ Email, Role string }
		if json.Unmarshal(a.Payload, &rolePayload) == nil {
			var curRole string
			tx.QueryRow("select role from users where email=$1", rolePayload.Email).Scan(&curRole)
			if approve && rolePayload.Role != "owner" {
				var owners int
				tx.QueryRow("select count(*) from users where role='owner'").Scan(&owners)
				if curRole == "owner" && owners <= 1 {
					return a, fmt.Errorf("cannot remove the last owner")
				}
			}
			if approver == rolePayload.Email || isSameActorOrAlias(approver, rolePayload.Email) {
				return a, fmt.Errorf("user cannot approve their own role change")
			}
			var approverRole string
			tx.QueryRow("select role from users where email=$1", approver).Scan(&approverRole)
			if (curRole == "owner" || rolePayload.Role == "owner") && approverRole != "owner" {
				return a, fmt.Errorf("only an owner can approve role changes involving the owner role")
			}
		}
	}
	if a.Kind == "ignore" || a.Kind == "sentinix:ignore" {
		var ignPayload struct {
			Owner  string `json:"owner"`
			Author string `json:"author"`
			Until  int64  `json:"until"`
		}
		if json.Unmarshal(a.Payload, &ignPayload) == nil {
			author := ignPayload.Owner
			if author == "" {
				author = ignPayload.Author
			}
			if author != "" && (approver == author || isSameActorOrAlias(approver, author)) {
				return a, fmt.Errorf("author cannot approve their own suppression")
			}
			if ignPayload.Until > 0 && ignPayload.Until <= time.Now().Unix() {
				return a, fmt.Errorf("suppression has expired")
			}
		}
	}

	newStatus := "rejected"
	if approve {
		newStatus = "approved"
	}

	res, err := tx.Exec("update approvals set status=$2,approver=$3 where id=$1 and status='pending'", approvalID, newStatus, approver)
	if err != nil {
		return a, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return a, fmt.Errorf("already %s", a.Status)
	}

	beforeStatus := a.Status
	a.Status = newStatus
	a.Approver = approver

	if err := addAuditTx(tx, Audit{
		Actor:    approver,
		Action:   a.Status + ":" + a.Kind,
		Target:   approvalID,
		Approver: approver,
		Before:   beforeStatus,
		After:    newStatus,
	}, s.auditKey); err != nil {
		return a, err
	}

	if approve {
		if err := applyTx(tx, a); err != nil {
			return a, err
		}
	}

	if err := tx.Commit(); err != nil {
		return a, err
	}
	return a, nil
}

func applyTx(tx *sql.Tx, a Approval) error {
	var raw map[string]any
	if err := json.Unmarshal(a.Payload, &raw); err != nil {
		return err
	}
	getString := func(k string) string {
		v, _ := raw[k].(string)
		return v
	}
	getInt64 := func(k string) int64 {
		switch v := raw[k].(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		case json.Number:
			n, _ := v.Int64()
			return n
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.Unix()
			} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
		return 0
	}
	getBool := func(k string) bool {
		b, _ := raw[k].(bool)
		return b
	}

	currentTime := now()
	switch a.Kind {
	case "role_change", "role":
		_, err := tx.Exec("update users set role=$1 where email=$2", getString("role"), getString("email"))
		return err
	case "branch":
		_, err := tx.Exec("update projects set branch=$1 where id=$2", getString("branch"), getString("project"))
		return err
	case "schedule":
		_, err := tx.Exec("update projects set schedule=$1 where id=$2", getString("schedule"), getString("project"))
		return err
	case "pack_pins":
		pins, _ := json.Marshal(raw["pack_pins"])
		_, err := tx.Exec("update projects set pack_pins=$1 where id=$2", string(pins), getString("project"))
		return err
	case "false_positive":
		findingID := getString("finding")
		if findingID != "" {
			_, err := tx.Exec("update findings set status='false_positive', owner=$2, accepted_reason=$3, updated=$4 where id=$1",
				findingID, getString("owner"), getString("reason"), currentTime)
			return err
		}
		return nil
	case "accept_risk":
		_, err := tx.Exec("update findings set status='accepted', owner=$2, accepted_reason=$3, accepted_until=$4, updated=$5 where id=$1",
			getString("finding"), getString("owner"), getString("reason"), getInt64("until"), currentTime)
		return err
	case "disable_rule":
		sc := getString("scope")
		if sc == "" {
			sc = "*"
		}
		_, err := tx.Exec("insert into rule_overrides(rule,disabled,severity,scope,expires_at) values($1,1,-1,$2,$3) on conflict(rule, scope) do update set disabled=1, expires_at=excluded.expires_at",
			getString("rule"), sc, getInt64("until"))
		return err
	case "loosen_severity":
		s, _ := engines.ParseSeverity(getString("severity"))
		_, err := tx.Exec("insert into rule_overrides(rule,disabled,severity,scope,expires_at) values($1,0,$2,'*',0) on conflict(rule, scope) do update set disabled=0, severity=$2",
			getString("rule"), int(s))
		return err
	case "demote_rule", "rule_demotion":
		_, err := tx.Exec("insert into rule_overrides(rule,disabled,severity,scope,expires_at) values($1,0,$2,'*',0) on conflict(rule, scope) do update set disabled=0, severity=$2",
			getString("rule"), int(engines.Info))
		return err
	case "category_policy":
		projID := getString("project")
		cat := getString("category")
		mode := getString("mode")
		thresh := getString("threshold")
		_, err := tx.Exec("delete from project_policies where project_id=$1 and category=$2", projID, cat)
		if err != nil {
			return err
		}
		_, err = tx.Exec("insert into project_policies(project_id, category, mode, threshold) values($1, $2, $3, $4)",
			projID, cat, mode, thresh)
		if err != nil {
			return err
		}
		if cat == engines.CategorySecurity {
			enf := "block"
			if mode == "comment" || mode == "off" {
				enf = "report"
			}
			_, err = tx.Exec("update projects set enforcement=$1, fail_on=$2 where id=$3", enf, thresh, projID)
			return err
		}
		return nil
	case "enforcement":
		enf := getString("enforcement")
		projID := getString("project")
		_, err := tx.Exec("update projects set enforcement=$1 where id=$2", enf, projID)
		if err != nil {
			return err
		}
		mode := "block"
		if enf == "report" {
			mode = "comment"
		}
		_, _ = tx.Exec("delete from project_policies where project_id=$1 and category='security'", projID)
		_, _ = tx.Exec("insert into project_policies(project_id, category, mode, threshold) values($1, 'security', $2, 'high')", projID, mode)
		return nil
	case "fail_on":
		fo := getString("fail_on")
		projID := getString("project")
		_, err := tx.Exec("update projects set fail_on=$1 where id=$2", fo, projID)
		if err != nil {
			return err
		}
		_, _ = tx.Exec("delete from project_policies where project_id=$1 and category='security'", projID)
		_, _ = tx.Exec("insert into project_policies(project_id, category, mode, threshold) values($1, 'security', 'block', $2)", projID, fo)
		return nil
	case "model_policy":
		_, err := tx.Exec("update projects set model_policy=$1 where id=$2", getString("model_policy"), getString("project"))
		return err
	case "ai":
		aiVal := 0
		if getBool("ai") {
			aiVal = 1
		}
		_, err := tx.Exec("update projects set ai=$1 where id=$2", aiVal, getString("project"))
		return err
	}
	return nil
}

func (s *Store) Approval(id string) (Approval, error) {
	var a Approval
	var p string
	err := s.db.QueryRow("select id,kind,payload,proposer,status,approver,ts from approvals where id=$1", id).Scan(&a.ID, &a.Kind, &p, &a.Proposer, &a.Status, &a.Approver, &a.TS)
	a.Payload = json.RawMessage(p)
	return a, err
}

func (s *Store) Approvals(status string) ([]Approval, error) {
	rows, err := s.db.Query("select id,kind,payload,proposer,status,approver,ts from approvals where status=$1 order by ts desc, id", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Approval
	for rows.Next() {
		var a Approval
		var p string
		if err := rows.Scan(&a.ID, &a.Kind, &p, &a.Proposer, &a.Status, &a.Approver, &a.TS); err != nil {
			return nil, err
		}
		a.Payload = json.RawMessage(p)
		out = append(out, a)
	}
	return out, rows.Err()
}

type User struct {
	ID, Email, Name, Role string
	RevokedBefore         int64 // sessions issued before this (unix ms) are dead
}

// RevokeSessions kills every session the user holds now. They can sign in again unless the IdP stops them.
func (s *Store) RevokeSessions(email string) error {
	r, err := s.db.Exec("update users set revoked_before=$2 where email=$1", email, time.Now().UnixMilli()+1)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpsertUser creates the user with role if new; an existing user keeps their role.
func (s *Store) UpsertUser(email, name, role string) (User, error) {
	if _, err := s.db.Exec("insert into users(id,email,name,role,created) values($1,$2,$3,$4,$5) on conflict(email) do update set name=excluded.name", id(), email, name, role, now()); err != nil {
		return User{}, err
	}
	var u User
	err := s.db.QueryRow("select id,email,name,role from users where email=$1", email).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	return u, err
}

func (s *Store) SetRole(email, role string) error {
	_, err := s.db.Exec("update users set role=$2 where email=$1", email, role)
	return err
}

func (s *Store) Users() ([]User, error) {
	rows, err := s.db.Query("select id,email,name,role,revoked_before from users order by email")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.RevokedBefore); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UserByEmail(email string) (User, error) {
	var u User
	err := s.db.QueryRow("select id,email,name,role,revoked_before from users where email=$1", email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.RevokedBefore)
	return u, err
}

