# Sentinix AI: User Guide & Operational Operations

**Module:** `sentinix-ai`  
**Author:** Kolta Labs Security & Systems Engineering Group  
**Audience:** Security Engineers, Developers, SOC Analysts, System Administrators  
**Scope:** CLI command reference, baseline operations, rule pack authoring, web dashboard workflows, and reporting.

---

## 1. CLI Reference & Local Workflows

The `sentinix` CLI is a single static binary providing local code scanning, attack surface analysis, rule pack signing, and offline benchmarking.

### Command Structure
```bash
sentinix scan [flags] [path]
sentinix surface [base-ref]
sentinix packs keygen|sign|verify [args]
sentinix bench <directory>
sentinix audit verify [-db <dsn>]
sentinix version
```

### 1.1 `sentinix scan`
Scans a target directory using all available engines, applies baseline exclusions, and checks findings against the gate threshold.

```bash
# Scan current directory with default high-severity gate
sentinix scan .

# Scan specific path and output SARIF
sentinix scan -fail-on medium -sarif findings.sarif /path/to/project

# Run in report-only mode (prints issues, never exits with code 1)
sentinix scan -report-only .
```

#### Flags Reference

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-baseline <file>` | `""` | Path to `.sentinix-baseline.json`. Findings present in the baseline are ignored. |
| `-update-baseline` | `false` | Writes all detected findings to the `-baseline` file and exits `0`. |
| `-fail-on <level>` | `high` | Severity threshold to trigger failure exit code (`info`, `low`, `medium`, `high`, `critical`). |
| `-report-only` | `false` | Prints findings but always exits `0` (unless an internal engine crashes). |
| `-sarif <file>` | `""` | Exports findings formatted in standard SARIF v2.1.0. |
| `-packs <dir>` | `""` | Directory containing additional signed rule pack `.json` files. |
| `-trust <file>` | `""` | Path to JSON file containing trusted Ed25519 public keys for rule packs. |
| `-osv <url>` | `""` | OSV vulnerability API endpoint (e.g. `https://api.osv.dev`). When omitted, dependency network matching is skipped. |
| `-server <url>` | `""` | Address of central `sentinixd` server to upload scan results. |
| `-project <id>` | `""` | Project UUID on the central server (required with `-server`). |
| `-token-env <var>`| `SENTINIX_TOKEN` | Environment variable holding the project CI authorization token. |

#### CLI Exit Codes
* `0`: Scan passed. No new findings at or above the `-fail-on` gate threshold.
* `1`: Scan failed. At least one finding detected at or above the threshold.
* `2`: Usage error or invalid command-line configuration.
* `3`: Engine failure. An underlying scanner crashed or could not complete. **Sentinix fails closed.**

---

### 1.2 Managing Baselines (`.sentinix-baseline.json`)

When introducing Sentinix to an existing repository, pre-existing issues should not block active development. Baselines record current technical debt so developers are only gated on **new** vulnerabilities.

#### Step 1: Establish the Initial Baseline
From the repository root on your default branch:
```bash
sentinix scan -baseline .sentinix-baseline.json -update-baseline .
```
Sentinix analyzes the entire codebase, calculates content-based fingerprints for all issues, and writes `.sentinix-baseline.json`.

#### Step 2: Commit Baseline into Git
```bash
git add .sentinix-baseline.json
git commit -m "chore(security): establish Sentinix baseline"
git push origin main
```

#### Step 3: Run Future Scans Against Baseline
```bash
sentinix scan -baseline .sentinix-baseline.json -fail-on high .
```
Only vulnerabilities introduced *after* the baseline was established will trigger warnings or non-zero exit codes. Because fingerprints are calculated using content evidence rather than line numbers, normal code editing and line shifts will not invalidate existing baseline entries.

---

### 1.3 `sentinix surface` (Attack Surface Differential)
Analyzes `git diff` between your branch and the base ref to compute what endpoints, permissions, or dependencies were added or altered.

```bash
# Compare current HEAD against origin/main
sentinix surface origin/main

# Compare against a specific release tag
sentinix surface v2.4.0
```

**Example Output:**
```
[surface] +endpoint POST /api/v1/auth/reset-password (pkg/api/auth.go:45)
[surface] +permission android.permission.ACCESS_FINE_LOCATION (AndroidManifest.xml:14)
[surface] +dependency com.squareup.okhttp3:okhttp:4.12.0 (gradle/libs.versions.toml:18)
3 attack-surface change(s) detected in this branch.
```

---

## 2. Rule Packs & Custom Policy Authoring

