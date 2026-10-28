# Atlassian Confluence Integration & Setup Guide

**Module:** `scribex-ai`  
**Platform:** Atlassian Confluence Cloud & Confluence Data Center / Server  
**Audience:** Technical Documentation Leads, Engineering Managers, DevOps Engineers  
**Scope:** Bi-directional synchronization, draft review workflows, API tokens, manifest mapping, and private network deployment.

---

## 1. Overview & Architectural Model

Atlassian Confluence is the enterprise standard for engineering documentation, API specifications, and architectural runbooks. However, Confluence spaces frequently turn into documentation graveyards because they are separated from the git pull request workflow.

Scribex AI bridges this gap with an **In-Repo Grounded, Draft-Only Push Architecture**:

```
[ Git Repository ]                                      [ Atlassian Confluence ]
pkg/auth/jwt.go                                          Space: ENG
└── func VerifyToken(...) ──┐                            Page ID: 109823485
                            │                            "Auth API Specification"
.scribex/manifest.yaml      ▼                                      │
└── page: 109823485   ──► scribex confluence push ──► Creates DRAFT Revision
    symbols: [jwt.go]     (Calculates Signatures)     (Diff view for Tech Lead)
                                                                   │
                                                      Tech Lead Approves & Publishes
```

### Key Guarantees
1. **Never Overwrites Published Pages:** `scribex confluence push` only creates **draft revisions** (`status: draft`). A human tech lead or documentation editor reviews the visual diff inside Confluence and publishes it with a single click.
2. **Push Idempotence:** Scribex calculates the SHA-256 hash of the generated Confluence storage format. If code signatures have not changed since the last push, `push` skips re-creating the draft.
3. **Payload Capping:** Draft payloads are strictly capped at 1 MiB to prevent denial-of-service or memory bloat.

---

## 2. Prerequisites & Authentication Setup

### 2.1 Confluence Cloud Setup

1. **Log in to Atlassian:** Navigate to [id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens).
2. **Create API Token:**
   - Click **Create API token**.
   - Label: `scribex-sync-token`.
   - Copy the generated token immediately.
3. **Identify Required Variables:**
   - **`ATLASSIAN_URL`**: Your Confluence instance root URL (e.g., `https://yourcompany.atlassian.net`). Do not include `/wiki` or trailing slashes.
   - **`ATLASSIAN_USER`**: The email address of the Atlassian user account (e.g., `bot-scribex@company.com`).
   - **`ATLASSIAN_TOKEN`**: The API token generated above.

### 2.2 Confluence Data Center / On-Premise Setup

For self-hosted Confluence Data Center or Server:
1. **Generate Personal Access Token (PAT):**
   - Click your profile icon in Confluence $\rightarrow$ **Settings** $\rightarrow$ **Personal Access Tokens**.
   - Create a token with read/write permissions to target spaces.
2. **Configure Variables:**
   - `ATLASSIAN_URL`: `https://confluence.internal.corp`
   - `ATLASSIAN_USER`: Your internal username (or leave blank if using Bearer PAT).
   - `ATLASSIAN_TOKEN`: The Personal Access Token.
3. **Enable Private Subnet Access:**
   Because internal instances run on RFC 1918 subnets (`10.x.x.x`, `172.16.x.x`, `192.168.x.x`), activate private network routing:
   ```bash
   export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
   ```

---

## 3. Local Credential Configuration

Store credentials securely in `.scribex/secrets.env` (never commit plaintext tokens):

```bash
# 1. Set environment variables or store in Scribex local vault:
printf %s "your-atlassian-api-token" | scribex secrets set ATLASSIAN_TOKEN

# 2. Add URLs and User email to your shell environment (.env or .zshrc):
export ATLASSIAN_URL="https://yourcompany.atlassian.net"
export ATLASSIAN_USER="scribex-bot@yourcompany.com"
```

Verify that the secret is loaded properly:
```bash
scribex secrets list
# Output:
# ATLASSIAN_TOKEN (set, value hidden)
```

---

## 4. Configuring `.scribex/manifest.yaml`

The manifest file binds Confluence pages to code symbols in your repository.

### 4.1 How to Find the Numeric Confluence Page ID

1. Open your target page in Confluence.
2. Inspect the browser URL:
   - Modern URL: `https://yourcompany.atlassian.net/wiki/spaces/ENG/pages/109823485/Auth+API+Specification`
   - Short URL: Click **Share** $\rightarrow$ **Copy link** $\rightarrow$ extracts `/pages/109823485`.
3. The numeric string (`109823485`) is the **Page ID**.

### 4.2 Manifest Schema & Validation Rules

Create or edit `.scribex/manifest.yaml` in your repository root:

