# sentinix-hunt-validation (Validation Pipeline, PoC & Gating)

Goal: multi-stage false-positive reduction, verified PoC exploitability, and guarded gating policy.

Pipeline Stages:
1. Schema & Confidence: validate model JSON; drop confidence < min_confidence or malformed outputs.
2. Mitigation Verification: inspect code/manifest for permissions, exported=false, sanitizers, auth guards.
3. PoC Execution with Feedback: runner stdout/stderr captured; model given up to max_attempts revisions.
   Compile/setup errors DO NOT count as reproduced (differentiates compile failure vs assertion failure).
4. Severity Calibration: CVSS vector estimation + reasoning; model may not inflate base severity.
5. Deduplication & Clustering: group findings by root cause / sink to eliminate duplicate alerts.

Exploitability Levels:
- `confirmed`: sandboxed PoC test fails on vulnerable code and passes on fixed code (reproduced).
- `likely`: strong architectural reachability and absent mitigations, but automated PoC timed out or unconfigured.
- `unconfirmed`: model hypothesis without proof; advisory only.

Gating Policy:
- Default: `gate: advisory`. Findings have Source="llm", notice level, never block PR or CI status.
- Opt-in: `gate: confirmed-only`. Blocks ONLY on exploitability == "confirmed" AND severity >= threshold.
- Unconfirmed findings and standard llm findings never gate under any mode.
- Enabling `confirmed-only` is a safeguard loosening: requires second approver and audit log entry.

Where: sentinix-ai/pkg/hunt/validation.go, sentinix-ai/pkg/policy/policy.go
