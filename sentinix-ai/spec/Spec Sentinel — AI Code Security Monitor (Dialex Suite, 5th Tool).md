# Spec: Sentinix AI — Code Security Monitor (Dialex Suite, 5th Tool)

Oct 14, 2026 · @Sir

## Overview

Sentinix AI is a defensive tool that watches a codebase on every change, finds security issues, and keeps the project's security tests current. It is the fifth tool in the Dialex Suite, next to Dialex AI, Artix AI, Kritix AI and the Socratix Engine.

**Problem.** Scanners produce more findings than teams can read, most of them false positives, and security tests go stale as features ship. Teams stop trusting the tool or stop updating it.

**Approach.** Bundle proven open-source engines behind one plugin interface, and build the platform around them: diff-aware attack-surface mapping, policy and gating, baselines, a web dashboard, SSO and reporting. AI is an optional add-on for triage, test generation and patch drafts, reusing Socratix, Artix and Kritix.

**Positioning.** Defensive and continuous, not offensive. It reads code and diffs, runs only in the owner's repositories, and never attacks live systems on its own.

**Product shape.** A self-hosted server with a web dashboard as the only interface. Teams get SSO, roles, an audit log, and integrations with their git host, CI, ticketing and chat. Stack-specific rule packs plug in, and the AI add-on can be switched on per project.

## Goals and non-goals

v1 ships one thing well: a self-hosted web platform that reviews every PR or MR and scans whole repositories, for web backends, Android and KMP projects.

**Goals for v1**

- Review every PR or MR, run scheduled and manual full scans, and re-check projects when a new advisory is published.
- Run the bundled engines with no AI and no model access, so results are repeatable and can gate CI.
- Let teams add stack-specific rule packs, starting with a Kotlin, Android and KMP pack.
- Provide a web dashboard with SSO, roles, an audit log and approval of rule changes.
- Integrate with GitHub, GitLab, CI, Jira or Linear, and Slack or Teams.
- Report by audience (developers, reviewers, managers, auditors), with SBOM export.
- Ship as container images that run self-hosted.
- After v1: an optional AI add-on for council triage, security test generation and patch drafts.

**Non-goals for v1**

- Exploiting or probing live systems, or any offensive mode.
- Writing every scanner from scratch; mature open-source engines are bundled or wrapped.
- Mobile apps or a desktop client; the web dashboard is the only interface.
- Runtime mobile instrumentation, iOS binary analysis and Flutter AOT analysis (later phases).
- Replacing a human security review for regulated products.

## Why not just linters

Linters like detekt and ktlint ask whether code is clean, one file at a time. Sentinix AI asks whether a change made the product less safe and who needs to act, so most of its value sits in the layer around the engines.

| Need | Linters and style tools | Sentinix AI |
| --- | --- | --- |
| Scope of analysis | One file at a time | Cross-file reasoning and a diff-to-attack-surface map |
| Dependencies | Not checked | SBOM matched to advisories; alerts when a new CVE hits a shipped project |
| Secrets and config | Limited | Secrets in code and git history; manifest and config checks |
| Existing code | Reports every issue on every run | Baselines, so each PR shows only what it introduced |
| Visibility | Per-repo CI logs | Org-wide dashboard with trends, owners and time-to-fix |
| Governance | None | SSO, roles, approvals, audit trail, exportable reports |
| Workflow | CI warnings that scroll past | Tickets with owner and due date, and one pass or fail gate across languages |

**Honest caveat.** The engines alone overlap with what a team can assemble from Semgrep, Gitleaks and OSV-Scanner, and detekt has some security-related rules. The defensible value is the combination: the platform and governance layer, diff-aware surface mapping, curated rule packs for Kotlin and Android, and the optional AI add-on.

## Product tiers

The deterministic engines are the product's core, and AI is an optional layer on top. Same code and same rule pack versions give the same findings every time.

|  | Core | AI add-on |
| --- | --- | --- |
| What it does | Bundled engines, rule packs, diff mapping, baselines, gating, dashboard, SSO, integrations, reports | Council triage of findings, logic-flaw detection, security test generation, patch drafts, plain-language explanations |
| Needs a model | No | Yes, local or cloud, set per project |
| Result stability | Repeatable; suitable as a CI gate | Can vary between runs; advisory by default |
| Where code goes | Nowhere outside the customer's network | Only where the project's model policy allows |
| Fits | Air-gapped and regulated customers | Teams that want fewer false positives and faster fixes |

