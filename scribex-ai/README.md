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
| `scribex sync` | refresh `sig=` hash + ```` ```signature ```` block (prose untouched) |
| `scribex diagram` | rebuild `<!-- @scribex:diagram -->` Mermaid block from Go imports |

Languages: Go (AST), TS/JS, Python, Kotlin, Java (regex). Phase 1 only; see `spec/SPEC.md`.
CI: `action/action.yml`. Not yet: companion commits, bot comments, Atlassian, dashboard.
