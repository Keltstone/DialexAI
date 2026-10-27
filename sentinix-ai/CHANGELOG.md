# <img src="logos/logo_sentinix.png" width="32" height="32" style="vertical-align:middle; margin-right:8px;" /> Sentinix AI Changelog

All notable changes to **Sentinix AI** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Conventional Commits](https://www.conventionalcommits.org/).

---

## [1.0.0] - 2026-10-25

### 🌟 Initial Production GA Release
Initial general availability release of **Sentinix AI**, the continuous security review, pull request gate daemon, and vulnerability posture engine designed for modern engineering teams.

### 🛡️ 10 Technology Stacks & 135 Enterprise Rules
100% fixture-tested regression suites across 10 technology ecosystems:
- **Android Pack (60 Rules)**:
  - Keystore cryptographic hygiene, biometric authentication gates, tapjacking protection (`filterTouchesWhenObscured`).
  - Certificate pinning validation, cleartext traffic enforcement (`usesCleartextTraffic=false`).
  - Exported components, intent redirection, deep link path validation, pending intent mutability flags.
  - WebView XSS prevention, JavaScript interface isolation, debuggable flags rejection, backup enablement gates.
- **iOS / Swift Pack (15 Rules)**:
  - Keychain accessibility classes (`kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`).
  - App Transport Security (ATS) exceptions, biometric local authentication, Core Data file protection.
  - Custom URL schemes, pasteboard persistence, secure enclave integration.
- **Go Pack (10 Rules)**:
  - Path traversal (CWE-22 / G304), SQL injection (CWE-89 / G201), unhandled errors (G104).
  - Weak TLS configurations, broad network interface binding (`0.0.0.0`).
- **Python / Django / FastAPI Pack (10 Rules)**:
  - Arbitrary code execution (`eval`/`exec`), raw ORM query injection, Django CSRF exemptions.
  - Insecure deserialization (`pickle`, `yaml.load`), hardcoded debug flags in ASGI/WSGI.
- **Java / Spring Boot Pack (10 Rules)**:
  - Spring SpEL expression injection, JPA/Hibernate string concatenation, Actuator endpoint exposure.
  - XML External Entity (XXE) injection, Java native deserialization.
- **Node.js / React Pack (10 Rules)**:
  - Cross-Site Scripting via `dangerouslySetInnerHTML`, command injection (`child_process.exec`).
  - Overly permissive CORS with credentials, Prototype Pollution, JWT `none` algorithm vulnerability.
- **Cloud & Terraform / IaC Pack (5 Rules)**:
  - AWS S3 public bucket ACLs, Security Group ingress open to `0.0.0.0/0` on management ports (SSH/RDP).
  - Wildcard IAM policies (`*`), RDS public accessibility.
- **Kubernetes / Helm Pack (5 Rules)**:
  - Pod privileged mode, root user execution, hostPID/hostNetwork escape, default service accounts.
- **Docker / Container Pack (5 Rules)**:
  - Missing `USER` directive (running as root), unpinned `latest` base tags, sensitive ports exposed.
  - Docker daemon socket mounting, plain secrets in `ENV`/`ARG`.
- **CI/CD / GitHub Actions Pack (5 Rules)**:
  - `pull_request_target` checkout vulnerabilities, unpinned 3rd-party actions, inline script injection, plaintext tokens in logs.

### ⚙️ Core Engines & Architecture
- **Dual-Mode Execution**:
  - Deterministic standalone CLI (`sentinix`) with `scan`, `sign`, `surface`, `rules`, `policy`, `hook`, `bench`, and `audit` subcommands.
  - Background Review Daemon (`sentinixd`) providing HTTP/2 REST APIs, webhooks, worker queues, and an embedded web dashboard.
- **Git Host Webhook Automation**:
  - Webhook handlers for GitHub, GitLab, and Bitbucket.
  - Automated PR comments with concise markdown summaries, inline diff annotations, and commit status checks (`pending`, `success`, `failure`).
- **Policy Engine & Progressive Adoption**:
  - Modular policy file (`.sentinix/policy.json`) controlling per-rule actions (`Block`, `Warn`, `Notice`, `Ignore`).
  - Baseline diffing (`sentinix scan --baseline .sentinix-baseline.json`) enabling zero-noise progressive adoption on legacy repositories.
- **Reporting & Standards Export**:
  - Native export to SARIF v2.1.0 (for GitHub Code Scanning & IDE integrations), JUnit XML (for CI pipelines), Markdown, and JSON.
  - Mapping to OWASP Top 10 (2021), OWASP MASVS v2.0, CWE, and NIST SP 800-53 standards.
- **Dual SQLite / PostgreSQL Persistence**:
  - Zero-config local SQLite store (`sqlite:sentinix.db`) for workstation and small team use.
  - Enterprise PostgreSQL backend with connection pooling, transactional state management, and RBAC isolation.

### 🎯 Hunt v2: Configurable AI Security Audit Engine
- **Declarative YAML/JSON Taskflows**:
  - Pre-built taskflows (`android-audit`, `backend-audit`, `ios-audit`, `variant-analysis`, `triage-codeql`).
  - Safe Mustache templating without shell or network escape.
  - Cryptographic ed25519 bundle signing and verification via `sentinix hunt bundle sign/verify`.
- **Precedence Hierarchy & Safeguard Protection**:
  - Multi-tier configuration (CLI flags > Org ceilings > Project config > Taskflow > Defaults).
  - Two-person reviewer approval workflow before loosening audit ceilings or disabling security checks.
- **Multi-Model Consensus & Budget Accounting**:
  - Role-based model assignment (`classify`, `reason`, `verify`) with majority voting and dissenting evidence capture.
  - Atomic tracking of model calls, tokens, and USD costs with hard cutoff guarantees.
- **Bounded Tool Registry**:
  - 10 read-only code analysis tools with traversal sandboxing via Go `os.OpenRoot()`, byte caps, and full database audit logging.
- **Multi-Stage Validation Pipeline**:
  - Automatic mitigation filtering (`android:exported="false"`, signature permissions, auth middleware).
  - Dynamic PoC test verification in isolated Docker sandbox with compiler feedback loop.
  - Severity calibration and historical false-positive auto-demotion.
- **CodeQL & Semgrep Data-Flow Integration**:
  - SARIF trace parsing converting complex sinks to prompt context.
  - Deterministic Semgrep taint rule synthesis with fixture verification.
- **Diff Mode & Audit Lifecycle**:
  - Incremental scanning (`sentinix hunt --diff`) for sub-minute PR turnaround.
  - Persistent run history, resume, task-level rerun, and MCP tools (`sentinix_hunt_plan`, `sentinix_hunt_results`).

---

[1.0.0]: https://github.com/Keltstone/DialexAI/releases/tag/sentinix-v1.0.0