Without the add-on, noise is handled by baselines, rule confidence levels and per-rule precision tracking. The add-on is also the natural paid tier.

## Placement in the suite

Sentinix AI is a new top-level folder that reuses Socratix, Artix and Kritix only for the optional AI add-on. The server is Go with Postgres and the dashboard is a separate web app, so it departs from the Compose Desktop pattern of Artix and Kritix. It joins the existing `go.work` workspace.

| Existing piece | Reused for |
| --- | --- |
| Socratix Engine | AI add-on only: council triage, model runners, local and cloud routing |
| Artix AI (`forge`, `coder`) | AI add-on only: patch drafts for confirmed findings |
| Kritix AI | AI add-on only: generating and running security tests, including its DAST checks |

Proposed layout, adapted from `artix-ai/`:

```
sentinix-ai/
├── cmd/
│   ├── sentinixd/    # server: API, webhooks, scheduler, job queue
│   └── sentinix/     # CLI: scan, report, packs (CI and local use)
├── pkg/
│   ├── engines/      # engine plugin interface and bundled engines
│   ├── diffmap/      # diff to attack-surface map
│   ├── policy/       # gates, baselines, accepted risks
│   ├── packs/        # rule pack loader, signing, versioning
│   ├── advisories/   # SBOM and CVE matching
│   ├── scheduler/    # PR/MR, scheduled and manual scan jobs
│   ├── sandbox/      # one container per scan
│   ├── integrations/ # GitHub, GitLab, Jira, Linear, Slack, Teams
│   ├── report/       # PR comments, SARIF, PDF, CSV, SBOM export
│   ├── auth/         # OIDC, SAML, roles, audit log
│   └── ai/           # optional add-on: triage, testgen, patch drafts
├── web/              # dashboard (TypeScript web app)
├── deploy/           # Dockerfiles, compose file, Helm chart
└── go.mod
```

## Scan modes and pipeline

One engine runs in three modes, and every scan passes the same six stages. The AI add-on is a branch, so the core flow works without it.

&#91;embedded content: scan pipeline · 6 stages, 1 decision\]

The top row runs left to right, then the flow turns down. With the add-on off, it skips straight to the report.

| Mode | Trigger | Scope | Purpose |
| --- | --- | --- | --- |
| PR or MR scan | Webhook | Changed files and their dependents | Block new issues early |
| Full scan | Manual (dashboard, CLI, API) or scheduled | Whole repo, chosen branch or paths | Catch old code, new rules and drift |
| Advisory re-scan | New CVE published | Only projects whose SBOM matches | Fast and cheap; no code re-scan |

1. **Trigger.** A PR or MR webhook, a schedule, a manual "Scan now", or a new advisory.
2. **Surface map.** The diff becomes a list of new or changed endpoints, permissions, dependencies, storage and network settings.
3. **Engines.** Bundled engines and the project's rule packs run in a per-scan container, and results use one finding schema.
4. **Policy.** Findings are compared with the base branch baseline, so only new issues count. Severity overrides, accepted risks and gates are then applied.
5. **AI add-on (optional).** Council verdicts on medium and above, security tests verified to fail on bad code, and patch drafts.
6. **Report.** A PR comment, dashboard updates, SARIF, tickets and notifications.

**How a PR or MR is fetched and scanned**

- The webhook signature is verified, then a pending check goes up on the PR so the author sees work has started.
- A worker clones the head commit shallowly with a short-lived token, computes the diff against the base, and scans inside a container with no network and a read-only source mount.
- Results are compared with the base branch scan, so the developer sees only what this change introduced.
- PRs from forks are untrusted: no secrets are available to the scan, and results are posted from the server side.
- The checkout and container are deleted afterward. Only findings, short snippets and metadata are stored.
- Behind a firewall, a small relay forwards webhooks, or the server falls back to polling.
- Permissions: read on contents; write only on checks and pull requests (GitLab: the equivalent API scopes).

**Manual and scheduled full scans**

- "Scan now" per project, with branch or tag, paths and rule packs selectable. Limited to Security reviewer and above.
- Per-project schedules (nightly, weekly or cron) with a timezone, and per-org concurrency limits so many 2 a.m. jobs do not overwhelm workers.
- Results are cached by file hash and lockfile hash, so unchanged code is not re-scanned.
- Each run records the commit, rule pack versions, duration and counts, and compares with the previous run: new, fixed, unchanged.
- A finding is marked fixed when a later scan no longer reports it, and reopens if it returns.
- Large monorepos can be split by path or module.

