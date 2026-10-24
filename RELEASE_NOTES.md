# DialexAI Ecosystem v1.0.0 Release Notes

The **DialexAI Ecosystem** implements an **Independent Software Release Model**, enabling each tool to ship upon reaching production maturity while sharing core deliberation, cryptographic, and packaging standards.

### 🚀 Current Software Releases
- 🟢 **Sentinix AI (`v1.0.0`)**: **Production GA Release — Publishing Now** ([`sentinix-ai/RELEASE_NOTES.md`](sentinix-ai/RELEASE_NOTES.md) · [`sentinix-ai/CHANGELOG.md`](sentinix-ai/CHANGELOG.md))
- 🟢 **Socratix Engine (`v1.0.0`)**: Deliberation & Consensus Daemon ([`socratix-engine/CHANGELOG.md`](socratix-engine/CHANGELOG.md))
- 🟡 **Dialex AI (`v1.0.0`)**: Desktop & Mobile Deliberation Platform ([`dialex-ai/CHANGELOG.md`](dialex-ai/CHANGELOG.md))
- 🔵 **Artix AI (`v0.11.0-enterprise`)**: Autonomous Coding & Stakeholder Council ([`artix-ai/RELEASE_NOTES.md`](artix-ai/RELEASE_NOTES.md) · [`artix-ai/CHANGELOG.md`](artix-ai/CHANGELOG.md))
- 🔵 **Kritix AI (`v0.3.0-enterprise`)**: Autonomous QA Testing Studio & Fuzzing ([`kritix-ai/CHANGELOG.md`](kritix-ai/CHANGELOG.md))

---

## 🚀 Key Product Highlights

### 1. Sentinix AI (v1.0.0)
- **Continuous Security Review & PR Gate Daemon**:
  - High-performance Go CLI (`sentinix`) and daemon (`sentinixd`) for repository scanning, security linting, and automated pull request governance.
  - Multi-platform webhook integrations (GitHub, GitLab, Bitbucket) with automated PR comments, inline diff annotations, and commit status checks.
- **10 Curated Technology Stacks & 135 Enterprise Rules**:
  - 100% fixture-tested regression suites across 10 technology ecosystems:
    - **Android** (60 rules): Keystore, biometric auth, tapjacking, certificate pinning, WebView XSS, exported components, deep link validation.
    - **iOS / Swift** (15 rules): Keychain `kSecAttrAccessible`, ATS bypasses, biometric local authentication, Core Data encryption, URL scheme validation.
    - **Go** (10 rules): Path traversal, SQL injection, broad interface binding, unhandled errors, weak TLS.
    - **Python / Django / FastAPI** (10 rules): `eval`/`exec`, raw ORM SQL injection, Django CSRF exemptions, pickle deserialization, yaml unsafe load.
    - **Java / Spring Boot** (10 rules): Spring SpEL expression injection, JPA concatenation, Actuator exposure, XML external entities (XXE), unsafe deserialization.
    - **Node.js / React** (10 rules): `dangerouslySetInnerHTML`, `eval`/`child_process.exec`, Express CORS `*` with credentials, prototype pollution, JWT `none` algorithm.
    - **Cloud & Terraform / IaC** (5 rules): AWS S3 public bucket ACLs, Security Group ingress `0.0.0.0/0` SSH/RDP, IAM wildcard policies, RDS public accessibility.
    - **Kubernetes / Helm** (5 rules): Pod privileged mode, root user execution, hostPID/hostNetwork escape, default service accounts.
    - **Docker / Container** (5 rules): Missing USER directive, latest base tags, exposed sensitive ports, Docker socket mounting, secrets in `ENV`/`ARG`.
    - **CI/CD / GitHub Actions** (5 rules): `pull_request_target` with checkout, unpinned 3rd-party actions, inline script injection, plaintext tokens.
- **Rule Customization & Policy Governance**:
  - Modular policy engine (`policy.json`) supporting max severity gates, allowed CVSS thresholds, fail-on-critical, and mandatory manual review policies.
  - Baseline diffing (`--baseline .sentinix-baseline.json`) enabling zero-noise progressive adoption on legacy codebases.
