# Compliance & Audit Traceability Evidence Guide (SOC2 / ISO 27001)

**Module:** `scribex-ai`  
**Platform:** Enterprise Compliance Vaults (Vanta, Drata, Secureframe, AWS S3 / GCS Audit Buckets)  
**Audience:** Compliance Auditors, Information Security Officers (CISO), DevSecOps Leads, Engineering Directors  
**Scope:** Continuous traceability matrix generation, formula-injection defense, cryptographic verification sidecars, and automated auditor evidence export.

---

## 1. The Compliance Imperative: Traceability & Change Management

During annual **SOC2 Type II**, **ISO/IEC 27001**, **HIPAA**, and **FedRAMP** audits, the change management control (e.g., CC8.1, ISO Control A.12.1.2) requires proof that:

1. Every production code change corresponds directly to an authorized business requirement (Jira Ticket).
2. The pull request was reviewed and passed automated quality and security gates.
3. System documentation, architecture diagrams, and interface contracts were updated concurrently with code changes.
4. An immutable, tamper-evident audit trail links:
   $$\text{Jira Ticket} \longleftrightarrow \text{PR Number} \longleftrightarrow \text{AST Symbol Delta} \longleftrightarrow \text{In-Repo Docs} \longleftrightarrow \text{Confluence Page}$$

Traditionally, engineering teams spend weeks manually piecing together spreadsheets, screenshots, and git logs before an audit.

**Scribex AI** automates this entire audit chain deterministically using `scribex trace`.

---

## 2. Generating the Traceability Matrix (`scribex trace`)

The `scribex trace` command computes the full bidirectional matrix between tickets, pull requests, AST symbol deltas, in-repo documentation anchors, and external Confluence pages.

### 2.1 Basic CLI Syntax

```bash
scribex trace -base origin/main -pr 482 -key PROJ-1234 -out evidence_pr482.csv
```

### 2.2 CLI Flags & Options

| Flag | Description | Mandatory? |
| :--- | :--- | :---: |
| **`-base <ref>`** | Git reference to diff against (e.g., `origin/main`). | Yes |
| **`-pr <N>`** | Pull Request number. | Recommended |
| **`-key <K>`** | Jira issue key (auto-detected from branch if omitted). | Recommended |
| **`-out <file.csv>`** | Output CSV file path (defaults to stdout). | Recommended |
| **`-verify-jira`** | Calls Jira API to verify ticket status and existence. | Optional |
| **`-require-jira`** | Fails execution if Jira credentials or key are missing. | Optional |
| **`-allow-no-key`** | Allows empty Jira key in trace evidence for chores. | Optional |

---

## 3. The `scribex-trace/1` CSV Schema

When writing to `-out evidence.csv`, Scribex formats records under the versioned `scribex-trace/1` schema:

| Column | Type | Description | Example |
| :--- | :--- | :--- | :--- |
| `schema` | String | Fixed schema identifier. | `scribex-trace/1` |
| `scribex_version` | String | Scribex binary version that produced evidence. | `1.0.0` |
| `base_sha` | Hex SHA | Git commit SHA of the base branch. | `4b825dc642cb6eb9a060e54bf8d69288fbee4904` |
| `head_sha` | Hex SHA | Git commit SHA of the current head commit. | `a3f890b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7` |
| `head_commit_time_utc`| Timestamp | UTC commit timestamp of head commit. | `2026-10-25T14:32:00Z` |
| `jira` | String | Associated Jira issue key. | `PROJ-1234` |
| `jira_verified` | Boolean | `true` if ticket was verified against Jira API. | `true` |
| `pr` | Integer | Pull Request number. | `482` |
| `file` | String | Source code file path where change occurred. | `pkg/auth/jwt.go` |
| `symbol` | String | Exported code symbol name. | `VerifyToken` |
| `change` | Enum | Nature of contract delta (`added`, `modified`, `deleted`). | `modified` |
| `repo_docs` | String | Semicolon-delimited list of in-repo doc anchors. | `docs/api/auth.md;README.md` |
| `confluence_pages` | String | Semicolon-delimited list of Confluence page IDs. | `109823485;204859182` |