Sentinix includes built-in detection packs across **10 technology stacks** comprising **135 enterprise rules**, plus full support for custom organization rule packs, per-rule enforcement configuration, and team-level custom rules.

### 2.1 Built-in Detection Scope (10 Stacks, 135 Rules)

Each pack runs via deterministic, zero-dependency RE2 regex patterns at sub-millisecond speeds:

| Stack Pack | Rules | Detection Trigger | Documentation |
| :--- | :--- | :--- | :--- |
| **Android** | 60 | `AndroidManifest.xml`, `com.android.*` Gradle plugin | [Android Pack Guide](packs/01_ANDROID_PACK.md) |
| **Kotlin Multiplatform (KMP)** | 8 | `commonMain/` folder, `multiplatform` plugin | [KMP Pack Guide](packs/02_KMP_PACK.md) |
| **Kotlin Backend (Spring/Ktor)** | 10 | `*.kt` with Spring/Ktor dependencies | [Kotlin Backend Guide](packs/03_KOTLIN_BACKEND_PACK.md) |
| **iOS & Swift** | 8 | `*.swift`, `Info.plist`, `Podfile`, `Package.swift` | [iOS Swift Guide](packs/04_IOS_SWIFT_PACK.md) |
| **Go** | 8 | `*.go`, `go.mod` | [Go Guide](packs/05_GO_PACK.md) |
| **.NET & C#** | 8 | `*.cs`, `*.csproj`, `appsettings*.json` | [.NET Guide](packs/06_DOTNET_PACK.md) |
| **PHP** | 8 | `*.php`, `composer.json` | [PHP Guide](packs/07_PHP_PACK.md) |
| **Rust** | 7 | `*.rs`, `Cargo.toml` | [Rust Guide](packs/08_RUST_PACK.md) |
| **Python** | 9 | `*.py`, `requirements.txt`, `pyproject.toml` | [Python Guide](packs/09_PYTHON_PACK.md) |
| **Node.js & React** | 9 | `*.js`, `*.jsx`, `*.ts`, `*.tsx`, `package.json` | [Node/React Guide](packs/10_NODE_REACT_PACK.md) |

For comprehensive catalogues of all individual rules, consult the [Packs Directory](packs/INDEX.md).

---

### 2.2 Per-Rule Enforcement Configuration (`.sentinix/policy.json`)

Maintainers can configure the enforcement level for every rule in any pack:

```json
{
  "rules": {
    "android/exported-component": "Block",
    "android/allow-backup": "Warn",
    "android/mock-location-allowed": "Notice",
    "go/command-injection": "Block",
    "node/insecure-cookie-flag": "Ignore"
  }
}
```

* **`Block`:** Fails the pull request or scan gate (exit code `1`).
* **`Warn`:** Posts a prominent warning on the PR without blocking merge.
* **`Notice`:** Recorded quietly in findings and audit log for visibility.
* **`Ignore`:** Completely suppresses rule execution.

Policies can also be configured via the REST API (`PUT /api/projects/:id/policy`) and managed in the Web Dashboard.

---

### 2.3 Team-Level In-Repo Custom Rules (`.sentinix/custom-rules.json`)

Development teams can commit custom security patterns directly within their repository:

```json
{
  "rules": [
    {
      "id": "team/prohibit-raw-http-client",
      "severity": "high",
      "cwe": "CWE-693",
      "message": "Direct OkHttpClient usage prohibited; use SecureInternalClient with mTLS",
      "remediation": "Replace with SecureInternalClient.sharedInstance",
      "files": ["*.kt", "*.java"],
      "pattern": "OkHttpClient\\.Builder\\(\\)"
    }
  ]
}
```

Sentinix scans custom rules alongside built-in packs with identical sub-millisecond evaluation and baseline support.

---

### 2.4 Authoring & Signing Cryptographic Rule Packs

Organizations distributing company-wide rule packs can sign them with Ed25519 keys:

```bash
# 1. Generate an Ed25519 key pair
sentinix packs keygen sec-team

# 2. Sign the rule pack (automatically runs embedded bad/good tests)
sentinix packs sign company-security-pack.json sec-team.key > company-security-pack.signed.json

# 3. Create trusted keys registry (trusted-keys.json)
echo '{"keys": ["'$(cat sec-team.pub)'"]}' > trusted-keys.json

# 4. Execute scan with signed pack
sentinix scan -packs ./custom-packs -trust trusted-keys.json .
```

---

## 3. Web Dashboard Operations

The `sentinixd` server provides an authenticated, hardened web interface accessible at `http://<host>:8080`.

```
┌────────────────────────────────────────────────────────────────────────┐
│ Sentinix    [Posture] [Findings] [Attack surface] [Scans] [Settings]   │
└────────────────────────────────────────────────────────────────────────┘
```