## Rule packs and plugins

Rule packs are the main extension point: a team, or a consulting client, adapts Sentinix AI to a tech stack without touching the core.

**A pack is a versioned, signed bundle:**

- **Manifest:** name, version, target stacks, minimum Sentinix AI version, author, signature.
- **Rules:** mostly declarative (code patterns, manifest and config checks, dependency rules), each with severity, a CWE or OWASP tag and remediation text.
- **Tests:** good and bad code samples for every rule, required before a pack can be published.
- **AI hints (optional):** stack-specific context for the add-on, for example "in Ktor, check that `authenticate {}` wraps this route".
- **Engine adapters (optional):** code plugins for advanced cases.

**Two trust levels:**

| Level | What it can do | Who can install |
| --- | --- | --- |
| Declarative pack | Data only; cannot execute anything | Any Admin |
| Executable plugin | Runs inside the scan container with declared permissions and no network | Owner approval, recorded in the audit log |

**Lifecycle and governance:**

- Sources: the built-in set, an org-private registry, and later a public registry.
- Stacks are auto-detected: a `build.gradle.kts` suggests the Kotlin pack, an `AndroidManifest.xml` the Android pack.
- Versions are pinned per project; upgrades show a diff of rules before they apply.
- Severity can be overridden per rule. Disabling a rule or pack goes through the approval queue.
- Per-rule precision statistics cover pack rules too, so noisy rules surface quickly.

**First-party pack to launch with:** Kotlin, Android and KMP: exported components, insecure storage, WebView misuse, Ktor and Spring misconfigurations, and shared-code pitfalls in KMP. Mainstream tools cover this area thinly, so it is the most credible launch story.

**Supply-chain rules:** packs must be signed, a pack can never call the network, and every pack ships with its own tests.

## Web dashboard, roles and SSO

The web dashboard is the only interface. It is a separate web app over the server's API, so the CLI, CI and dashboard all see the same data.

**Screens for v1:**

- **Posture:** open findings by severity, new vs. fixed per week, time-to-fix, oldest open criticals.
- **Findings inbox:** filter by project, severity, status or owner; assign, comment, mark accepted risk or false positive.
- **Finding detail:** code, evidence, remediation, history, and with the add-on, the council's verdict and where models disagreed.
- **Attack surface:** endpoints, permissions and dependencies, with what changed since the last release.
- **Scans:** run history, schedules, "Scan now", and run-to-run comparison.
- **Rule packs:** installed packs, versions, per-rule precision, pending upgrades.
- **Approvals:** queue for anything that loosens a rule, removes a test or accepts a risk, with a diff and an audit trail.
- **Settings:** projects and repos, scope, model policy, integrations, API tokens.

**Roles:**

| Role | Can do |
| --- | --- |
| Owner | Everything, including billing, SSO setup and executable plugins |
| Admin | Manage projects, integrations, packs and users |
| Security reviewer | Triage findings, run full scans, approve rule changes |
| Developer | See findings and fix suggestions for their own repos |
| Auditor | Read-only access to findings, reports and the audit log |

**Access rules:**

- SSO through OIDC and SAML, with SCIM provisioning later. A library or an identity server such as Keycloak or Authentik can save time.
- Separation of duties: the person who proposes loosening a rule cannot approve it.
- API tokens are scoped per project and meant for CI.
- UI stack: a mainstream TypeScript web app is assumed, because the dashboard is table, filter and chart heavy. Compose for Web is an open question.

## Integrations

Git hosts come first because every scan starts there; the rest follow in priority order.

| Priority | Integration | What it does |
| --- | --- | --- |
| 1 | GitHub and GitLab apps | Webhooks, checks, PR or MR comments, SARIF upload |
| 2 | CI steps (GitHub Actions, GitLab CI) | Run the CLI and apply the gate in a pipeline |
| 3 | Jira and Linear | Turn findings into tickets with owner and due date |
| 4 | Slack and Teams | Alerts, for example only when new criticals appear |
| 5 | Outbound webhooks and public API | Feed other systems and client dashboards |

GitHub App tokens and GitLab project tokens are short-lived and scoped to the minimum needed. Whether GitLab is in phase 1 or follows GitHub is an open question.

## Reporting

Reporting is designed by audience, because developers, reviewers, managers and auditors need different things.

