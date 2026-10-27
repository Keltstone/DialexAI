# Traceability Evidence File (`scribex trace`)

The `scribex trace` command generates an immutable CSV evidence record and cryptographic SHA-256 sidecar linking Jira tickets, pull requests, public API symbol changes, anchored repository documentation, and mapped Confluence documentation pages at specific Git commit SHAs.

This artifact is designed for compliance regimes including SOC 2 Type II, ISO/IEC 27001, and FedRAMP change management audits.

---

## Command Usage

```bash
scribex trace -base <ref> [-key <JIRA-KEY>] [-pr <N>] [-out <file.csv>] [-verify-jira] [-require-jira] [-allow-no-key]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-base <ref>` | *(required)* | Base Git reference (branch, tag, or commit) to diff against |
| `-key <KEY>` | From branch | Jira ticket key (e.g. `PROJ-1234`). Format must match `^[A-Z][A-Z0-9_]{1,10}-[0-9]{1,9}$` |
| `-pr <N>` | `""` | Pull request number |
| `-out <file.csv>` | `stdout` | Output CSV path. When specified, atomic writes create `<file.csv>` and sidecar `<file.csv>.json` |
| `-verify-jira` | `false` | Explicitly verifies Jira ticket existence against Atlassian REST API |
| `-require-jira` | `false` | Requires valid Jira credentials and ticket; exits with code 2 if credentials are missing |
| `-allow-no-key` | `false` | Permits generating evidence with an empty Jira ticket key |

---

## Schema and Columns

Header format (`schema: scribex-trace/1`):

```csv
schema,scribex_version,base_sha,head_sha,head_commit_time_utc,jira,jira_verified,pr,file,symbol,change,repo_docs,confluence_pages
```

1. **`schema`**: Always `scribex-trace/1`.
2. **`scribex_version`**: Scribex release version generating the evidence.
3. **`base_sha`**: Full 40-character hexadecimal commit SHA of the base reference.
4. **`head_sha`**: Full 40-character hexadecimal commit SHA of `HEAD`.
5. **`head_commit_time_utc`**: ISO 8601/RFC 3339 UTC timestamp of the `HEAD` commit (e.g. `2026-10-27T12:00:00Z`).
6. **`jira`**: Validated Jira ticket key or empty if `-allow-no-key` was supplied.
7. **`jira_verified`**: `true` if verified against Jira API; `false` otherwise.
8. **`pr`**: Pull request number or empty string.
9. **`file`**: Relative repository path to source code file containing changed symbol.
10. **`symbol`**: Qualified symbol identifier (e.g. `AuthService.Authenticate`).
11. **`change`**: One of `added`, `modified`, `deleted`, or `none`.
12. **`repo_docs`**: Semicolon-delimited, sorted list of anchored documentation files referencing the symbol.
13. **`confluence_pages`**: Semicolon-delimited, sorted list of mapped Confluence page IDs.

### Zero Deltas

When `HEAD` contains no public contract changes relative to `base`, Scribex writes exactly one data row with `change=none` and empty `file` and `symbol` columns:

```csv
scribex-trace/1,1.0.0,base_sha,head_sha,2026-10-27T12:00:00Z,PROJ-1234,true,42,,,none,,
```

---

## Sidecar File (`<file.csv>.json`)

When invoked with `-out f.csv`, Scribex simultaneously and atomically writes `f.csv.json` and prints the SHA-256 digest on stdout:

```json
{
  "schema": "scribex-trace-meta/1",
  "status": "complete",
  "rows": 1,
  "csv_sha256": "4a72d7335272a8c88f11467be6477e1df230b7e2ccab0a26685de060a4e7ab56",
  "generated_at_utc": "2026-10-27T12:00:00Z",
  "scribex_version": "1.0.0",
  "base_sha": "a1b2c3d4...",
  "head_sha": "e5f6a7b8...",
  "jira_verified": true,
  "unknown_files": []
}
```

---

## Formula Neutralisation & Security

To prevent CSV formula injection (OWASP CSV Injection / Excel Macro Injection):
- Every cell value is passed through `trace.Cell()`.
- If a string begins with `=`, `+`, `-`, `@`, `\t`, or `\r` (including leading whitespace variations), an apostrophe (`'`) is prepended to force spreadsheet applications to treat the field as literal text.
- Newlines (`\r\n` and `\n`) are normalized to spaces.
- Writes are atomic (`fsx.WriteAtomic`); symlinks and directory targets are refused.

---

## What the Evidence Proves (and Does NOT Prove)

### What it proves
- **Structural Linkage**: Mechanically links the Git commit SHAs, PR number, Jira ticket key, and public symbol changes.
- **Documentation Coverage**: Records every Markdown document and Confluence page referencing the modified contract symbols at that exact commit.
- **Commit Integrity**: Cryptographically binds the base and head commits, author commit timestamp, and CSV contents via SHA-256.

### What it does NOT prove
- **Semantic Truth**: It does not certify that the English prose in documentation is accurate or up to date.
- **Ticket Validity/Approval**: It does not prove that the Jira ticket is the correct ticket for this change, or that change advisory board (CAB) or management approval was granted.
- **Human Review**: It does not prove human code review occurred (review approval is evidenced by GitHub branch protection / review logs).

---

## Verifying Integrity

To verify the evidence in an audit pack:

```bash
# Verify the CSV checksum against the sidecar metadata
computed_sha=$(sha256sum f.csv | awk '{print $1}')
recorded_sha=$(jq -r .csv_sha256 f.csv.json)

if [ "$computed_sha" = "$recorded_sha" ]; then
  echo "Integrity OK: $computed_sha"
else
  echo "Integrity Failure: mismatch detected" >&2
  exit 1
fi
```

### Retention
Retention of the CSV and JSON sidecar files is the customer's responsibility. Organizations should archive these files into their compliance evidence repository (e.g. S3 bucket with Object Lock or Jira/ServiceNow release ticket attachment) at release cut time.
