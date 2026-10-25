# Scribex AI

Keeps docs in sync with code. Anchor a doc section to a symbol:

```markdown
<!-- @scribex:anchor(pkg/auth/jwt.go#VerifyToken) -->
Validates JWT bearer tokens.
<!-- @scribex:end -->
```

| Command | Does |
|---|---|
| `scribex init` | create `.scribex/manifest.yaml` |
| `scribex check [-C dir] [-base ref]` | exit 1 on stale/dead anchors; with `-base`, exit 0 early (0 tokens) if no public signature changed |
| `scribex sync [-commit]` | (-commit: companion commit of changed .md) refresh `sig=` hash + ```` ```signature ```` block (prose untouched) |
| `scribex diagram` | rebuild `<!-- @scribex:diagram -->` Mermaid block from Go imports |
| `scribex scavenge` | dead anchors + broken relative links (run on a cron) |
| `scribex comment -repo o/r -pr N [-base ref]` | upsert one PR comment with the report (`GITHUB_TOKEN`) |
| `scribex confluence push\|pull [page]` | push draft revisions / pull bodies per `.scribex/manifest.yaml` (`ATLASSIAN_URL/USER/TOKEN`) |
| `scribex jira -base ref [-key K] [-post]` | flag undeclared breaking changes / scope creep vs ticket |
| `scribex trace -base ref [-key K] [-pr N] [-out f.csv]` | Jira ↔ PR ↔ symbol ↔ docs ↔ Confluence matrix (SOC2/ISO evidence) |
| `scribex serve -repos f -rbac f [-addr :8080]` | doc-health dashboard, team-scoped; `scribex hash-token <t>` for the rbac file |

Repos file: `name path team`. RBAC file: `sha256(token) admin|viewer team,team`; send the token as the HTTP Basic password.

Languages: Go (AST), Kotlin, Java, TS/JS, Python (lexical scanner). Spec: `spec/SPEC.md`.
CI: `action/action.yml`. Prose is never LLM-rewritten; mismatches are flagged, signatures synced deterministically.

## Configure the AI, secrets, CI

```bash
scribex config init                              # .scribex/config.yaml (committed; holds NO secrets)
scribex config set ai.model claude-sonnet-5-5    # ai.provider: anthropic | openai (+ ai.base_url for compatible APIs)
printf %s "$KEY" | scribex secrets set ANTHROPIC_API_KEY   # .scribex/secrets.env, 0600, auto-gitignored
scribex config test                              # one tiny call to verify credentials
scribex sync -ai                                 # LLM updates prose only for anchors whose signature CHANGED
scribex ci init github                           # or gitlab: workflow with secrets wired as env
```

- `ai.api_key_env` is the *name* of the env var; pasting a key there is rejected.
- Locally, real env vars win over `secrets.env`; in CI use the platform's secret store.
- Token control: zero-token gate, first-time syncs never call the LLM, the system prompt (incl. `ai.style_guide`) is prompt-cached, and `ai.max_calls` is a hard cap per run.
- The dashboard `/config` page (admin) shows config and set/unset secret status, read-only: secrets are never editable over HTTP.

## MCP for documentation systems

`.scribex/mcp.json` uses the standard `mcpServers` shape (drop-in for a client's `.mcp.json` via `scribex mcp export`). Credentials are only `${ENV_VAR}` references; literal secrets are rejected.

```bash
scribex mcp add confluence -preset atlassian-token   # Confluence+Jira via ATLASSIAN_URL/USER/TOKEN
scribex mcp add confluence -preset confluence-oauth  # browser OAuth, no stored secret
scribex mcp add docs -url https://mcp.example.com -header 'Authorization=Bearer ${DOCS_TOKEN}'
scribex mcp check                                    # which servers are missing credentials
scribex mcp add scribex -preset scribex              # expose Scribex itself (read-only tools) to agents
```

Presets: `scribex mcp add -h` lists them (scribex, confluence-oauth, atlassian-token, github, notion). Package names are upstream's; verify before relying on them. Scribex manages and serves MCP config; it does not itself call those servers.

## Wiki (interlinked markdown)

`wiki.dir` (default `docs/wiki`). Pages use `[[Page]]`, `[[Page|label]]` or normal relative `.md` links; names are case/space/hyphen-insensitive.

```bash
scribex wiki new "Getting Started"   # page with frontmatter (title, tags)
scribex wiki lint                    # broken links, orphans, duplicate names (exit 1)
scribex wiki backlinks Home | search <q>
scribex wiki render -out ../repo.wiki   # relative links + "Linked from" footers + Home.md/_Sidebar.md for GitHub's wiki
```
