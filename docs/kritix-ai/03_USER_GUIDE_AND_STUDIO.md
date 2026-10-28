# Kritix AI Platform: User Guide & Testing Workflows

**Platform:** `kritix-ai` (CLI, Web Studio & Desktop Cockpit)  
**Author:** Keltstone Systems Quality Engineering & Developer Experience  
**Audience:** QA Engineers, SDETs, Developers, DevOps Leads  
**Scope:** CLI usage, Web Studio demonstration recorder, Compose Desktop Cockpit, OWASP DAST audits, and Playwright repro exports.

---

## 1. Quickstart & Binary Compilation

```bash
cd kritix-ai

# Build the native binary:
go build -ldflags="-s -w" -o bin/kritix ./cmd/kritix

# Verify installation:
./bin/kritix --help
```

---

## 2. CLI Command Reference

```text
Usage:
  kritix [command] [options] [arguments]

Core Commands:
  studio                   Launch embedded local web studio in browser (http://localhost:9090)
  serve [--port 9090]      Run headless background server daemon
  workflows                List all prebuilt and custom workflow blueprints
  run <blueprint-name>     Execute a quality DAG pipeline (e.g. pr-smoke-guard)
  spec <title> <desc>      Ingest multi-artifact documentation (PRD + Copy + Tagging)
  roi                      Calculate token optimization ROI and savings vs raw DOM
  auth token <role>        Issue HMAC-SHA256 session token (Admin, Developer, CIRunner)
  auth verify-audit        Verify cryptographic integrity of hash-chained audit ledger
```

---

## 3. Embedded Local Web Studio (Zero-Setup)

Kritix includes a zero-setup local Web Studio built directly into the single Go binary—requiring **zero Node.js, npm, or JVM dependencies**:

```bash
cd kritix-ai
./bin/kritix studio
```
The command automatically starts the HTTP daemon on `http://localhost:9090` and opens your default browser.

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│  KRITIX AI WEB STUDIO  [● Recording]  [Journeys]  [Quarantine]  [Security]  [ROI]     │
├────────────────────────────────────────────────────────────────────────────────────────┤
│  Target URL: [ https://staging.example.com/checkout/step2                             ]│
│  [⏺ Record Journey]  [⏹ Stop & Synthesize BDD]  [▶ Playback Journey]  [⚡ Run DAST]    │
├────────────────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────┬───────────────────────────────────┐  │
│  │ Live Chromium CDP Viewport                   │ Synthesized BDD Gherkin Spec      │  │
│  │                                              │                                   │  │
│  │  [ User enters shipping address ]            │ Feature: Checkout Submission      │  │
│  │  [ Clicks 'Proceed to Payment'  ]            │   Scenario: Credit Card Purchase  │  │
│  │  [ Completes 3DS Authentication ]            │     Given user on checkout step 2 │  │
│  │                                              │     When user enters valid card   │  │
│  │                                              │     Then order confirmed (200 OK) │  │
│  └──────────────────────────────────────────────┴───────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Visual Features
1. **"Teach the Agent" Journey Recorder**: Click around your web app in the live viewport. Kritix captures user actions, generates resilient accessibility locators, and synthesizes clean BDD Gherkin specs.
2. **Interactive Self-Healing Diff Inspector**: When an element shifts, view the side-by-side visual selector diff and click **"Approve Selector Patch"**.

---

## 4. Running OWASP Top 10 DAST Security Audits

To audit your staging web application against active vulnerabilities:

```bash
# 1. Authorize staging host in environment:
export KRITIX_ALLOWED_TARGETS=".staging.example.com,localhost"

# 2. Run the security audit blueprint:
./bin/kritix run security-dast-audit \
  --target "https://staging.example.com/api" \
  --fuzz-depth "deep"
```

### Vulnerability Checks Performed
- **SQL Injection (SQLi)**: Error-based and boolean blind payloads.
- **Cross-Site Scripting (XSS)**: Context-aware reflection probes.
- **Server-Side Request Forgery (SSRF)**: Egress probes targeting metadata endpoints (`169.254.169.254`).
- **PII Leakage in Responses**: Scans for unredacted credit cards, SSNs, and bearer tokens.

---

## 5. Exporting Standard Enterprise Artifacts (`pkg/triage`)

When a test failure or security vulnerability is detected, Kritix exports reproducible, standard enterprise artifacts:

### 5.1 Playwright TypeScript Reproduction (`repro.spec.ts`)
Generates standalone, copy-pasteable Playwright tests with **zero proprietary runner dependencies**:

```typescript
import { test, expect } from '@playwright/test';

test('reproduce checkout failure #142', async ({ page }) => {
  await page.goto('https://staging.example.com/checkout');
  await page.getByRole('button', { name: 'Complete Order' }).click();
  await expect(page.getByText('Payment Succeeded')).toBeVisible({ timeout: 5000 });
});
```

### 5.2 Industry Standard Test & Vulnerability Exports
- **OASIS SARIF 2.1.0 (`results.sarif`)**: Imports directly into GitHub Advanced Security, Snyk, and DefectDojo.
- **JUnit XML (`junit.xml`)**: Integrates into GitHub Actions, GitLab CI, and Jenkins test result dashboards.

---

## 6. Managing Flaky Test Quarantine SLAs

To prevent intermittent test failures from breaking main builds:

```bash
# Quarantine a flaky test:
./bin/kritix quarantine add "tests/checkout_spec.go:TestPaymentTimeout" \
  --ticket "ENG-4821" \
  --owner "payments-squad" \
  --sla "7d"

# Inspect active quarantine items:
./bin/kritix quarantine list
```

If a test remains in quarantine past its 7-day SLA without resolution, the CLI automatically **fails the CI merge gate**, preventing permanent accumulation of untested code.

---

*Next: Continue to [Kritix AI Platform: Developer Guide & Enterprise Administration](../kritix-ai/04_DEVELOPER_GUIDE_AND_ENTERPRISE.md).*
