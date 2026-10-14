# Sentinix AI

Defensive code security monitor. It reviews every pull/merge request, runs scheduled and manual full scans, tracks findings over time and keeps the report honest about what it did not check. It never attacks live systems.

## What it does

- **Reviews PRs/MRs** on GitHub, GitHub Enterprise, GitLab (cloud or self-managed) and Bitbucket Cloud. Only findings the change introduced are reported (head vs base).
- **Engines:** a built-in secrets scanner, signed declarative rule packs (built-in Kotlin/Android/KMP, Ktor and Spring packs), and `semgrep`, `gitleaks`, `osv-scanner` when installed. Dependency advisories (OSV, opt-in) from an SBOM built out of go.mod, package-lock/package.json, requirements.txt, Gradle and pom.xml.
- **Full scans** per project, manual or on a cron schedule; findings become new, fixed or reopened across runs; stored SBOMs are re-matched against OSV without rescanning code.
- **Governance:** OIDC sign-in (SAML through Keycloak/Authentik), five roles, audit log, approvals where the proposer can never approve their own change (accepting a risk, disabling or loosening a rule), project-scoped CI tokens.
- **Reports:** CSV, JSON, SARIF, CycloneDX SBOM, and a print-ready HTML report (Save as PDF) with a coverage statement and OWASP mapping. Slack, Teams, signed webhooks, Jira and Linear tickets.
- **Optional AI add-on** (off by default): council triage with dissent, patch drafts and test generation, per-project model policy (local-only, cloud-redacted, cloud). Verdicts are advisory. A generated test is kept only if it fails on the vulnerable code and passes after the patch.

## Run it

```bash
# server (SQLite for a trial, Postgres for production)
export SENTINIX_SESSION_KEY=<32+ random characters>
export OIDC_ISSUER=https://idp.example.com OIDC_CLIENT_ID=... OIDC_CLIENT_SECRET=... OIDC_REDIRECT_URL=https://sentinix.example.com/auth/callback
export SENTINIX_OWNER_EMAIL=you@example.com
sentinixd -config sentinix.json -db postgres://... -addr :8080
```

`sentinix.json` lists git hosts; `$VAR` values are read from the environment. Self-hosted instances set `api`:

```json
[
  {"name": "gh",  "kind": "github",    "secret": "$GH_SECRET", "token": "$GH_TOKEN"},
  {"name": "ghe", "kind": "github",    "api": "https://ghe.example.com/api/v3", "secret": "$GHE_SECRET", "token": "$GHE_TOKEN"},
  {"name": "gl",  "kind": "gitlab",    "api": "https://gitlab.example.com/api/v4", "secret": "$GL_SECRET", "token": "$GL_TOKEN"},
  {"name": "bb",  "kind": "bitbucket", "secret": "$BB_SECRET", "token": "$BB_TOKEN", "public_url": "https://sentinix.example.com"}
]
```

Point each host's webhook at `POST /webhook/<name>` (GitHub `pull_request`, GitLab merge request events, Bitbucket `pullrequest:created` and `pullrequest:updated`). Tokens need read on contents and write on commit statuses and PR/MR comments. Bitbucket Server/Data Center and Gitea are not supported by the server; use the CLI in CI for those.

Optional flags: `-packs dir -trust keys.pub` (signed extra packs), `-osv https://api.osv.dev` (advisories; sends package names and versions only), `-integrations file.json`, `-ai models.json`, `-sandbox-image image` (external engines and generated tests run in a container with no network), `-workers n`, `-fail-on high`. White label reports with `SENTINIX_BRAND_NAME` and `SENTINIX_BRAND_ACCENT`.

## CLI (any git server, any CI)

```bash
sentinix scan -baseline .sentinix-baseline.json -fail-on high -sarif out.sarif .
sentinix scan -baseline .sentinix-baseline.json -update-baseline .
sentinix scan -server https://sentinix.example.com -project <id> .   # token in $SENTINIX_TOKEN
sentinix surface origin/main
sentinix packs keygen mykey && sentinix packs sign pack.json mykey.key > pack.signed.json
sentinix bench bench/vulnerable-kmp
```

CI examples are in `deploy/ci/`. Container and Helm files are in `deploy/`.

## Limits

- Developers can see findings for every project, not only their own.
- Engines run on the host unless `-sandbox-image` is set; with the sandbox, Semgrep rules must be baked into the image.
- A new comment is posted on every push; results are not cached by file hash.
- Run one server replica: scheduled scans run in-process.
- A clean report only means these checks found nothing. Every report lists the engines and rule pack versions that ran.
