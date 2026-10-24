# Kritix AI (Autonomous End-to-End Enterprise Testing Platform)

**Kritix AI** is an autonomous, high-throughput end-to-end testing platform designed to automate and augment traditional QA, SDET, Security, and Triage functions while drastically reducing testing cycle times and AI token costs.

---

## 🚀 Release Details (v0.3.0-enterprise-hardened)

- **Current Version:** `v0.3.0-enterprise-hardened`
- **Release Changelog:** [CHANGELOG.md](CHANGELOG.md)
- **Supported Platforms:** macOS (`darwin/arm64`, `darwin/amd64`), Linux (`linux/amd64`, `linux/arm64`), Windows (`windows/amd64`)
- **Quick Install via Homebrew:**
  ```bash
  brew install keltstone/tap/kritix --with-engine
  ```
- **Desktop Application:** `brew install --cask kritix-ai` (or `kritix-ai-bundled`)

---

## ⚡ The Core Philosophy: Deterministic-First Token Optimization

Most AI testing agents dump full raw HTML DOMs and network event streams into massive LLM context windows, burning millions of tokens per test run ($10–$50 per suite) and causing crippling latency.

**Kritix reverses this model:**
- **Deterministic Native Go Algorithms First**: DOM pruning to semantic interactive trees, state hashing, regex assertions, TOTP MFA calculations, and AST diffing run natively in Go (<1ms, **0 LLM tokens**).
- **Structural DOM Pruning Engine**: Transforms full HTML DOMs into compact semantic accessibility trees before model submission, stripping decorative styling and non-interactive nodes.
- **Model & Provider Agnostic**: Kritix does not lock you into expensive on-premise GPU clusters. It is 100% agnostic to your intelligence substrate:
  - **Commercial Cloud APIs**: OpenAI (`gpt-4o`), Anthropic (`claude-3-5-sonnet`), Google (`gemini-2.0`), or DeepSeek.
  - **Flat-Rate Developer Subshells**: Zero-token-cost subshells via existing employee developer CLI seats (`claude`, `codex`, `agy`).
  - **Air-Gapped / Sovereign On-Premise**: Pure offline local runtimes (Ollama/vLLM with Qwen 2.5 Coder or Gemma 2) for defense, banking, and strict compliance environments.

> **Note on Performance Metrics**: Laboratory token compression metrics on testdata are labelled `synthetic-corpus`. Production CI times, multi-app end-to-end token costs, and live model usage remain unmeasured until run against containerized target applications via the full evidence harness.


---

## 🛡️ Enterprise-Grade Reliability & Governance

To satisfy strict CISO, QA Architect, and VP of Engineering requirements, Kritix enforces four enterprise governance guarantees:

### 1. Vault-Grade Session Management (`pkg/auth`)
- **Zero Plaintext Token Leaks**: Replaces static Playwright `storageState.json` files with **AES-256-GCM encrypted state** and **in-memory EphemeralSessionStore** with automatic process wipe.
- **Strict TTL Expiration**: Session cookies and tokens automatically expire after configured TTL (default 4 hours), preventing session hijacking on shared CI runners.

### 2. "Alert & Propose" Self-Healing Governance (`pkg/sdet`)
- **No Silent Regression Masking**: Self-healing locator resolution supports three governance modes:
  - `strict` (Default everywhere; set `KRITIX_HEAL_MODE` or the `heal_mode` input to change it): If an exact locator (`test-id`, `id`) is broken, Kritix **does NOT silently click a shifted element** to force a false-positive pass. It marks the regression and generates a **proposed selector patch and diff**.
  - `advisory`: Executes against the best heuristic candidate but emits an audit warning for team review.
  - `permissive`: Permissive auto-healing for rapid exploratory crawling.

A broken locator is only ever healed onto an element that shares its original visible text. Role, tag or position alone never qualify, so a different button that moved into the old spot is a regression, not a heal. Healed passes (`PASSED_WITH_HEALING`) fail the CLI merge gate unless `--allow-healed-override` is passed. The `self-healing-maintenance` blueprint runs advisory because it only proposes a PR for human review.

### 2b. Active-Scan Scope (default-deny)
Every block that sends traffic to a target (`exec.fuzz`, `exec.owasp-dast`, `perf.latency`, `exec.matrix`, `agent.vision-crawl`) and `kritix fuzz` refuses any host not listed in `KRITIX_ALLOWED_TARGETS` (comma-separated host suffixes, e.g. `.staging.example.com,localhost`). Unset means everything is denied and no request is sent.

### 3. Quarantine SLA & Tech-Debt Prevention (`pkg/quarantine`)
- **Prevent Test Rot**: Intermittent / flaky tests isolated in quarantine are tagged with squad ownership and Jira/Linear ticket IDs.
- **Build-Failing SLA**: If a quarantined test exceeds its resolution SLA (e.g. 7 days), it **automatically fails the CI build**, preventing permanent accumulation of untested code.

### 4. Standard Enterprise Artifacts (`pkg/triage`)
- **JUnit XML**: Native export of test executions for GitHub Actions, GitLab CI, and Jenkins.
- **OASIS SARIF 2.1.0**: Native export of DAST security vulnerabilities and PII leaks for GitHub Advanced Security, Snyk, and DefectDojo.
- **Playwright TypeScript**: Clean, copy-pasteable reproduction specs (`repro.spec.ts`) with zero proprietary runner dependencies.

