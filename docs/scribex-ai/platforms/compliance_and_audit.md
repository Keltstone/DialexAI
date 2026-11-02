# Compliance & Audit Traceability Setup Guide (SOC2 / ISO 27001)

**Module:** `scribex-ai`  
**Platform:** SOC2 / ISO 27001 Compliance Vaults & Audit Engines (Vanta, Drata, Secureframe, AWS S3 / GCS)  
**Audience:** Compliance Officers, Security Engineers, DevSecOps Leads, Engineering Directors  
**Scope:** Streamlined setup: repo implementation, compliance settings, PR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Generate Traceability Evidence
Generate the deterministic traceability matrix linking Jira tickets, pull requests, AST symbol deltas, in-repo docs, and Confluence pages:

```bash
scribex trace -base origin/main -pr 482 -key PROJ-1234 -out evidence_pr482.csv
```

### 1.2 Verify Cryptographic Sidecar
When writing `-out evidence.csv`, Scribex atomically writes a cryptographic companion metadata file: `evidence.csv.json`:
- Contains row count, base/head commit SHAs, UTC timestamp, and the **SHA-256 digest** of the CSV.
- Formula-injection defense: all cells are sanitized (neutralizing `=, +, -, @, \t, \r` prefixes) to prevent spreadsheet formula injection attacks.

---

## Step 2: Compliance Settings & Auditor Alignment

### 2.1 Audit Control Mapping
Map Scribex trace evidence directly to your compliance framework controls:
- **SOC2 Type II (CC8.1):** Authorizes that all production code changes map to approved tickets and that specifications were updated concurrently.
- **ISO/IEC 27001 (A.12.1.2):** Verifies change management procedures and documentation synchronization.

### 2.2 Strict Jira Verification Flags
To enforce strict compliance policies where unverified tickets or missing keys cause an error:
- **`-verify-jira`**: Queries Jira REST API to verify ticket validity and status.
- **`-require-jira`**: Fails execution if Jira credentials or key are missing.
- **`-allow-no-key`**: Explicitly permits empty Jira keys only for internal chores.

---

## Step 3: CI/CD & PR Gate Settings

Configure your release or merge pipeline to automatically generate and archive signed evidence on every merged PR.

### 3.1 Post-Merge Audit Evidence Upload
Add to `.github/workflows/scribex-compliance.yml`:

```yaml
name: Generate SOC2 Trace Evidence
on:
  pull_request:
    types: [closed]
    branches: [main]

jobs:
  compliance-matrix:
    if: github.event.pull_request.merged == true
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
      - name: Generate Trace Matrix
        env:
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: |
          PR_NUM="${{ github.event.pull_request.number }}"
          BASE_SHA="${{ github.event.pull_request.base.sha }}"
          scribex trace -base "$BASE_SHA" -pr "$PR_NUM" -verify-jira -out "trace-pr-${PR_NUM}.csv"

      - name: Archive to Compliance S3 Vault
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AUDIT_S3_KEY }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AUDIT_S3_SECRET }}
        run: |
          aws s3 cp trace-pr-${{ github.event.pull_request.number }}.csv s3://compliance-vault/soc2-traces/
          aws s3 cp trace-pr-${{ github.event.pull_request.number }}.csv.json s3://compliance-vault/soc2-traces/
```

---

## Step 4: Scribex Dashboard Items

When monitored via `scribex serve`, compliance status feeds directly into the governance control plane:

### 4.1 Dashboard Overview Items
- **Trace Readiness Metric:** Percentage of pull requests with complete, verified Jira $\leftrightarrow$ PR $\leftrightarrow$ Doc traceability.
- **Unverified Tickets Counter:** Alerts security officers when code landed with unverified Jira keys.
- **Tamper-Evident Export:** Direct one-click export of verified evidence across all tracked repositories via `GET /api/v1/trace.csv`.
