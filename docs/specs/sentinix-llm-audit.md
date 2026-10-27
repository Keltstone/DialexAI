# sentinix-llm-audit (`sentinix hunt` v2)

Goal: configurable AI security audit engine for defensive whole-repo entry point auditing.

Lifecycle:
- Stage 1 Discovery: deterministic enumeration (Android, iOS/Swift, backend routes, custom regex/globs).
- Stage 2 Taskflows: declarative tasks, role-based model routing, safe templating, fresh contexts.
- Stage 3 Validation: multi-pass consensus, mitigation check, sandboxed PoC with feedback, CVSS calibration.
- Stage 4 Accounting & Gating: token/cost accounting, audit trail, advisory by default (confirmed-only opt-in).

Threat Model (OWASP Top 10 for LLM Applications):
- LLM01 Prompt Injection: repo code, tools, PoC output, and advisories are wrapped in ai.Untrusted; strict JSON schemas only.
- LLM02 Insecure Output Handling: model text is never eval'd or executed; PoC tests run strictly in networkless sandboxes.
- LLM06 Excessive Agency: read-only tool registry confined by os.OpenRoot; non-read-only MCP tools require confirmation.
- LLM07 Sensitive Info Disclosure: strict policy check (local-only default, cloud-redacted redaction fail-closed).
- LLM10 Unbounded Consumption: hard ceilings on calls, tokens, cost ($), wall-time; stops cleanly with Gap().

Contracts & Invariants:
- CLI: `sentinix hunt [flags] [path]`; backwards compatible with v1 flags; `hunt config init|show|validate`.
- Engine: `hunt.Engine` implements `engines.Engine`, Name()="llm-audit".
- Findings: `Source="llm"`, `Rule="hunt/<class>"`, default `Level="notice"` (never blocks unless confirmed-only gate).
- State: DB tables `hunt_runs`, `hunt_tasks`, `hunt_tool_calls` with offline JSON fallback.

Where: sentinix-ai/pkg/hunt, cmd/sentinix, cmd/sentinixd, bench/hunt-android.
Status: in-progress v2 upgrade. Companion specs: sentinix-hunt-config.md, sentinix-hunt-taskflows.md, sentinix-hunt-validation.md.
