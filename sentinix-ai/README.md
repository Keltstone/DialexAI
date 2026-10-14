# Sentinix AI

Sentinix AI is a defensive code security monitor. It reviews every pull request or merge request, runs scheduled and manual full scans, tracks findings over time, and says in every report what it did and did not check. It reads code and diffs in repositories you own. It never probes or attacks live systems.

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

---

## 1. What it is

Linters ask whether code is clean, one file at a time. Sentinix asks whether a change made the product less safe, and who needs to act.

| Need | Linters | Sentinix |
| --- | --- | --- |
| Existing code | Report everything on every run | Baselines and head-vs-base comparison: a PR shows only what it introduced |
| Dependencies | Not checked | SBOM matched against advisories, re-checked without rescanning code |
| Secrets | Limited | Secrets in source, masked in every output |
| Visibility | Per-repo CI logs | One dashboard with trends, owners and time to fix |
| Governance | None | SSO, roles, approvals with separation of duties, audit log |
| Workflow | Warnings that scroll past | One pass/fail gate, tickets, chat alerts |

**Two tiers.** The core is deterministic: the same code and the same rule pack versions give the same findings, with no model and no network, so it is safe as a CI gate and in air-gapped networks. An **AI add-on** (off by default) adds triage, patch drafts and test generation on top; its output is advisory.