### 3.1 Posture Dashboard (`#/posture`)
Provides high-level health metrics across projects:
* **Severity Breakdown:** Open findings grouped by `critical`, `high`, `medium`, and `low`.
* **Fix Velocity:** Ratio of newly introduced findings versus remediated findings over a 90-day window.
* **Mean Time to Remediate (MTTR):** Average days from first detection to verified fix.
* **Oldest Criticals Table:** Identifies longstanding critical findings requiring immediate remediation.

### 3.2 Finding Lifecycle Management (`#/findings`)
Every vulnerability follows an auditable lifecycle:

```
[Detected] ──► [Open] ──► [Remediated] ──► [Fixed]
                 │
                 ├──► [Marked False Positive] (Requires Reviewer role)
                 │
                 └──► [Proposed Accept Risk] ──► [Peer Approval] ──► [Accepted]
                                                                        │
                                                                 (After TTL Expires)
                                                                        │
                                                                        ▼
                                                                   [Reopened]
```

* **AI Council Triage:** Click **"AI triage"** on any finding to dispatch the snippet to your configured LLMs. The dashboard displays the consensus decision, confidence rating, and dissenting model opinions.
* **Draft Patch:** Click **"Draft patch"** to generate a unified diff for the vulnerability.
* **Issue Ticketing:** Click **"Create Ticket"** to export the finding directly to Jira or Linear with an automated remediation due date.

### 3.3 Two-Person Approvals Queue (`#/approvals`)
When an engineer proposes accepting a risk or disabling a rule:
1. The action enters the **Approvals Queue** in a `pending` state.
2. The UI prevents the original author from approving their own request.
3. A second engineer with `reviewer` or `owner` privileges reviews the justification, confirms the expiration date, and clicks **Approve** or **Reject**.
4. All decisions are cryptographically written to the immutable **Audit Log**.

---

## 4. Reports & Exports

Sentinix generates multiple report formats via HTTP API or dashboard export:

```bash
# Fetch HTML report for project
curl -H "Authorization: Bearer <TOKEN>" \
  https://sentinix.corp.internal/api/projects/<PROJECT_ID>/report/html > report.html

# Fetch CycloneDX SBOM
curl -H "Authorization: Bearer <TOKEN>" \
  https://sentinix.corp.internal/api/projects/<PROJECT_ID>/report/sbom > sbom.json
```

| Format | Content-Type | Use Case |
| :--- | :--- | :--- |
| **`html`** | `text/html` | Print-ready compliance audit report. Includes executive summaries, cryptographic coverage statement, and OWASP Top 10 mapping. |
| **`sbom`** | `application/vnd.cyclonedx+json` | CycloneDX v1.5 standard Software Bill of Materials with mapped OSV vulnerability records. |
| **`sarif`** | `application/sarif+json` | Standard SARIF format for ingestion into GitHub Security, GitLab, or IDE plugins. |
| **`csv`** | `text/csv` | Spreadsheet export. All formula prefixes (`=`, `+`, `-`, `@`) are safely escaped to prevent CSV formula injection attacks. |
| **`json`** | `application/json` | Raw programmatic data export for enterprise SIEM/SOAR platforms. |

---

## 5. Hunt v2: Configurable AI Security Audit Operations

`sentinix hunt` executes declarative, benchmark-parity AI security audits targeting application entry points across mobile, backend, and iOS targets.

### 5.1 CLI Workflow

```bash
# Initialize project configuration
sentinix hunt config init

# Inspect effective configuration and layered provenance
sentinix hunt config show --explain

# Dry run: preview entry points and budget limits without making model calls
sentinix hunt --taskflow android-audit --dry-run .

# Run full audit or fast diff audit on changed entry points
sentinix hunt --diff .

# Review audit runs, metrics, and tool calls
sentinix hunt runs
sentinix hunt show <run-id>

# Community Pack Bundles
sentinix hunt bundle new custom-audit-pack.json
sentinix hunt bundle test custom-audit-pack.json
sentinix hunt bundle sign custom-audit-pack.json --key author.key > custom-audit-pack.signed.json
sentinix hunt bundle verify custom-audit-pack.signed.json --trust trusted.pub
```

### 5.2 Configuration Precedence & Governance

Audit configuration resolves through a strict hierarchy:
`CLI Flags > Organization Ceilings > Project Config (.sentinix/hunt.json) > Taskflow Defaults > Engine Defaults`

Any change that loosens safeguards (such as increasing budget ceilings or disabling vulnerability classes) triggers a requirement for two-person reviewer approval before enforcement.

