package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"artix/pkg/policy"
)

// Store manages persistence of Knowledge Items with schema versioning and enterprise isolation.
type Store struct {
	projectDir     string
	globalDir      string
	enterpriseMode bool
	mu             sync.RWMutex
}

// NewStore creates a knowledge store for a repository.
func NewStore(projectDir string) *Store {
	isEnterprise := policy.IsEnterprise()

	globalDir := ""
	if !isEnterprise {
		home, _ := os.UserHomeDir()
		if home != "" {
			globalDir = filepath.Join(home, ".artix", "knowledge")
		}
	}

	pDir := ""
	if projectDir != "" {
		pDir = filepath.Join(projectDir, ".artix", "knowledge")
		_ = os.MkdirAll(pDir, 0755)
	}

	return &Store{
		projectDir:     pDir,
		globalDir:      globalDir,
		enterpriseMode: isEnterprise,
	}
}

// SetEnterpriseMode enables or disables enterprise isolation. When enabled, global directories are disabled.
func (s *Store) SetEnterpriseMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enterpriseMode = enabled
	if enabled {
		s.globalDir = ""
	} else {
		home, _ := os.UserHomeDir()
		if home != "" {
			s.globalDir = filepath.Join(home, ".artix", "knowledge")
		}
	}
}

// IsEnterpriseMode returns true if enterprise isolation is active.
func (s *Store) IsEnterpriseMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enterpriseMode
}

// computeHash calculates a canonical SHA-256 fingerprint of the item's semantic content
// via JSON marshaling to eliminate delimiter injection collisions.
func computeHash(ki *KnowledgeItem) string {
	type canonicalContent struct {
		Category     KnowledgeCategory `json:"category"`
		Title        string            `json:"title"`
		Context      string            `json:"context"`
		Breakthrough string            `json:"breakthrough"`
	}
	payload, _ := json.Marshal(canonicalContent{
		Category:     ki.Category,
		Title:        ki.Title,
		Context:      ki.Context,
		Breakthrough: ki.Breakthrough,
	})
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum)
}

var fileLockMu sync.Mutex

func withFileLock(path string, fn func() error) error {
	fileLockMu.Lock()
	defer fileLockMu.Unlock()

	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire flock on %s: %w", lockPath, err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}

func atomicWriteFile(targetFile string, data []byte) error {
	tmpFile := fmt.Sprintf("%s.tmp.%d", targetFile, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, targetFile); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	return nil
}

// Save persists a Knowledge Item to JSON disk conditionally (only on change)
// using atomic write and file locking.
func (s *Store) Save(ki *KnowledgeItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ki.SchemaVersion <= 0 {
		ki.SchemaVersion = CurrentSchemaVersion
	}
	if ki.ID == "" {
		ki.ID = fmt.Sprintf("ki-%d", time.Now().UnixNano())
	}
	if ki.CreatedAt.IsZero() {
		ki.CreatedAt = time.Now()
	}
	if ki.ContentHash == "" {
		ki.ContentHash = computeHash(ki)
	}

	dir := s.projectDir
	if dir == "" {
		if s.enterpriseMode {
			return fmt.Errorf("cannot save knowledge item: enterprise mode requires a valid project directory")
		}
		dir = s.globalDir
	}
	if dir == "" {
		return fmt.Errorf("no storage directory available to save knowledge item")
	}
	_ = os.MkdirAll(dir, 0755)

	targetFile := filepath.Join(dir, fmt.Sprintf("%s.json", ki.ID))

	return withFileLock(targetFile, func() error {
		// Read existing file if present to check if SessionCount or contents should be preserved
		if existingData, err := os.ReadFile(targetFile); err == nil {
			var existingKI KnowledgeItem
			if err := json.Unmarshal(existingData, &existingKI); err == nil {
				if existingKI.SessionCount > ki.SessionCount {
					ki.SessionCount = existingKI.SessionCount
				}
			}
		}

		data, err := json.MarshalIndent(ki, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal knowledge item: %w", err)
		}

		// Conditional save: if content matches disk exactly, skip write
		if existingData, err := os.ReadFile(targetFile); err == nil {
			if bytes.Equal(existingData, data) {
				return nil
			}
		}

		return atomicWriteFile(targetFile, data)
	})
}

// CompareAndSwapSessionCount atomically updates session count if it matches expected.
func (s *Store) CompareAndSwapSessionCount(id string, expected, newCount int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dirs := []string{s.projectDir}
	if !s.enterpriseMode && s.globalDir != "" {
		dirs = append(dirs, s.globalDir)
	}

	var targetFile string
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, fmt.Sprintf("%s.json", id))
		if _, err := os.Stat(candidate); err == nil {
			targetFile = candidate
			break
		}
	}
	if targetFile == "" {
		if s.projectDir != "" {
			targetFile = filepath.Join(s.projectDir, fmt.Sprintf("%s.json", id))
		} else if s.globalDir != "" {
			targetFile = filepath.Join(s.globalDir, fmt.Sprintf("%s.json", id))
		} else {
			return false, fmt.Errorf("no storage directory available to locate %s", id)
		}
	}

	var swapped bool
	err := withFileLock(targetFile, func() error {
		data, err := os.ReadFile(targetFile)
		if err != nil {
			return err
		}
		var ki KnowledgeItem
		if err := json.Unmarshal(data, &ki); err != nil {
			return err
		}
		if ki.SessionCount != expected {
			swapped = false
			return nil
		}
		ki.SessionCount = newCount
		marshaled, err := json.MarshalIndent(&ki, "", "  ")
		if err != nil {
			return err
		}
		if err := atomicWriteFile(targetFile, marshaled); err != nil {
			return err
		}
		swapped = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return swapped, nil
}

// IncrementSessionCount increments session count using compare-and-swap.
func (s *Store) IncrementSessionCount(id string) (int, error) {
	for retries := 0; retries < 100; retries++ {
		ki, err := s.Get(id)
		if err != nil {
			return 0, err
		}
		current := ki.SessionCount
		next := current + 1
		ok, err := s.CompareAndSwapSessionCount(id, current, next)
		if err != nil {
			return 0, err
		}
		if ok {
			return next, nil
		}
		time.Sleep(time.Duration(1+retries) * time.Millisecond)
	}
	return 0, fmt.Errorf("failed to increment session count for %s after retries", id)
}

// Get retrieves a Knowledge Item by ID and applies schema migrations if needed.
func (s *Store) Get(id string) (*KnowledgeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dirs := []string{s.projectDir}
	if !s.enterpriseMode && s.globalDir != "" {
		dirs = append(dirs, s.globalDir)
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, fmt.Sprintf("%s.json", id))
		if data, err := os.ReadFile(p); err == nil {
			var ki KnowledgeItem
			if err := json.Unmarshal(data, &ki); err == nil {
				// Migrate legacy items lacking schema version
				if ki.SchemaVersion <= 0 {
					ki.SchemaVersion = 1
				}
				if ki.ContentHash == "" {
					ki.ContentHash = computeHash(&ki)
				}
				return &ki, nil
			}
		}
	}

	return nil, fmt.Errorf("knowledge item %s not found", id)
}

