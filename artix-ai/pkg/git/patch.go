package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PatchSession tracks an applied patch with rollback capability.
type PatchSession struct {
	repoDir       string
	patchContent  string
	touchedFiles  []string
	backupContent map[string][]byte
	isApplied     bool
}

// NewPatchSession initializes a patch session.
func NewPatchSession(repoDir, patchContent string) *PatchSession {
	return &PatchSession{
		repoDir:       repoDir,
		patchContent:  patchContent,
		touchedFiles:  make([]string, 0),
		backupContent: make(map[string][]byte),
	}
}

// Check verifies whether the unified patch can be applied cleanly.
func (p *PatchSession) Check() error {
	p.extractTouchedFiles()
	for _, f := range p.touchedFiles {
		clean := filepath.Clean(f)
		if strings.HasPrefix(clean, ".git") || strings.Contains(clean, ".git/") || strings.Contains(clean, "hooks/") {
			return fmt.Errorf("patching .git repository metadata or hooks is strictly forbidden: %s", f)
		}
	}

	cmd := exec.Command("git", "-C", p.repoDir, "apply", "--check", "-")
	cmd.Stdin = strings.NewReader(p.patchContent)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("patch pre-check failed: %w (details: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Apply verifies, backs up affected files, and applies the unified patch.
func (p *PatchSession) Apply() error {
	p.extractTouchedFiles()

	// 1. Pre-check
	if err := p.Check(); err != nil {
		return err
	}

	// 2. Backup existing files
	for _, relFile := range p.touchedFiles {
		absPath := filepath.Join(p.repoDir, relFile)
		if content, err := os.ReadFile(absPath); err == nil {
			p.backupContent[relFile] = content
		}
	}

	// 3. Apply patch
	cmd := exec.Command("git", "-C", p.repoDir, "apply", "-")
	cmd.Stdin = strings.NewReader(p.patchContent)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		p.Rollback()
		return fmt.Errorf("git apply failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	p.isApplied = true
	return nil
}

// Rollback restores touched files to their pre-patch state.
func (p *PatchSession) Rollback() error {
	var errs []string

	// 1. Try git apply --reverse first
	if p.isApplied {
		cmd := exec.Command("git", "-C", p.repoDir, "apply", "--reverse", "-")
		cmd.Stdin = strings.NewReader(p.patchContent)
		if err := cmd.Run(); err == nil {
			p.isApplied = false
			return nil
		}
	}

	// 2. Fallback: restore backed up file contents directly
	for relFile, content := range p.backupContent {
		absPath := filepath.Join(p.repoDir, relFile)
		if err := os.WriteFile(absPath, content, 0644); err != nil {
			errs = append(errs, fmt.Sprintf("failed to restore %s: %v", relFile, err))
		}
	}

	// Remove any newly created files that didn't exist in backup
	for _, relFile := range p.touchedFiles {
		if _, existed := p.backupContent[relFile]; !existed {
			absPath := filepath.Join(p.repoDir, relFile)
			_ = os.Remove(absPath)
		}
	}

	p.isApplied = false
	if len(errs) > 0 {
		return fmt.Errorf("rollback errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// TouchedFiles returns the list of file paths affected by this patch.
func (p *PatchSession) TouchedFiles() []string {
	return p.touchedFiles
}

func (p *PatchSession) extractTouchedFiles() {
	lines := strings.Split(p.patchContent, "\n")
	seen := make(map[string]bool)

	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			file := strings.TrimPrefix(line, "+++ b/")
			file = strings.TrimSpace(file)
			if file != "" && !seen[file] {
				seen[file] = true
				p.touchedFiles = append(p.touchedFiles, file)
			}
		}
	}
}
