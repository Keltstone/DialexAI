package store

import (
	"encoding/json"
	"fmt"
)

type Audit struct {
	ID, Actor, Action, Target, Commit, Approver string
	TS                                          int64
}

func (s *Store) AddAudit(a Audit) error {
	_, err := s.db.Exec("insert into audit(id,ts,actor,action,target,commit_sha,approver) values($1,$2,$3,$4,$5,$6,$7)",
		id(), now(), a.Actor, a.Action, a.Target, a.Commit, a.Approver)
	return err
}

func (s *Store) AuditLog(limit int) ([]Audit, error) {
	rows, err := s.db.Query("select id,ts,actor,action,target,commit_sha,approver from audit order by ts desc, id limit $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Audit
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.TS, &a.Actor, &a.Action, &a.Target, &a.Commit, &a.Approver); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

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

// Decide enforces separation of duties: the proposer cannot decide their own proposal.
func (s *Store) Decide(approvalID, approver string, approve bool) (Approval, error) {
	a, err := s.Approval(approvalID)
	if err != nil {
		return a, err
	}
	if a.Status != "pending" {
		return a, fmt.Errorf("already %s", a.Status)
	}
	if a.Proposer == approver {
		return a, fmt.Errorf("proposer cannot approve their own change")
	}
	a.Status = "rejected"
	if approve {
		a.Status = "approved"
	}
	a.Approver = approver
	if _, err := s.db.Exec("update approvals set status=$2,approver=$3 where id=$1", approvalID, a.Status, approver); err != nil {
		return a, err
	}
	return a, s.AddAudit(Audit{Actor: approver, Action: a.Status + ":" + a.Kind, Target: approvalID, Approver: approver})
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
	rows, err := s.db.Query("select id,email,name,role from users order by email")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
