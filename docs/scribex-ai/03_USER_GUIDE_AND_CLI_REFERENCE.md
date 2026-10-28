# Scribex AI: Complete User Guide & CLI Reference Manual

**Module:** `scribex-ai`  
**Author:** Keltstone Systems Core Engineering & Knowledge Architecture Group  
**Audience:** Software Engineers, Tech Leads, DevSecOps Engineers, Documentation Architects  
**Scope:** Exhaustive CLI syntax, configuration schemas, operational playbooks, and environment controls.

---

## 1. Installation & Binary Verification

Scribex AI is distributed as a single static binary with no external runtime dependencies.

### Installation Options

```bash
# Option A: Homebrew (macOS / Linux)
brew install keltstone/tap/scribex

# Option B: One-Line Universal Installer (macOS, Linux)
curl -sSL https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.sh | bash -s -- --app scribex

# Option C: Build from Source (Go 1.27+ required)
go install github.com/Keltstone/DialexAI/scribex-ai/cmd/scribex@main
```

Verify the installation:
```bash
scribex version
# Output: Scribex AI version 1.0.0
```

---

## 2. Exhaustive Command Reference

```
usage: scribex <cmd> [flags]
```

### 2.1 Initialization & Core Verification

#### `scribex init`
Initializes a repository for Scribex documentation tracking.
- Creates `.scribex/manifest.yaml` with template mappings for Confluence pages and symbols.
- Creates `.scribex/` directory with strict security constraints (must not be a symlink).
- Idempotent: If `.scribex/manifest.yaml` already exists, it is preserved without modification.

#### `scribex check [-C dir] [-base ref] [-report f.md]`
Inspects documentation anchors across the codebase and verifies them against source code symbols.
- **`-C dir`**: Repository root directory (default: current directory `.`).
- **`-base ref`**: Git reference (e.g., `origin/main`) to diff against. Activates the **Zero-Token Impact Gate**. If no exported signatures changed relative to `-base`, exits `0` immediately.
- **`-report f.md`**: Writes a comprehensive Markdown drift report to the specified file (atomic write; refuses to overwrite symlinks).
- **Exit Code:** Exits `0` if all anchors are valid and synchronized; exits `1` if dead or stale anchors are detected; exits `2` on usage errors.