| Audience | Output | Where |
| --- | --- | --- |
| Developers | Summary comment by severity, inline comments with a fix, link to detail | In the PR or MR |
| Security reviewers | Inbox, assignment, statuses, trends, time-to-fix | Dashboard |
| Managers and clients | One-page executive summary; detailed report per scan or release | PDF, CSV, SARIF, JSON |
| Auditors | Findings mapped to CWE and OWASP, audit trail, scheduled delivery | Dashboard and exports |

**Built in from the start:**

- **Accepted risk** needs a reason and an expiry date, so exceptions get revisited.
- **SBOM export** in CycloneDX or SPDX, plus vulnerable libraries with fixed versions.
- **Coverage statement** in every report: what was and was not scanned and which rule pack versions ran, so a clean report is not read as "secure".
- **White-label branding** for consulting use.
- **Delivery** by email, Slack or Teams, and webhook, with options such as "only if new criticals".
- **Report API** so clients can pull data into their own dashboards.

**Posture score:** shown with its breakdown and never used as a hard gate, since one critical and fifty lows can produce a misleading number.

**PDF in v1:** open question. A clean print-friendly web report is the fallback, with PDF following.

## Components and interfaces

Everything the user touches is a thin layer over one server, so the dashboard, CLI and CI behave the same.

| Component | Role | Interface |
| --- | --- | --- |
| `sentinixd` | Server: API, webhooks, scheduler, job queue, authentication | HTTP API and webhooks |
| Web dashboard | The only user interface | Calls the server API |
| `sentinix` CLI | Local scans, CI step, pack tools | `scan`, `gate`, `report`, `packs` |
| Engine plugins | Run bundled engines (Semgrep, OSV-Scanner, Gitleaks; others after license review) and normalize output | One finding schema, SARIF-compatible |
| Scan workers | Run each scan in its own container | Job queue |
| Diff mapper | Turns a diff into new or changed endpoints, permissions, dependencies and data flows | Attack-surface map as JSON |
| Policy engine | Baselines, severity overrides, accepted risks, gates | Evaluated on every scan |
| AI add-on service | Council triage, test generation, patch drafts; off by default | Calls Socratix, Kritix and Artix |
| Reporter and connectors | PR comments, SARIF, PDF, CSV, SBOM export, tickets, chat | GitHub, GitLab, Jira, Linear, Slack, Teams |

**Core data records**

- **Finding:** id, rule, file and line, engine, evidence, first-seen commit, status.
- **Scan run:** commit, mode, rule pack versions, duration, counts, comparison with the previous run.
- **SurfaceItem:** type (endpoint, permission, dependency, storage, network), change (added, modified, removed), commit.
- **RulePack and RuleStat:** pack version and signature; per rule, findings, confirmed, dismissed, precision over the last 90 days.
- **Verdict (add-on only):** severity, exploitability, confidence, reasoning, models used, dissent noted.
- **Audit event:** actor, action, target, time, approver where one is required.

## Security and trust model

A security tool that reads untrusted code is itself an attack target, so these rules are requirements, not options.

1. **Scanned content is untrusted.** Code, comments, commit messages and dependency files can contain prompt injection. The AI add-on's agent is read-only, has no network access and no shell, and its output is treated as data.
2. **One container per scan.** Each scan runs in a throwaway container with no network, a read-only source mount and CPU and memory limits. Artix's README says its isolation is not a defence against a hostile repository, so Sentinix AI needs container-level isolation before it scans third-party code. The Docker socket is never mounted into the app; workers use a rootless runtime or a separate worker host.
3. **No silent weakening.** Rules and tests can be added automatically. Removing, disabling or loosening any rule, test or gate needs approval from a different person, recorded in the audit log.
4. **Generated tests must prove themselves.** A test is accepted only if it fails on a known-bad sample and passes on the fixed version.
5. **Signed packs, no network.** Packs must be signed and can never call the network. Executable plugins need Owner approval.
6. **Scope and authorization.** Active checks run only against targets in the project's scope file, after an owner acknowledges authorization. The default is passive.
7. **Secrets handling.** Found secrets are masked in every report and log. Model provider keys use encrypted storage, and git tokens are short-lived.
8. **Forks are untrusted.** Fork PRs scan with no secrets available.
9. **Audit log.** Every scan, verdict, approval, role change and pack install is logged with the commit it applies to.
10. **Model data policy.** The core never sends code to a model. For the add-on, a per-project setting chooses local-only, cloud-allowed, or cloud for redacted snippets only.

