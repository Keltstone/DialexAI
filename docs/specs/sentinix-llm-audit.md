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
- CLI: `sentinix hunt [flags] [path]`; flags `-ai models.json (provider "anthropic" or OpenAI-compatible) -policy local-only|cloud-redacted|cloud -max-calls N -sarif f`.
- Engine: `hunt.Engine{Models, Policy, MaxCalls}` implements engines.Engine, Name()="llm-audit".
- Rule id `hunt/<class>`; class vocabulary: confused-deputy, deeplink-host, webview-bridge, path-traversal, authz, injection, intent-redirect.
- Model JSON answer: `{"vulnerable":bool,"severity":"low|medium|high|critical","confidence":0..1,"reason":"short","poc":"optional"}`.
- No hunt_results table: finding Source="llm", Match=entry:class, Evidence="PoC reproduced|unconfirmed: ...". Dashboard shows "AI-suggested, unverified".
- Never gates: policy.Fails/CategoryFails skip Source=llm; EffectiveLevel(hunt/*)=notice unless a rule policy says otherwise.
- Call cap -> Engine.Gap() -> scan gaps (not Skipped(): that fails PR status).
- PoC (opt-in): `-sandbox-image img -poc-cmd "cmd"`; model writes test, ai.Reproduce runs it on current tree in sandbox; must FAIL (exploit works).
- Server: `sentinixd -hunt-max-calls N`, only AI-enabled projects.
- MCP: `sentinix mcp serve [dir]`, read-only tools sentinix_scan, sentinix_entrypoints.
- Hunt packs: `-hunt-packs dir -trust keys`, signed JSON {name,version,classes:{kind:[{id,cwe,hint}]}}; reuse packs.Sign/Verify.
- Tools: `-tools` read-only read_file/grep/entrypoints via JSON reply loop, root-confined (os.OpenRoot), output wrapped untrusted + redacted under cloud-redacted; no write/shell/net so no confirm step needed; each step counts as a call.
- State: `-resume file` caches tasks by sha(code)|file|class|pass; `-only` reruns one task. Platform: `-platform mobile|backend` (component/deeplink=mobile, route=backend).
- Taint: engines.CodeQL (opt-in SENTINIX_CODEQL=1), per-language db (build-mode none), security-extended suite, SARIF; missing binary = coverage gap.
- PoC retry: one revision if test passes on vulnerable code (extra call not budgeted; runner gives pass/fail only).
- Variant: `-variant advisory.txt` or `-variant-ghsa GHSA-id` (public API, id only leaves machine) adds class hunt/variant (advisory text wrapped as untrusted).

Edge:
- Code is untrusted: wrapped via ai untrusted(); answers parsed as data only; no tools/shell/network for models.
- Engine must be listed in `ran` or RecordScan marks its findings fixed.
- Budget hit / no model -> gap "llm-audit not run", never a clean result.
- Non-deterministic: baselines suppress repeats by fingerprint; not a CI gate.

Where: sentinix-ai/pkg/hunt, cmd/sentinix/main.go, bench/hunt-android (own expected.json, keyed hunt/<class>).

Status: done. Pending: live-model benchmark (opt-in), PR-comment rendering of gaps.