#### `scribex sync [-commit] [-ai]`
Synchronizes documentation anchors with source code signatures.
- **Default (Mechanical Sync):** Updates the cryptographic signature hash (`sig=...`) and refreshes ```` ```signature ```` blocks in Markdown. Leaves human-written prose untouched.
- **`-commit`**: Automatically creates a companion git commit containing all modified documentation files (`docs: sync signatures for changed code`).
- **`-ai`**: Leverages the configured AI provider (Anthropic, OpenAI) to rewrite explanatory prose for anchors whose code signatures changed.
  - *Safety Guard:* Strictly refuses to run on pull request events (`pull_request`, `merge_request_event`). Must be executed on post-merge default branches.
  - *Attribution:* Appends `<!-- @scribex:ai sig=<hash> -->` to updated sections for human audit.

#### `scribex diagram`
Rebuilds dynamic architecture dependency diagrams inside documentation files.
- Locates `<!-- @scribex:diagram -->` comment blocks.
- Scans Go imports across repository packages, resolves internal topological relationships, and synthesizes updated Mermaid.js diagrams.
- Replaces diagram blocks atomically.

#### `scribex scavenge`
Orphan and link scavenger designed for weekly cron execution.
- Identifies dead anchors pointing to non-existent symbols or deleted files.
- Detects broken relative links across all in-repo Markdown documents.
- Exits `1` if broken links or dead anchors are found.

---

### 2.2 CI/CD & Review Automation

#### `scribex comment -repo o/r -pr N [-base ref] [-body-file path] [-author LOGIN]`
Upserts an automated status report comment on a GitHub pull request.
- **`-repo owner/name`**: GitHub repository in `owner/repo` format (defaults to `GITHUB_REPOSITORY` env var).
- **`-pr N`**: Pull request number.
- **`-base ref`**: Upstream base ref to diff against.
- **`-body-file path`**: Post pre-rendered report from file instead of running live check.
- **`-author LOGIN`**: Override comment author login identity (default: authenticated bot user).
- **Idempotence:** Searches existing PR comments for the Scribex signature marker and updates the comment in place rather than creating duplicate notifications.

#### `scribex ci init github|gitlab [-force]`
Generates production-grade, hardened CI/CD workflow definitions.
- **`github`**: Creates:
  - `.github/workflows/scribex-pr.yml`: Fast PR anchor verification and PR comment posting.
  - `.github/workflows/scribex-docs.yml`: Post-merge automatic `sync -ai` with docs PR creation.
  - `.github/workflows/scribex-scavenge.yml`: Weekly scheduled scavenge and wiki linting.
- **`gitlab`**: Creates `.gitlab-ci.scribex.yml` for GitLab MR pipelines.
- **`-force`**: Overwrite existing workflow files.

---

### 2.3 Enterprise Integrations (Atlassian & Compliance)

#### `scribex confluence push|pull [page]`
Synchronizes documentation with Atlassian Confluence Cloud or Server/DC.
- Requires environment variables: `ATLASSIAN_URL`, `ATLASSIAN_USER`, `ATLASSIAN_TOKEN`.
- **`push`**: Reads `.scribex/manifest.yaml` and opens non-destructive **draft revisions** in Confluence with updated code signatures. Never overwrites published pages directly. Idempotent based on content hash.
- **`pull [page]`**: Pulls current storage-format XHTML body from Confluence into local `.scribex/pages/` for diffing.

#### `scribex jira -base ref [-key K] [-post]`
Performs an automated reality check between code changes and Jira user stories.
- **`-base ref`**: Upstream git ref.
- **`-key K`**: Jira issue key (e.g., `PROJ-1234`). If omitted, extracts the key automatically from the current git branch name.
- **`-post`**: Posts the reality check findings directly to the Jira ticket comments. Idempotent: uses `<!-- scribex-reality-check:<hash> -->` marker to prevent duplicate comments.
- **Flags:** Detects undeclared breaking changes and possible scope creep (new public APIs unmentioned in the ticket acceptance criteria).

#### `scribex trace -base ref [-key K] [-pr N] [-out f.csv] [flags]`
Generates a deterministic SOC2 / ISO 27001 traceability evidence matrix.
- Generates formula-injection neutralised CSV mapping: `Jira Issue ↔ PR Number ↔ Changed Symbol ↔ In-Repo Doc ↔ Confluence Page`.
- Generates companion JSON sidecar (`f.csv.json`) containing cryptographic SHA-256 checksum and audit metadata.
- **Flags:**
  - **`-verify-jira`**: Queries Jira REST API to verify ticket validity and status.
  - **`-require-jira`**: Fails if Jira credentials or issue key are missing.
  - **`-allow-no-key`**: Permits empty Jira key in trace evidence for internal maintenance PRs.

---

### 2.4 Wiki, MCP & Governance Daemon

#### `scribex wiki new <title> | lint | search <q> | backlinks <page> | render -out dir`
Manages the interlinked documentation wiki (`docs/wiki/` by default).
- **`new <title>`**: Creates a new wiki page with standardized YAML frontmatter.
- **`lint`**: Validates wikilink references, detects broken links, duplicate titles, and orphan pages.
- **`search <q>`**: Fast case-insensitive text search across page titles and bodies.
- **`backlinks <page>`**: Lists all documents linking to the specified page.
- **`render -out dir`**: Renders internal `[[Wikilinks]]` into standard relative Markdown links and generates `Home.md` and `_Sidebar.md` for GitHub Wiki publishing.

#### `scribex mcp add <name> [...] | list | remove <name> | check | export | serve`
Manages Model Context Protocol (MCP) servers in `.scribex/mcp.json`.
- **`add <name> -preset <preset>`**: Configures upstream MCP servers from vetted presets (`confluence-oauth`, `atlassian-token`, `github`, `notion`, `scribex`).
- **`add <name> -url <url>`**: Configures remote MCP server over SSE.
- **`add <name> -cmd <cmd> -args <a,b>`**: Configures local stdio MCP server.
- **`check`**: Validates all configured servers and reports missing environment variables.
- **`export`**: Prints `.scribex/mcp.json` for drop-in use in Claude Desktop, Cursor, or Antigravity.
- **`serve`**: Runs Scribex itself as an MCP stdio server, exposing read-only documentation tools to AI agents.

#### `scribex serve -repos file -rbac file -allowed-root dir [flags]`
Launches the centralized documentation governance daemon and dashboard.
- **`-repos file`**: Registry mapping repositories to teams: `repo_name path team`.
- **`-rbac file`**: Access control mapping: `sha256_token admin|viewer team,team`.
- **`-allowed-root dir`**: Root path restricting repository filesystem access.
- **`-addr 127.0.0.1:8080`**: HTTP listen address.
- **`-cache-ttl 30s`**: In-memory cache duration for anchor metrics.
- **`-tls-cert f -tls-key f`**: TLS certificate and private key for HTTPS termination.
- **`-behind-proxy`**: Trust `X-Forwarded-For` and `X-Forwarded-Proto` headers when deployed behind reverse proxies.
- **`-trusted-proxy cidr`**: Comma-separated list of trusted proxy CIDRs (e.g., `10.0.0.0/8`).

#### `scribex hash-token <token>`
Generates the SHA-256 hash of a plain token string for inclusion in the `rbac` file.

---

### 2.5 Configuration & Secrets Management

#### `scribex config init | show | set <k> <v> | validate | test`
- **`init`**: Generates default `.scribex/config.yaml`.
- **`show`**: Prints active configuration.
- **`set <key> <value>`**: Sets a configuration value (e.g., `scribex config set ai.model claude-sonnet-5-5`).
- **`validate`**: Validates configuration syntax and constraints.
- **`test`**: Performs a test ping against the configured AI provider.

#### `scribex secrets set <NAME> | list | check`
- **`set <NAME>`**: Stores a secret securely in `.scribex/secrets.env` (permissions `0600`, automatically added to `.gitignore`). Value is read from stdin:
  ```bash
  printf %s "$ANTHROPIC_KEY" | scribex secrets set ANTHROPIC_API_KEY
  ```
- **`list`**: Lists configured secret names (values remain strictly masked).
- **`check`**: Verifies that all required secrets referenced in `config.yaml` are present in the environment or secrets store.

---

## 3. Configuration File Schemas

### 3.1 `.scribex/config.yaml`
Committed to git. Stores project settings, but **never contains secrets**.

```yaml
version: "1.0"