- **Reporting & Standards Export**:
  - Native export to SARIF v2.1.0, JUnit XML, Markdown PR summaries, and machine-readable JSON.
  - Standards mapping to OWASP Top 10, CWE, and NIST SP 800-53.

### 2. Dialex AI (v1.0.0)
- **Hegelian Multi-Agent Adversarial Deliberation**:
  - Pits up to 6 competing frontier AI models (Claude, GPT, Gemini, Grok, DeepSeek, Mistral, Ollama) in multi-round structured dialectic debates.
  - Moderator frames topic agendas, leads round-by-round syntheses, and generates consensus verdicts.
- **No Extra Per-Token Billing via Local CLIs or Ollama**:
  - Direct execution through local authenticated developer subshells (`claude`, `codex`, `antigravity`/`agy`, `ollama`) eliminating metered API markups.
- **Persona Registry & Style Modifiers**:
  - 10 Universal General Debate Archetypes (*Facilitator*, *Devil's Advocate*, *Optimist*, *Pragmatist*, *Contrarian*, *Expert*, *Risk Analyst*, *Ethicist*, *Historian*, *Futurist*).
  - 40+ Domain-Specific Personas spanning Distributed Systems, Cybersecurity, AI Infrastructure, FinTech, and Legal Compliance.
- **Generative Deliverables**:
  - One-tap synthesis: Architecture Decision Records (ADR-042), Weighted Decision Matrices, Executive Memorandums, and Code Diffs.
- **Multiplatform Clients**:
  - Compose Multiplatform Desktop (macOS DMG, Windows MSI/EXE, Linux DEB/RPM).
  - Android Mobile application with radial stadium arena and voice dilemma intake.

### 3. Artix AI (v1.0.0)
- **Autonomous Software Engineering & Spec-to-Patch Deliberation**:
  - Multi-agent Stakeholder Council exploring competing architectural paths and trade-offs before code modifications.
  - Interactive Engineering Lab with diff viewer, patch synthesizer, and AST verification.
- **CLI & Background Daemon**:
  - High-performance Go CLI (`artix`) and daemon (`artixd`) with `plan`, `steer`, `persona`, and `replay` workflows.
  - Integration with Git, Forge APIs, LSP servers, and Model Context Protocol (MCP) toolchains.

### 4. Kritix AI (v1.0.0)
- **Autonomous QA Testing Studio & Cockpit**:
  - Visual driver and semantic DOM accessibility exploration.
  - Dynamic invariant checking and synthetic crash reproduction.
- **OWASP DAST & Security Auditing**:
  - Automated security fuzzing across SQL injection, Cross-Site Scripting (XSS), and PII leak detection boundaries.
  - Real-time token compression and ROI metrics calculator.
- **Enterprise Verification CLI**:
  - Standalone Go CLI (`kritix`) with headless test execution, model configuration, and truth audits.

### 5. Socratix Engine (v1.0.0)
- **Multi-Agent Consensus & Credence Engine**:
  - Standalone Go daemon (`socratix`) serving high-throughput SSE APIs on port 7890.
  - Mathematical consensus detection, tension resolution, and Bayesian credence tracking across turn state machines.
  - SQLite deliberation graph persistence and encrypted credential vault.
- **Benchmark Suite**:
  - `socratixbench` CLI tool for measuring convergence, stance independence, and rubric scoring across model matrices.

---

## 📦 Release Distribution Matrix

| Software | Variant | macOS (`.dmg` / `.tar.gz`) | Windows (`.msi` / `.exe` / `.zip`) | Linux (`.deb` / `.rpm` / `.tar.gz`) | Homebrew Target |
|---|---|---|---|---|---|
| **Dialex AI** | With Engine | `DialexAI-1.0.0-with-engine.dmg` | `DialexAI-1.0.0-with-engine.msi`<br>`DialexAI-1.0.0-with-engine.exe` | `DialexAI-1.0.0-with-engine.deb`<br>`DialexAI-1.0.0-with-engine.rpm` | `brew install --cask dialex-ai-bundled` |
| **Dialex AI** | Without Engine | `DialexAI-1.0.0-without-engine.dmg` | `DialexAI-1.0.0-without-engine.msi`<br>`DialexAI-1.0.0-without-engine.exe` | `DialexAI-1.0.0-without-engine.deb`<br>`DialexAI-1.0.0-without-engine.rpm` | `brew install --cask dialex-ai` |
| **Artix AI** | With Engine | `ArtixAI-1.0.0-with-engine.dmg` | `ArtixAI-1.0.0-with-engine.msi`<br>`ArtixAI-1.0.0-with-engine.exe` | `ArtixAI-1.0.0-with-engine.deb`<br>`ArtixAI-1.0.0-with-engine.rpm` | `brew install --cask artix-ai-bundled` |
| **Artix AI** | Without Engine | `ArtixAI-1.0.0-without-engine.dmg` | `ArtixAI-1.0.0-without-engine.msi`<br>`ArtixAI-1.0.0-without-engine.exe` | `ArtixAI-1.0.0-without-engine.deb`<br>`ArtixAI-1.0.0-without-engine.rpm` | `brew install --cask artix-ai` |
| **Artix AI CLI** | Standalone CLI | `artix-v1.0.0-darwin-*.tar.gz` | `artix-v1.0.0-windows-amd64.zip` | `artix-v1.0.0-linux-*.tar.gz` | `brew install artix` (`--with-engine`) |
| **Kritix AI** | With Engine | `KritixAI-1.0.0-with-engine.dmg` | `KritixAI-1.0.0-with-engine.msi`<br>`KritixAI-1.0.0-with-engine.exe` | `KritixAI-1.0.0-with-engine.deb`<br>`KritixAI-1.0.0-with-engine.rpm` | `brew install --cask kritix-ai-bundled` |
| **Kritix AI** | Without Engine | `KritixAI-1.0.0-without-engine.dmg` | `KritixAI-1.0.0-without-engine.msi`<br>`KritixAI-1.0.0-without-engine.exe` | `KritixAI-1.0.0-without-engine.deb`<br>`KritixAI-1.0.0-without-engine.rpm` | `brew install --cask kritix-ai` |
| **Kritix AI CLI** | Standalone CLI | `kritix-v1.0.0-darwin-*.tar.gz` | `kritix-v1.0.0-windows-amd64.zip` | `kritix-v1.0.0-linux-*.tar.gz` | `brew install kritix` (`--with-engine`) |
| **Socratix Engine** | Daemon & Bench | `socratix-v1.0.0-darwin-*.tar.gz` | `socratix-v1.0.0-windows-amd64.zip` | `socratix-v1.0.0-linux-*.tar.gz` | `brew install socratix` |
| **Sentinix AI** | Security CLI & Server | `sentinix-v1.0.0-darwin-*.tar.gz` | `sentinix-v1.0.0-windows-amd64.zip` | `sentinix-v1.0.0-linux-*.tar.gz` | `brew install sentinix` |

---

## 🛠️ Verification & Build Commands

On a clean clone of the repository:

```bash
# 1. Verify Go builds across all modules
go build ./socratix-engine/...
go build ./artix-ai/...
go build ./kritix-ai/...
go build ./sentinix-ai/...

# 2. Run unit and integration tests
go test -count=1 ./socratix-engine/...
go test -count=1 ./artix-ai/...
go test -count=1 ./kritix-ai/...
go test -count=1 ./sentinix-ai/...

# 3. Verify all 135 security rule pack fixtures
go test -v ./sentinix-ai/pkg/packs/...

# 4. Verify test retention gate
./scripts/check-test-retention.sh

# 5. Build full CLI binaries & cross-compilation packages
./scripts/build-dist.sh --cli --cross-compile --app all

# 6. Verify generated SHA256 checksums
cd dist && sha256sum -c SHA256SUMS
```

---

## 📜 Cryptographic Trust Root & Signatures

All release binaries, archives, and tags are cryptographically signed:
- **Signer Identity / Principal**: `releases@dialex.ai`
- **Key Type**: `ED25519 (SSH format)`
- **Published Checksums**: `dist/SHA256SUMS`
- **Official Repository**: `https://github.com/Keltstone/DialexAI`
