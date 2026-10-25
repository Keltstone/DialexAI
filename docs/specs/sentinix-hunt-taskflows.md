# sentinix-hunt-taskflows (Declarative Audit Taskflows)

Goal: declarative, reproducible audit task sequences with role-based routing and isolated state.

Taskflow Schema:
- Header: `version: 1`, `name: string`, `description: string`, `globals: map[string]string`.
- Tasks: ordered list of tasks: `name`, `model` (role: discover|classify|validate|poc|report),
  `prompt` (safe template), `for_each` (entrypoints|classes|findings), `passes`, `tools` (allowlist),
  `budget`, `must_complete`, `depends_on`, `fresh_context: true` (default).

Execution Semantics:
- Fresh Context: each task invocation executes with a sterile context; prior results pass via state store.
- Safe Templating: variables `{{ globals.x }}`, `{{ entry.x }}`, `{{ prev.key }}` replaced safely.
  No arbitrary code execution, no file includes, all user and repo strings wrapped as untrusted data.
- Built-in Taskflows: embedded assets (`android-audit`, `backend-audit`, `ios-audit`, `variant-analysis`, `triage-codeql`).
- Default: v1 behavior is faithfully reproduced by the `android-audit` taskflow with identical outputs.

Signing & Integrity:
- Taskflows and community suites can be cryptographically signed with ed25519 (reusing `packs.Sign/Verify`).
- Untrusted or tampered taskflows are rejected unless signed by a trusted key in `-trust`.

Where: sentinix-ai/pkg/hunt/taskflow.go, sentinix-ai/pkg/hunt/flows/
