package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"scribex/pkg/gitx"
)

func TestWritesE2E_SymlinkTargetRefused(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, repo, outside string) (args []string, stdin string, targetFile string)
		wantErr string
	}{
		{
			name: "ci_init_github_symlinked_dir",
			setup: func(t *testing.T, repo, outside string) ([]string, string, string) {
				outsideTarget := filepath.Join(outside, "target.yml")
				_ = os.WriteFile(outsideTarget, []byte("orig-ci"), 0o644)
				_ = os.Symlink(outside, filepath.Join(repo, ".github"))
				return []string{"ci", "init", "github"}, "", outsideTarget
			},
			wantErr: "symlink",
		},
		{
			name: "mcp_add_symlinked_mcp_json",
			setup: func(t *testing.T, repo, outside string) ([]string, string, string) {
				outsideFile := filepath.Join(outside, "mcp.json")
				_ = os.WriteFile(outsideFile, []byte("{\"mcpServers\":{}}"), 0o644)
				_ = os.MkdirAll(filepath.Join(repo, ".scribex"), 0o755)
				_ = os.Symlink(outsideFile, filepath.Join(repo, ".scribex", "mcp.json"))
				return []string{"mcp", "add", "x", "-preset", "scribex"}, "", outsideFile
			},
			wantErr: "outside root",
		},
		{
			name: "wiki_new_symlinked_page",
			setup: func(t *testing.T, repo, outside string) ([]string, string, string) {
				outsideFile := filepath.Join(outside, "Evil.md")
				_ = os.WriteFile(outsideFile, []byte("orig-wiki"), 0o644)
				wikiDir := filepath.Join(repo, "docs", "wiki")
				_ = os.MkdirAll(wikiDir, 0o755)
				_ = os.Symlink(outsideFile, filepath.Join(wikiDir, "Evil.md"))
				return []string{"wiki", "new", "Evil"}, "", outsideFile
			},
			wantErr: "exists",
		},
		{
			name: "wiki_render_symlinked_child",
			setup: func(t *testing.T, repo, outside string) ([]string, string, string) {
				wikiDir := filepath.Join(repo, "docs", "wiki")
				_ = os.MkdirAll(wikiDir, 0o755)
				_ = os.WriteFile(filepath.Join(wikiDir, "Home.md"), []byte("# Home\n"), 0o644)

				outDir := filepath.Join(repo, "rendered")
				_ = os.MkdirAll(outDir, 0o755)
				outsideFile := filepath.Join(outside, "Home.md")
				_ = os.WriteFile(outsideFile, []byte("orig-home"), 0o644)
				_ = os.Symlink(outsideFile, filepath.Join(outDir, "Home.md"))

				return []string{"wiki", "render", "-out", outDir}, "", outsideFile
			},
			wantErr: "outside root",
		},
		{
			name: "secrets_set_symlinked_gitignore",
			setup: func(t *testing.T, repo, outside string) ([]string, string, string) {
				outsideFile := filepath.Join(outside, "gitignore")
				_ = os.WriteFile(outsideFile, []byte("orig-gi"), 0o644)
				_ = os.Symlink(outsideFile, filepath.Join(repo, ".gitignore"))
				return []string{"secrets", "set", "MY_SECRET"}, "supersecret\n", outsideFile
			},
			wantErr: "symlink",
		},
		{
			name: "trace_out_symlink",
			setup: func(t *testing.T, repo, outside string) ([]string, string, string) {
				_ = os.WriteFile(filepath.Join(repo, "code.go"), []byte("package main\n"), 0o644)
				_, _ = gitx.Run(repo, "config", "user.name", "Tester")
				_, _ = gitx.Run(repo, "config", "user.email", "test@example.com")
				_, _ = gitx.Run(repo, "add", "code.go")
				_, _ = gitx.Run(repo, "commit", "-m", "init")

				outsideFile := filepath.Join(outside, "trace.csv")
				_ = os.WriteFile(outsideFile, []byte("orig-trace"), 0o644)
				traceLink := filepath.Join(repo, "link.csv")
				_ = os.Symlink(outsideFile, traceLink)
				return []string{"trace", "-base", "HEAD", "-out", traceLink}, "", outsideFile
			},
			wantErr: "symlink",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestGitRepo(t)
			outside := t.TempDir()

			args, stdinStr, targetFile := tc.setup(t, repo, outside)
			origBytes, err := os.ReadFile(targetFile)
			if err != nil {
				t.Fatalf("setup target file missing: %v", err)
			}

			c := exec.Command(binaryPath, args...)
			c.Dir = repo
			if stdinStr != "" {
				c.Stdin = strings.NewReader(stdinStr)
			}
			out, err := c.CombinedOutput()
			if err == nil {
				t.Fatalf("expected error from command %v, got success: %s", args, string(out))
			}
			if tc.wantErr != "" && !strings.Contains(strings.ToLower(string(out)), tc.wantErr) {
				t.Fatalf("expected %q in output, got: %s", tc.wantErr, string(out))
			}

			// Verify outside target was untouched
			afterBytes, err := os.ReadFile(targetFile)
			if err != nil {
				t.Fatalf("target file removed: %v", err)
			}
			if string(origBytes) != string(afterBytes) {
				t.Fatalf("outside target modified:\nbefore: %q\nafter: %q", string(origBytes), string(afterBytes))
			}
		})
	}
}
