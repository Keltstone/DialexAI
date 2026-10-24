# <img src="../../../logos/sentinix/logo_sentinix.png" width="32" height="32" style="vertical-align:middle; margin-right:8px;" /> Sentinix AI: Rule Packs Ecosystem & Stack Guide

**Module:** `sentinix-ai`  
**Total Rules:** 135 Built-in Enterprise Rules  
**Supported Stacks:** Android, Kotlin Multiplatform (KMP), Kotlin Backend (Spring/Ktor), iOS (Swift), Go, .NET (C#), PHP, Rust, Python (Django/Flask), Node.js / React  
**Engine Architecture:** Deterministic RE2-compatible pattern matcher, zero external runtime dependencies, sub-millisecond execution per file.

---

## 1. Ecosystem Overview

Sentinix AI provides **10 technology-specific security detection packs** containing 135 verified rules. These rules protect applications against OWASP Top 10, OWASP Mobile Application Security Verification Standard (MASVS), and enterprise financial compliance threats.

Each pack is self-contained, data-only JSON with no executable script overhead or network dependencies, making it 100% deterministic, audit-compliant, and safe to execute in air-gapped CI environments.

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Sentinix Rule Packs Engine                      │
├────────────────────────────────────────────────────────────────────────┤
│ Mobile & Multiplatform  │  Backend & Systems     │ Web & Cloud         │
│ • Android (60 rules)    │  • Go (8 rules)        │ • Node/React (9)    │
│ • KMP (8 rules)         │  • Rust (7 rules)      │ • Python/Django (9) │
│ • iOS / Swift (8 rules) │  • .NET / C# (8 rules) │ • PHP (8 rules)     │
│                         │  • Kotlin Backend (10) │                     │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Pack Catalogue & Index

| Pack ID | Stack Target | Rules | Detection Trigger | Documentation |
|---|---|---|---|---|
| `android` | Android (Java/Kotlin/XML) | **60** | `AndroidManifest.xml`, `build.gradle*` with `com.android` | [01_ANDROID_PACK.md](01_ANDROID_PACK.md) |
| `kmp` | Kotlin Multiplatform | **8** | `commonMain/` directory, `kotlin("multiplatform")` plugin | [02_KMP_PACK.md](02_KMP_PACK.md) |
| `kotlin-backend` | Spring Boot & Ktor | **10** | Kotlin files, Spring/Ktor dependencies | [03_KOTLIN_BACKEND_PACK.md](03_KOTLIN_BACKEND_PACK.md) |
| `ios-swift` | iOS & Swift | **8** | `*.swift`, `Info.plist`, `Podfile`, `Package.swift` | [04_IOS_SWIFT_PACK.md](04_IOS_SWIFT_PACK.md) |
| `go` | Go | **8** | `*.go`, `go.mod` | [05_GO_PACK.md](05_GO_PACK.md) |
| `dotnet` | .NET & C# | **8** | `*.cs`, `*.csproj`, `appsettings*.json` | [06_DOTNET_PACK.md](06_DOTNET_PACK.md) |
| `php` | PHP & Laravel/Symfony | **8** | `*.php`, `composer.json` | [07_PHP_PACK.md](07_PHP_PACK.md) |
| `rust` | Rust | **7** | `*.rs`, `Cargo.toml` | [08_RUST_PACK.md](08_RUST_PACK.md) |
| `python` | Python, Django, Flask | **9** | `*.py`, `requirements.txt`, `pyproject.toml` | [09_PYTHON_PACK.md](09_PYTHON_PACK.md) |
| `node-react` | Node.js & React | **9** | `*.js`, `*.jsx`, `*.ts`, `*.tsx`, `package.json` | [10_NODE_REACT_PACK.md](10_NODE_REACT_PACK.md) |

---

## 3. Policy Enforcement & Configuration

Every rule in every pack can be individually configured by repository maintainers without modifying the underlying pack or binary.

### 3.1 Repository Policy (`.sentinix/policy.json`)

To override rule severities or gating behavior for a repository, place a `.sentinix/policy.json` file in your repository root:

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

### 3.2 Enforcement Levels

* **`Block`:** Triggers a failing CI check and blocks merge if the repository operates under blocking gate mode. Exits CLI with code `1`.
* **`Warn`:** Emits a warning status and high-visibility comment on the pull request, but does not fail the gate.
* **`Notice`:** Recorded in the audit log and scan summary for visibility, but does not notify developers or impact build status.
* **`Ignore`:** Completely suppresses the rule from running on this repository.

### 3.3 Dynamic Management via REST API

Policies can also be configured dynamically via the Sentinix server:

```bash
# Get project policy
curl -H "Authorization: Bearer $SENTINIX_TOKEN" \
  https://sentinix.corp.internal/api/projects/:id/policy

# Update rule policy (Requires Reviewer or Admin role)
curl -X PUT -H "Authorization: Bearer $SENTINIX_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"rules": {"android/biometric-weak-crypto": "Block"}}' \
  https://sentinix.corp.internal/api/projects/:id/policy
```

---

## 4. Custom Team Security Rules

In addition to the 135 built-in rules, engineering teams can author and commit custom rules tailored to their proprietary APIs, internal SDKs, or architectural conventions.

### 4.1 In-Repo Custom Rules (`.sentinix/custom-rules.json`)

```json
{
  "rules": [
    {
      "id": "corp/prohibit-direct-http-client",
      "severity": "high",
      "cwe": "CWE-693",
      "message": "Direct OkHttpClient usage prohibited; use InternalCorpHttpClient with mTLS",
      "remediation": "Replace OkHttpClient() with InternalCorpHttpClient.createAuthenticatedClient()",
      "files": ["*.kt", "*.java"],
      "pattern": "OkHttpClient\\.Builder\\(\\)"
    }
  ]
}
```

Sentinix automatically picks up `.sentinix/custom-rules.json` during scanning and applies the same zero-overhead verification, audit trail, and CI gating.