### 5. Enterprise Role-Based Access Control (RBAC) & Audit Trails (`pkg/auth/rbac.go`)
- **Persona-Driven Governance**:
  - `Admin`: Full governance, user provisioning, model/API key rotation, and SOC 2 audit inspection.
  - `TestArchitect`: Exclusive permission to design/publish DAG blueprints, configure model routing matrices, and set quarantine SLAs.
  - `Developer`: Can execute test blueprints, record journeys in Studio, and inspect repros/diffs; **cannot** alter global workflows or model routes.
  - `Viewer / Auditor`: Read-only access to test reports, JUnit/SARIF artifacts, and ROI summaries.
  - `CIRunner`: Scoped headless execution token for CI/CD runners (GitHub Actions, Jenkins).
- **Cryptographic Tokens**: HMAC-SHA256 signed session tokens (`KRITIX_TOKEN`) with expiration validation. Identity comes *only* from a valid token; there is no unsigned role override and no default secret. `KRITIX_AUTH_SECRET` (min 32 bytes) is required, and the CLI refuses to start without it. Issue tokens with `kritix auth token <role>`.
- **Tamper-Evident Audit Log**: Every grant and rejection is appended to a hash-chained JSON-lines file (`KRITIX_AUDIT_LOG`, default `.kritix/audit.log`, mode 0600) that continues across runs. `kritix auth verify-audit` detects edited, removed or reordered entries, and the CLI refuses to append to a broken log. Export (admin only) to CloudTrail/Splunk/Datadog/Elastic with `auth export-audit`. The file is tamper-*evident*, not tamper-proof: a host admin can still delete it, so ship it to your SIEM.

---

## 🖥️ User Interfaces (Web Studio & Desktop Cockpit)

Kritix AI provides two visual interfaces designed for solo developers (Android, KMP, Python, Web) and small teams:

### 1. Embedded Local Web Studio (Zero-Setup, Any Browser)
Starts an embedded local web server and automatically opens your browser on `http://localhost:9090` without needing Node.js or a JVM:
```bash
cd kritix-ai
./bin/kritix studio
# Or run headlessly on a custom port:
./bin/kritix serve --port 9090
```

### 2. Compose Desktop Cockpit (Native macOS / Windows / Linux)
A native desktop application built with Kotlin Multiplatform and Compose Desktop:
```bash
cd kritix-ai
./gradlew :app:run
```

### 3. Developer Guidance & How-To Guides
For detailed step-by-step guides on testing specific tech stacks:
👉 **[Read the Complete Developer How-To Guide](docs/HOW_TO_GUIDE.md)**
- Testing Web SPAs (React, Next.js, Vue, Vite)
- Testing Python Web Apps (Django, Flask, FastAPI, Streamlit)
- Testing Kotlin Multiplatform (KMP) & Mobile Web
- Using the Visual Studio demonstration recorder
- Running OWASP Top 10 DAST & k6 performance audits
- Adding GitHub Actions automated PR smoke checks

---

## 🚀 CLI Quickstart

```bash
# Build the binary
cd kritix-ai
go build -o bin/kritix ./cmd/kritix

# Launch the visual studio in your browser
./bin/kritix studio

# View prebuilt workflow blueprints
./bin/kritix workflows

# Run a quality pipeline
./bin/kritix run pr-smoke-guard

# Ingest multi-artifact documentation (PRD + Copy + Tagging)
./bin/kritix spec "Checkout Flow" "Verify primary user journey"

# Calculate token optimization ROI
./bin/kritix roi
```

---

## 📦 Package Architecture

| Package | Responsibility |
|---|---|
| `app/` | Compose Multiplatform Desktop Cockpit (macOS DMG, Windows MSI, Linux Deb) |
| `pkg/server` | Embedded HTTP API, SSE live telemetry stream, and embedded Web UI |
| `pkg/studio` | "Teach the Agent" human demonstration recorder & BDD synthesizer |
| `pkg/model` | Tri-mode routing (Cloud API, Developer CLI subshells, Local Ollama/vLLM) |
| `pkg/optimizer` | DOM semantic pruning (95% token compression), state hashing cache, ROI reporting |
| `pkg/workflow` | Composable block DAG pipeline engine with prebuilt blueprints |
| `pkg/auth` | AES-256-GCM encrypted sessions, ephemeral memory cache, and RFC 6238 TOTP generator |
| `pkg/sdet` | Multi-factor self-healing locators with strict CI regression gating |
| `pkg/driver` | CDP virtual browser driver, accessibility tree, semantic element matching |
| `pkg/security` | OWASP Top 10 DAST fuzzing payloads and PII leak detection |
| `pkg/perf` | k6 load/spike/soak scenario generation and SLA budget evaluation |
| `pkg/triage` | Playwright repro generator, curl command builder, SARIF & JUnit XML exporters |
| `pkg/quarantine`| Flaky test statistical retries, exponential backoff, and SLA enforcement |
| `pkg/sandbox` | Test environment checkpointing and clean-state rollback hooks |
| `pkg/tracker` | Bi-directional sync with Jira, Linear, and GitHub Issues |
