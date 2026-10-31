# scribex-generate: implementation cards 1-3

Parent spec: scribex-generate.md. Rules for every card: write the failing test first, stdlib only (go.mod has zero deps and CI ships an SBOM; do not add one), keep existing tests green, `go vet ./... && go test -race ./...` in `scribex-ai/`, no unrelated refactors, smallest diff.

---
## Card 1 - Generated CI installs a pinned release binary

Problem: `ghPRWorkflow`, `ghDocsWorkflow`, `ghScavengeWorkflow` (4 steps) in `cmd/scribex/cmd_ci.go` run `go build ... ./cmd/scribex`, which only works in the Scribex repo itself. Any consuming repo fails at the first step. `glJob` already supports `SCRIBEX_RELEASE_URL` + `SCRIBEX_SHA256`.

Behaviour:
- Replace each "Build scribex" step with an "Install scribex" step. Shared script text lives in ONE Go const used by all four steps.
- Inputs come from GitHub repository **variables** (non-secret): `vars.SCRIBEX_RELEASE_URL`, `vars.SCRIBEX_SHA256`, passed through `env:`, never interpolated inside `run:` (existing `checkNoIllegalRunInterpolation` must still pass).
- Script (bash, `set -euo pipefail`):
  1. If `cmd/scribex/main.go` exists in the checkout AND `go.mod` first line is `module scribex` -> build from source as today (keeps this repo's own use working). Keep setup-go step.
  2. Else require both vars. URL must match `^https://[^[:space:]]+$`; SHA must match `^[0-9a-f]{64}$`. Otherwise print `scribex: set repository variables SCRIBEX_RELEASE_URL and SCRIBEX_SHA256 (https URL, 64-hex sha256)` to stderr and exit 2.
  3. `curl -fsSL --proto '=https' --max-time 60 -o "$RUNNER_TEMP/bin/scribex" "$SCRIBEX_RELEASE_URL"`; `echo "$SHA  path" | sha256sum -c -`; `chmod +x`; add dir to `GITHUB_PATH`.
- The `comment` job (unprivileged, no PR code checkout) uses the same install step.
- `ci init gitlab`: no template change; add the same URL/SHA regex validation to the existing `before_script` branch.
- `scribex ci init` prints one extra line to stdout: `set repo variables SCRIBEX_RELEASE_URL and SCRIBEX_SHA256 (see scripts/build.sh SHA256SUMS)`.

Files: `cmd/scribex/cmd_ci.go`, `cmd/scribex/ci_templates_test.go`, `docs/user/ci.md` (document the variables).

Failing tests first (`cmd/scribex/ci_templates_test.go`):
- `TestWorkflowsInstallPinnedBinary`: each generated workflow that invokes `scribex` contains `sha256sum -c`, contains no unguarded `go build` of `./cmd/scribex` (only inside the source-checkout branch), reads URL/SHA via `env:` from `vars.`.
- `TestInstallStepFailsClosed`: extract the install script from the generated YAML, run with `bash` in an empty temp dir (no `cmd/scribex`) for: both vars unset; URL `http://x`; URL ok + SHA `abc`; SHA uppercase. Each exits 2 with the message above. No network used.
- `TestInstallStepSourceBranch`: temp dir containing `cmd/scribex/main.go` + `go.mod` `module scribex` takes the source branch (assert via a stub `go` on PATH that records args).
- Existing `TestActionlintExecution`, `TestWorkflowTemplatesHardening` still pass.

Done when: `scribex ci init github` in a fresh non-Scribex repo yields workflows that pass actionlint and fail with the clear message until variables are set.

---
## Card 2 - LLM backends: `cli` (default) and `ollama` beside anthropic/openai

Problem: `pkg/llm` supports only anthropic|openai with API keys; `config.Validate` rejects anything else. Requirement: default to vendor CLIs, also Ollama, API keys optional.

Design:
- `llm.Completer` interface `Complete(system, user string) (string, error)`; `llm.New(config.AI)` returns it, dispatching on `ai.provider`. Existing `*Client` unchanged and still returned for anthropic|openai (existing callers/tests keep working; adjust types only where needed).
- New provider values: `cli`, `ollama`.
- `ollama`: reuse the OpenAI-compatible path against `ai.base_url` default `http://127.0.0.1:11434` (`/v1/chat/completions`). No API key. Allowed over plain HTTP ONLY when host resolves to loopback; any non-loopback host requires https. Route through `trust.CheckAIEndpoint` and `httpx` (SSRF rules intact); do not weaken `SCRIBEX_ALLOW_INSECURE_LOCAL` semantics for other providers.
- `cli`: run a vendor CLI as a subprocess. **Security-critical (repo config is attacker-controlled):**
  - `ai.cli_command` must be one of a fixed allowlist table in code: `claude`, `codex`, `agy` (+ add others only by code change). Resolved with `exec.LookPath`. Arbitrary paths/args from config are rejected. Per-vendor non-interactive argument vectors live in that table; the implementer verifies each from the vendor's `--help` and records the checked CLI version in a comment. If a CLI has no mode that disables tool use/file edits, mark it `agentic: true` and refuse unless the user passes `--allow-agentic-cli` / env `SCRIBEX_ALLOW_AGENTIC_CLI=1`.
  - `exec.CommandContext`, no shell, prompt via stdin, `Dir` = fresh empty temp dir (never the repo), env scrubbed to `PATH`, `HOME`, `LANG`, `TERM`, plus vendor auth vars only if the table lists them; timeout 180s; stdout cap 2 MiB; stderr truncated and passed through `secrets` redaction before being returned in errors.
  - The model name from `ai.model` is passed via the table's model flag when set; empty = CLI default.
- Config: new keys `ai.cli_command`; `Default()` provider becomes `cli`, `cli_command` `claude`, model `claude-sonnet-5-5` (still the latest Sonnet-class ID; re-check at implementation time). `Validate`: provider in {cli, ollama, anthropic, openai}; `cli` requires allowlisted command and does NOT require `api_key_env`; `ollama` requires model; api providers unchanged. Existing config files that name a provider keep working unchanged (no silent switch).
- `scribex config test` works for all four (one tiny call).

Files: `pkg/config/config.go` (+test), `pkg/llm/llm.go` (+ new `pkg/llm/cli.go`, `pkg/llm/ollama.go`), `pkg/trust/trust.go` only if needed for loopback, `cmd/scribex/cmd_config.go`, `docs/user` config section.

Failing tests first:
- `config`: `TestValidateProviders` (cli ok without key env; ollama needs model; unknown rejected), `TestCLICommandAllowlist` (`/bin/sh`, `claude --x`, `../claude`, empty rejected), `TestDefaultIsCLI`, `TestExistingAnthropicConfigUnchanged`.
- `llm/cli_test.go` with a **fake executable** written to a temp dir placed first on PATH named `claude`: asserts argv equals the table, stdin equals prompt, cwd is NOT the repo and is empty, env contains none of `ANTHROPIC_API_KEY`/`GITHUB_TOKEN`/`ATLASSIAN_TOKEN` even if set in the test process, timeout kills a sleeping fake, >2 MiB output errors, non-zero exit returns redacted error, repo files unchanged.
- `TestCLIAgenticRefused` for a table entry flagged agentic.
- `llm/ollama_test.go` with `httptest` loopback server: request shape, no Authorization header, model passed; non-loopback http URL rejected; https non-loopback allowed by trust rules.
- Existing `llm_test.go`, `ai_e2e_test.go` pass unchanged.

Done when: with only `claude` installed and logged in, `scribex config test` and `scribex sync -ai` work with no env vars; with Ollama running, the same with `ai.provider=ollama`.

---
## Card 3 - Doc-type catalog (`doctypes.json`)

Problem: generation needs a configurable list of documentation kinds (see parent spec "Doc types"). Format is **JSON** (`encoding/json`, `DisallowUnknownFields`) because the module has no YAML dependency and the dashboard will read/write it; comments are not supported.

Behaviour:
- New package `pkg/doctypes`. File `.scribex/doctypes.json`: `{"features": {"<name>": ["glob",...]}, "types": [ {...} ]}`.
- Type fields: `id` (`^[a-z][a-z0-9-]{1,40}$`), `title`, `audience` (dev|qa|po|exec|ops), `enabled` bool, `granularity` (repo|module|package|feature|file|symbol), `include`/`exclude` []glob, `output` string template, `grounding` (symbol|derived|human), `template` repo-relative path (required for derived/human), `inputs` []repo-relative path, `id_policy` (none|preserve, default none), `max_calls` int >=0, `language` string (default `en`).
- Built-ins (the 8 in the parent spec) are compiled in as defaults; `tech-reference` enabled, rest disabled. A file entry with a built-in `id` overrides field-by-field; built-ins cannot be deleted, only disabled.
- `Load(root) (Catalog, error)`: missing file = built-ins only. `Validate` returns all problems, not just the first:
  - unknown JSON fields, duplicate ids, bad enums/regex;
  - `output` allows only placeholders `{module}`, `{package}`, `{feature}`, `{file}`; must be relative, no `..`, no absolute, no symlink escape (use `safepath`), must end `.md`; a `symbol`-grounded type must not use `{feature}`;
  - `template` and `inputs` confined to the repo via `safepath`, must exist for enabled types;
  - `grounding: human` forbids any model-driven field (`max_calls` must be 0);
  - `granularity: feature` requires a non-empty `features` map or the type declares `"discover": "auto"`;
  - globs validated with `path.Match` (reject `**` unless implemented consistently with existing scan code - reuse whatever `pkg/symbols`/`docs` already use; do not invent a second glob dialect).
- `Resolve(type, source path) -> output path`: pure function, deterministic, fully tested, never writes.
- CLI: `scribex doctypes list` (id, enabled, grounding, audience, output), `scribex doctypes show <id>` (effective merged config as JSON), `scribex doctypes validate` (exit 1 with every problem). No generation yet.
- Writes (used later by the dashboard): `Save(root, Catalog)` via `fsx.WriteFileIn`, atomic, only user overrides persisted (not the full built-in set), stable key order.

Files: `pkg/doctypes/{doctypes.go,builtin.go,doctypes_test.go}`, `cmd/scribex/cmd_doctypes.go` (+ registration in `main.go`), `docs/user/doctypes.md`.

Failing tests first (`pkg/doctypes`):
- `TestBuiltinsPresent`, `TestOnlyTechReferenceEnabledByDefault`, `TestOverrideMerges`, `TestCannotDeleteBuiltin`.
- `TestValidateRejects` table: unknown field, dup id, bad id, bad audience, `output` with `..`, absolute, `{bogus}`, non-`.md`, symlink escape, template missing, human with max_calls>0, feature granularity without features, symbol type with `{feature}`.
- `TestResolveOutput` table incl. nested modules and path separators on Windows-style input.
- `TestSaveRoundTrip` and `TestSaveOnlyOverrides`.
- CLI e2e in `cmd/scribex`: `doctypes list` on empty repo shows 8 rows; `validate` on a bad file exits 1 listing every problem; no writes outside `.scribex/`.

Done when: `scribex doctypes validate` is green on a repo with no file and red with clear messages on each rule above.

---
Cards 4+ (generate plan/apply/publish, derived provenance, dashboard, CI job, acceptance) are written after 1-3 land and are reviewed, so they use the real APIs.
