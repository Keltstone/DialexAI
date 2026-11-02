# Atlassian Confluence Setup Guide

**Module:** `scribex-ai`  
**Platform:** Atlassian Confluence Cloud & Confluence Data Center / Server  
**Audience:** Software Engineers, Tech Leads, Engineering Managers  
**Scope:** Streamlined setup: repo implementation, Confluence settings, PR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Initialize Scribex in Your Repo
Run `scribex init` in the root of your project:
```bash
scribex init
```
This generates the `.scribex/manifest.yaml` mapping template.

### 1.2 Map Confluence Pages to Code Symbols
Edit `.scribex/manifest.yaml` to associate target Confluence pages with the code symbols they document:

```yaml
# Confluence page-to-symbol mappings
mappings:
  - page: "109823485"                   # Numeric Confluence page ID from URL
    space: "ENG"                        # Confluence space key
    title: "Authentication API Spec"    # Page title
    symbols:
      - "pkg/auth/jwt.go#VerifyToken"
      - "pkg/auth/session.go#NewSession"
```

> **How to find Page ID:** Open your target page in Confluence. The numeric ID in the browser URL (`.../pages/109823485/...`) is your Page ID.

### 1.3 Verify Initial Setup Locally
Pull page bodies from Confluence and test local draft creation:
```bash
# Pull current storage-format content from Confluence:
scribex confluence pull

# Run a dry-run draft push:
scribex confluence push
```
- `scribex confluence push` computes code signatures, formats them into XHTML, and creates a **draft revision** (`status: draft`) in Confluence.
- Existing published pages are **never overwritten directly**; a tech lead reviews the diff in Confluence before publishing.

---

## Step 2: Confluence Settings & Authentication

### 2.1 Generate Atlassian API Token
1. Go to [id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens).
2. Click **Create API token**, label it `scribex-sync`, and copy the token.
3. For Confluence Data Center / On-Premise, generate a **Personal Access Token (PAT)** in Confluence Profile $\rightarrow$ **Settings** $\rightarrow$ **Personal Access Tokens**.

### 2.2 Configure Space Permissions
In Confluence: **Space Settings** $\rightarrow$ **Permissions**:
- Ensure the account associated with the token has **Add (Pages)** and **Edit (Pages)** permissions in the target space (`ENG`).

### 2.3 Store Secrets in Scribex
Store your API token securely in Scribex's local vault:
```bash
printf %s "your_atlassian_token" | scribex secrets set ATLASSIAN_TOKEN
```

Export instance configuration in your shell or `.env`:
```bash
export ATLASSIAN_URL="https://yourcompany.atlassian.net"
export ATLASSIAN_USER="scribex-bot@yourcompany.com"

# For On-Premise / Data Center behind corporate firewalls:
export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
```

---

## Step 3: CI/CD & PR Gate Settings

Configure your CI pipeline to verify anchors on Pull Requests and update Confluence drafts upon merge.

### 3.1 PR Gate: Block Pull Requests with Stale Signatures
Add to `.github/workflows/scribex-pr.yml` (or GitLab equivalent):

```yaml
name: Scribex PR Check
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
      - name: Run PR Gate
        run: scribex check -base "origin/${{ github.base_ref }}"
```
- **PR Gate Behavior:** If a PR modifies a public symbol mapped to Confluence without updating documentation, `scribex check` fails with exit code `1`.
- **Zero-Token Gate:** If the PR only changes internal function logic or private tests, it exits `0` with 0 tokens consumed.

### 3.2 Post-Merge Action: Push Confluence Draft
Add a post-merge job on push to `main`:

```yaml
name: Confluence Living Sync
on:
  push:
    branches: [main]

jobs:
  confluence-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27.0'
      - name: Build Scribex
        run: go build -trimpath -o /usr/local/bin/scribex ./cmd/scribex
      - name: Push Drafts
        env:
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: scribex confluence push
```

---

## Step 4: Scribex Dashboard Items

When running the centralized governance daemon (`scribex serve`), Confluence integration appears across several key dashboard items:

### 4.1 Dashboard Overview Items
- **Mapped Pages Count:** Number of external Confluence pages declared in `.scribex/manifest.yaml`.
- **Sync Status & Last Draft Timestamp:** Displays whether the Confluence draft is in sync with current `HEAD` commit signatures.
- **Stale Anchor Indicators:** Highlights symbols whose code changed in git but haven't had their draft pushed to Confluence.

### 4.2 Team Scope & RBAC (`repos.txt`)
In your dashboard's `/etc/scribex/repos.txt`, assign repository ownership:
```text
auth-service /var/repos/auth-service auth
```
- Developers on the `auth` team will see the Confluence sync health for `auth-service`.
- Global administrators see cross-team Confluence drift metrics.

### 4.3 Configuration & Secrets Health (`/config`)
- The dashboard `/config` page displays read-only status:
  - `ATLASSIAN_URL`: Configured
  - `ATLASSIAN_USER`: Configured
  - `ATLASSIAN_TOKEN`: `set` (masked; secret values are never exposed or editable over HTTP).
