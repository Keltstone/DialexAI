# Contributing

Dialex AI Project is young. The fastest way to make it better, and to be credited for it, is to take a scoped piece of work below.

## Ground rules
- Every change ships with a test that fails if the logic breaks. Real-binary integration tests beat mocks.
- Keep PRs small and single-purpose. Match the surrounding style.
- Read the steering notes in `CLAUDE.md` (architecture, MVI, theming) before touching the Kotlin client.
- Contributors are listed in `CONTRIBUTORS.md` for what they actually build.
- Do not add telemetry or unverified claims to the docs.

## License of contributions
The project is dual-licensed (see [COMMERCIAL_LICENSE.md](COMMERCIAL_LICENSE.md)). By opening a pull request you confirm that you wrote the change or have the right to submit it, and you grant Kolta Labs a perpetual, worldwide, non-exclusive, royalty-free, irrevocable license to use, modify, sublicense and distribute your contribution under the PolyForm Noncommercial License and under commercial licenses. You keep your copyright. Sign off commits with `git commit -s`. _This grant text should be reviewed by a lawyer before you rely on it._

## Setup
```bash
(cd socratix-engine && go test ./pkg/...)
(cd artix-ai && go test ./pkg/...)
(cd dialex-ai && ./gradlew :shared:allTests)
```
The Kotlin client needs the public [Kolt](https://github.com/keltstone/Kolt) libraries (Apache-2.0) as a sibling checkout: `git clone https://github.com/keltstone/Kolt.git KoltLibs` next to this repo (see `dialex-ai/settings.gradle.kts`).

## Scoped starter work

| Area | Task | Size |
|---|---|---|
| Benchmark | Add 10 new cases to `socratix-engine/pkg/benchmark/bundled.go` in a domain you know (legal, security, product) | S |
| Benchmark | Run `Dialex-Bench-10` council vs single model and publish method, cost, latency, results in `dialex-ai/docs/` | M |
| Engine | OpenAI-compatible runner (covers OpenRouter, vLLM, LM Studio) in `socratix-engine/pkg/runner` with a contract test | M |
| Engine | Runner health check: probe CLI version/flags at startup and disable a broken runner cleanly | M |
| Artix | Container or VM execution for `artix-ai/pkg/sandbox` (today: `sandbox-exec`/`bwrap`; reads are unrestricted) | L |
| Artix | Make `artix plan` a real model-backed council (use `coder.NewAPIRunnerFromEnv`; keep the template as offline fallback) | L |
| Artix | Model-backed reviewer that evaluates acceptance criteria, alongside the rule-based checks | L |
| Artix | Give the TUI REPL and `forge` worker a model via `coder.PatchGenerator` | M |
| Artix | Deeper tests for `lsp`, `mcp`, `knowledge`, `plugins`, `forge` (each has only 2-5 test functions) | M |
| Tests | Deepen `socratix-engine/pkg/graph` tests (temporal decay, FTS5 recall) | S |
| Docs | Verify one end-to-end demo per Artix feature (shadow worktrees, time travel, LSP, knowledge items) or mark it "planned" in the docs | M |
| Platform | Linux and Windows desktop smoke test in CI | M |
| Design partner | Try Dialex on a real decision at your company and write up what broke | S |
| Security | Native OIDC login (today: trusted-proxy headers only); tamper-evident audit log | L |

Open an issue before starting anything L-sized.