// List returns all stored Knowledge Items, deduplicated by ID and content hash.
func (s *Store) List() ([]KnowledgeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	itemMap := make(map[string]KnowledgeItem)
	seenHashes := make(map[string]string) // ContentHash -> ID

	dirs := []string{s.projectDir}
	if !s.enterpriseMode && s.globalDir != "" {
		// In non-enterprise mode, include global items
		dirs = append([]string{s.globalDir}, s.projectDir)
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				p := filepath.Join(dir, e.Name())
				if data, err := os.ReadFile(p); err == nil {
					var ki KnowledgeItem
					if err := json.Unmarshal(data, &ki); err == nil {
						if ki.SchemaVersion <= 0 {
							ki.SchemaVersion = 1
						}
						if ki.ContentHash == "" {
							ki.ContentHash = computeHash(&ki)
						}

						// If duplicate content exists, prefer project-scoped version
						if prevID, exists := seenHashes[ki.ContentHash]; exists && prevID != ki.ID {
							// Replace only if current item is from projectDir
							if dir == s.projectDir {
								delete(itemMap, prevID)
								itemMap[ki.ID] = ki
								seenHashes[ki.ContentHash] = ki.ID
							}
							continue
						}

						itemMap[ki.ID] = ki
						seenHashes[ki.ContentHash] = ki.ID
					}
				}
			}
		}
	}

	items := make([]KnowledgeItem, 0, len(itemMap))
	for _, ki := range itemMap {
		items = append(items, ki)
	}
	return items, nil
}

// ListActive returns only non-expired knowledge items honoring TTL expiration.
func (s *Store) ListActive() ([]KnowledgeItem, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var active []KnowledgeItem
	for _, ki := range all {
		if ki.IsActive() {
			active = append(active, ki)
		}
	}
	return active, nil
}

// Delete removes a Knowledge Item from the project store.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.projectDir != "" {
		p := filepath.Join(s.projectDir, fmt.Sprintf("%s.json", id))
		return os.Remove(p)
	}
	return nil
}

// Prune removes knowledge items older than maxAge from the project store (and global store when not in enterprise mode).
func (s *Store) Prune(maxAge time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dirs := []string{s.projectDir}
	if !s.enterpriseMode && s.globalDir != "" {
		dirs = append(dirs, s.globalDir)
	}

	pruned := 0
	now := time.Now()
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				p := filepath.Join(dir, e.Name())
				data, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				var ki KnowledgeItem
				if err := json.Unmarshal(data, &ki); err == nil {
					if !ki.CreatedAt.IsZero() && now.Sub(ki.CreatedAt) > maxAge {
						if err := os.Remove(p); err == nil {
							pruned++
						}
					}
				}
			}
		}
	}
	return pruned, nil
}
