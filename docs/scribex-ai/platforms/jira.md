# Atlassian Jira Setup Guide

**Module:** `scribex-ai`  
**Platform:** Atlassian Jira Cloud & Jira Data Center / Server  
**Audience:** Product Managers, Tech Leads, QA Leads, Developers  
**Scope:** Streamlined setup: repo implementation, Jira settings, PR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Branch Naming Convention
Scribex automatically links pull requests to Jira issues by parsing the issue key from your git branch name or the `-key` flag.

Use standard Jira ticket naming in your branch names:
```bash
# Recommended branch naming:
git checkout -b feature/PROJ-1234-auth-revamp
# or
git checkout -b bugfix/CORE-567-jwt-expiration
```
*Key format requirement: `^[A-Z][A-Z0-9_]{1,30}-[1-9][0-9]{0,9}$` (e.g., `PROJ-1`, `TEAM_A-1234`).*

### 1.2 Run Initial Reality Check Locally
Before pushing code, run `scribex jira` to reality-check your contract changes against the Jira ticket:

```bash
scribex jira -base origin/main
```

**What Scribex Checks:**
- Reads the Jira ticket's **Summary** and **Description** (where Acceptance Criteria live).
- Compares public AST deltas against the ticket.
- **Flags Undeclared Breaking Changes:** Methods altered or deleted that are not mentioned in the ticket.
- **Flags Possible Scope Creep:** New public APIs introduced that were not requested.

---

## Step 2: Jira Settings & Authentication

### 2.1 Jira API Credentials
Scribex uses the Atlassian REST API v2 (`/rest/api/2/issue/{key}`):
1. **Atlassian Cloud:** Use the same API token created at [id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens).
2. **Jira Data Center:** Generate a Personal Access Token (PAT) under Profile $\rightarrow$ **Settings** $\rightarrow$ **Personal Access Tokens**.

### 2.2 Jira Project Permissions
In your Jira Project $\rightarrow$ **Project Settings** $\rightarrow$ **Permissions**:
- Ensure the bot user or service account has **Browse Projects** and **Add Comments** permissions.

### 2.3 Store Credentials in Scribex
Store your API token in the local secrets vault:
```bash
printf %s "your_jira_token" | scribex secrets set ATLASSIAN_TOKEN
```

Export instance configuration:
```bash
export ATLASSIAN_URL="https://yourcompany.atlassian.net"
export ATLASSIAN_USER="scribex-bot@yourcompany.com"

# For On-Premise / Jira Data Center inside a corporate VPC:
export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
```

---

## Step 3: CI/CD & PR Gate Settings

Configure your CI pipeline to reality-check every Pull Request against its Jira ticket and post audit comments.

### 3.1 PR Gate & Idempotent Ticket Commenting
Add to `.github/workflows/scribex-jira.yml`:

```yaml
name: Scribex Jira Gate
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  jira-check:
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
      - name: Run Jira Reality Check & Comment
        env:
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: |
          scribex jira -base "origin/${{ github.base_ref }}" -post
```

### 3.2 Anti-Spam Comment Idempotence
When `-post` is specified:
- Scribex searches up to 5 pages of 100 comments on the ticket for the hidden cryptographic marker:
  ```html
  <!-- scribex-reality-check:<sha256-hash> -->
  ```
- If a comment with the exact same findings already exists, posting is skipped idempotently.
- Subsequent pushes to the PR will never spam Jira tickets unless code changes alter the findings.

---

## Step 4: Scribex Dashboard Items

When monitored via `scribex serve`, Jira verification feeds into centralized governance:

### 4.1 Dashboard Overview Items
- **Jira Key Verification Status:** Validates whether the branch ticket exists, is active, and is accessible.
- **Undeclared Breaking Changes Alert:** Highlights pull requests altering exported interfaces without ticket authorization.
- **Scope Creep Alert Counter:** Tracks new public APIs introduced outside ticket scope.

### 4.2 SOC2 / ISO 27001 Traceability Export
The dashboard links Jira issue verification directly into compliance matrices via `scribex trace`:
- Verified tickets display `jira_verified: true` in the exported evidence CSV.
- Maps Jira ticket ID $\leftrightarrow$ PR Number $\leftrightarrow$ Modified Code Symbol $\leftrightarrow$ Documentation Anchors.