**Supported git hosts:** GitHub and GitHub Enterprise, GitLab (cloud and self-managed), Bitbucket Cloud. Any other git server works through the CLI in CI ([section 15](#15-ci-usage-without-the-server)).

**Out of the box it checks** Kotlin, Android and Kotlin Multiplatform projects (manifest settings, WebView misuse, insecure storage, weak crypto, SQL built from strings, shared-code pitfalls), Ktor and Spring misconfigurations, hardcoded secrets, and, when enabled, known-vulnerable dependencies for Go, npm, PyPI and Maven. `semgrep`, `gitleaks` and `osv-scanner` are used automatically when installed.

## 2. How it works

```
trigger ──► surface map ──► engines ──► policy ──► AI (optional) ──► report
```

| Mode | Trigger | Scope | Result |
| --- | --- | --- | --- |
| PR / MR review | Webhook | Head commit compared with the base branch | Comment and commit status on the PR |
| Full scan | "Scan now", cron schedule, or CI upload | Whole branch | Findings tracked as new, fixed or reopened |
| Advisory re-scan | Timer or API call | Stored SBOMs only | New dependency advisories, no code fetched |

A PR review:

1. Verifies the webhook signature, then sets a pending status.
2. Shallow-fetches the head commit and the base branch into a throwaway directory. The git token is used only for the fetch and never reaches a scan process. Fork heads are fetched without a token.
3. Runs every engine on head and on base. Only findings that are not in the base are reported, so existing issues never block a PR.
4. Diffs head against base into attack-surface items (new or removed endpoints, permissions, dependency lines).
5. Posts a comment, sets the status (failing at or above `-fail-on`), and deletes the checkout. Only findings, short evidence and metadata are stored.

A finding's identity is a fingerprint of rule, file and masked evidence, not the line number, so unrelated edits do not make old findings look new.

**Components**

| Path | Role |
| --- | --- |
| `cmd/sentinixd` | Server: webhooks, API, scheduler, dashboard |
| `cmd/sentinix` | CLI: `scan`, `surface`, `packs`, `bench` |
| `pkg/engines` | Engine interface, built-in secrets scanner, SARIF wrappers |
| `pkg/packs` | Rule pack loader, signing, built-in Kotlin/Android/KMP packs |
| `pkg/githost` | GitHub, GitLab and Bitbucket providers |
| `pkg/scheduler` | PR review, full scan, cron |
| `pkg/store` | Postgres or SQLite persistence |
| `pkg/server`, `pkg/web` | HTTP API and the dashboard |
| `pkg/advisories` | SBOM extraction and OSV matching |
| `pkg/report`, `pkg/integrations` | Exports, chat, webhooks, tickets |
| `pkg/ai`, `pkg/sandbox` | Optional AI add-on and container sandbox |
| `bench/` | Labelled sample project for precision and recall |

## 3. Quick start: scan a project from the CLI

Requirements: Go 1.27 and `git`. The CLI needs no server, database or network.

```bash
cd sentinix-ai && go build -o sentinix ./cmd/sentinix
```

Scan a directory:

```bash
./sentinix scan /path/to/your/project
```

Example output:

```
[medium] android/exported-component app/src/main/AndroidManifest.xml:10 Exported component without a permission (launcher activities are exempt)
[high] secret.aws-access-key config/Settings.kt:12 possible hardcoded secret
2 finding(s). Engines run: secrets, packs. Not covered: engines not installed, AI triage.
```

The exit code is 1 when any finding is at or above `-fail-on` (default `high`), so the same command works as a CI gate. Exit codes: `0` clean at the gate, `1` gate tripped, `2` usage or fatal error, `3` an engine failed (a broken engine never looks like a clean scan).

**Baselines: adopt it on an existing codebase.** Record today's findings once, then fail only on new ones:

```bash
./sentinix scan -baseline .sentinix-baseline.json -update-baseline .
```

```bash
./sentinix scan -baseline .sentinix-baseline.json -fail-on high .
```

Commit `.sentinix-baseline.json`. Entries are fingerprints, so they survive line shifts.

**Suppress one line** (for example a documented test fixture) by adding `sentinix:ignore` anywhere on that line. This applies to the built-in secrets scanner.

**More engines:** if `semgrep`, `gitleaks` or `osv-scanner` is on your `PATH`, they run automatically and their SARIF output is merged into the same schema. The last line of the output lists the engines that ran.

**SARIF output** for code-scanning UIs: `-sarif out.sarif`.

**Attack surface of a branch:**

```bash
./sentinix surface origin/main
```

lists added and removed endpoints, permissions and dependency lines in `git diff origin/main...HEAD`.

## 4. How to run the server

The server gives you PR reviews, scheduled scans, the dashboard, roles, approvals and reports.

**Requirements:** a reachable host or container, a database (SQLite for a trial, Postgres for production), a git host token per host, and an OIDC provider if you want the dashboard.

**1. Build and configure**

```bash
cd sentinix-ai && go build -o sentinixd ./cmd/sentinixd
```

Create `sentinix.json` listing your git hosts ([section 5](#5-how-to-connect-a-git-host)). Values written as `$NAME` are read from the environment, so secrets stay out of the file.

**2. Required environment**

```bash
export SENTINIX_SESSION_KEY=$(openssl rand -hex 32)   # 32+ characters, signs session cookies
```

**3. Optional sign-in (needed for the dashboard)**

```bash
export OIDC_ISSUER=https://idp.example.com
export OIDC_CLIENT_ID=<client id>
export OIDC_CLIENT_SECRET=<client secret>
export OIDC_REDIRECT_URL=https://sentinix.example.com/auth/callback
export SENTINIX_OWNER_EMAIL=you@example.com   # this identity becomes Owner on first sign-in
```

Any OIDC provider works (Keycloak, Authentik, Okta, Entra ID, Google). For SAML, put Keycloak or Authentik in front and connect Sentinix to it over OIDC. Without `OIDC_ISSUER`, PR reviews, webhooks and CI token uploads still work; only the dashboard login is disabled. Over plain HTTP (local trials only) add `SENTINIX_INSECURE_COOKIES=1`; cookies are `Secure` by default.

**4. Start**

```bash
./sentinixd -config sentinix.json -db sqlite:sentinix.db -addr :8080
```

```bash
./sentinixd -config sentinix.json -db "postgres://user:pass@host:5432/sentinix?sslmode=require"
```

Tables are created on start. Open `http://localhost:8080`, sign in, and add a project under **Settings** (host name from `sentinix.json`, repository as `owner/name`).

**Server flags**

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `sentinix.json` | Git host list |
| `-addr` | `:8080` | Listen address |
| `-db` | `sqlite:sentinix.db` | `postgres://…` or `sqlite:<path>` |
| `-workers` | `2` | Concurrent scans |
| `-fail-on` | `high` | PR status fails at or above this severity: `info`, `low`, `medium`, `high`, `critical` |
| `-packs`, `-trust` | none | Extra signed rule packs and trusted public keys ([section 7](#7-rule-packs)) |
| `-osv` | off | OSV API URL for dependency advisories ([section 8](#8-dependencies-sbom-and-advisories)) |
| `-advisory-interval` | `6h` | How often stored SBOMs are re-matched |
| `-integrations` | none | Chat, webhook and ticket settings ([section 11](#11-integrations)) |
| `-ai` | none | Model list for the AI add-on ([section 12](#12-ai-add-on-optional)) |
| `-sandbox-image` | none | Run external engines and generated tests in this container image ([section 13](#13-sandbox)) |

**Other environment variables**

| Variable | Purpose |
| --- | --- |
| `SENTINIX_BRAND_NAME`, `SENTINIX_BRAND_ACCENT` | White-label name and `#rrggbb` accent in HTML reports |
| `SENTINIX_CONTAINER_BIN` | `podman` instead of `docker` for the sandbox |
| `SENTINIX_SEMGREP_CONFIG` | Semgrep rules path or `auto` (default); `auto` needs network |

Run one server replica: scheduled scans run inside the process.

## 5. How to connect a git host

`sentinix.json` is a list. Each entry becomes a webhook endpoint `POST /webhook/<name>`.

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
| `name` | Used in the webhook path and as a project's `host` |
| `kind` | `github`, `gitlab` or `bitbucket` |
| `api` | API base URL. Defaults: `https://api.github.com`, `https://gitlab.com/api/v4`, `https://api.bitbucket.org/2.0`. Set it for GitHub Enterprise and self-managed GitLab |
| `web_url` | Web root used for clone URLs. Derived from `api` when empty |
| `secret` | Webhook secret (GitHub and Bitbucket sign the body; GitLab sends a secret token) |
| `token` | Token used to fetch code, set statuses and post comments |
| `public_url` | Linked from Bitbucket build statuses (your Sentinix URL) |

A host with an empty `secret` or `token` is refused at startup.

**Token permissions**

| Host | Needs |
| --- | --- |
| GitHub | Contents: read; Commit statuses: write; Pull requests: write |
| GitLab | A project or group access token with `api` scope (read repository, statuses, merge request notes) |
| Bitbucket Cloud | Repository read and write for statuses; Pull requests write (repository or workspace access token) |

**Webhook setup**

| Host | Where | Events | Secret |
| --- | --- | --- | --- |
| GitHub | Settings, Webhooks, content type `application/json` | Pull requests | Secret field |
| GitLab | Settings, Webhooks | Merge request events | Secret token |
| Bitbucket | Repository settings, Webhooks | Pull request created, updated | Secret |

Payload URL: `https://<your-sentinix-host>/webhook/<name>`. Reviews run on opened, updated (new commits) and reopened PRs/MRs. The server must be reachable from the git host; behind a firewall, use a reverse proxy or a tunnel.

**Making it a gate:** mark the `sentinix` status as a required check in branch protection (GitHub, GitLab) or required build (Bitbucket).

Not supported by the server: Bitbucket Server/Data Center and Gitea. Use the CLI in CI for those.

## 6. Projects, scans and findings

**Project settings** (Settings, or `PATCH /api/projects/{id}`)

| Setting | Meaning |
| --- | --- |
| Branch | Branch used for full scans (default `main`) |
| Schedule | `nightly` (02:00), `weekly` (Sunday 03:00), or a 5-field cron such as `30 1 * * 1-5`; empty disables |
| Timezone | IANA name such as `Europe/Berlin`; empty is UTC |
| AI add-on, model policy | See [section 12](#12-ai-add-on-optional) |

Bad schedules are rejected when saved, not at 2 a.m.

**Scans.** "Scan now" (Security reviewer and above) and schedules run a full scan of the branch. Each run records commit, mode, duration, engines, rule pack versions and counts of new, fixed and unchanged findings compared with the previous run.

**Finding lifecycle**

- `open`: reported by the latest scan.
- `fixed`: an engine that ran no longer reports it. Engines that did not run never mark anything fixed, so a missing binary cannot look like a clean repo.
- It **reopens** if it returns.
- `false_positive`: dismissed by a reviewer.
- `accepted`: accepted risk. Needs a reason and an expiry date, and a second person's approval ([section 9](#9-users-roles-and-approvals)). When the expiry passes the finding reopens on the next scan.

**Per-rule precision.** Each rule's findings and dismissals over 90 days are shown under **Rule packs**. A rule with at least 10 findings and under 50% precision is shown as informational and flagged. It is never deleted automatically.

**Attack surface.** The **Attack surface** screen lists endpoints, permissions and dependency changes seen in reviewed PRs of registered projects.

## 7. Rule packs

Packs are the main extension point. A pack is data only: it cannot run code or reach the network.

**Built in** (always loaded, chosen by detected stack):

| Pack | Applies when | Rules |
| --- | --- | --- |
| `android` | `AndroidManifest.xml` or a Gradle file using `com.android.` | Exported components, backup, cleartext traffic, `debuggable`, WebView JS interface / file access / SSL bypass, world-accessible files, secrets in SharedPreferences, trust-all certificates, weak crypto, SQL string templates |
| `kotlin-backend` | Kotlin files | Spring CSRF disabled and `permitAll`, CORS any origin, exposed actuator endpoints, SQL string templates, weak crypto |
| `kmp` | `commonMain` directory or the multiplatform Gradle plugin | Cleartext URLs, secrets printed, secrets in plain Settings in shared code |

Finding rule IDs look like `android/debuggable`. Launcher activities are exempt from the exported-component rule.

**Writing a pack** (`mypack.json`):

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

Rule fields:

| Field | Meaning |
| --- | --- |
| `id`, `severity`, `message` | Required. Severity: `info`, `low`, `medium`, `high`, `critical` |
| `cwe`, `remediation` | Shown in findings and reports |
| `files` | Basename globs, for example `*.kt` or `AndroidManifest.xml` |
| `path_contains` | Only files whose path contains this text, for example `commonMain` |
| `pattern` | Go (RE2) regular expression |
| `unless` | Skip the hit when the matched text (the line, or the whole match for `multiline`) matches this |
| `multiline` | Apply `pattern` to the whole file instead of line by line |

**Every rule needs a test.** The pack is rejected unless each rule's `bad` sample is detected and its `good` sample is not. `stacks` may be empty (all stacks).

**Signing and using external packs**

```bash
./sentinix packs keygen mykey               # writes mykey.key (private) and mykey.pub
```

```bash
./sentinix packs sign mypack.json mykey.key > mypack.signed.json
```

```bash
./sentinix packs verify mypack.signed.json mykey.pub
```

Put signed packs in a directory and trust the public key (one base64 key per line):

```bash
./sentinix scan -packs ./packs -trust trusted-keys.pub .
```

`sentinixd` takes the same `-packs` and `-trust` flags. A pack with an untrusted signature, a failing test or a bad regex stops startup: a half-loaded rule set is worse than none. Keep the private key out of the repository.

**Changing rules at runtime** (Disable, loosen severity) is done through approvals ([section 9](#9-users-roles-and-approvals)).

## 8. Dependencies, SBOM and advisories

Sentinix builds a software bill of materials from `go.mod`, `package-lock.json` (or exact versions in `package.json`), `requirements.txt` (`==` pins), Gradle build files and `libs.versions.toml`, and `pom.xml`. Unpinned or variable versions are skipped, never guessed.

Advisory matching is **off by default** because it sends package names and versions (never code) to an OSV service. Turn it on:

```bash
./sentinixd -osv https://api.osv.dev …
```

```bash
./sentinix scan -osv https://api.osv.dev .
```

Use a self-hosted OSV mirror URL to keep lookups inside your network. Each advisory becomes a finding `advisory/<id>` with severity from the advisory and a suggested fixed version. A failed lookup is an error, never "no vulnerabilities".

**Re-scans without code.** The server keeps each project's SBOM from its latest full scan and re-matches it every `-advisory-interval`, or on demand (`POST /api/advisories/rescan`). New advisories open findings and trigger alerts; ones that disappear are marked fixed.

Export the SBOM as CycloneDX 1.5 with open advisories attached: `GET /api/projects/{id}/report/sbom`.

## 9. Users, roles and approvals

Users sign in through OIDC and start as **Developer**. The identity in `SENTINIX_OWNER_EMAIL` becomes **Owner**.

| Role | Can do |
| --- | --- |
| Owner | Everything, including granting Owner and AI test generation |
| Admin | Manage projects, tokens, users and settings; read; audit log |
| Security reviewer | Triage findings, run full scans, propose and decide approvals, ticket findings, AI triage; read; audit log |
| Developer | Read |
| Auditor | Read; audit log |

**Approvals (separation of duties).** These changes wait in the **Approvals** queue until a different person with Reviewer or Owner approves:

- accepting a risk (`accept_risk`)
- disabling a rule (`disable_rule`)
- loosening a rule's severity (`loosen_severity`)

The proposer cannot approve their own change. Every proposal and decision is written to the audit log.

**API tokens** are created per project by an Admin or Owner, shown once, stored hashed, and meant for CI. A token can upload scans for its own project and read that project's findings, nothing else.

**Audit log** records scans, triage, approvals, role changes, token creation, tickets and AI actions with actor, target and commit where one applies.

## 10. Reports and exports

`GET /api/projects/{id}/report/{format}` with `?status=open` (default), `fixed`, `accepted` or `false_positive`:

| Format | Content type | Use |
| --- | --- | --- |
| `html` | `text/html` | Print-ready report: summary, coverage statement, findings with CWE and OWASP Top 10 mapping. Use the browser's "Save as PDF" |
| `csv` | `text/csv` | Spreadsheets. Cells starting with `=`, `+`, `-` or `@` are prefixed so spreadsheets never evaluate them |
| `sarif` | `application/sarif+json` | Code-scanning tools |
| `json` | `application/json` | Other systems |
| `sbom` | `application/vnd.cyclonedx+json` | CycloneDX SBOM with vulnerabilities and fixed versions |

Every HTML report carries a **coverage statement**: last scan commit and mode, engines that ran, rule pack versions, and a reminder that a clean report is not proof of security. Rebrand reports with `SENTINIX_BRAND_NAME` and `SENTINIX_BRAND_ACCENT`.

The **posture** view shows open findings by severity, new vs fixed per week, average time to fix and the oldest open criticals. There is deliberately no single posture score.

## 11. Integrations

`-integrations integrations.json`:

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

- **Slack and Teams:** a message after each recorded scan with new findings. With `only_critical`, messages are sent only when a new critical appears.
- **Webhooks:** JSON `{project, scan, commit, mode, new: [{Rule, File, Severity}], critical}` signed in the `X-Sentinix-Signature: sha256=<hex>` header (HMAC-SHA256 of the body with your secret). Always verify it.
- **Tickets:** on a finding page (Security reviewer and above) choose Create Jira or Linear ticket. The key is stored on the finding. A finding gets one ticket.

## 12. AI add-on (optional)

Off by default and never needed for the core. It needs models configured on the server and the project opted in.

`-ai models.json`: any OpenAI-compatible chat endpoint (Ollama, LM Studio, vLLM, hosted APIs):

```json
[
  {"name": "local-qwen", "url": "http://localhost:11434/v1", "model": "qwen2.5-coder", "local": true},
  {"name": "cloud-a", "url": "https://api.provider.example/v1", "model": "model-id", "key": "$PROVIDER_KEY", "local": false}
]
```

**Model policy per project** controls where code snippets may go:

| Policy | Models used | Snippet |
| --- | --- | --- |
| `local-only` (default) | Models marked `local: true` | As is |
| `cloud-redacted` | All | Secrets masked first |
| `cloud` | All | As is |

**What it does** (buttons on a finding; Security reviewer and above, except test generation):

- **AI triage:** medium and above only. Every allowed model answers whether the finding is real. The result is a council verdict with severity, confidence, reasoning and which models disagreed. It is advisory and never dismisses a finding.
- **Draft patch:** a unified diff for the finding's file only, size-limited. Never applied or committed for you.
- **Test generation** (`POST /api/findings/{id}/ai/test`, Owner only, needs the sandbox): proposes a patch and a regression test, runs the test you specify in the sandbox on the vulnerable code and on the patched code, and keeps both only if it fails before and passes after.

Only a short snippet around the finding is sent. It is wrapped and labelled as untrusted data, models have no tools, shell or network, and their output is parsed as data.

## 13. Sandbox

By default engines run on the server host. For third-party or untrusted repositories use a container:

```bash
sentinixd -sandbox-image sentinix-scanner:latest …
```

External engines then run with no network, all capabilities dropped, `no-new-privileges`, a read-only source mount, a non-root user and CPU, memory and process limits. The Docker socket is never mounted. Use a rootless runtime where you can (`SENTINIX_CONTAINER_BIN=podman`). Generated tests run in the same container with no network.

Build the scanner image from `deploy/Dockerfile.scanner`. The sandbox has no network, so Semgrep rules must be baked in (`/rules`) and `SENTINIX_SEMGREP_CONFIG=/rules` set on the server.

The built-in secrets scanner and rule packs run in the server process; they only read files.

## 14. Deploying

| File | Purpose |
| --- | --- |
| `deploy/Dockerfile` | Server image (`docker build -f deploy/Dockerfile -t sentinix .` from `sentinix-ai/`) |
| `deploy/Dockerfile.scanner` | Scanner image for the sandbox |
| `deploy/docker-compose.yml` | Server plus Postgres; create an untracked `.env` with the variables from [section 4](#4-how-to-run-the-server) and `POSTGRES_PASSWORD` |
| `deploy/helm/sentinix` | Minimal Helm chart; secrets come from an existing Kubernetes Secret (`SENTINIX_DB`, `SENTINIX_SESSION_KEY`, host tokens, `OIDC_*`) |
| `deploy/ci/` | CI examples for GitHub Actions, GitLab CI, Bitbucket Pipelines |

Health check: `GET /healthz`. Put TLS in front (reverse proxy or ingress). Back up the database; it holds findings, approvals and the audit log. Upgrading is replacing the binary or image; tables are created and extended on start.

## 15. CI usage without the server

The CLI works in any pipeline on any git server:

```bash
sentinix scan -baseline .sentinix-baseline.json -fail-on high -sarif sentinix.sarif .
```

To also record the run on the server (trends, findings inbox, tickets), create a project API token and:

```bash
export SENTINIX_TOKEN=<project token>
sentinix scan -server https://sentinix.example.com -project <project id> .
```

The scan is not uploaded when an engine failed, so the server never marks findings fixed from a partial run. Examples for GitHub Actions, GitLab CI and Bitbucket Pipelines are in `deploy/ci/`.

## 16. Reference

### CLI

| Command | Purpose |
| --- | --- |
| `sentinix scan [flags] [path]` | Scan a directory (default `.`) |
| `sentinix surface [base-ref]` | Attack-surface items in `git diff base-ref...HEAD` (default `origin/main`) |
| `sentinix packs keygen <prefix>` | Create an ed25519 key pair |
| `sentinix packs sign <body.json> <key>` | Sign a pack (refuses a pack whose tests fail); signed JSON on stdout |
| `sentinix packs verify <signed.json> <pubkeys>` | Check signature and tests |
| `sentinix bench <dir>` | Score the built-in packs on a labelled sample project |

`scan` flags: `-baseline <file>`, `-update-baseline`, `-fail-on <severity>`, `-sarif <file>`, `-packs <dir>`, `-trust <file>`, `-osv <url>`, `-server <url>`, `-project <id>`, `-token-env <name>` (default `SENTINIX_TOKEN`).

### HTTP API

Authentication is a session cookie (dashboard) or `Authorization: Bearer <project token>`. Cookie-authenticated writes must send `X-Requested-With: sentinix` (CSRF protection). Errors are plain text with a status code.

| Method and path | Needs | Purpose |
| --- | --- | --- |
| `GET /healthz` | none | Liveness |
| `POST /webhook/{name}` | signature | Git host webhooks |
| `GET /auth/login`, `/auth/callback`, `POST /auth/logout` | none | OIDC sign-in |
| `GET /api/me` | read | Current user |
| `GET /api/posture?project=` | read | Posture numbers |
| `GET /api/projects`, `POST /api/projects` | read / manage | List, create |
| `PATCH /api/projects/{id}` | manage | `Branch`, `Schedule`, `TZ`, `ModelPolicy`, `AI`, `PackPins` |
| `POST /api/projects/{id}/scan` | scan | Queue a full scan (`{"Branch": "x"}` optional) |
| `GET /api/projects/{id}/scans`, `/surface`, `/sbom` | read | Run history, attack surface, components |
| `POST /api/projects/{id}/tokens` | manage | Create a CI token (`{"Name": "ci"}`) |
| `POST /api/projects/{id}/ingest` | project token | Upload CLI scan results |
| `GET /api/projects/{id}/report/{format}` | read | [Reports](#10-reports-and-exports) |
| `GET /api/findings` | read | Filters `project`, `status`, `severity` (minimum), `owner`, `limit` |
| `GET /api/findings/{id}` | read | Finding plus AI verdict if any |
| `POST /api/findings/{id}/triage` | triage | `{"status","owner","reason","until"}`; `until` is a Unix time; `accepted` goes to approvals |
| `POST /api/findings/{id}/ticket` | triage | `{"tracker":"jira"\|"linear","due":"YYYY-MM-DD"}` |
| `POST /api/findings/{id}/ai/triage`, `/ai/patch` | triage | [AI add-on](#12-ai-add-on-optional) |
| `POST /api/findings/{id}/ai/test` | owner | Verified test generation |
| `GET /api/approvals?status=`, `POST /api/approvals` | read / triage | Queue; propose `disable_rule` or `loosen_severity` |
| `POST /api/approvals/{id}/decide` | approve | `{"approve": true\|false}` |
| `GET /api/audit?limit=` | audit | Audit log |
| `GET /api/users`, `PUT /api/users/role` | manage | Users; `{"email","role"}` (only an Owner can grant or remove Owner) |
| `GET /api/packs` | read | Loaded packs, rule precision, disabled rules |
| `POST /api/advisories/rescan` | scan | Re-match all SBOMs now |

## 17. Security model

- **Scanned content is untrusted.** Checkouts are shallow, temporary and deleted after each scan. Git tokens are sent only over HTTPS, only for fetching, and are scrubbed from error output. External engines get a minimal environment with no server tokens.
- **Fork PRs** are fetched without a token.
- **Packs** are signed data and cannot execute code or use the network; each rule ships with tests.
- **No silent weakening.** Loosening rules or accepting risk needs a different person and is audited.
- **Secrets** found in code are masked in findings, reports and logs, and redacted before any cloud model sees a snippet.
- **Dashboard** is served with a strict Content Security Policy, builds the page with `textContent` only, and has no inline script, so a malicious finding cannot run code in a reviewer's browser. HTML reports escape every value.
- **Sessions** are HMAC-signed, `HttpOnly`, `SameSite=Lax`, and `Secure` unless you opt out for local HTTP.
- **Defaults are passive and local:** advisories, AI, sandbox and integrations are all off until configured.

## 18. Testing and benchmarks

```bash
cd sentinix-ai && go test ./...
```

`bench/vulnerable-kmp/` is a small Kotlin/Android/KMP project with planted bugs (`expected.json`) and a `clean/` folder of safe look-alikes:

```bash
./sentinix bench bench/vulnerable-kmp
```

It prints planted bugs, findings, precision and recall and exits non-zero below 80% precision or 70% recall. This sample was written with the rules, so it guards against regressions; it is not an independent accuracy measurement. Run Sentinix on your own projects and mark false positives, and use the per-rule precision on the **Rule packs** screen to find noisy rules.

## 19. Troubleshooting

| Symptom | Check |
| --- | --- |
| Webhook delivery shows 401 | Secret in `sentinix.json` differs from the one set on the git host |
| Webhook shows 404 | The path name does not match a `name` in `sentinix.json` |
| Status ends as "scan error" | Server log: usually the git fetch failed; check token scope and repository access |
| No PR comment or status appears | Token lacks Commit statuses / Pull requests write; look at the git host's webhook delivery log |
| PR scan is not in the dashboard | Register the project (host name and `owner/name`) under Settings |
| Cannot sign in | `OIDC_ISSUER` unset, wrong redirect URL, or the identity provider returns an unverified email |
| Login loops on HTTP | Set `SENTINIX_INSECURE_COOKIES=1` for local trials only |
| `SENTINIX_SESSION_KEY must be at least 32 characters` | Generate one with `openssl rand -hex 32` |
| Findings never become fixed | The engine that found them did not run in later scans (missing binary) |
| Advisories missing | `-osv` not set, or the OSV URL is unreachable (the scan then errors instead of reporting clean) |
| Pack rejected at startup | Untrusted signature, a rule without a test, or a failing sample; run `sentinix packs verify` |
| Report says "engines not installed" | Install `semgrep`, `gitleaks`, `osv-scanner` or use the scanner image |

## 20. Limits and what is not included

- Developers can see findings for every project, not only their own.
- Bitbucket Server/Data Center and Gitea are not supported by the server.
- Engines run on the host unless `-sandbox-image` is set.
- A new comment is posted on every push; results are not cached by file hash.
- One server replica only; scheduled scans run in the process.
- Rule packs are regular-expression based: they find known risky patterns, not data flows or logic flaws.
- The dashboard is a plain-JavaScript page served by the server, not a separate TypeScript application. SCIM provisioning, white-label logos and a native PDF renderer are not included.
- iOS binary analysis, Flutter/Dart AOT analysis and runtime mobile instrumentation are not covered.
- Sentinix does not replace a human security review for regulated products.
