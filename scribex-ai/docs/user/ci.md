# Scribex in Continuous Integration (CI)

This document describes how Scribex behaves in CI environments, its exit code conventions, and how to safely run `scribex sync -commit`.

## Exit Codes

Scribex follows standard POSIX exit status semantics:

| Exit Code | Meaning | Description |
|-----------|---------|-------------|
| `0` | **Success** | All checks passed cleanly, or sync succeeded with no unresolved issues. |
| `1` | **Findings / Issues** | Issues were detected (e.g. stale/broken/dead anchors, unconfined paths, read-only file write refusal `write-refused`, or commit preconditions failed). |
| `2` | **Usage / CLI Error** | Invalid flags, unknown subcommands, or syntax errors. |

In CI pipelines, any non-zero exit code (`1` or `2`) will fail the step.

## Safe Automated Commits: `scribex sync -commit`

The `-commit` flag instructs `scribex sync` to commit updated documentation files automatically when anchors are synchronized.

### Semantics
- **Minimal staging**: Scribex only commits files that Scribex itself modified during this specific run (`git commit --only -- <changed...>`).
- **Preserves existing work**: Unstaged changes to code or other documents remain unstaged. Staged WIP commits in the index remain staged.
- **Literal paths**: All paths are passed literally with pathspec guards to prevent glob expansion on unusual filenames.

### Preconditions for `-commit`
Scribex checks several preconditions before attempting to commit:
1. **Git available**: The `git` executable must be present on the system `PATH`.
2. **Attached HEAD**: Commits cannot be created in a detached HEAD state.
3. **No pre-dirty documentation files**: If a doc file that Scribex needs to modify was already dirty (modified in index or working tree) before Scribex ran, Scribex aborts the commit to avoid overwriting uncommitted user work.
4. **Resolved author identity**: Git commit requires a committer identity. Scribex will refuse to commit if no identity is found.

### Identity Environment Variables
In headless CI environments where standard `git config` is unset, you can provide identity through environment variables:
- `SCRIBEX_GIT_NAME`: The committer and author name (e.g., `Scribex Bot`).
- `SCRIBEX_GIT_EMAIL`: The committer and author email (e.g., `scribex-bot@example.com`).

These take precedence when `git` author configuration is not set globally or locally in the repository.

### CI Security: Hooks Neutralized
To guarantee deterministic execution and protect CI environments against arbitrary code execution from untrusted repository hooks, `scribex sync -commit` neutralizes git hooks by configuring `core.hooksPath` to `/dev/null` (or `NUL` on Windows). Repository-defined client-side hooks (`pre-commit`, `commit-msg`, etc.) are not executed during automated sync commits.