```yaml
# Confluence page-to-symbol mappings
mappings:
  - page: "109823485"
    space: "ENG"
    title: "Authentication API Specification"
    symbols:
      - "pkg/auth/jwt.go#VerifyToken"
      - "pkg/auth/jwt.go#GenerateClaims"
      - "pkg/auth/session.go#NewSession"

  - page: "204859182"
    space: "ARCH"
    title: "Storage Engine & Data Vault"
    symbols:
      - "pkg/vault/vault.go#StorageVault"
      - "pkg/vault/crypto.go#EncryptEnvelope"
```

### Validation Constraints Enforced by Scribex
- **`page`**: Must contain 1 to 20 digits only. Path traversals (`..`), letters, or special characters are rejected immediately.
- **`space`**: Must match `^[A-Za-z0-9~][A-Za-z0-9_~-]{0,254}$`.
- **`title`**: Non-empty, max 255 runes, no control characters or newlines.
- **`symbols`**: Each entry must follow `<relative-filepath>#<SymbolName>` with exactly one `#`. File paths must be clean relative paths without `..` or leading slashes.

---

## 5. Daily Operations & CLI Workflow

### 5.1 Pulling Page Bodies from Confluence
To pull the current live storage-format XHTML content into `.scribex/pages/`:

```bash
# Pull all mapped pages:
scribex confluence pull

# Or pull a specific page by ID:
scribex confluence pull 109823485
```

The content is saved to `.scribex/pages/109823485.xml`.

### 5.2 Pushing Draft Revisions to Confluence
When source code signatures change:

```bash
scribex confluence push
```

**What Happens Under the Hood:**
1. Scribex parses all source files referenced in `.scribex/manifest.yaml`.
2. Computes the latest normalized signatures for all mapped symbols.
3. Formats signatures into clean Confluence XHTML storage format (`<h3>Symbol</h3><pre>Signature</pre>`).
4. Checks the SHA-256 hash against `.scribex/state.json`.
5. Calls `POST /wiki/rest/api/content` with `"status": "draft"`.
6. Prints the draft revision ID:
   ```
   pushed draft for page 109823485 (revision 42)
   ```

### 5.3 Reviewing the Draft in Confluence
1. Open the page in Confluence.
2. A banner appears: *"Unpublished changes: A draft exists for this page."*
3. Click **Edit** $\rightarrow$ Confluence displays the diff between the previous published version and the Scribex draft.
4. Click **Publish** to make the changes live for the company.

---

## 6. CI/CD Automation (GitHub Actions & GitLab CI)

### 6.1 GitHub Actions Workflow (`.github/workflows/scribex-confluence.yml`)

Add an automated post-merge push to Confluence when changes land on `main`:

```yaml
name: Confluence Living Sync
on:
  push:
    branches: [main]
    paths:
      - 'pkg/**'
      - '.scribex/manifest.yaml'
  workflow_dispatch:

permissions:
  contents: read

jobs:
  confluence-sync:
    name: Push Confluence Drafts
    runs-on: ubuntu-latest
    environment: scribex-ai
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.27.0'

      - name: Build Scribex
        run: |
          go build -trimpath -ldflags "-s -w" -o /usr/local/bin/scribex ./cmd/scribex

      - name: Push Drafts
        env:
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: |
          scribex confluence push
```

---

## 7. Model Context Protocol (MCP) Setup for Confluence

To allow AI agents (Claude Desktop, Cursor, Antigravity) to query Confluence documentation alongside your code:

```bash
# Add Confluence MCP server with Atlassian token preset:
scribex mcp add confluence -preset atlassian-token

# Check credentials:
scribex mcp check

# Export configuration for your client:
scribex mcp export > ~/.claude/mcp.json
```

This registers the upstream `mcp-atlassian` server with environment references (`${ATLASSIAN_URL}`, `${ATLASSIAN_TOKEN}`), ensuring no literal secrets are committed or leaked.

---

## 8. Troubleshooting & FAQ

### Q: `scribex: connection refused to 10.0.1.20`
- **Cause:** You are connecting to an on-premise Confluence Data Center on a private corporate network, and Scribex's SSRF guard is blocking RFC 1918 addresses.
- **Fix:** Set `export SCRIBEX_ALLOW_PRIVATE_HOSTS=1` in your environment.

### Q: `scribex: 401 Unauthorized`
- **Cause:** Invalid token, incorrect email, or token lacking read/write space permissions.
- **Fix:** Ensure `ATLASSIAN_USER` is the exact email associated with the API token, not a display name. Ensure the account has "Add Page" and "Edit Page" permissions in the target space.

### Q: `scribex: page body exceeds 1 MiB limit`
- **Cause:** Mapping dozens of large symbols to a single Confluence page exceeded the 1 MiB safety threshold.
- **Fix:** Split your documentation into multiple smaller Confluence pages (e.g., one page per package) and update `.scribex/manifest.yaml`.
