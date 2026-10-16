package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
)

type Project struct {
	ID, Name, Host, Repo, Branch string
	Schedule                     string            // "", "nightly", "weekly" or "m h dom mon dow"
	TZ                           string            // IANA name; "" = UTC
	PackPins                     map[string]string // pack name -> pinned version
	AI                           bool
	ModelPolicy                  string // local-only | cloud-redacted | cloud
	Enforcement                  string // block: a failing status stops the merge; report: comment only, never blocks
	FailOn                       string // severity threshold for this project; "" uses the server default
	Created                      int64
}

const projCols = "id,name,host,repo,branch,schedule,tz,pack_pins,ai,model_policy,created,enforcement,fail_on"

func scanProject(r interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var pins string
	var ai int
	err := r.Scan(&p.ID, &p.Name, &p.Host, &p.Repo, &p.Branch, &p.Schedule, &p.TZ, &pins, &ai, &p.ModelPolicy, &p.Created, &p.Enforcement, &p.FailOn)
	p.AI = ai == 1
	json.Unmarshal([]byte(pins), &p.PackPins)
	return p, err
}

func (s *Store) CreateProject(p Project) (Project, error) {
	p.ID, p.Created = id(), now()
	if p.Branch == "" {
		p.Branch = "main"
	}
	if p.ModelPolicy == "" {
		p.ModelPolicy = "local-only"
	}
	if p.Enforcement == "" {
		p.Enforcement = "block"
	}
	pins, _ := json.Marshal(p.PackPins)
	ai := 0
	if p.AI {
		ai = 1
	}
	_, err := s.db.Exec("insert into projects("+projCols+") values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)",
		p.ID, p.Name, p.Host, p.Repo, p.Branch, p.Schedule, p.TZ, string(pins), ai, p.ModelPolicy, p.Created, p.Enforcement, p.FailOn)
	return p, err
}

func (s *Store) Project(id string) (Project, error) {
	return scanProject(s.db.QueryRow("select "+projCols+" from projects where id=$1", id))
}

func (s *Store) ProjectByRepo(host, repo string) (Project, error) {
	return scanProject(s.db.QueryRow("select "+projCols+" from projects where host=$1 and repo=$2", host, repo))
}

func (s *Store) Projects() ([]Project, error) {
	rows, err := s.db.Query("select " + projCols + " from projects order by name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProject(p Project) error {
	pins, _ := json.Marshal(p.PackPins)
	ai := 0
	if p.AI {
		ai = 1
	}
	_, err := s.db.Exec("update projects set name=$2,branch=$3,schedule=$4,tz=$5,pack_pins=$6,ai=$7,model_policy=$8,enforcement=$9,fail_on=$10 where id=$1",
		p.ID, p.Name, p.Branch, p.Schedule, p.TZ, string(pins), ai, p.ModelPolicy, p.Enforcement, p.FailOn)
	return err
}

// API tokens: scoped to one project, stored hashed, shown once.
func (s *Store) CreateToken(project, name string, expiresAt ...int64) (string, error) {
	raw := "sx_" + id() + id()
	h := sha256.Sum256([]byte(raw))
	var exp int64
	if len(expiresAt) > 0 {
		exp = expiresAt[0]
	}
	_, err := s.db.Exec("insert into tokens(hash,project_id,name,created,expires_at,revoked) values($1,$2,$3,$4,$5,0)",
		hex.EncodeToString(h[:]), project, name, now(), exp)
	return raw, err
}

func (s *Store) RevokeToken(project, rawOrHash string) error {
	h := sha256.Sum256([]byte(rawOrHash))
	hashStr := hex.EncodeToString(h[:])
	_, err := s.db.Exec("update tokens set revoked=1 where (hash=$1 or hash=$2 or name=$2) and (project_id=$3 or $3='')", hashStr, rawOrHash, project)
	return err
}

func (s *Store) DeleteToken(project, rawOrHash string) error {
	h := sha256.Sum256([]byte(rawOrHash))
	hashStr := hex.EncodeToString(h[:])
	_, err := s.db.Exec("delete from tokens where (hash=$1 or hash=$2 or name=$2) and (project_id=$3 or $3='')", hashStr, rawOrHash, project)
	return err
}

func (s *Store) TokenProject(raw string) (string, bool) {
	h := sha256.Sum256([]byte(raw))
	var p string
	var exp, revoked int64
	err := s.db.QueryRow("select project_id, expires_at, revoked from tokens where hash=$1", hex.EncodeToString(h[:])).Scan(&p, &exp, &revoked)
	if err != nil {
		return "", false
	}
	if revoked == 1 {
		return "", false
	}
	if exp > 0 && now() > exp {
		return "", false
	}
	return p, true
}

var ErrNotFound = sql.ErrNoRows
