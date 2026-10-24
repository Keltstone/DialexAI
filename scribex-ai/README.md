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

Languages: Go (AST), TS/JS, Python, Kotlin, Java (regex). Spec: `spec/SPEC.md` (all 4 phases implemented).
CI: `action/action.yml`. Prose is never LLM-rewritten; mismatches are flagged, signatures synced deterministically.