ai:
  provider: anthropic                   # anthropic | openai
  model: claude-sonnet-5-5              # LLM model identifier
  api_key_env: ANTHROPIC_API_KEY       # Environment variable NAME (never the key itself)
  max_calls: 10                         # Hard upper bound on LLM calls per run
  style_guide: docs/STYLE_GUIDE.md      # Optional markdown or text style guide (<16KB)
  base_url: ""                          # Optional custom base URL for OpenAI-compatible APIs

wiki:
  dir: docs/wiki                        # Directory containing wiki markdown files
  home: Home                            # Default root landing page name
```

### 3.2 `.scribex/manifest.yaml`
Committed to git. Defines external platform page mappings.

```yaml
mappings:
  - page: "109823485"                   # Confluence numeric page ID
    space: "ENG"                        # Confluence Space Key
    title: "Authentication API Spec"    # Page Title
    symbols:                            # Associated code symbols
      - "pkg/auth/jwt.go#VerifyToken"
      - "pkg/auth/session.go#NewSession"
```

### 3.3 `.scribex/mcp.json`
Committed to git. Standard MCP server configuration with environment variable references.

```json
{
  "mcpServers": {
    "confluence": {
      "command": "uvx",
      "args": ["mcp-atlassian"],
      "env": {
        "CONFLUENCE_URL": "${ATLASSIAN_URL}/wiki",
        "CONFLUENCE_USERNAME": "${ATLASSIAN_USER}",
        "CONFLUENCE_API_TOKEN": "${ATLASSIAN_TOKEN}",
        "JIRA_URL": "${ATLASSIAN_URL}",
        "JIRA_USERNAME": "${ATLASSIAN_USER}",
        "JIRA_API_TOKEN": "${ATLASSIAN_TOKEN}"
      }
    },
    "scribex": {
      "command": "scribex",
      "args": ["mcp", "serve", "-C", "."]
    }
  }
}
```

---

## 4. Operational Playbooks

### Playbook A: Developer Local PR Preparation
Before pushing a branch with public signature changes:

```bash
# 1. Verify what public contracts changed relative to main:
scribex check -base origin/main

# 2. Update signatures deterministically:
scribex sync

# 3. Create a clean companion commit:
git commit -am "docs: sync signatures with auth API refactor"
git push origin feature/auth-refactor
```

### Playbook B: Post-Merge Automated Sync & PR Creation
Executed automatically in CI on push to `main`:

```bash
# 1. Validate environment and secrets:
scribex config validate
scribex secrets check

# 2. Synchronize signatures and request AI prose drafts:
scribex sync -ai

# 3. If docs changed, open an automated review PR:
git checkout -b scribex/docs-sync-$(git rev-parse --short HEAD)
git commit -am "docs: automated scribex sync"
git push origin HEAD
gh pr create --title "docs: scribex living sync" --body "Automated documentation sync."
```