---

## 4. Cryptographic Security & Defense-in-Depth

### 4.1 Formula-Injection Neutralization (`CSV / Excel Injection`)
Malicious contributors can theoretically submit pull requests containing symbol names or git commit metadata starting with `=cmd|' /C calc'!A0` or `@SUM(...)` to trigger remote code execution when an auditor opens the CSV in Microsoft Excel or Google Sheets.

Scribex implements strict **formula-injection neutralization** on every single CSV cell:
- All newlines (`\r\n`, `\n`) are stripped and replaced with spaces.
- If the first character of any cell begins with `=`, `+`, `-`, `@`, `\t`, or `\r`, Scribex prepends a single apostrophe (`'`):
  ```go
  if trimmed[0] == '=' || trimmed[0] == '+' || trimmed[0] == '-' || trimmed[0] == '@' {
      return "'" + s
  }
  ```
- This forces spreadsheet software to treat the data strictly as literal text.

---

### 4.2 Cryptographic Verification Sidecar (`.csv.json`)
Whenever Scribex writes an evidence CSV to disk (e.g., `-out evidence.csv`), it atomically writes a cryptographic companion metadata file: `evidence.csv.json`.

```json
{
  "schema": "scribex-trace/1",
  "status": "ok",
  "rows": 4,
  "csv_sha256": "8f3b20c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
  "generated_at_utc": "2026-10-28T18:15:30Z",
  "scribex_version": "1.0.0",
  "base_sha": "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
  "head_sha": "a3f890b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7",
  "jira_verified": true,
  "unknown_files": []
}
```

Auditors and automated ingestion pipelines verify that:
```bash
echo "$EXPECTED_SHA  evidence.csv" | sha256sum -c -
```
This guarantees that the evidence spreadsheet has not been tampered with post-generation.

---

## 5. Automated CI/CD Evidence Upload Pipeline

Integrate continuous audit evidence collection into your release or PR pipeline:

```yaml
name: Generate SOC2 Audit Evidence
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
        run: |
          go build -trimpath -ldflags "-s -w" -o /usr/local/bin/scribex ./cmd/scribex

      - name: Generate Trace Matrix
        env:
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: |
          PR_NUM="${{ github.event.pull_request.number }}"
          BASE_SHA="${{ github.event.pull_request.base.sha }}"
          OUT_FILE="trace-pr-${PR_NUM}.csv"

          scribex trace \
            -base "$BASE_SHA" \
            -pr "$PR_NUM" \
            -verify-jira \
            -out "$OUT_FILE"

      - name: Archive to Compliance Audit Vault (S3 / GCS)
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AUDIT_S3_KEY }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AUDIT_S3_SECRET }}
        run: |
          # Upload both CSV and JSON checksum sidecar to WORM (Write Once, Read Many) bucket
          aws s3 cp trace-pr-${{ github.event.pull_request.number }}.csv s3://company-soc2-vault/pr-traces/
          aws s3 cp trace-pr-${{ github.event.pull_request.number }}.csv.json s3://company-soc2-vault/pr-traces/
```

---

## 6. Integration with Automated Compliance Platforms (Vanta / Drata)

Modern compliance platforms (Vanta, Drata, Secureframe) provide API endpoints for automated evidence ingestion:

1. **Vanta Custom Evidence API:** Post the generated CSV and JSON sidecar directly to the Vanta Documents API under the *Change Management* requirement.
2. **Drata Evidence Upload:** Trigger a webhook uploading the signed artifact to Drata's *Automated Testing & Documentation* control.
3. **Auditor Portal Access:** When the external auditor arrives, export the combined CSV directly from your S3 audit bucket or run a consolidated report across all PRs merged in the audit window.
