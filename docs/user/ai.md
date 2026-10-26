# Scribex AI Prose Rewriting

Scribex provides an optional AI-assisted prose rewriter (`scribex sync -ai`) that updates documentation prose when a code symbol signature changes.

## Threat Model and Guarantees

AI provider replies are treated as untrusted and potentially attacker-influenced (e.g. prompt injection embedded in repository code or comments, or compromised endpoints).

1. **Prose-Only Guarantee**: The model may only update plain prose within a single section. It cannot inject HTML, markdown fences, new anchors, or end markers.
2. **All-or-Nothing Sync**: If the model output violates any validation rule or fails, the anchor is left completely untouched (byte-identical, keeping the old signature hash), marked stale, and retried on the next run.
3. **AI Marker**: Every section updated by AI is marked with `<!-- @scribex:ai sig=<hash> -->` as the last line in the section so human reviewers can identify AI-generated drafts.

## Trust & CI

Repo configuration (`.scribex/config.yaml`) is controlled by git commits and may be modified by fork PRs or unauthorized contributors. To prevent credential exfiltration to rogue endpoints, Scribex enforces operator-level endpoint trust and CI event gating:

1. **Operator-Level Endpoint Trust**:
   - Built-in trusted hosts: `api.anthropic.com` (Anthropic), `api.openai.com` (OpenAI).
   - Custom endpoints must be explicitly trusted outside repository control via `SCRIBEX_AI_ALLOWED_HOSTS` (comma-separated list of `host` or `host:port`) or the operator config file `~/.config/scribex/trust.yaml` (or `$XDG_CONFIG_HOME/scribex/trust.yaml`).
   - Repository-located files (`.scribex/config.yaml`, `.scribex/trust.yaml`) are NEVER consulted for endpoint trust.
   - Loopback endpoints (`127.0.0.1`, `::1`) over HTTP are only allowed if `SCRIBEX_ALLOW_INSECURE_LOCAL=1` is set AND the host:port is listed in `SCRIBEX_AI_ALLOWED_HOSTS`.

2. **Pull-Request Event Gating**:
   - `scribex sync -ai` strictly refuses to run on pull-request events (including GitHub `pull_request`, `pull_request_target`, GitLab `merge_request_event`, Bitbucket PRs, Azure PR builds, Jenkins PR builds, and Travis PRs).
   - No CLI flag overrides this (`-force` does not bypass).
   - AI updates must run on the default branch after merge.
   - API keys must only be exposed to default-branch jobs using protected environments and secrets (e.g. GitHub environment secrets or protected variables). Anyone who controls workflow files in untrusted PR branches could otherwise manipulate environment variables.

3. **HTTP Transport Hardening & SSRF Protection**:
   - Only `https` is permitted (plain `http` only for loopback with `SCRIBEX_ALLOW_INSECURE_LOCAL=1`).
   - Dial-time IP validation rejects loopback, RFC1918 private ranges, CGNAT, benchmark, documentation, and link-local/cloud metadata (`169.254.169.254`, `fe80::/10`) addresses.
   - Private network addresses can be enabled for on-premise deployments via `SCRIBEX_ALLOW_PRIVATE_HOSTS=1`. Link-local and cloud metadata addresses are never allowed even with this flag.
   - Redirects are strictly constrained: cross-host redirects, scheme downgrades (https to http), port changes, and hops > 3 are refused. Custom headers like `x-api-key` and `Authorization` never reach untrusted redirect destinations.
   - Returned errors scrub secret keys and sensitive authorization headers.

4. **HTTP Proxy Note**:
   - When an HTTP proxy is configured via standard environment variables (`HTTPS_PROXY`, `HTTP_PROXY`), Scribex runs a best-effort pre-flight DNS check on the target host before delegating connection routing to the proxy. The IP guard is delegated to the proxy.

## Known Limits

- **Semantic Correctness**: Validation cannot detect a fluent but WRONG or refusal-style reply of plausible length; AI-drafted sections carry the `@scribex:ai` marker and the docs-PR/approval flow (Phase 1–2) is what makes a human check them.
- **Mechanical vs AI Sync**: Mechanical `sync` (without `-ai`) refreshes `sig=` while leaving prose untouched — Phase 1 reports those anchors as "prose not reviewed".
- **HTTP Proxy Delegation**: Through an HTTP proxy, the dial-time IP guard is delegated to the proxy. A pre-flight DNS lookup is executed against the target, but rebinding between pre-flight lookup and proxy resolution is out of scope.
- **Host-Level Trust**: Trust is enforced at the host level. A trusted host that is itself compromised or malicious would still receive prompts and keys.
- **CI Environment Authority**: `SCRIBEX_AI_ALLOWED_HOSTS` set by whoever controls the CI job environment could defeat host allowlisting. For this reason, workflow files and CI variables must be treated as privileged and AI must run only on post-merge default-branch jobs.
