# Sentinix AI

Defensive code security monitor. Reviews every PR/MR and reports only what the change introduced.

## Git hosts

`sentinixd` serves `POST /webhook/<name>` for each host in its config. Self-hosted instances set `api`.

```json
[
  {"name": "gh",      "kind": "github",    "secret": "$GH_SECRET", "token": "$GH_TOKEN"},
  {"name": "ghe",     "kind": "github",    "api": "https://ghe.example.com/api/v3", "secret": "$GHE_SECRET", "token": "$GHE_TOKEN"},
  {"name": "gl",      "kind": "gitlab",    "api": "https://gitlab.example.com/api/v4", "secret": "$GL_SECRET", "token": "$GL_TOKEN"},
  {"name": "bb",      "kind": "bitbucket", "secret": "$BB_SECRET", "token": "$BB_TOKEN", "public_url": "https://sentinix.example.com"}
]
```

```bash
sentinixd -config sentinix.json -addr :8080
```

Supported: GitHub and GitHub Enterprise, GitLab and self-managed GitLab, Bitbucket Cloud.
Not supported yet: Bitbucket Server / Data Center, Gitea. For those, and for any other git server, run the CLI from CI.

Webhook events: GitHub `pull_request`; GitLab `merge_request`; Bitbucket `pullrequest:created` and `pullrequest:updated`.
Token needs: read on contents, write on commit statuses and PR/MR comments.

## CLI (works with any git server)

```bash
sentinix scan -baseline .sentinix-baseline.json -fail-on high -sarif out.sarif .
sentinix scan -baseline .sentinix-baseline.json -update-baseline .
sentinix surface origin/main
```

Engines: a built-in secrets scanner, plus `semgrep`, `gitleaks` and `osv-scanner` when installed.
Reports end with the list of engines that ran; a clean report is not proof of security.
Engines run on the host, not in a per-scan container.
