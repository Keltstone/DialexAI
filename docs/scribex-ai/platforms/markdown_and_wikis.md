# In-Repo Markdown, Living Diagrams & Wikis Setup Guide

**Module:** `scribex-ai`  
**Platform:** In-Repo Markdown, Living Diagrams (Mermaid.js), and Technical Wikis  
**Audience:** Software Engineers, Technical Writers, Open Source Maintainers  
**Scope:** Streamlined setup: repo implementation, wiki settings, PR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Add Code-to-Doc Anchors
Embed anchors directly into your `README.md` or any Markdown file in `docs/`:

```markdown
<!-- @scribex:anchor(pkg/auth/jwt.go#VerifyToken) -->
### VerifyToken
Validates incoming JWT bearer tokens using the RS256 algorithm.

```signature
func VerifyToken(tokenString string, keyFunc jwt.Keyfunc) (*jwt.Token, error)
```
<!-- /@scribex:anchor -->
```

### 1.2 Add Living Mermaid Architecture Diagrams
Insert a diagram placeholder into your documentation:

```markdown
<!-- @scribex:diagram -->
```mermaid
graph TD
    %% Scribex will populate this automatically
```
<!-- /@scribex:diagram -->
```

Run diagram generation:
```bash
scribex diagram
```
Scribex scans Go package imports and populates the Mermaid diagram with actual internal dependency relationships.

### 1.3 Verify and Synchronize Anchors
```bash
# Check if anchors match code signatures:
scribex check

# Synchronize signatures deterministically:
scribex sync

# Or create an automatic companion commit:
scribex sync -commit
```

---

## Step 2: Wiki Settings & Configuration

Configure the internal documentation wiki in `.scribex/config.yaml`:

### 2.1 Configuration Schema
```yaml
version: "1.0"
wiki:
  dir: docs/wiki          # Directory containing interlinked markdown pages
  home: Home              # Landing page name (Home.md)
```

### 2.2 Create New Wiki Pages
```bash
scribex wiki new "Getting Started"
```
Creates `docs/wiki/getting-started.md` with standardized metadata frontmatter.

### 2.3 Wikilinks Syntax
- Link pages using double brackets: `[[Getting Started]]` or `[[Getting Started|Click Here]]`.
- Links are case, space, and hyphen-insensitive (`[[getting started]]` matches `getting-started.md`).

### 2.4 Lint the Wiki
Run the wiki linter to catch broken links, duplicate titles, and orphan pages:
```bash
scribex wiki lint
```

---

## Step 3: CI/CD & PR Gate Settings

Configure your CI pipeline to gate pull requests on stale anchors and broken wiki links.

### 3.1 PR Gate Workflow (`.github/workflows/scribex-pr.yml`)
```yaml
name: Scribex Documentation Gate
on: [pull_request]

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27.0'
      - name: Build Scribex
        run: go build -trimpath -o /usr/local/bin/scribex ./cmd/scribex
      - name: Run Anchor PR Gate
        run: scribex check -base "origin/${{ github.base_ref }}"
      - name: Run Wiki Linter
        run: scribex wiki lint
```
- **PR Gate Behavior:** If a PR modifies an exported symbol without updating its anchored documentation, `scribex check` fails with exit code `1`.
- **Zero-Token Gate:** If the PR only touches internal implementations, `scribex check` exits `0` with 0 tokens consumed.

### 3.2 Weekly Scavenger Cron (`scribex scavenge`)
Run weekly to catch dead anchors and broken relative links across all `.md` files:
```yaml
name: Weekly Scavenge
on:
  schedule:
    - cron: "0 6 * * 1" # Every Monday at 06:00 UTC

jobs:
  scavenge:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27.0'
      - name: Build Scribex
        run: go build -trimpath -o /usr/local/bin/scribex ./cmd/scribex
      - name: Run Scavenge
        run: scribex scavenge
```

### 3.3 Exporting to GitHub Wiki (`.wiki.git`) or Static Site Generators
Render wikilinks into clean relative Markdown links with automated backlink footers:
```bash
# Render directly to GitHub Wiki repository clone:
scribex wiki render -out ../repo.wiki
```

---

## Step 4: Scribex Dashboard Items

When monitored via `scribex serve`, in-repo documentation provides real-time health metrics:

### 4.1 Dashboard Overview Items
- **Total In-Repo Anchors:** Count of all active `<!-- @scribex:anchor -->` blocks in the repository.
- **Stale Anchors Counter:** Number of anchors whose stored `sig=` hash does not match current code.
- **Dead Anchors Alert:** Red alert if any anchor points to a deleted code file or deleted function symbol.
- **Wiki Health Score:** Summary of broken wikilinks, orphan pages, and duplicate names.

### 4.2 Team Scope & Drift Trends
In `/etc/scribex/repos.txt`, map repositories to teams:
```text
core-engine /var/repos/core-engine core
```
- Displays historical anchor freshness trends for the team.
- Alerts engineering leads before documentation rot accumulates.