## Targets and rollout

Web backends come first, then Android and KMP; iOS and Flutter wait because their tooling is thinner. Phases are equal bands, not drawn to scale, and gate thresholds match the quality targets in this spec.

&#91;embedded content: rollout · 4 phases, 3 gates\]

A phase ends only when its gate is met, so the AI add-on cannot start on unproven engines.

| Target | Approach | Phase |
| --- | --- | --- |
| Web backends | Bundled engines, dependency and secret scanning, endpoint diff mapping; generated auth and injection tests with the add-on | 1 and 3 |
| Android | Kotlin and Android pack: manifest and permission review, exported components, insecure storage, hardcoded secrets, WebView misuse | 2 |
| KMP | Shared Kotlin scanned as source; Android target uses the Android checks; Kotlin/Native and iOS targets limited to dependency and secret checks | 2 |
| iOS | Plist and entitlements review, binary analysis | After v1 |
| Flutter | Dependency, secret, storage and traffic checks; Dart AOT code analysis is limited | After v1 |

## Quality metrics and benchmarks

The README says no debate-versus-single-model results are published yet, so Sentinix AI should publish its own numbers before making claims. Targets below are starting proposals to be revised after the first measurements.

| Metric | How measured | v1 target |
| --- | --- | --- |
| Precision after policy | Share of reported findings a reviewer confirms as real, engines only | 80% or higher |
| Recall on known bugs | Share of planted bugs in vulnerable sample apps that are found | 70% or higher |
| AI triage lift (add-on) | Precision with the council vs. engines only vs. a single model | Council beats both |
| Test validity (add-on) | Generated tests that fail on bad code and pass on fixed code | 95% or higher |
| PR latency | Time from webhook to PR comment on a typical diff, add-on off | Under 5 minutes |
| Full scan time | Full scan of a mid-size repo, cold and cached | Tracked, no target yet |
| Cost per PR (add-on) | Model spend per scanned PR, local vs. cloud | Tracked, no target yet |

**Benchmark set.** Start with OWASP Juice Shop and DVWA for web, an intentionally vulnerable Android app for mobile, and a small purpose-built vulnerable KMP app. Add a labelled false-positive set, since reducing noise is the main claim. Publish the harness and results in the repo, in the same spirit as `Dialex-Bench-10`.

**Per-rule tracking.** Each rule records how often its findings are confirmed or dismissed. Rules whose precision falls below a set floor are demoted to informational and flagged for review, never deleted automatically.

## Risks and open questions

Phases and their exit gates are in the rollout section; no calendar dates are set yet.

| Risk | Why it matters | Mitigation |
| --- | --- | --- |
| Engines are easy to replicate | A good DevOps engineer can wire up Semgrep, Gitleaks and OSV-Scanner | Compete on the platform layer, Kotlin and Android packs, diff-aware mapping and the AI add-on |
| Licensing of bundled engines | Some engines are copyleft or restrict commercial use of their rule sets | Verify each license; call installed binaries or make engines user-installable plugins |
| Noise erodes trust | Teams drop tools that cry wolf | Baselines, rule confidence, per-rule precision, published benchmarks, AI triage |
| Pack supply chain | A malicious pack could weaken or abuse scans | Signing, no network, required tests, approval for executable plugins |
| Prompt injection via repo content | Hostile code could steer the AI add-on | Read-only, no-network agent; container isolation |
| Self-hosting support burden | Customer environments vary widely | Container images, Helm chart, health checks, clear upgrade path |
| Scan cost on large repos | Full scans of monorepos can be slow and costly | Hash-based caching, path splitting, concurrency limits |
| Dual-use perception | Security tooling can be misused | Defensive scope, scope file, passive default, audit log |
| Thin tooling for iOS and Flutter | Fewer mature analyzers | Later phases; start with traffic and storage checks |

**Open questions**

- [ ] Name: Sentinix AI is the chosen name; check the domain, GitHub name and trademarks before committing.
- [ ] Self-hosted first or hosted SaaS first? This spec assumes self-hosted.
- [ ] Dashboard stack: a TypeScript web app (assumed) or Compose for Web?
- [ ] GitLab in phase 1, or GitHub first?
- [ ] PDF export in v1, or a print-friendly web report first?
- [ ] Commercial model: open core under the existing license, with the AI add-on as the paid tier?
- [ ] Which engines to bundle after license review?
- [ ] Separate product or a mode inside Kritix? This spec assumes separate.
