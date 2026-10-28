# Atlassian Jira Integration & Reality-Check Guide

**Module:** `scribex-ai`  
**Platform:** Atlassian Jira Cloud & Jira Data Center / Server  
**Audience:** Product Managers, Tech Leads, Quality Assurance Leads, DevSecOps Engineers  
**Scope:** Automated acceptance criteria verification, scope-creep detection, undeclared breaking change alerts, and idempotent ticket commenting.

---

## 1. Overview: The Problem of Jira vs. Code Drift

In modern agile teams, a significant disconnect exists between project management specifications and code implementation:

- **Undeclared Breaking Changes:** An engineer implements a bug fix or feature requested in a Jira ticket, but in the process alters a public method signature or drops an interface without realizing it affects upstream callers.
- **Scope Creep:** A pull request introduces three new exported endpoints or classes that were never specified in the ticket's Acceptance Criteria. Product Managers are unaware of the expanding API surface until after release.
- **Compliance Disconnect:** Auditors (SOC2, ISO 27001) require proof that every pull request corresponds to an approved Jira issue and that acceptance criteria were verified.

**Scribex AI** bridges this gap deterministically using `scribex jira`:

```
┌─────────────────────────────────┐           ┌────────────────────────────────┐
│      Jira Ticket (PROJ-102)     │           │    Git Pull Request #482       │
│ Summary: "Support RS256 Tokens" │           │ Branch: feat/PROJ-102-rs256    │
│ Description: Acceptance criteria│           │ AST Delta:                     │
│ specifies VerifyToken algorithm.│           │ - pkg/auth/jwt.go#VerifyToken  │
│                                 │           │ - pkg/auth/oauth.go#DropTable  │
└────────────────┬────────────────┘           └───────────────┬────────────────┘
                 │                                            │
                 └──────────────────────┬─────────────────────┘
                                        ▼
                            [ scribex jira -base origin/main ]
                                        │
                 ┌──────────────────────┴──────────────────────┐
                 ▼                                             ▼
       [Matched Acceptance Criteria]            [Flags Detected]
       VerifyToken is documented in ticket.     - Undeclared Breaking Change: DropTable
                                                - Possible Scope Creep: oauth.go
```

---

## 2. Prerequisites & Authentication

Scribex interacts with the Jira REST API v2 (`/rest/api/2/issue/{key}`). It shares credentials with Confluence:

### Required Credentials
- **`ATLASSIAN_URL`**: Instance base URL (e.g., `https://yourcompany.atlassian.net` or `https://jira.internal.corp`).
- **`ATLASSIAN_USER`**: Account email or service user username.
- **`ATLASSIAN_TOKEN`**: Atlassian API Token (Cloud) or Personal Access Token (Data Center).

### Storing Credentials Locally
```bash
printf %s "your-atlassian-token" | scribex secrets set ATLASSIAN_TOKEN
export ATLASSIAN_URL="https://yourcompany.atlassian.net"
export ATLASSIAN_USER="scribex-bot@yourcompany.com"
```

For on-premise Jira Data Center instances hosted inside private VPCs:
```bash
export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
```

---

## 3. Jira Key Extraction & Strict Validation

Scribex extracts the Jira issue key automatically from:
1. The **`-key` flag** explicitly passed on the command line: `-key PROJ-1234`.
2. The active **git branch name**: e.g., `feature/PROJ-1234-jwt-support` or `bugfix/CORE-987`.
3. The **git commit message** or PR title.

### Strict Key Validation Grammar
To prevent path traversal, SQL injection, or malformed queries, all Jira keys must conform to:
```
^[A-Z][A-Z0-9_]{1,30}-[1-9][0-9]{0,9}$
```

- **Valid Keys:** `PROJ-1`, `TEAM_A-1234`, `CORE-987654`.
- **Rejected Keys:** `proj-1` (lowercase), `PROJ-0` (number starting with zero), `PROJ-1/../x` (traversal sequences), `PROJ-1?x=1` (query injections).

---

## 4. Running the Jira Reality Check

### 4.1 CLI Command Syntax

```bash
scribex jira -base origin/main [-key PROJ-1234] [-post]
```

- **`-base <ref>`**: The target base branch (e.g., `origin/main` or `upstream/master`).
- **`-key <key>`**: Jira ticket key (optional if branch name contains the key).
- **`-post`**: Posts the verification findings directly as a comment to the Jira ticket.

### 4.2 Example Output

```bash
$ scribex jira -base origin/main
jira: checking issue PROJ-1234 (Support RS256 JWT tokens)
jira: 3 public contract delta(s) found in git diff

Flags:
- undeclared breaking change: pkg/auth/jwt.go#ValidateToken (modified)
- possible scope creep: new public pkg/auth/oauth.go#InitOAuthProvider

Acceptance Criteria Match:
- pkg/auth/jwt.go#VerifyToken matches ticket description.
```

---

## 5. Idempotent Comment Posting (`-post`)

When the `-post` flag is passed, Scribex posts the audit findings directly to the Jira ticket so product managers and QA engineers are alerted in real time.

### Preventing Comment Spam with Cryptographic Markers
To ensure CI pipelines do not post duplicate comments on every push to a pull request:
1. Scribex calculates the SHA-256 hash of the generated comment body.
2. Scribex fetches existing comments on the ticket (paginating up to 5 pages of 100 comments each).
3. If an existing comment contains the cryptographic marker:
   ```html
   <!-- scribex-reality-check:a1b2c3d4 -->
   ```
   Scribex **idempotently skips posting**, avoiding notification spam.
4. If code changes alter the findings, the hash changes, and Scribex updates or appends the new status.

---

## 6. GitHub Actions CI Integration

Integrate the Jira reality check into your pull request pipeline:

```yaml
name: Jira PR Reality Check
on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: read

jobs:
  jira-check:
    name: Verify Jira Ticket & Contracts
    runs-on: ubuntu-latest
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

      - name: Run Jira Reality Check
        env:
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: |
          # Automatically detects Jira key from branch name; posts comment if flags exist
          scribex jira -base "origin/${{ github.base_ref }}" -post
```

---

## 7. Compliance Traceability Matrix (`scribex trace`)

Jira integration is a core pillar of Scribex's SOC2 and ISO 27001 audit engine. To export the full audit matrix:

```bash
scribex trace -base origin/main -pr 482 -verify-jira -out audit_evidence.csv
```

- Verifies that the Jira issue exists, is active, and corresponds to the PR.
- Generates a formula-injection neutralized CSV and cryptographic `.csv.json` sidecar.
- See the [Compliance & Audit Evidence Guide](compliance_and_audit.md) for full details.

---

## 8. Troubleshooting & FAQ

### Q: `scribex: cannot find Jira key in branch or flags`
- **Cause:** Your git branch name does not contain a standard ticket format (e.g., branch is named `fix-auth-bug`).
- **Fix:** Either rename your branch to include the key (`feature/PROJ-1234-fix-auth`) or pass the key explicitly with `-key PROJ-1234`.

### Q: `scribex: jira key mismatch: expected PROJ-1234, got proj-1234`
- **Cause:** Jira keys are strictly uppercase.
- **Fix:** Use uppercase letters for project identifiers.

### Q: `scribex: 403 Forbidden on comment endpoint`
- **Cause:** The Atlassian service account lacks the "Add Comments" permission in the target Jira project.
- **Fix:** In Jira Project Settings $\rightarrow$ **Permissions**, ensure the service user has permission to comment on issues.
