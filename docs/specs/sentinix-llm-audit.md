# sentinix-llm-audit (`sentinix hunt`)

Goal: opt-in, advisory LLM audit of entry points; finds logic bugs rule packs can't.

Behaviour:
- Stage 1 entrypoints: deterministic whole-repo enumeration (exported components, deeplinks/intent filters, WebViews, HTTP routes). No model.
- Stage 2 classify: per entry point x vuln class, fresh LLM call, small code slice. Strict + broad prompt pass.
- Stage 3 validate: schema-check answers; drop low-severity/no-state-reachable claims; optional PoC via ai.Verify in sandbox.
- Stage 4 report: engines.Finding, Source=llm, default level Notice (never blocks). Side table hunt_results.
- State passes between stages via data, never via chat history.
- Off unless model config given AND project AI opt-in AND model policy allows (reuse ai.Allowed / RedactSafe).
- Budget: max_calls hard cap; stop and report gap when hit.

Contracts:
- CLI: `sentinix hunt [flags] [path]`; flags `-ai models.json -policy local-only|cloud-redacted|cloud -max-calls N -sarif f`.
- Engine: `hunt.Engine{Models, Policy, MaxCalls}` implements engines.Engine, Name()="llm-audit".
- Rule id `hunt/<class>`; class vocabulary: confused-deputy, deeplink-host, webview-bridge, path-traversal, authz, injection, intent-redirect.
- Model JSON answer: `{"vulnerable":bool,"severity":"low|medium|high|critical","confidence":0..1,"reason":"short","poc":"optional"}`.
- Table hunt_results(finding_id, entrypoint, class, pass, model, status, poc).

Edge:
- Code is untrusted: wrapped via ai untrusted(); answers parsed as data only; no tools/shell/network for models.
- Engine must be listed in `ran` or RecordScan marks its findings fixed.
- Budget hit / no model -> gap "llm-audit not run", never a clean result.
- Non-deterministic: baselines suppress repeats by fingerprint; not a CI gate.

Where: sentinix-ai/pkg/hunt, cmd/sentinix/main.go, bench/hunt-android (own expected.json, keyed hunt/<class>).

Status: in-progress (stage 1-3 + CLI; server/dashboard, MCP, hunt packs later).
