# scribex-generate: implementation cards 1-12

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
  - `ai.cli_command` must be one of a fixed allowlist table in code: `claude`, `codex`, `gemini`, `agy` (others only by code change). Resolved with `exec.LookPath`. Arbitrary paths/args from config are rejected. Per-vendor non-interactive argument vectors live in that table; the implementer verifies each from the vendor's `--help` and records the checked CLI version in a comment. If a CLI has no mode that disables tool use/file edits, mark it `agentic: true` and refuse unless the user passes `--allow-agentic-cli` / env `SCRIBEX_ALLOW_AGENTIC_CLI=1`.
  - `exec.CommandContext`, no shell, prompt via stdin, `Dir` = fresh empty temp dir (never the repo), env scrubbed to `PATH`, `HOME`, `LANG`, `TERM`, plus vendor auth vars only if the table lists them; timeout 180s; stdout cap 2 MiB; stderr truncated and passed through `secrets` redaction before being returned in errors.
  - The model name from `ai.model` is passed via the table's model flag when set; empty = CLI default.
- **CLI table** (`pkg/llm/cli.go`, one struct per entry: `Name`, `Bin`, `Args` for headless/no-tools mode, `ModelFlag`, `Agentic bool`, `Suggested []string`, `VerifiedVersion`). Initial entries; the implementer MUST confirm each `Args` vector against the installed CLI's `--help` and record the version, because vendor flags change faster than docs:
  | Name | Bin | Headless invocation (verify) | Model flag | Suggested models (advisory) |
  |---|---|---|---|---|
  | `claude` | `claude` | `-p` (prompt on stdin), `--output-format text`; find the tools-off flag in `claude --help` (a `--tools` option is reported but unverified), else `Agentic: true` | `--model` | `claude-fable-5-1`, `claude-opus-5-5`, `claude-sonnet-5-5` (default), `claude-haiku-5-5`; aliases `sonnet`, `opus` also accepted by the CLI |
  | `codex` | `codex` | `exec` with `--sandbox read-only` (documented default for `exec`), prompt on stdin | `-m` / `--model` | none pinned: docs show `gpt-5.6`, third-party pages claim a newer one; leave `ai.model` empty to use the CLI's own default |
  | `gemini` | `gemini` | `-p` / stdin, `--approval-mode plan` (read-only) | `-m` / `--model` | aliases `auto`, `pro`, `flash`, `flash-lite` (avoids pinning a version) |
  | `agy` | `agy` (Antigravity CLI; Google moved consumer Gemini CLI access to it in June 2026) | verify via `agy --help` | verify | none pinned; empty = CLI default |
  Ollama is a provider, not a CLI: model is whatever `ollama list` shows (e.g. user-pulled coding models); no default.
