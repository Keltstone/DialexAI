<div align="center">

<img src="logos/logo_sentinix.png" alt="Sentinix AI Logo" width="150" style="border-radius: 20px; margin-bottom: 16px;" />

# Sentinix AI
### Continuous Security Review, PR Gate & Vulnerability Posture Engine

**Security review that keeps up with your code, and tells you the truth about what it checked.**

[![Version](https://img.shields.io/badge/Version-1.0.0_GA-success.svg?style=for-the-badge)](CHANGELOG.md)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![Rules](https://img.shields.io/badge/Rules-135_Across_10_Stacks-blueviolet.svg?style=for-the-badge)](#7-rule-packs)
[![SARIF](https://img.shields.io/badge/Export-SARIF_v2.1.0_%7C_JUnit-orange.svg?style=for-the-badge)](#10-reports-and-exports)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg?style=for-the-badge)](LICENSE)

</div>

Sentinix AI watches every pull request, scans your repositories on a schedule, and tracks each security finding from the day it appears to the day it is fixed. It is built for teams who have been burned by scanners that bury them in noise: a pull request only ever shows what *that change* introduced, every report states exactly which checks ran, and nothing that weakens your safeguards can be approved by the person who proposed it.

It is defensive by design. Sentinix reads code and diffs in repositories you own. It never probes, scans or attacks a live system.

> **Status:** **v1.0.0 General Availability (GA)**. Tested against 10 technology stacks and 135 rule fixtures. The core (CLI, rule packs, PR review, dashboard, reports) is fully verified with 100% test pass rate.

---

## 🚀 Release Details (v1.0.0 — Production GA)

- **Current Version:** `1.0.0` (Production GA)
- **Release Date:** October 25, 2026
- **Release Changelog:** [CHANGELOG.md](CHANGELOG.md)
- **License:** Apache-2.0
- **Supported Platforms:** macOS (`darwin/arm64`, `darwin/amd64`), Linux (`linux/amd64`, `linux/arm64`), Windows (`windows/amd64`)
- **Quick Install via Homebrew:**
  ```bash
  brew install keltstone/tap/sentinix
  ```
- **Universal One-Line Install:**
  ```bash
  curl -fsSL https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.sh | bash -s -- --sentinix
  ```
- **Release Verification:**
  - Standalone binaries: `sentinix` (CLI), `sentinixd` (Daemon & Web Dashboard)
  - Pre-built distribution archives and cryptographic signatures available in [`dist/SHA256SUMS`](../dist/SHA256SUMS)

---

## Contents

1. [What it is](#1-what-it-is)
2. [How it works](#2-how-it-works)
3. [Quick start: scan a project from the CLI](#3-quick-start-scan-a-project-from-the-cli)
4. [How to run the server](#4-how-to-run-the-server)
5. [How to connect a git host](#5-how-to-connect-a-git-host)
6. [Projects, scans and findings](#6-projects-scans-and-findings)
7. [Rule packs](#7-rule-packs)
8. [Dependencies, SBOM and advisories](#8-dependencies-sbom-and-advisories)
9. [Users, roles and approvals](#9-users-roles-and-approvals)
10. [Reports and exports](#10-reports-and-exports)
11. [Integrations](#11-integrations)
12. [AI add-on (optional)](#12-ai-add-on-optional)
13. [Sandbox](#13-sandbox)
14. [Deploying](#14-deploying)
15. [CI usage without the server](#15-ci-usage-without-the-server)
16. [Reference](#16-reference)
17. [Security model](#17-security-model)
18. [Testing and benchmarks](#18-testing-and-benchmarks)
19. [Troubleshooting](#19-troubleshooting)
20. [Limits and what is not included](#20-limits-and-what-is-not-included)
21. [Frequently asked questions](#21-frequently-asked-questions)

---

## 1. What it is

### The problem it solves

Most teams do not lack security scanners. They lack scanners they still trust after six months. The usual story goes like this: a scanner is switched on, reports four hundred findings on day one, the team triages the first twenty, and the rest becomes background noise. Meanwhile new features ship, the security tests go stale, and a dependency that was fine last quarter picks up a critical advisory that nobody notices.

Sentinix is designed around that failure mode:

- **It reports changes, not history.** Every pull request is compared with its base branch, so existing issues never block a developer who did not cause them.
- **It remembers.** Findings are tracked across scans as new, fixed or reopened, with owners, tickets and an audit trail, so "did we fix that?" has an answer.
- **It is honest about coverage.** Every report lists the engines and rule pack versions that ran and what was not covered, so a clean result is never mistaken for a guarantee.
- **It does not let safeguards erode quietly.** Disabling a rule, loosening its severity or accepting a risk needs a second person, and is written to the audit log.
- **You choose how hard it pushes.** Each project can *block* merge requests that introduce serious findings, or run as an *optional report* that comments but never stops a merge. Start in report mode, build trust, then switch to blocking when you are ready ([section 5](#choose-block-or-report-only)).

### Linters versus Sentinix

Linters such as detekt and ktlint ask whether code is clean, one file at a time. Sentinix asks a different question: *did this change make the product less safe, and who needs to act?*

| Need | Linters and style tools | Sentinix |
| --- | --- | --- |
| Existing code | Report every issue on every run | Baselines and head-versus-base comparison: a PR shows only what it introduced |
| Dependencies | Not checked | Software bill of materials matched against advisories, re-checked without rescanning code |
| Secrets | Limited | Secrets in source, masked in every output |
| Visibility | Per-repository CI logs | One dashboard with trends, owners and time to fix |
| Governance | None | Single sign-on, roles, approvals with separation of duties, audit log |
| Workflow | Warnings that scroll past | One pass-or-fail gate, tickets, chat alerts |

### Two tiers

The **core** is deterministic. The same code and the same rule pack versions produce the same findings every time, with no model and no network access. That makes it safe as a CI gate and usable in air-gapped environments.

The **AI add-on** is optional and off by default. It adds council triage of findings, patch drafts and test generation on top of the core. Its output is advisory: it informs a human and never dismisses a finding on its own.

### What it checks out of the box

- **10 Technology Stacks (135 Built-in Rules):** Android (60 rules covering OWASP MASVS L1/L2 and fintech hardening), Kotlin Multiplatform (8 rules), Kotlin Backend (10 rules), iOS / Swift (8 rules), Go (8 rules), .NET / C# (8 rules), PHP (8 rules), Rust (7 rules), Python / Django (9 rules), Node.js / React (9 rules).
- **Per-Rule Policy Enforcement:** Maintainers can configure any rule to `Block`, `Warn`, `Notice`, or `Ignore` via `.sentinix/policy.json` or REST API.
- **Team-Level Custom Rules:** Drop-in declarative `.sentinix/custom-rules.json` for repository-specific standards.
- **Secrets:** Cloud API keys, private keys, high-entropy tokens, with line-level masking and `sentinix:ignore` suppression.
- **Dependencies** (opt-in): Known-vulnerable packages for Go, npm, PyPI and Maven via OSV.
- **Anything else:** `semgrep`, `gitleaks` and `osv-scanner` are picked up automatically when installed, and you can add your own signed rule packs.

### Supported git hosts

GitHub and GitHub Enterprise, GitLab (cloud and self-managed), and Bitbucket Cloud. Any other git server works through the command-line tool in CI ([section 15](#15-ci-usage-without-the-server)).

---

## 2. How it works

Every scan, whatever triggers it, follows the same path:

```
trigger ──► surface map ──► engines ──► policy ──► AI (optional) ──► report
```

1. **Trigger:** a pull request webhook, a schedule, a manual "Scan now", a CI upload, or a new advisory.
2. **Surface map:** the diff is turned into a list of added or removed endpoints, permissions and dependency lines.
3. **Engines:** the built-in secrets scanner, rule packs and any installed external engines run, and all results are normalised into one finding format.
4. **Policy:** findings already present on the base branch are removed, so only what the change introduced counts. Severity overrides and the gate are applied.
5. **AI add-on (optional):** triage verdicts and patch drafts for findings that need a second opinion.
6. **Report:** a PR comment and status, dashboard updates, exports, tickets and notifications.

### Three scan modes

| Mode | Trigger | Scope | Result |
| --- | --- | --- | --- |
| **PR / MR review** | Webhook | The head commit compared with the base branch | A comment and a commit status on the pull request |
| **Full scan** | "Scan now", cron schedule, or CI upload | The whole branch | Findings tracked as new, fixed or reopened |
| **Advisory re-scan** | A timer, or an API call | Stored dependency lists only | New advisories, with no code fetched |

### What happens during a pull request review

1. The webhook signature is verified, then a *pending* status is set so the author sees work has started.
2. Sentinix shallow-fetches the head commit and the base branch into a throwaway directory. The git token is used only to fetch and never reaches a scan process. Pull requests from forks are fetched without any token.
3. Every engine runs on the head and on the base. Only findings that are **not** in the base are reported.
4. The diff becomes attack-surface items for the dashboard.
5. A comment is posted, the status is set (failing at or above your chosen severity), and the checkout is deleted. Only findings, short evidence and metadata are stored.

### How a finding keeps its identity

A finding's identity is a fingerprint of the **rule, the file and the masked evidence**, deliberately not the line number. If someone adds ten lines above an old finding, it is still the same finding, not a new one, and baselines keep working.

### Components

| Path | Role |
| --- | --- |
| `cmd/sentinixd` | The server: webhooks, API, scheduler, dashboard |
| `cmd/sentinix` | The command-line tool: `scan`, `surface`, `packs`, `bench` |
| `pkg/engines` | Engine interface, built-in secrets scanner, SARIF wrappers |
| `pkg/packs` | Rule pack loader, signing, built-in Kotlin, Android and KMP packs |
| `pkg/githost` | GitHub, GitLab and Bitbucket providers |
| `pkg/scheduler` | PR review, full scan, cron timing |
| `pkg/store` | Postgres or SQLite persistence |
| `pkg/server`, `pkg/web` | HTTP API and the dashboard |
| `pkg/advisories` | Dependency extraction and OSV matching |
| `pkg/report`, `pkg/integrations` | Exports, chat, webhooks and tickets |
| `pkg/ai`, `pkg/sandbox` | Optional AI add-on and container sandbox |
| `bench/` | A labelled sample project for measuring precision and recall |

---

## 3. Quick start: scan a project from the CLI

This is the fastest way to see what Sentinix finds. It needs no server, database or network.

**Requirements:** Go 1.27 and `git`.

```bash
cd sentinix-ai && go build -o sentinix ./cmd/sentinix
```

Scan any directory:

```bash
./sentinix scan /path/to/your/project
```

You will see one line per finding, then a summary that names the engines that ran:

```
[medium] android/exported-component app/src/main/AndroidManifest.xml:10 Exported component without a permission (launcher activities are exempt)
[high] secret.aws-access-key config/Settings.kt:12 possible hardcoded secret
2 finding(s). Engines run: secrets, packs. Not covered: engines not installed, AI triage.
```

That last sentence matters. It is there so a short list is never mistaken for a clean bill of health.

**Exit codes**, so the same command works as a CI gate:

| Code | Meaning |
| --- | --- |
| `0` | Nothing at or above the gate (default gate: `high`) |
| `1` | At least one finding at or above the gate |
| `2` | Usage error or fatal problem |
| `3` | An engine failed. A broken engine must never look like a clean scan |

Prefer a report that never fails the build? Add `-report-only`: findings are printed (and uploaded, if you use `-server`) but the exit code stays `0` unless an engine failed.

### Adopting Sentinix on an existing codebase (5-Minute Setup)

Protecting your repository takes less than 5 minutes and requires **zero servers or databases**:

#### Step 1: Install the CLI
```bash
# macOS & Linux (Homebrew)
brew install keltstone/tap/sentinix

# Or via 1-Line Universal Installer (macOS, Linux)
curl -sSL https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.sh | bash -s -- --app sentinix

# Windows (PowerShell)
irm https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.ps1 | iex
```

#### Step 2: Establish the baseline
Run it on a mature project and you will get findings that have lived there for years. You do not want them to block anyone tomorrow, and you do not want to ignore them forever. A **baseline** records today's findings once with content-based fingerprints, so future scans **only** fail on newly introduced issues:

```bash
sentinix scan -baseline .sentinix-baseline.json -update-baseline .
```

#### Step 3: (Optional) Tune policy and custom rules
- **`.sentinix/policy.json`:** Control which rules `Block`, `Warn`, `Notice`, or `Ignore` in this repository:
  ```json
  {
    "rules": {
      "android/exported-component": "Block",
      "android/allow-backup": "Warn",
      "node/insecure-cookie-flag": "Ignore"
    }
  }
  ```
- **`.sentinix/custom-rules.json`:** Add proprietary team checks (e.g., internal mTLS HTTP client enforcement).

#### Step 4: Add the GitHub Actions workflow
Create `.github/workflows/sentinix.yml`:
```yaml
name: Sentinix Security Gate
on: [push, pull_request]
permissions:
  contents: read
  security-events: write

jobs:
  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install Sentinix
        run: brew install keltstone/tap/sentinix || (curl -sSL https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.sh | bash -s -- --app sentinix)
      - name: Scan PR Changes
        run: sentinix scan -baseline .sentinix-baseline.json -fail-on high -sarif sentinix.sarif .
      - uses: github/codeql-action/upload-sarif@v3
        if: always()
        with:
          sarif_file: sentinix.sarif
```

#### Step 5: Commit and push
```bash
git add .sentinix-baseline.json .sentinix/ .github/workflows/sentinix.yml
git commit -m "chore(security): protect repo with Sentinix AI"
git push
```

Commit `.sentinix-baseline.json`. Because it stores content hashes rather than line numbers, normal code editing and line shifts will never invalidate your baseline.

### Handy details

- **Silence one line**, for example a documented test fixture, by adding `sentinix:ignore` anywhere on it. This applies to the built-in secrets scanner.
- **More engines:** if `semgrep`, `gitleaks` or `osv-scanner` is on your `PATH`, it runs automatically and its SARIF output joins the same list. The summary line shows what ran.
- **SARIF output** for code-scanning interfaces: add `-sarif out.sarif`.
- **Attack surface of a branch:** `./sentinix surface origin/main` lists endpoints, permissions and dependency lines added or removed in `git diff origin/main...HEAD`.

---

## 4. How to run the server

The server adds pull request reviews, scheduled scans, the dashboard, roles, approvals and reports.

**You will need:** a host or container to run it on, a database (SQLite for a trial, Postgres for production), one token per git host, and an OpenID Connect (OIDC) provider if you want the dashboard.

### Step 1: build and configure

```bash
cd sentinix-ai && go build -o sentinixd ./cmd/sentinixd
```

Create `sentinix.json` listing your git hosts (see [section 5](#5-how-to-connect-a-git-host)). Any value written as `$NAME` is read from the environment, which keeps secrets out of the file.

### Step 2: set the required secret

```bash
export SENTINIX_SESSION_KEY=$(openssl rand -hex 32)   # at least 32 characters; signs session cookies
```

### Step 3: set up sign-in (needed for the dashboard)

```bash
export OIDC_ISSUER=https://idp.example.com
export OIDC_CLIENT_ID=<client id>
export OIDC_CLIENT_SECRET=<client secret>
export OIDC_REDIRECT_URL=https://sentinix.example.com/auth/callback
export SENTINIX_OWNER_EMAIL=you@example.com   # this person becomes the Owner on first sign-in
```

Any OIDC provider works: Keycloak, Authentik, Okta, Microsoft Entra ID, Google and others. If your organisation uses SAML, put Keycloak or Authentik in front and let Sentinix talk to it over OIDC.

If you skip this step, everything except the dashboard login still works: webhooks, PR reviews and CI uploads do not need it. For local trials over plain HTTP, set `SENTINIX_INSECURE_COOKIES=1`. Cookies are `Secure` by default.

### Step 4: start it

```bash
./sentinixd -config sentinix.json -db sqlite:sentinix.db -addr :8080
```

For production, point it at Postgres instead:

```bash
./sentinixd -config sentinix.json -db "postgres://user:pass@host:5432/sentinix?sslmode=require"
```

Tables are created on start. Open the address in a browser, sign in, and add your first project under **Settings** (the host name from `sentinix.json`, and the repository as `owner/name`).

### Server flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `sentinix.json` | Git host list |
| `-addr` | `:8080` | Listen address |
| `-db` | `sqlite:sentinix.db` | `postgres://…` or `sqlite:<path>` |
| `-workers` | `2` | Scans that may run at the same time |
| `-fail-on` | `high` | Default severity at which a blocking PR status fails: `info`, `low`, `medium`, `high`, `critical`. Projects can override |
| `-enforcement` | `block` | Default for repos: `block` (a failing status can stop the merge) or `report` (comment only, never blocks). Projects can override |
| `-packs`, `-trust` | none | Extra signed rule packs and the public keys you trust ([section 7](#7-rule-packs)) |
| `-osv` | off | OSV API address for dependency advisories ([section 8](#8-dependencies-sbom-and-advisories)) |
| `-advisory-interval` | `6h` | How often stored dependency lists are re-matched |
| `-integrations` | none | Chat, webhook and ticket settings ([section 11](#11-integrations)) |
| `-ai` | none | Model list for the AI add-on ([section 12](#12-ai-add-on-optional)) |
| `-sandbox-image` | none | Run external engines and generated tests in this container image ([section 13](#13-sandbox)) |

### Other environment variables

| Variable | Purpose |
| --- | --- |
| `SENTINIX_BRAND_NAME`, `SENTINIX_BRAND_ACCENT` | Your name and a `#rrggbb` accent colour in HTML reports |
| `SENTINIX_CONTAINER_BIN` | Use `podman` instead of `docker` for the sandbox |
| `SENTINIX_SEMGREP_CONFIG` | Semgrep rules path, or `auto` (the default; needs network access) |

Run a single server replica. Scheduled scans run inside the process.

---

## 5. How to connect a git host

`sentinix.json` is a list. Each entry becomes a webhook endpoint at `POST /webhook/<name>`.

```json
[
  {"name": "gh",  "kind": "github",    "secret": "$GH_SECRET",  "token": "$GH_TOKEN"},
  {"name": "ghe", "kind": "github",    "api": "https://ghe.example.com/api/v3", "secret": "$GHE_SECRET", "token": "$GHE_TOKEN"},
  {"name": "gl",  "kind": "gitlab",    "api": "https://gitlab.example.com/api/v4", "secret": "$GL_SECRET", "token": "$GL_TOKEN"},
  {"name": "bb",  "kind": "bitbucket", "secret": "$BB_SECRET", "token": "$BB_TOKEN", "public_url": "https://sentinix.example.com"}
]
```

| Field | Meaning |
| --- | --- |
| `name` | Used in the webhook path, and as a project's `host` |
| `kind` | `github`, `gitlab` or `bitbucket` |
| `api` | API base address. Defaults: `https://api.github.com`, `https://gitlab.com/api/v4`, `https://api.bitbucket.org/2.0`. Set it for GitHub Enterprise and self-managed GitLab |
| `web_url` | Web root used to build clone addresses. Derived from `api` when empty |
| `secret` | Webhook secret. GitHub and Bitbucket sign the request body; GitLab sends a secret token |
| `token` | Used to fetch code, set statuses and post comments |
| `public_url` | Linked from Bitbucket build statuses (your Sentinix address) |

A host with an empty `secret` or `token` is refused at startup, so a misconfiguration fails loudly instead of leaving an open endpoint.

### Token permissions

Give the token the least it needs: read access to code, and write access to commit statuses and PR or MR comments.

| Host | Needs |
| --- | --- |
| GitHub | Contents: read. Commit statuses: write. Pull requests: write |
| GitLab | A project or group access token with the `api` scope |
| Bitbucket Cloud | Repository read and write (for statuses), and Pull requests write, via a repository or workspace access token |

### Webhook setup

| Host | Where | Events | Secret |
| --- | --- | --- | --- |
| GitHub | Settings, Webhooks, content type `application/json` | Pull requests | The Secret field |
| GitLab | Settings, Webhooks | Merge request events | Secret token |
| Bitbucket | Repository settings, Webhooks | Pull request created, updated | Secret |

Set the payload address to `https://<your-sentinix-host>/webhook/<name>`. Reviews run when a pull request is opened, updated with new commits, or reopened. The server must be reachable from the git host; behind a firewall, use a reverse proxy or a tunnel.

### Choose: block, or report only

Sentinix always posts a comment and a `sentinix` commit status on the pull request. What that status *does* is up to you, per project:

| Mode | Status on new findings | Merge | Comment says |
| --- | --- | --- | --- |
| **Block** (default) | Fails when a new finding is at or above the threshold | Stopped, if the status is a required check | "Merge is blocked by new findings at **high** or above." |
| **Report only** | Always passes, with a description such as "3 new finding(s), report only" | Never affected | "Report only: these findings do not block the merge." |

Set it in the dashboard (**Settings**, then *Enforcement* and *Fail at*), through the API (`PATCH /api/projects/{id}` with `{"enforcement":"report"}` and, optionally, `{"fail_on":"medium"}`), or set a server-wide default with `-enforcement` and `-fail-on`. A project's own settings win; a repository that is not registered as a project uses the server defaults. Changing enforcement creates an approval and requires a second person to decide.

A sensible rollout is to run in **report only** for a couple of weeks so the team sees real findings and you can tune noisy rules, then switch the important projects to **block** at `high` and later at `medium`.

### Review categories and per-category gating

Sentinix classifies findings into five categories: `security`, `architecture`, `style`, `accessibility`, and `quality`.

Each category can be configured with its own `mode` (`block`, `comment`, or `off`) and severity `threshold` (`info`, `low`, `medium`, `high`, `critical`):

| Category | Default Mode | Default Threshold | Description |
| --- | --- | --- | --- |
| **security** | `block` | `high` (or project `FailOn`) | Security vulnerabilities and secret leaks |
| **architecture** | `comment` | `medium` | Structural and layer boundary rules (Konsist, packs) |
| **accessibility** | `comment` | `medium` | Accessibility violations (contentDescription, touch targets) |
| **style** | `comment` | `low` | Linter formatting and style warnings (Detekt, ktlint), collapsed in comments |
| **quality** | `comment` | `medium` | Code quality and general lint warnings |

- **Backward compatibility:** Existing `Enforcement` (`block` | `report`) and `FailOn` remain fully supported as the `security` category configuration.
- **PR comment grouping:** Pull request review comments group findings by category. Security findings are shown open by default; non-security findings are collapsed in expandable `<details>` sections to reduce noise.
- **Coverage footer:** The comment includes a coverage footer showing the status of expected tools. Unsigned ingest uploads are explicitly labeled as `uploaded by CI (unverified)` rather than claiming proof of execution, and missing uploads are marked as `not received`. Attested uploads (cryptographically verified server-side with an ed25519 `X-Signature` bound to `project_id|commit|pr|engine|format|timestamp|sha256(body)`) display `received (<timestamp>)`.
- **Cryptographic Ingest Attestation (`X-Signature`):**
  - **Context-bound signature:** Signatures must be computed over `project_id|commit|pr|engine|format|timestamp|sha256(body)`.
  - **Mandatory timestamp:** Requests with `X-Signature` must supply `X-Signature-Timestamp` (within a 900s expiry and 300s clock skew window).
  - **Replay protection & Nonce cache:** The server maintains a shared database-backed replay cache of signature nonces. Unsigned, missing timestamp, expired timestamp, or replayed nonce uploads fail closed and land as `ci-unattested` (or 4xx). Replaying signatures across different projects, commits, PRs, or engines is rejected as `ci-unattested`.
  - **Signing helper CLI:** Run `sentinix sign -key <ed25519-key> -project <proj-id> -commit <sha> -pr <pr> -engine <engine> -format <format> <file>` to generate headers, or use the OpenSSL ed25519 recipe.
  - **Fork PR policy:** Workflows triggered by pull requests from external forks do not have access to repository secrets (`SENTINIX_TOKEN`, `SENTINIX_SIGNING_KEY`). Consequently, uploads from fork PRs are unsigned (`ci-unattested`), capped at Medium severity, excluded from gating and AI context, and cannot reopen or close findings.
- **Fallback comment:** Before a scan exists, Sentinix posts a comment stating `scan pending`, omitting the blocked/clean line until scan results arrive.
- **Status checks & signed vs unsigned gating:**
  - Sentinix publishes the overall `sentinix` check for security gating, plus per-category checks (`sentinix/security`, `sentinix/architecture`, `sentinix/style`, `sentinix/accessibility`, `sentinix/quality`).
  - **Security gate (`sentinix`):** Only server-run scanners or cryptographically signed uploads (`X-Signature`) can fail the security status. Unsigned/unattested CI uploads are capped at Medium, excluded from the primary security check, and excluded from AI context to prevent merge DoS.
  - **Non-security category checks (`sentinix/<cat>`):** By default, unsigned uploads are comment-only for `style`, `quality`, and `accessibility` to prevent merge DoS from unverified CI scripts. Only compiled architecture tests (Konsist in `architecture` category) may block from unsigned uploads when configured with `mode: block`. Unsigned callers cannot choose `category=architecture` for other engines to gain blocking. Gating other non-security categories requires cryptographically signed uploads (`X-Signature`).
  - **Lifecycle closure & link table:** Ingest uploads only close findings for engines that are NOT server-owned (never `secrets`, `gitleaks`, `semgrep`, `osv-scanner`, `packs`, or `advisories`), only findings belonging to the same ingest origin, and tracked per-PR via a link table (`finding_prs`). A clean upload on PR 2 removes PR 2's association but never closes a finding that PR 1 still reports; findings are only marked fixed when no other PR/branch still reports them. Older clean uploads cannot close newer findings. Unsigned uploads cannot reopen fixed findings or close server findings.
  - **Engine allowlist & SARIF resilience:** Engine names must match `^(pack:[a-z0-9._-]+(@[a-z0-9._+-]+)?|[a-z0-9._-]{1,64})$` (others rejected with 400), supporting standard tools and CLI pack engines. SARIF results containing path traversal (`..`) are dropped per finding instead of aborting the whole report.
- **Safeguards:** Loosening any category mode (e.g. `block` -> `comment`), raising a severity threshold in block mode, or turning a check `off` requires two-person approval. Validation ensures only valid categories, modes, and severities can be written.

### Make blocking real

The status only stops a merge if the git host is told to require it. Mark the `sentinix` status as a **required check** in your branch protection rules (GitHub, GitLab) or as a required build (Bitbucket). Note that required checks should treat Sentinix as blocking only in block mode. For report-only projects, scanner errors post a non-red status across git hosts (GitHub: neutral check-run; GitLab: success with "scanner unavailable"; Bitbucket: STOPPED) so that merges are never blocked on report-only projects when scanners are unavailable.

**Not supported by the server:** Bitbucket Server and Data Center, and Gitea. Use the command-line tool in CI for those.

---

## 6. Projects, scans and findings

### Project settings

Configure these under **Settings**, or with `PATCH /api/projects/{id}`.

| Setting | Meaning |
| --- | --- |
| Enforcement | `block` (default) or `report`. See [section 5](#choose-block-or-report-only) |
| Fail at | Severity at which a blocking status fails (`low`, `medium`, `high`, `critical`). Empty uses the server default |
| Branch | Branch used for full scans (default `main`) |
| Schedule | `nightly` (02:00), `weekly` (Sunday 03:00), or a five-field cron expression such as `30 1 * * 1-5`. Empty turns scheduling off |
| Timezone | An IANA name such as `Europe/Berlin`. Empty means UTC |
| AI add-on and model policy | See [section 12](#12-ai-add-on-optional) |

An invalid schedule is rejected the moment you save it, not at 2 a.m.

### Scans

"Scan now" (available to Security reviewers and above) and schedules run a full scan of the branch. Each run records the commit, mode, duration, engines used, rule pack versions, and how many findings are new, fixed or unchanged compared with the previous run.

### The life of a finding

- **Open:** reported by the latest scan.
- **Fixed:** an engine that ran no longer reports it. Engines that did *not* run never mark anything fixed, so a missing binary can never make a repository look clean.
- **Reopened:** a fixed finding that comes back is reopened automatically.
- **False positive:** dismissed by a reviewer.
- **Accepted:** an accepted risk. It needs a reason, an expiry date and a second person's approval ([section 9](#9-users-roles-and-approvals)). When the expiry passes, the finding reopens on the next scan, so exceptions are revisited instead of forgotten.

### Learning which rules are noisy

Each rule's findings and dismissals over the last 90 days are shown under **Rule packs**. A rule with at least 10 findings and precision under 50% is flagged and shown as informational. Sentinix never deletes a rule automatically; it makes the noise visible and leaves the decision with you.

### Attack surface

The **Attack surface** screen lists endpoints, permissions and dependency changes seen in reviewed pull requests of registered projects, so a reviewer can see at a glance what a release added.

---

## 7. Rule packs

Rule packs are how you teach Sentinix about your stack, with no changes to Sentinix itself. A pack is **data only**: it cannot run code or reach the network, which is what makes it safe to share.

### Built in (10 Stacks, 135 Rules)

These are always available, and are chosen automatically from what the repository contains:

| Pack | Rules | Applies when | Focus & Key Protections | Documentation |
| --- | --- | --- | --- | --- |
| `android` | 60 | `AndroidManifest.xml` or `com.android.*` Gradle plugin | Exported components, task hijacking (StrandHogg), App Links autoVerify, OAuth schemes, hardware Keystore biometric invalidation, clipboard masking, unencrypted Room, dynamic code loading, cleartext traffic | [docs/packs/01_ANDROID_PACK.md](docs/packs/01_ANDROID_PACK.md) |
| `kmp` | 8 | `commonMain/` folder or `multiplatform` plugin | Multiplatform cleartext URLs, log leaks, plain Settings secrets, mutable companion state, Ktor certs | [docs/packs/02_KMP_PACK.md](docs/packs/02_KMP_PACK.md) |
| `kotlin-backend` | 10 | Kotlin files with Spring/Ktor | Spring CSRF disable, `permitAll` root trees, open CORS, actuator exposure, SQL injection, JWT weak HMAC | [docs/packs/03_KOTLIN_BACKEND_PACK.md](docs/packs/03_KOTLIN_BACKEND_PACK.md) |
| `ios-swift` | 8 | `*.swift`, `Info.plist`, `Podfile`, `Package.swift` | ATS disabled, insecure Keychain accessibility (`kSecAttrAccessibleAlways`), CC_MD5/SHA1, UIWebView, pasteboard leak | [docs/packs/04_IOS_SWIFT_PACK.md](docs/packs/04_IOS_SWIFT_PACK.md) |
| `go` | 8 | `*.go`, `go.mod` | SQL injection via Sprintf, `InsecureSkipVerify: true`, math/rand tokens, shell command injection, pprof exposed | [docs/packs/05_GO_PACK.md](docs/packs/05_GO_PACK.md) |
| `dotnet` | 8 | `*.cs`, `*.csproj`, `appsettings*.json` | SQL injection, `BinaryFormatter` deserialization, weak MD5/SHA1, connection strings, disabled antiforgery | [docs/packs/06_DOTNET_PACK.md](docs/packs/06_DOTNET_PACK.md) |
| `php` | 8 | `*.php`, `composer.json` | SQL injection, `system()` / `exec()` injection, `unserialize()` object injection, RFI, SSRF via curl | [docs/packs/07_PHP_PACK.md](docs/packs/07_PHP_PACK.md) |
| `rust` | 7 | `*.rs`, `Cargo.toml` | Unsafe blocks, `mem::transmute`, shell command injection, raw pointer deref, production panics, CString FFI | [docs/packs/08_RUST_PACK.md](docs/packs/08_RUST_PACK.md) |
| `python` | 9 | `*.py`, `requirements.txt`, `pyproject.toml` | SQL injection, `pickle` / `yaml.load` deserialization, `os.system` / `shell=True`, Django debug mode, SSRF | [docs/packs/09_PYTHON_PACK.md](docs/packs/09_PYTHON_PACK.md) |
| `node-react` | 9 | `*.js`, `*.jsx`, `*.ts`, `*.tsx`, `package.json` | `dangerouslySetInnerHTML`, `eval()` / `new Function()`, `child_process.exec`, SQL injection, JWT 'none' | [docs/packs/10_NODE_REACT_PACK.md](docs/packs/10_NODE_REACT_PACK.md) |

For comprehensive descriptions of all 135 rules, see [docs/packs/INDEX.md](docs/packs/INDEX.md).

Findings carry rule IDs such as `android/debuggable` or `go/command-injection`. Launcher activities are exempt from the exported-component rule, because Android requires them to be exported.

### Per-Rule Enforcement Policy (`.sentinix/policy.json`)

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

- **Block:** Gate fails on pull request (exit code `1`).
- **Warn:** Displays a warning comment without failing the gate.
- **Notice:** Recorded in findings and audit log for visibility.
- **Ignore:** Rule is skipped entirely.

Policies can also be configured dynamically via the REST API (`PUT /api/projects/:id/policy`) and Web Dashboard.

### Team Custom Rules (`.sentinix/custom-rules.json`)

Teams can declare repository-specific patterns directly in their project:

```json
{
  "rules": [
    {
      "id": "team/no-raw-http-client",
      "severity": "high",
      "cwe": "CWE-693",
      "message": "Use SecureHttpClient rather than raw OkHttpClient",
      "remediation": "Replace with SecureHttpClient.instance",
      "files": ["*.kt", "*.java"],
      "pattern": "OkHttpClient\\.Builder\\(\\)"
    }
  ]
}
```

Sentinix scans custom rules alongside built-in packs with identical sub-millisecond evaluation and baseline support.

### Writing your own

A pack is a JSON file such as `mypack.json`:

```json
{
  "name": "my-rules",
  "version": "1.0.0",
  "stacks": ["kotlin"],
  "author": "Platform team",
  "rules": [
    {
      "id": "no-eval",
      "severity": "high",
      "cwe": "CWE-95",
      "message": "eval() on dynamic input",
      "remediation": "Parse the input instead of evaluating it.",
      "files": ["*.kt", "*.java"],
      "pattern": "\\beval\\("
    }
  ],
  "tests": [
    {"rule": "no-eval", "file": "A.kt", "bad": "eval(userInput)", "good": "parse(userInput)"}
  ]
}
```

| Field | Meaning |
| --- | --- |
| `id`, `severity`, `message` | Required. Severity is `info`, `low`, `medium`, `high` or `critical` |
| `cwe`, `remediation` | Shown in findings and reports |
| `files` | Filename globs, for example `*.kt` or `AndroidManifest.xml` |
| `path_contains` | Only files whose path contains this text, for example `commonMain` |
| `pattern` | A Go (RE2) regular expression |
| `unless` | Skip the hit when the matched text (the line, or the whole match for `multiline`) matches this |
| `multiline` | Apply `pattern` to the whole file instead of line by line |
| `stacks` | Which detected stacks the pack applies to. Empty means all |

**Every rule must come with a test.** A pack is rejected unless each rule's `bad` sample is detected and its `good` sample is not. That single requirement keeps community and in-house packs honest.

### Signing and trusting packs

Signing proves a pack came from someone you trust and has not been altered.

```bash
./sentinix packs keygen mykey               # writes mykey.key (private) and mykey.pub
```

```bash
./sentinix packs sign mypack.json mykey.key > mypack.signed.json
```

```bash
./sentinix packs verify mypack.signed.json mykey.pub
```

Put signed packs in a directory and list the public keys you trust, one base64 key per line:

```bash
./sentinix scan -packs ./packs -trust trusted-keys.pub .
```

`sentinixd` accepts the same `-packs` and `-trust` flags. A pack with an untrusted signature, a failing test or a broken regular expression stops startup, because a half-loaded rule set is worse than none. Keep private keys out of source control.

To disable a rule or change its severity at runtime, use an approval ([section 9](#9-users-roles-and-approvals)).

---

## 8. Dependencies, SBOM and advisories

Sentinix builds a software bill of materials (SBOM) from:

- `go.mod`
- `package-lock.json`, or exact versions in `package.json`
- `requirements.txt` (`==` pins)
- Gradle build files and `libs.versions.toml`
- `pom.xml`

Unpinned or variable versions are skipped rather than guessed. A wrong guess would create false alarms or hide real ones.

### Turning on advisory matching

This is **off by default** because it sends package names and versions (never your code) to an OSV service.

```bash
./sentinixd -osv https://api.osv.dev …
```

```bash
./sentinix scan -osv https://api.osv.dev .
```

If package names must stay inside your network, point `-osv` at a self-hosted OSV mirror instead. Each advisory becomes a finding named `advisory/<id>`, with the advisory's severity and a suggested fixed version. If the lookup itself fails, you get an error, never a silent "no vulnerabilities".

### Re-checking without rescanning

The server keeps each project's dependency list from its latest full scan and re-matches it every `-advisory-interval`, or on demand with `POST /api/advisories/rescan`. When a new advisory lands that affects something you ship, it opens a finding and triggers an alert, without cloning a single repository. Advisories that no longer apply are marked fixed.

Export the SBOM as CycloneDX 1.5, with open advisories and fixed versions attached: `GET /api/projects/{id}/report/sbom`.

---

## 9. Users, roles and approvals

People sign in through OIDC and start as **Developer**. The identity in `SENTINIX_OWNER_EMAIL` becomes **Owner** on first sign-in.

| Role | Can do |
| --- | --- |
| **Owner** | Everything, including granting the Owner role and AI test generation |
| **Admin** | Manage projects, tokens, users and settings; read; view the audit log |
| **Security reviewer** | Triage findings, run full scans, propose and decide approvals, ticket findings, run AI triage; read; view the audit log |
| **Developer** | Read |
| **Auditor** | Read; view the audit log |

### Approvals: no one grades their own homework

These changes wait in the **Approvals** queue until a *different* Reviewer or Owner approves them:

- accepting a risk (`accept_risk`)
- disabling a rule (`disable_rule`)
- loosening a rule's severity (`loosen_severity`)

The person who proposed a change cannot approve it. Every proposal and decision is written to the audit log. This is the mechanism that stops a deadline-pressured team from quietly switching off the check that was inconvenient.

### API tokens

An Admin or Owner creates a token for one project. It is shown once, stored hashed, and meant for CI. A token can upload scans for its own project and read that project's findings. It can do nothing else.

### The audit log

The audit log records scans, triage, approvals, role changes, token creation, tickets and AI actions, each with the actor, target and commit where one applies. Auditors can read it without being able to change anything.

---

## 10. Reports and exports

Reports are designed for different readers: developers want to know what to fix, reviewers want a queue, managers want a summary, auditors want evidence.

`GET /api/projects/{id}/report/{format}`, optionally with `?status=open` (the default), `fixed`, `accepted` or `false_positive`.

| Format | Content type | Best for |
| --- | --- | --- |
| `html` | `text/html` | A print-ready report with summary, coverage statement, and findings with CWE and OWASP Top 10 mapping. Use the browser's **Save as PDF** |
| `csv` | `text/csv` | Spreadsheets. Cells starting with `=`, `+`, `-` or `@` are prefixed so a spreadsheet never evaluates them |
| `sarif` | `application/sarif+json` | Code-scanning tools |
| `json` | `application/json` | Feeding other systems |
| `sbom` | `application/vnd.cyclonedx+json` | A CycloneDX SBOM with vulnerabilities and fixed versions |

### The coverage statement

Every HTML report states the commit and mode of the last scan, the engines that ran, the rule pack versions, and a reminder that a clean report is not proof of security. It also lists what was not covered. This is deliberate: a report should never be able to be read as more reassuring than it is.

### Posture

The posture view shows open findings by severity, new versus fixed per week, average time to fix, and the oldest open criticals. There is intentionally no single "security score": one critical plus fifty lows can produce a number that misleads.

To rebrand HTML reports for clients, set `SENTINIX_BRAND_NAME` and `SENTINIX_BRAND_ACCENT`.

---

## 11. Integrations

Pass `-integrations integrations.json` to the server:

```json
{
  "slack": "$SLACK_WEBHOOK_URL",
  "teams": "$TEAMS_WEBHOOK_URL",
  "only_critical": true,
  "public_url": "https://sentinix.example.com",
  "webhooks": [{"url": "https://hooks.example.com/sentinix", "secret": "$HOOK_SECRET"}],
  "jira":   {"base": "https://yourorg.atlassian.net", "email": "bot@example.com", "token": "$JIRA_TOKEN", "project": "SEC"},
  "linear": {"key": "$LINEAR_API_KEY", "team": "<team id>"}
}
```

- **Slack and Teams:** a message after each recorded scan that found something new. With `only_critical`, a message is sent only when a new critical finding appears, which keeps channels useful.
- **Outbound webhooks:** a JSON body `{project, scan, commit, mode, new: [{Rule, File, Severity}], critical}`, signed in the `X-Sentinix-Signature: sha256=<hex>` header (HMAC-SHA256 of the body with your secret). Always verify the signature on the receiving side.
- **Tickets:** on a finding's page, Security reviewers and above can create a Jira or Linear ticket. Its key is stored on the finding, and a finding gets one ticket.

---

## 12. AI add-on (optional)

The AI add-on helps with the work that is hard to automate: deciding whether a finding is real, drafting a fix, and writing a regression test. It is off by default, the core never needs it, and nothing is sent to a model until you configure models *and* switch it on for a project.

### Configure models

Pass `-ai models.json`. Any OpenAI-compatible chat endpoint works: Ollama, LM Studio, vLLM, or a hosted API.

```json
[
  {"name": "local-qwen", "url": "http://localhost:11434/v1", "model": "qwen2.5-coder", "local": true},
  {"name": "cloud-a", "url": "https://api.provider.example/v1", "model": "model-id", "key": "$PROVIDER_KEY", "local": false}
]
```

### Choose a model policy per project

The policy decides where a code snippet is allowed to go.

| Policy | Models used | What the model sees |
| --- | --- | --- |
| `local-only` (default) | Only models marked `local: true` | The snippet as is |
| `cloud-redacted` | All | The snippet with secrets masked first |
| `cloud` | All | The snippet as is |

### What it can do

These actions appear as buttons on a finding.

- **AI triage** (medium severity and above, Security reviewer and above): each allowed model says whether the finding is real. You get a council verdict with severity, confidence, reasoning and which models disagreed. It is advisory. It never dismisses a finding on its own.
- **Draft patch:** a unified diff for the finding's file only, size-limited. It is shown to you and never applied or committed automatically.
- **Test generation** (`POST /api/findings/{id}/ai/test`, Owner only, needs the sandbox): proposes a patch and a regression test, runs the command you give it in the sandbox against the vulnerable code and the patched code, and keeps both only if the test **fails before and passes after**. A test that passes on the vulnerable code proves nothing, so it is discarded.

### Why prompt injection does not get far

Only a short snippet around the finding is sent. It is wrapped and labelled as untrusted data, models have no tools, no shell and no network access of their own, and everything they return is parsed as data and checked before use. A malicious comment in a repository can at worst produce a bad suggestion that a human has to accept.

---

## 13. Sandbox

By default engines run on the server host. That is fine for your own repositories. For third-party or untrusted code, run them in a container:

```bash
sentinixd -sandbox-image sentinix-scanner:latest …
```

External engines then run with:

- no network
- all Linux capabilities dropped and `no-new-privileges`
- a read-only source mount
- a non-root user
- CPU, memory and process limits

The Docker socket is never mounted. Where you can, use a rootless runtime (`SENTINIX_CONTAINER_BIN=podman`). Generated tests run in the same locked-down container.

Build the scanner image from `deploy/Dockerfile.scanner`. Because the sandbox has no network, Semgrep rules must be baked into the image (`/rules`), and `SENTINIX_SEMGREP_CONFIG=/rules` must be set on the server.

The built-in secrets scanner and rule packs run in the server process; they only read files.

---

## 14. Deploying

| File | Purpose |
| --- | --- |
| `deploy/Dockerfile` | Server image. From `sentinix-ai/`: `docker build -f deploy/Dockerfile -t sentinix .` |
| `deploy/Dockerfile.scanner` | Scanner image for the sandbox |
| `deploy/docker-compose.yml` | Server plus Postgres. Create an untracked `.env` with the variables from [section 4](#4-how-to-run-the-server) and `POSTGRES_PASSWORD` |
| `deploy/helm/sentinix` | A minimal Helm chart. Secrets come from an existing Kubernetes Secret (`SENTINIX_DB`, `SENTINIX_SESSION_KEY`, host tokens, `OIDC_*`) |
| `deploy/ci/` | CI examples for GitHub Actions, GitLab CI and Bitbucket Pipelines |

**Operational notes**

- Health check: `GET /healthz`.
- Put TLS in front of the server with a reverse proxy or an ingress.
- Back up the database. It holds findings, approvals and the audit log.
- Upgrading is replacing the binary or image. Tables are created and extended on start.

---

## 15. CI usage without the server

The command-line tool works in any pipeline on any git server, including ones the server does not support.

```bash
sentinix scan -baseline .sentinix-baseline.json -fail-on high -sarif sentinix.sarif .
```

To also record the run on the server (for trends, the findings inbox and tickets), create a project API token and add the server address and project:

```bash
export SENTINIX_TOKEN=<project token>
sentinix scan -server https://sentinix.example.com -project <project id> .
```

A scan is **not** uploaded if an engine failed, so the server never marks findings as fixed on the strength of a partial run. Ready-made examples for GitHub Actions, GitLab CI and Bitbucket Pipelines are in `deploy/ci/`.

---

## 16. Reference

### Command-line tool

| Command | Purpose |
| --- | --- |
| `sentinix scan [flags] [path]` | Scan a directory (default `.`) |
| `sentinix surface [base-ref]` | Attack-surface items in `git diff base-ref...HEAD` (default `origin/main`) |
| `sentinix packs keygen <prefix>` | Create an ed25519 key pair |
| `sentinix packs sign <body.json> <key>` | Sign a pack (refuses one whose tests fail). Signed JSON goes to stdout |
| `sentinix packs verify <signed.json> <pubkeys>` | Check a signature and the pack's tests |
| `sentinix bench <dir>` | Score the built-in packs on a labelled sample project |

`scan` flags: `-baseline <file>`, `-update-baseline`, `-fail-on <severity>`, `-report-only`, `-sarif <file>`, `-packs <dir>`, `-trust <file>`, `-osv <url>`, `-server <url>`, `-project <id>`, `-token-env <name>` (default `SENTINIX_TOKEN`).

### HTTP API

Authenticate with a session cookie (the dashboard) or `Authorization: Bearer <project token>`. Writes made with a cookie must send `X-Requested-With: sentinix`, which protects against cross-site request forgery. Errors are plain text with a status code.

| Method and path | Needs | Purpose |
| --- | --- | --- |
| `GET /healthz` | nothing | Liveness |
| `POST /webhook/{name}` | signature | Git host webhooks |
| `GET /auth/login`, `/auth/callback`, `POST /auth/logout` | nothing | OIDC sign-in |
| `GET /api/me` | read | The current user |
| `GET /api/posture?project=` | read | Posture numbers |
| `GET /api/projects`, `POST /api/projects` | read, manage | List, create |
| `PATCH /api/projects/{id}` | manage | `Enforcement` (`block`\|`report`), `FailOn`, `Policy`, `Branch`, `Schedule`, `TZ`, `ModelPolicy`, `AI`, `PackPins` |
| `GET /api/projects/{id}/policy` | read | Get effective per-category policies |
| `PATCH /api/projects/{id}/policy` | manage | Update category policies (loosening requires approval) |
| `POST /api/projects/{id}/scan` | scan | Queue a full scan (`{"Branch": "x"}` optional) |
| `GET /api/projects/{id}/scans`, `/surface`, `/sbom` | read | Run history, attack surface, components |
| `POST /api/projects/{id}/tokens` | manage | Create a CI token (`{"Name": "ci"}`) |
| `POST /api/projects/{id}/ingest` | project token | Upload CLI scan results, SARIF (Detekt, ktlint, Lint), or JUnit XML (Konsist) |
| `GET /api/projects/{id}/report/{format}` | read | [Reports](#10-reports-and-exports) |
| `GET /api/findings` | read | Filters: `project`, `status`, `severity` (minimum), `owner`, `limit` |
| `GET /api/findings/{id}` | read | The finding, plus its AI verdict if any |
| `POST /api/findings/{id}/triage` | triage | `{"status","owner","reason","until"}`. `until` is a Unix time. `accepted` goes to approvals |
| `POST /api/findings/{id}/ticket` | triage | `{"tracker":"jira"\|"linear","due":"YYYY-MM-DD"}` |
| `POST /api/findings/{id}/ai/triage`, `/ai/patch` | triage | [AI add-on](#12-ai-add-on-optional) |
| `POST /api/findings/{id}/ai/test` | owner | Verified test generation |
| `GET /api/approvals?status=`, `POST /api/approvals` | read, triage | The queue. Propose `disable_rule` or `loosen_severity` |
| `POST /api/approvals/{id}/decide` | approve | `{"approve": true\|false}` |
| `GET /api/audit?limit=` | audit | The audit log |
| `GET /api/users`, `PUT /api/users/role` | manage | Users. `{"email","role"}`. Only an Owner can grant or remove Owner |
| `GET /api/packs` | read | Loaded packs, rule precision, disabled rules |
| `POST /api/advisories/rescan` | scan | Re-match all dependency lists now |

---

## 17. Security model

A security tool that reads untrusted code is itself a target, so these are requirements, not options.

- **Scanned content is untrusted.** Checkouts are shallow, temporary and deleted after each scan. Git tokens travel only over HTTPS, only for fetching, and are scrubbed from error output. External engines get a minimal environment with no server tokens.
- **Forks are untrusted.** Fork pull requests are fetched without a token.
- **Packs are signed data.** They cannot execute code or use the network, and each rule ships with tests.
- **No silent weakening.** Loosening rules or accepting risk needs a different person and is audited.
- **Secrets are masked.** Secrets found in code are masked in findings, reports and logs, and redacted before a cloud model sees a snippet.
- **The dashboard is hardened.** It is served with a strict Content Security Policy, builds the page with `textContent` only, and uses no inline script, so a malicious finding cannot run code in a reviewer's browser. HTML reports escape every value.
- **Sessions are protected.** They are HMAC-signed, `HttpOnly`, `SameSite=Lax`, and `Secure` unless you opt out for local HTTP.
- **Defaults are passive and local.** Advisories, AI, the sandbox and integrations are all off until you configure them.

---

## 18. Testing and benchmarks

Run the full test suite:

```bash
cd sentinix-ai && go test ./...
```

`bench/vulnerable-kmp/` is a small Kotlin, Android and KMP project with planted bugs (listed in `expected.json`) and a `clean/` folder of safe look-alikes designed to catch false positives:

```bash
./sentinix bench bench/vulnerable-kmp
```

It prints the planted bugs, the findings, precision and recall, and exits non-zero below 80% precision or 70% recall.

**Read this number with care.** The sample was written alongside the rules, so it protects against regressions; it is not an independent measurement of accuracy. The best evidence is your own code: run Sentinix on your projects, mark false positives, and use the per-rule precision on the **Rule packs** screen to find rules that need tightening.

---

## 19. Troubleshooting

| Symptom | What to check |
| --- | --- |
| Webhook delivery shows 401 | The secret in `sentinix.json` differs from the one set on the git host |
| Webhook delivery shows 404 | The path name does not match a `name` in `sentinix.json` |
| Status ends as "scan error" | The server log. Usually the git fetch failed: check the token's scope and its access to the repository |
| No PR comment or status appears | The token lacks Commit statuses or Pull requests write. Check the git host's webhook delivery log |
| The status is green but the PR has findings | The project is in report-only mode ([section 5](#choose-block-or-report-only)), or the findings are below the project's *Fail at* threshold |
| A PR can merge despite a failing status | The `sentinix` status is not marked as a required check on the git host |
| A PR scan is missing from the dashboard | Register the project (host name and `owner/name`) under Settings |
| Cannot sign in | `OIDC_ISSUER` is unset, the redirect address is wrong, or the identity provider returns an unverified email |
| Login loops over HTTP | Set `SENTINIX_INSECURE_COOKIES=1`, for local trials only |
| `SENTINIX_SESSION_KEY must be at least 32 characters` | Generate one with `openssl rand -hex 32` |
| Findings never become fixed | The engine that found them did not run in later scans, for example a missing binary |
| Advisories are missing | `-osv` is not set, or the OSV address is unreachable (the scan then errors rather than reporting clean) |
| A pack is rejected at startup | Untrusted signature, a rule without a test, or a failing sample. Run `sentinix packs verify` |
| The report says "engines not installed" | Install `semgrep`, `gitleaks` and `osv-scanner`, or use the scanner image |

---

## 20. Limits and what is not included

Being clear about the edges is part of being trustworthy.

- **Developer visibility:** developers can see findings for every project, not only their own.
- **Host support:** Bitbucket Server, Data Center and Gitea are not supported by the server.
- **Isolation:** engines run on the host unless `-sandbox-image` is set.
- **Comments and caching:** a new comment is posted on every push, and results are not cached by file hash.
- **Scaling:** one server replica only, because scheduled scans run in the process.
- **Depth of analysis:** rule packs are pattern based. They find known risky constructs, not data flows or business-logic flaws.
- **Dashboard:** it is a plain-JavaScript page served by the server, not a separate TypeScript application. SCIM provisioning, logo branding and a native PDF renderer are not included.
- **Platforms:** iOS binary analysis, Flutter and Dart AOT analysis, and runtime mobile instrumentation are not covered.
- **Not a substitute:** Sentinix does not replace a human security review for regulated products.

---

## 21. Frequently asked questions

**Does my code leave my network?**
Not by default. The core scans locally and uses no model. Code snippets reach a model only if you configure the AI add-on, switch it on for a project, and your model policy allows a non-local model. Dependency advisory lookups send package names and versions only, and only when you set `-osv`; a self-hosted mirror keeps even that inside your network.

**Will it block my team on day one with years of old findings?**
No. Pull requests are compared with their base branch, so only what a change introduces is reported. For the CLI, create a baseline first ([section 3](#adopting-sentinix-on-an-existing-codebase)).

**What if a finding is wrong?**
Mark it as a false positive. Over time the **Rule packs** screen shows which rules are dismissed most, and rules with low precision are shown as informational so they stop blocking work while you decide what to do with them.

**Can I accept a risk I cannot fix right now?**
Yes. Accepting a risk needs a written reason, an expiry date and a second person's approval. When the date passes, the finding reopens so the decision is revisited.

**Can I run it without blocking anyone?**
Yes. Set the project to **report only**, or use `-enforcement report` as the server default, or `-report-only` in the CLI. Sentinix then comments with the findings and the status always passes. You can switch a project to blocking later without changing anything else, and you can set a different threshold per project.

**Can a developer disable a noisy rule to get a PR through?**
Not alone. Disabling or loosening a rule goes through the approval queue and needs someone other than the proposer.

**We use a git server that is not listed.**
Use the command-line tool in your CI pipeline ([section 15](#15-ci-usage-without-the-server)). It does not depend on the git host.

**Do I need the AI add-on?**
No. Everything in the core works without a model, and results are repeatable. The add-on is for teams who want help triaging and fixing.

**How do I write rules for my own framework?**
Write a rule pack ([section 7](#7-rule-packs)): a JSON file of patterns with a good and a bad example for every rule. Sign it and distribute it to your projects.

**How can I tell how accurate it is?**
Run it on your own projects and use the per-rule precision it records. The bundled benchmark guards against regressions but was written with the rules, so treat it as a floor, not a guarantee.
