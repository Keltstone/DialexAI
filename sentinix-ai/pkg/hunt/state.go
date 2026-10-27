package hunt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
)

// State keeps each task's result between runs, like the framework's shared store: a task is keyed by the
// code it saw, so re-running skips finished tasks (no model call, no budget) and edited code is re-audited.
type State struct {
	path string
	mu   sync.Mutex
	M    map[string]Result `json:"tasks"`
}

type Result struct {
	Vulnerable bool   `json:"vulnerable"`
	Severity   string `json:"severity,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
}

func LoadState(path string) (*State, error) {
	s := &State{path: path, M: map[string]Result{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, s)
}

func (s *State) get(k string) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.M[k]
	return r, ok
}

func (s *State) put(k string, r Result) {
	s.mu.Lock()
	s.M[k] = r
	s.mu.Unlock()
}

func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func taskKey(code, file, class, pass string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:6]) + "|" + file + "|" + class + "|" + pass
}