- Model rules: `ai.model` is optional for `cli` (empty = the CLI's own default, which avoids stale pins). When set it must match `^[A-Za-z0-9][A-Za-z0-9._:/-]{0,99}$` (**must not start with `-`**, so it can never be parsed as a flag) and is passed as a single argv element after the model flag. Suggested lists are advisory only: shown by `scribex config models [--cli name]` and the dashboard dropdown, never used to reject a model (new releases must work without a Scribex release). Each `Suggested` entry carries a `// checked YYYY-MM-DD` comment; a test fails if any entry is older than 120 days so the list gets reviewed.
- Config: new keys `ai.cli_command`; `Default()` provider becomes `cli`, `cli_command` `claude`, model `claude-sonnet-5-5` (still the latest Sonnet-class ID; re-check at implementation time). `Validate`: provider in {cli, ollama, anthropic, openai}; `cli` requires allowlisted command and does NOT require `api_key_env`; `ollama` requires model; api providers unchanged. Existing config files that name a provider keep working unchanged (no silent switch).
- `scribex config test` works for all four (one tiny call).

Files: `pkg/config/config.go` (+test), `pkg/llm/llm.go` (+ new `pkg/llm/cli.go`, `pkg/llm/ollama.go`), `pkg/trust/trust.go` only if needed for loopback, `cmd/scribex/cmd_config.go`, `docs/user` config section.

Failing tests first:
- `config`: `TestValidateProviders` (cli ok without key env; ollama needs model; unknown rejected), `TestCLICommandAllowlist` (`/bin/sh`, `claude --x`, `../claude`, empty rejected), `TestDefaultIsCLI`, `TestExistingAnthropicConfigUnchanged`.
- `llm/cli_test.go` with a **fake executable** written to a temp dir placed first on PATH named `claude`: asserts argv equals the table, stdin equals prompt, cwd is NOT the repo and is empty, env contains none of `ANTHROPIC_API_KEY`/`GITHUB_TOKEN`/`ATLASSIAN_TOKEN` even if set in the test process, timeout kills a sleeping fake, >2 MiB output errors, non-zero exit returns redacted error, repo files unchanged.
- `TestCLIAgenticRefused` for a table entry flagged agentic.
- `TestModelValidation`: empty ok for `cli`; `-rf`, `--dangerously-skip-permissions`, `a b`, 101 chars, newline rejected; `claude-sonnet-5-5`, `sonnet`, `llama3.2:3b` accepted; accepted model is exactly one argv element in the fake-executable test.
- `TestCLITableComplete`: every entry has Bin, Args, and either ModelFlag or documented "none"; `claude`/`codex`/`gemini`/`agy` present; Suggested freshness check (above).
- `TestConfigModels` (`scribex config models --cli claude` prints the four Claude IDs).
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

---
## Card 4 - `pkg/generate` plan phase (no model calls)

Depends on cards 1-3 landed. Reuse, do not re-implement: `docs.Files` / `docs.FilesWithSkips` (file walk + skips), `docs.Symbols(root, file)` and `symbols.ExtractDetailed` (public symbols + parser kind), `anchors.Process` / `docs.Check` (existing anchors, stale/dead state), `doctypes.Load/Resolve`, `safepath`/`fsx` for every path.

Behaviour:
- `generate.Plan(root string, opts Options) (Plan, error)`; `Options`: doc type ids (default = enabled types), include/exclude globs, `RunID` (default `YYYYMMDD-HHMMSS-<6hex>`).
- For each enabled doc type produce `Items`: `{TypeID, Unit (module|package|feature|file|symbol key), Sources []path@sha256, Output path, Action (create|update|skip), Reason, ParserKind (ast|lexical), EstCalls, EstTokens}`.
  - `symbol` types: one item per source file with public symbols; `update` if the output file exists with `@scribex:anchor` blocks for them (list new/changed/dead), `create` if absent, `skip` if every symbol is anchored and `sig=` current.
  - `derived` types: one item per unit; `skip` if the output exists and all recorded `sources=` hashes still match; `update` otherwise.
  - `human` types: skeleton `create` only when the output is absent; EstCalls 0.
  - Never plan to touch an output file whose non-`@scribex:ai` content would change.
- Token estimate: `ceil(bytes/4)` of the content that would be sent plus a fixed per-call overhead const; documented as an estimate, never billing-accurate.
- Totals + `WouldSend []path` (unique files whose content leaves the machine for non-local backends; empty for `ollama` on loopback).
- Writes ONLY `.scribex/runs/<RunID>/plan.json` (schema version field `"v":1`, stable ordering, no timestamps inside items so two plans on an unchanged repo are byte-identical) and `plan.md` (human table per doc type with coverage numbers and a "parser kind" column). `.scribex/runs/` is added to the auto-generated `.gitignore` entry the way `secrets.env` is today.
- No network, no exec of model CLIs, no repo file outside `.scribex/runs/` is written.

CLI: `scribex generate [-C dir] [--types a,b] [--include g] [--exclude g] [--json]` (no `--apply` yet: prints plan.md path and totals; exit 0; exit 2 on bad flags/doctypes; exit 1 only if the repo is dirty in a way that blocks a later apply, reported as a warning line).

Failing tests first (`pkg/generate/plan_test.go`, fixtures under `testdata/` with a tiny Go+Kotlin+TS repo):
- `TestPlanDeterministic`: two runs byte-identical `plan.json`.
- `TestPlanSymbolCreateUpdateSkip`: table of (no doc | doc with stale sig | doc fully current) -> create|update|skip with correct symbol lists.
- `TestPlanDerivedSourceHash`: unchanged sources -> skip; modify one source -> update.
- `TestPlanNeverTouchesHandWritten`: output file with hand prose + one ai section -> item Action update limited to the ai section, reason recorded.
- `TestPlanWritesOnlyRunsDir`: snapshot repo tree before/after.
- `TestPlanWouldSendEmptyForLoopbackOllama` / `TestPlanWouldSendListsFilesForCloud`.
- `TestPlanParserKind`: `.go` = ast, `.kt/.ts/.py/.java` = lexical.
- `TestPlanRespectsExcludeAndSymlinks`: symlinked file/dir skipped and listed in skips.
- CLI e2e: `generate` on fixture repo prints totals; `--json` is valid JSON matching schema.

Done when: plan on this repo (`scribex-ai` itself) runs in under 5s, creates nothing but `.scribex/runs/<id>/`, and a second run is byte-identical.

---
## Card 5 - Apply phase for `symbol` grounding (tech-reference, api-reference)

Depends on card 4, card 2 (`llm.New` returning `Completer`), card 3.

Behaviour: `generate.Apply(root, runID, opts)`; CLI `scribex generate --resume <runID> --apply [--confirm-send] [--max-calls N]`.
- Loads `plan.json`, REFUSES if repo HEAD or any recorded source hash differs from the plan ("plan is stale; re-run plan"), if doc files are dirty, HEAD is detached, or (non-local backend) `--confirm-send` is absent. Same precondition helpers as `sync -commit` (`gitx`); do not duplicate them, extract a shared function if needed.
- For each `create`/`update` item, in plan order, until `ai.max_calls` or `ai.max_tokens` is reached:
  1. Create/refresh `<!-- @scribex:anchor(file#Symbol) -->` blocks for each public symbol. Placeholder prose for `create` is a single line `<!-- @scribex:ai --> TODO` style marker per existing convention; do NOT call the model for anchor/signature generation (deterministic via `anchors.Process(sync=true)`).
  2. Prose only for symbols whose signature is new/changed, by calling the model through the existing `prose` path: output must pass `prose.Validate(old, new, secrets...)` and `prose.StripAIMark`/`Clean`; invalid output is dropped and the item marked `failed:invalid-output` (prose left as the placeholder), run continues.
  3. Prompts: system prompt (+ `ai.style_guide`) and the doc type `template` skeleton; user prompt = symbol signature + the symbol's source excerpt (capped), wrapped as untrusted data with the same delimiter helper `prose`/`mdsafe` use today.
- Per-item state recorded in `.scribex/runs/<id>/state.json` (`pending|done|failed|skipped`, calls used, error text redacted); a crash then `--resume` continues, never redoing `done` items and never exceeding totals across resumes.
- Ceiling hit: stop cleanly, exit 1, remaining items stay `pending`, print counts.
- After all writes run `docs.Check(root, false, nil)`; if it reports any issue for files this run touched, exit 1 and list them (a green `check` is the contract).
- Writes only through `fsx.WriteFileIn`, only to `Output` paths from the plan, never hand-written regions; a second `--apply` of the same plan on an unchanged repo makes zero writes and zero model calls.

Failing tests first (`pkg/generate/apply_test.go`, fake `Completer` returning canned text and counting calls):
- `TestApplyCreatesAnchoredDocAndCheckPasses`.
- `TestApplyIdempotent`: second apply = 0 calls, 0 file changes (compare tree hash).
- `TestApplyRefusesStalePlan`, `...DirtyDocs`, `...DetachedHead`, `...CloudWithoutConfirmSend`.
- `TestApplyHonoursMaxCallsAndResumes`: ceiling 2 of 5 items -> exit 1 -> resume with new budget finishes without redoing items; total calls == items.
- `TestApplyDropsInvalidModelOutput`: output with a secret-shaped string, with markdown/HTML injection, with a forged `@scribex:anchor` marker -> rejected, placeholder kept, `check` still green.
- `TestApplyPromptTreatsSourceAsData`: source excerpt containing "ignore previous instructions" is delimited; fake asserts it appears only inside the data block.
- `TestApplyNeverWritesOutsideOutputs`: tree diff limited to plan outputs + `.scribex/runs/`.
- CLI e2e with a fake `claude` on PATH (card 2 fake executable): full `generate --apply` on the fixture repo yields a PR-ready tree and `scribex check` exits 0.

Done when: on the `scribex-ai` repo itself with a fake completer, apply then `check` is green and a second apply is a no-op.

---

## Card 6 - Derived and human grounding, provenance, stale-by-source

Depends on 4, 5. Extends apply and `check`.

Behaviour:
- Section format for `derived` output (inside the generated doc, one block per unit):
  `<!-- @scribex:derived(id=<typeID>:<unit>) sources=<path>@<sha256-12>,<path>@<sha256-12> -->` ... prose ... `<!-- @scribex:end -->`, prose begins with the fixed banner line `> AI draft from code and specs. Confirm before relying on it.` Parser lives next to `pkg/anchors` (new file, same safety rules: no nested/forged markers inside prose, size caps, canonical form so output is stable).
- Apply for `derived`: build prompt from the doc type `template` skeleton + `inputs` + source excerpts (caps per file and per unit, all wrapped as untrusted data); model output passes `prose.Validate`, `mdsafe` sanitising, and a required-sections check against the template headings (missing heading = reject, keep previous/placeholder).
- Must not invent facts it cannot see: the system prompt instructs "use only the provided material; write `Unknown: <what>` when information is absent"; a test asserts that instruction is present. Requirement IDs: when `id_policy: preserve`, post-validate that the output contains no ID matching the repo's ID regex (config `id_pattern` on the doc type, e.g. `REQ-[A-Z]+-\d{3}`) that is absent from `inputs`; violation = reject.
- `human` types: apply writes the skeleton only (`<!-- @scribex:human(id=...) -->` with headings and `TODO(owner)` lines); never calls the model; never overwrites existing content.
- `scribex check`: new finding kind `stale-source` for a derived block whose recorded source hash no longer matches the file on disk (and `dead-source` if a source file is gone). Deterministic, zero tokens, exit 1 like other findings, listed in the PR report; its message says "regenerate or confirm", never "incorrect".
- `scribex confirm <doc>#<id>` (new, small): a human re-stamps `sources=` to current hashes and removes the banner after review, adding `confirmed=<YYYY-MM-DD>`; refuses if the working tree has other changes to that file.

Failing tests first: `TestDerivedBlockRoundTrip` (parse/render canonical), `TestDerivedRejectsForgedMarkers`, `TestStaleSourceDetected` / `TestDeadSourceDetected` / `TestUnchangedSourcesClean`, `TestApplyDerivedRejectsMissingRequiredSection`, `TestApplyDerivedPreserveIDsRejectsInventedID`, `TestApplyHumanNeverCallsModel`, `TestApplyHumanNeverOverwrites`, `TestCheckReportsStaleSourceInPRReport`, `TestConfirmRestampsAndRefusesDirty`, prompt test for the "Unknown:" instruction.

Done when: generating a feature-design doc on the fixture repo, editing one source file, then `check` exits 1 with `stale-source`, and `confirm` after a human edit makes it green.

---
## Card 7 - Socratix council review (optional)

Depends on 2 (CLI table), 6. Parent spec section "Socratix council review".

Behaviour:
- Add `socratix` to the allowlisted CLI table as a **reviewer-only** entry (`Reviewer: true`: cannot be selected as `ai.cli_command`). Invocation: `socratix ask -mode <premortem|redteam|tenthman> [-provider p] [-model m] [-rounds n] -out <tmpfile>` with the review prompt on stdin if `ask` accepts it, otherwise as the single positional argument (implementer verifies with `socratix ask -h`; flags above are confirmed for the current build). Same exec hardening as card 2 (no shell, empty temp cwd, scrubbed env but pass through that provider's key env var name from a table, timeout 300s, 1 MiB memo cap).
- Config: `ai.review` = `off` (default) | `socratix`; `ai.review_mode` default `redteam`; per doc type `"review": "socratix"` overrides on. Counts against `ai.max_calls` (one call per reviewed unit).
- Review prompt = the draft section + the same source excerpts (untrusted-data delimited) + instruction to list claims not supported by the sources, one per line prefixed `UNSUPPORTED:`. Scribex parses only lines with that prefix from the memo (everything else ignored, caps: 20 lines, 300 chars each, sanitised through `mdsafe`).
- Result recorded in the derived block header: `review=socratix mode=redteam verdict=clean|disputed` and, if disputed, an indented `> Disputed claims:` list under the banner. `disputed` also makes `check` print a `disputed` notice (non-failing by default, failing with `--strict`).
- Missing `socratix` binary or non-zero exit: record `review=skipped` (with reason code, no raw stderr), keep the draft, run continues. Never record `clean` unless a memo was actually parsed.
- Privacy: reviewing sends the draft + excerpts to whatever provider Socratix uses, so it requires the same `--confirm-send` unless Socratix's provider is `ollama` on loopback (decided by the `-provider` value Scribex passes; if unset Scribex must pass `-provider` explicitly, never let Socratix pick the first key silently).

Failing tests first (fake `socratix` executable on PATH): argv/cwd/env assertions, `TestReviewParsesOnlyPrefixedLines`, `TestReviewCapsAndSanitises`, `TestReviewMissingBinarySkips`, `TestReviewNeverCleanWithoutMemo`, `TestReviewRequiresConfirmSendForCloud`, `TestReviewDisabledByDefault`, `TestReviewerEntryNotSelectableAsBackend`, `TestStrictCheckFailsOnDisputed`.

Done when: with a fake `socratix` returning two `UNSUPPORTED:` lines, a generated feature doc shows `verdict=disputed` with both lines, and with no `socratix` installed the same run completes with `review=skipped`.

---
## Card 8 - Publish phase (branch, commit, optional push)

Depends on 5, 6. Scribex does not create PRs itself: the existing CI pattern uses `git` + `gh`, which keeps tokens out of the engine.

Behaviour: `scribex generate --resume <id> --apply --branch [--push]`.
- `--branch`: before the first write, create and switch to `scribex/docs-<shortsha>` (or `ai.branch_prefix` + `<shortsha>`, prefix validated `^[a-z0-9][a-z0-9/_-]{0,40}$`); refuse if it exists with different content. Never commits to the default or current protected branch. Writes only plan outputs, then ONE commit of exactly those paths (`git commit --only -- <paths>`, hooks neutralised exactly like `sync -commit`, identity from git config or `SCRIBEX_GIT_NAME/EMAIL`, refuse if none). Commit message: `docs: scribex generate <runID> (<n> files, types: a,b)` plus trailer lines listing `review=` summary counts; no model text in the message.
- `--push`: `git push origin HEAD:refs/heads/<branch>`; fails closed on any error, never force. Prints the branch name and, when remote is github.com/GHES/GitLab, the compare/MR URL built from the remote (validated, printed only, never fetched).
- Without `--branch` the run leaves changes in the working tree (as `sync` does) and prints a one-line summary.
- `plan.md`/state record the branch and commit sha.

Failing tests first (temp git repos): `TestPublishCreatesBranchAndSingleCommit`, `TestPublishTouchesOnlyPlannedPaths` (unrelated dirty file stays unstaged), `TestPublishRefusesExistingDivergentBranch`, `TestPublishHooksNeutralised`, `TestPublishNoIdentityRefuses`, `TestPushNeverForces` (bare remote), `TestCompareURLBuilders` table (github, ghes, gitlab, ssh remotes, malformed -> empty), `TestBranchPrefixValidation`.

Done when: from a clean checkout, `generate --apply --branch --push` against a bare-remote test repo leaves default branch untouched and the new branch with one commit.

---
## Card 9 - Dashboard: AI settings and doc-type editor

Depends on 2, 3. Extends `pkg/dashboard` (single file today; add `generate.go` in the package rather than growing `dashboard.go`).

Behaviour (all admin-role only, repo must be under `-allowed-root`, per-repo):
- `GET /api/settings?repo=<name>` -> non-secret AI settings (provider, model, cli_command, base_url, style_guide, max_tokens, max_calls, review, review_mode, branch_prefix) + `secrets` map of NAME -> set|unset (never values) + `suggestions` from `config models`. `PUT /api/settings?repo=` with a JSON body of allowed keys only; each key goes through `config.Set`-equivalent validation (reuse, do not copy); unknown keys 400; response is the new effective settings. `api_key_env` is editable only as a NAME (existing `ValidateAPIKeyEnv`). There is NO endpoint that accepts a secret value.
- `GET/PUT /api/doctypes?repo=` -> catalog (effective) / overrides; PUT validates with `doctypes.Validate` and returns all problems (400 with list) or saves via `doctypes.Save`.
- Pages: extend the existing plain-JS UI with "AI settings" and "Doc types" tabs (checkbox per type, audience, grounding badge, output path, template path, scope globs). No new JS framework; CSP unchanged; any inline script/style follows the existing hash approach in `computeStyleHash`.
- CSRF: browsers auto-send Basic auth, so every state-changing request requires header `X-Scribex-CSRF` equal to a token from `GET /api/csrf` (HMAC of principal + day + server secret generated at start) AND a same-origin `Origin`/`Sec-Fetch-Site` check; missing = 403. Applies to all PUT/POST added in cards 9-10.
- Audit: each successful write logs principal, repo, keys changed (names only) via the existing access logger.
- Existing read-only `/config` page stays and links here.

Failing tests first (`pkg/dashboard/generate_test.go`, `httptest`): `TestSettingsRequireAdmin` (viewer 403, anonymous 401), `TestSettingsNeverReturnsSecretValues` (set a fake key in env, grep response), `TestPutSettingsRejectsUnknownAndInvalid`, `TestPutSettingsRejectsKeyLikeValueInAPIKeyEnv`, `TestRepoOutsideAllowedRootRefused`, `TestCSRFRequired` (no header, wrong header, cross-site Origin), `TestDoctypesPutValidationErrors`, `TestDoctypesRoundTrip`, `TestWritesAreAudited`, `TestCSPUnchanged`.

Done when: an admin can change provider/model and enable `feature-design` from the browser, `.scribex/config.yaml` and `doctypes.json` update, and a viewer or cross-site form post cannot.

---
## Card 10 - Dashboard: Generate docs page and job runner

Depends on 4, 5, 8, 9.

Behaviour:
- Disabled unless `scribex serve --enable-generate`. When enabled and the bind address is not loopback, TLS (or `-behind-proxy`) is required (reuse `ValidateServeConfig` rules); otherwise `serve` refuses to start.
- Endpoints (admin, CSRF, allowed-root): `POST /api/generate/plan` body `{repo, types[], include[], exclude[]}` -> starts job, returns `{runId}`; `POST /api/generate/apply` body `{repo, runId, confirmSend bool, branch bool, push bool}`; `GET /api/runs?repo=` (history from `.scribex/runs/*/state.json`); `GET /api/runs/{id}?repo=` (state + plan summary + log tail); `POST /api/runs/{id}/cancel`.
- Job runner: re-executes the same binary (`os.Executable()`) as `scribex generate ...` with cwd = repo, env scrubbed to the same allowlist as card 2 plus the configured `api_key_env` NAME's value if set; stdout/stderr captured to `.scribex/runs/<id>/log.txt` with secret redaction and a 1 MiB cap; wall-time limit (default 30 min, `--generate-timeout`); exactly one running job per repo (second request 409); process group killed on cancel/timeout; server restart marks orphaned `running` jobs `interrupted`.
- `confirmSend` must be explicitly true for non-local backends; the UI shows the plan's `WouldSend` file list and a checkbox next to the Apply button.
- UI: repo picker, doc-type checkboxes, "Plan" -> table (type, unit, action, est. tokens, parser kind, WouldSend), per-type include toggles, "Apply" with confirm checkbox, progress by polling every 2s, run history with links to branch/compare URL. Reports `disputed` counts and `stale-source` counts.
- No websockets, no queue, no database.

Failing tests first: `TestGenerateDisabledByDefault`, `TestServeRefusesGenerateOnPublicBindWithoutTLS`, `TestPlanJobRunsAndReportsState` (fake binary via env override `SCRIBEX_SELF` used in tests only), `TestSecondJobSameRepoConflicts`, `TestApplyRequiresConfirmSendForCloud`, `TestCancelKillsProcessGroup`, `TestTimeoutKills`, `TestLogRedactsSecretsAndCaps`, `TestOrphanedRunsMarkedInterrupted`, `TestRunIdPathTraversalRejected` (`../`, absolute, symlink), `TestViewerCannotRun`, `TestEnvScrubbed` (child sees none of `GITHUB_TOKEN`, `ATLASSIAN_TOKEN`).

Done when: from the browser, plan -> review table -> apply (with fake backend) produces a branch commit and the history list shows it; a viewer, a cross-site POST, or a second concurrent run is refused.

---
## Card 11 - CI generate workflow

Depends on 1, 8.

Behaviour: `scribex ci init github` additionally writes `.github/workflows/scribex-generate.yml`; `ci init gitlab` adds a `scribex-generate` job to `.gitlab-ci.scribex.yml` (manual, `when: manual`).
- Trigger: `workflow_dispatch` only, with typed inputs `types` (string, default `tech-reference`), `include`, `backend` (choice: cli, ollama, anthropic, openai), `confirm_send` (boolean, default false). No `pull_request`, no `pull_request_target`, no schedule.
- Same hardening as the existing workflows: SHA-pinned actions, top-level `permissions: {}`, job permissions `contents: write` + `pull-requests: write` only on the generate job, `environment: scribex-ai` for secrets, inputs passed via `env:` and validated by regex in the script (never interpolated in `run:`), card-1 pinned install step, concurrency group per ref.
- Script: `scribex generate --types "$TYPES" ... ` (plan) -> if `confirm_send` or backend local -> `--apply --branch --push` -> `gh pr create` with title `docs: scribex generate <runID>` and a body built from `plan.md` totals and `disputed`/`stale-source` counts (body from a file generated by scribex, size-capped; no PR-controlled text).
- `cli` backend is NOT offered in the hosted-runner workflow choices by default (no logged-in vendor CLI on a runner); it is listed only with a comment explaining self-hosted runners.
- Fails the job if `scribex check` is non-zero after apply.

Failing tests first (extend `ci_templates_test.go`): `TestGenerateWorkflowHardening` (pins, permissions, no PR triggers, inputs only via env, regex validation present, `environment` set), `TestGenerateWorkflowActionlint`, `TestGenerateWorkflowNoCLIBackendChoiceOnHosted`, `TestGitlabGenerateJobManualOnly`, existing hardening tests include the new file automatically.

Done when: actionlint passes and a dry run (`workflow_dispatch` with `confirm_send=false`, backend `ollama` unavailable) fails closed with a clear message instead of publishing.

---
## Card 12 - Acceptance, docs, release gate

Depends on 1-11.

- Real-repo runs (record results in `scribex-ai/docs/user/generate-acceptance.md`, with commands and counts, no screenshots of private code):
  1. On `scribex-ai` itself with a fake completer: plan, apply, `check` green, second apply no-op, wall time and call counts noted.
  2. On a copy of a real Android/Kotlin app repo: types `tech-reference` and `api-reference` first, count symbols the lexical scanner skips or mis-parses (hand-sample 30), report the number honestly in the doc.
  3. Derived types on the same copy with `ai.review=off` then `socratix`: hand-count unsupported claims per 10 sections each way; publish both numbers, whatever they are. If review shows no benefit, say so and leave it off by default.
  4. Idempotence and crash-resume run (kill mid-apply, resume).
- Security review pass before release (checklist doc, tests named): arbitrary-command config attempt, path traversal via doctype `output`/`template`/`inputs`, prompt-injection in source files and in existing docs, secret in model output, forged anchor/derived markers, CSRF, viewer privilege, run-id traversal, env leakage to child processes, symlink swaps between plan and apply.
- User docs: `docs/user/generate.md` (CLI, dashboard, CI, doc types, grounding meaning, privacy/`--confirm-send`, CLI/model table), update `README.md` command table and `docs/user/ci.md`; suite docs: Scribex entry in root `README.md` and `CHANGELOG.md` (new minor version section) and `docs/scribex-ai/03_USER_GUIDE_AND_CLI_REFERENCE.md`. State limits plainly: lexical scanners, derived docs unverified, review unproven.
- `go vet ./... && go test -race ./...`, `govulncheck`, fuzz targets for the new parsers (derived block, doctypes JSON, plan JSON) run in `scribex.yml`, SBOM unchanged (no new deps), reproducible-build check still green.
- Version bump to the next minor in `cmd/scribex/main.go` and release notes listing known gaps (unsigned binaries, Windows untested, Kotlin Compose coverage).

Done when: every item above has a recorded result, the release gate in `scribex.yml` is green, and the numbers from acceptance step 2 and 3 are in the docs as measured, not projected.
