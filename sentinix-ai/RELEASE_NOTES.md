<div align="center">

<img src="logos/logo_sentinix.png" alt="Sentinix AI Logo" width="120" style="border-radius: 16px; margin-bottom: 12px;" />

# Sentinix AI v1.0.0 Release Notes

</div>

## 🌟 General Availability (GA) Release Summary

**Sentinix AI `v1.0.0`** is the initial general availability release establishing a deterministic, honest, and high-performance continuous security review and automated pull request governance engine.

Sentinix AI provides:
1. **10 Curated Technology Stacks & 135 Enterprise Rules**: Complete regression suites tested against 135 real code fixtures with zero false positives.
2. **Deterministic-First Core Engine**: Fast local repository scanning (<2s for full codebase evaluation) in Go with zero external model dependencies required for core gating.
3. **PR Gate Daemon (`sentinixd`)**: Integrated GitHub, GitLab, and Bitbucket webhook handlers with automated PR comments, inline diff annotations, and commit status checks.
4. **Per-Rule Policy Governance**: Modular `.sentinix/policy.json` allowing fine-grained control (`Block`, `Warn`, `Notice`, `Ignore`) by rule ID, category, or CVSS severity.
5. **Progressive Adoption with Baseline Diffing**: Generate `.sentinix-baseline.json` to freeze legacy tech debt and ensure only *new* regressions block PRs.
6. **Industry Standard Reporting**: Native SARIF v2.1.0, JUnit XML, Markdown, and JSON export mapped to OWASP Top 10, OWASP MASVS v2.0, CWE, and NIST SP 800-53.

---

## 🛡️ Rule Packs Breakdown (135 Built-in Rules)

| Technology Stack | Rules Count | Key Vulnerability Coverage |
|---|---|---|
| **Android** | **60 Rules** | Keystore cryptography, Biometric auth gates, Tapjacking protection (`filterTouchesWhenObscured`), Certificate pinning, Cleartext HTTP prevention, Exported activities/services/receivers, Intent redirection, Deep link path traversal, Pending intent mutability, WebView XSS, JavaScript bridge isolation, Debuggable/backup flags. |
| **iOS / Swift** | **15 Rules** | Keychain accessibility (`kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`), ATS domain exceptions, LocalAuthentication biometrics, Core Data encryption, URL schemes validation, Pasteboard persistence. |
| **Go** | **10 Rules** | Path traversal (G304 / CWE-22), SQL injection (G201 / CWE-89), Interface binding to `0.0.0.0`, Unhandled errors (G104), Insecure TLS versions (<1.2). |
| **Python / Django / FastAPI** | **10 Rules** | Arbitrary execution (`eval`/`exec`), Raw ORM injection, CSRF exemptions (`@csrf_exempt`), Insecure deserialization (`pickle`, `yaml.load`), Plaintext debug secrets. |
| **Java / Spring Boot** | **10 Rules** | Spring SpEL expression injection, JPA/Hibernate concatenation, Actuator endpoints exposure, XML external entities (XXE), Java native object deserialization. |
| **Node.js / React** | **10 Rules** | React `dangerouslySetInnerHTML`, Command injection (`child_process.exec`), Express CORS wildcard with credentials, Prototype pollution, JWT `none` algorithm. |
| **Cloud & Terraform / IaC** | **5 Rules** | AWS S3 public bucket ACLs, Security Group ingress open to `0.0.0.0/0` (SSH 22 / RDP 3389), Wildcard IAM policies (`*`), RDS public accessibility. |
| **Kubernetes / Helm** | **5 Rules** | Pod privileged containers, Root user execution, HostPID/HostNetwork escapes, Default service accounts. |
| **Docker / Container** | **5 Rules** | Missing `USER` instruction, `latest` base image tags, Sensitive port exposure, Docker socket mounting, Secrets in `ENV`/`ARG`. |
| **CI/CD / GitHub Actions** | **5 Rules** | `pull_request_target` with code checkout, Unpinned 3rd-party actions, Inline script expression injection, Plaintext secrets in logs. |

---

## 📦 Distribution Packages & Verification

Pre-compiled standalone binaries and platform archives:

| Platform | Architecture | Archive Package | Binaries Included |
|---|---|---|---|
| **macOS** | Apple Silicon (`arm64`) | `sentinix-v1.0.0-darwin-arm64.tar.gz` | `sentinix`, `sentinixd` |
| **macOS** | Intel (`amd64`) | `sentinix-v1.0.0-darwin-amd64.tar.gz` | `sentinix`, `sentinixd` |
| **Linux** | 64-bit (`amd64`) | `sentinix-v1.0.0-linux-amd64.tar.gz` | `sentinix`, `sentinixd` |
| **Linux** | ARM64 (`arm64`) | `sentinix-v1.0.0-linux-arm64.tar.gz` | `sentinix`, `sentinixd` |
| **Windows**| 64-bit (`amd64`) | `sentinix-v1.0.0-windows-amd64.zip` | `sentinix.exe`, `sentinixd.exe` |

Cryptographic SHA256 checksums are published in [`dist/SHA256SUMS`](../dist/SHA256SUMS).

---

## ⚡ Quick Install

### Homebrew (macOS & Linux)
```bash
brew install keltstone/tap/sentinix
```

### Universal One-Line Installer
```bash
curl -fsSL https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.sh | bash -s -- --sentinix
```

### Build from Source
```bash
git clone https://github.com/Keltstone/DialexAI.git && cd DialexAI/sentinix-ai
go build -ldflags="-s -w -X main.version=1.0.0" -o sentinix ./cmd/sentinix
go build -ldflags="-s -w -X main.version=1.0.0" -o sentinixd ./cmd/sentinixd
```
