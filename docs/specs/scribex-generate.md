# scribex-generate (`scribex generate` + dashboard "Generate docs")

Goal: on-demand, one-time (re-runnable) whole-repo doc scan that creates or updates anchored docs, reviewed as a PR. Occasional use, not a daemon.

Status: IMPLEMENTED in scribex-ai v1.1.0 (cards 1-12). Deviations and limits: see scribex-ai/docs/user/generate.md "Known limits".

Implementation notes that differ from the design: the dashboard plans in-process and runs apply as a child process; the Socratix verdict is parsed from `UNSUPPORTED:` lines in the memo (heuristic, unmeasured); `test-requirements` reads nearby/same-stem test files only; dead anchors are reported by check and plan (with `-prune`, AI-managed orphan sections are removed; hand-written orphans are never deleted automatically); hosted CI runners cannot use a vendor CLI.

## Decisions
- Scope: whole repo (all supported languages), narrowed by `--include/--exclude` globs.
- Entry points: CLI (`scribex generate`), dashboard page (admin), CI `workflow_dispatch`. All three call ONE engine package; no logic in the UI or CLI layer.
- LLM backends, configurable, default order **cli > ollama > api**: `cli` (runs `claude`/`codex`/`agy`-style vendor CLIs as subprocess), `ollama` (local), `anthropic`, `openai` (API keys). Today only anthropic|openai exist in `pkg/config`/`pkg/llm`; extend.
- Model names are config, never hardcoded in logic. Defaults: Anthropic `claude-sonnet-5-5` (latest at time of writing: fable-5-1, opus-5-5, sonnet-5-5, haiku-5-5). OpenAI/Ollama/CLI: no default; `config validate` fails if unset. Re-check latest names when implementing.
- No new infra: no DB, queue or scheduler. Job state = files under `.scribex/runs/<id>/`.

## Engine: `pkg/generate` (new)
Phases, each resumable from `.scribex/runs/<id>/plan.json`:
1. **Plan** (default, no model call): scan repo via `pkg/symbols`; per file list public symbols, existing anchors, coverage (anchored / unanchored / stale); estimate calls+tokens; list files whose content would be sent to a model. Writes plan.json + `plan.md`.
2. **Apply** (`--apply`, needs `--confirm-send` when backend is not local): for each approved item either insert a new anchored section (new doc file or appended block) or update an existing `<!-- @scribex:ai -->` section. Then run `sync` to write `sig=` and signature blocks.
3. **Publish** (`--branch` / `--push`): commit to `scribex/docs-<shortsha>` using `gitx` (hooks neutralized, as `sync -commit`); PR is opened by CI (`gh pr create`) or by hand, never by the engine. Never pushes to the default branch.

## Doc types (what to generate) — `.scribex/doctypes.json`
User picks which kinds of documentation to produce; each kind is a config entry, built-ins ship as defaults, users add custom ones. Dashboard edits the same file (non-secret).

Fields per type: `id`, `title`, `audience` (dev|qa|po|exec|ops), `enabled`, `granularity` (repo|module|package|feature|file|symbol), `include`/`exclude` globs, `output` path template (`docs/tech/{module}.md`), `grounding`, `template` (path to markdown skeleton with required sections + prompt), `inputs` (extra human context files, e.g. a product brief), `id_policy` (`none`|`preserve`), `max_calls` share, `language`.

`grounding` is the key honesty switch:
- `symbol`: anchored to code symbols; fully drift-gated by `check` (AST/lexical contract hashes).
- `derived`: summarised from code/tests/specs with no single symbol (feature design, test requirements, requirements, PO/business). Not contract-gated. Each section records `sources=[path@hash,...]` and is marked `<!-- @scribex:ai -->` + "AI draft, confirm" banner; `check` reports it **stale-by-source** when any source hash changes (cheap, deterministic, no model). Never claimed as verified.
- `human`: skeleton only; Scribex never writes prose (e.g. business rules, priorities, acceptance sign-off).

Built-in catalog (all disabled except `tech-reference` until the user enables):
| id | audience | granularity | grounding |
|---|---|---|---|
| tech-reference | dev | symbol | symbol |
| architecture-overview | dev | module | derived (+ `diagram` Mermaid from imports) |
| api-reference | dev | package | symbol |
| feature-design | dev/po | feature | derived |
| test-requirements | qa | feature | derived (from tests + code; coverage gaps listed, no invented cases) |
| requirements | po | feature | derived or human |
| po-feature-summary | po/exec | feature | derived |
| runbook-ops | ops | repo | derived |

"feature" needs a discovery rule (not guessable): `features:` list in `.scribex/doctypes.json` mapping name -> globs, or auto-heuristic by top-level package/screen/route; plan shows the discovered list for approval before any model call.

Rules:
- `id_policy: preserve` (required for repos with immutable requirement IDs, e.g. a consuming app repo using `REQ-*`): generate never mints, renumbers or deletes IDs; may only fill text for existing IDs and list "unmapped" candidates for a human.
- Derived/human types may read existing docs and a product brief as inputs; plan shows them.
- Plan output is per doc type: files, calls, tokens, coverage; user approves per type.
- Existing hand-written docs at an `output` path are never overwritten; generate appends/refreshes only `@scribex:ai` sections.

## Socratix council review (optional, derived docs only)
Decision: Socratix is an optional reviewer, not Scribex's LLM transport. Bulk `symbol` docs use one direct model call; the council would multiply cost for no gain.
- Applies only to `grounding: derived` sections, per feature (bounded cost), after the draft is written. Off by default; enabled per doc type (`"review": "socratix"`) and globally by `ai.review` config.
- Integration is exec of `socratix ask` added to the allowlisted CLI table (no daemon, no login, no Go dependency; Scribex stays stdlib-only and fully works without Socratix). Verified surface: `socratix ask -mode premortem|redteam|tenthman -provider <p> -model <m> -rounds N -out <file>`; it uses Socratix's own provider keys or Ollama, not vendor CLI logins. Scribex input = draft + the listed source excerpts, wrapped as untrusted data; output = memo file read back and size-capped.
- Result is stored on the section: `review=socratix mode=redteam sources=[...] verdict=clean|disputed` plus the memo's unsupported-claim list. `disputed` sections render a visible banner and never auto-merge-quietly; humans decide.
- Same privacy gate as any model call (`--confirm-send`); counts against `ai.max_calls`.
- If `socratix` is missing: skip review and record `review=skipped` (never pretend it ran).
- Measure before trusting: acceptance card runs generation with and without review on a real repo and hand-counts caught unsupported claims; results go in docs. No claim that review improves quality until that exists.

## Invariants (each needs a failing test first)
- Hand-written prose is never modified; only sections marked `<!-- @scribex:ai -->` are regenerated.
- Idempotent: second `generate --apply` on an unchanged repo produces zero diff; `scribex check` exits 0 afterwards.
- Refuses on dirty doc files, detached HEAD, or no git identity (reuse `sync -commit` preconditions).
- Hard ceilings, all configurable: `ai.max_calls`, `ai.max_tokens`, wall-time; stops cleanly, plan keeps remaining items pending.
- All writes confined via `safepath`/`fsx`; model output sanitized via `mdsafe`/`docs/sanitize` before write; repo content wrapped as untrusted data in prompts (prompt-injection safe); model text never executed.
- Plan lists exactly which files leave the machine; cloud backend without `--confirm-send` (or dashboard checkbox) fails closed.
- Secrets never in plan.json, logs, or dashboard; CLI backend needs none.
- Per-language honesty: plan reports parser kind (AST vs lexical) per file so coverage claims are not overstated.

## CLI
`scribex generate [-C dir] [--types id,id] [--include g] [--exclude g] [--backend cli|ollama|anthropic|openai] [--model m] [--cli claude|codex|gemini|agy] [--max-calls N] [--apply] [--confirm-send] [--branch] [--push] [--resume <id>] [--json]`
Exit 0 ok/no-op, 1 issues (ceiling hit, refused), 2 usage.

## Dashboard (extends `pkg/dashboard`, `scribex serve`)
- New admin-only pages: **AI settings** (backend, model, CLI command, ceilings, branch prefix, PR target), **Generate docs** (pick repo, scope, Plan -> review table -> Apply), run history with diff summary and PR link.
- Settings editing writes non-secret keys to `.scribex/config.yaml` only. Secrets stay env/`scribex secrets set`; the page shows set/unset, never values (existing rule kept).
- "Run" executes the engine as a subprocess of the server, one job at a time per repo, only for repos under `-allowed-root`; CSRF token + admin role required; default bind stays 127.0.0.1. Non-localhost needs TLS + RBAC (existing flags).
- CLI backend on a shared server runs under the server user's CLI login: document this; recommend localhost use or the CI route for shared setups.
- Progress via polling `GET /api/runs/<id>` (no websockets).

## CI
`scribex ci init github|gitlab` adds a `scribex-generate` job: `workflow_dispatch` / manual pipeline only, protected environment for secrets, inputs: scope, backend, `--confirm-send`. Opens a PR; humans review via CODEOWNERS.
Prerequisite fix (separate card): generated workflows must install a pinned, checksummed release binary (`SCRIBEX_RELEASE_URL` + `SCRIBEX_SHA256`) instead of `go build ./cmd/scribex`, which only works inside this repo.

## Not needed
Scheduler/cron, queue, database, websockets, auto-merge, multi-user job concurrency, auto-run on every push (that is `sync`/`check`).

## Cards (order)
1. CI binary-install fix (`ci init`).
2. Backend abstraction in `pkg/llm`: cli + ollama + existing APIs; config keys; `config validate`.
3. Doc-type catalog: `doctypes.json` schema, loader, validation, built-in defaults, `scribex doctypes list|validate`.
4. `pkg/generate` plan phase + plan.json schema + coverage report, per doc type; feature discovery.
5. Apply phase (insert/update, sync, idempotence, ceilings, resume); `symbol` grounding first (tech-reference, api-reference).
6. Derived grounding: `sources=[path@hash]` provenance, stale-by-source in `check`, AI-draft banner; then `human` skeletons.
6b. Socratix council review of derived sections (`ai.review`, `"review": "socratix"`, `review=` provenance, `disputed` banner).
7. Publish phase (branch, commit, PR).
8. CLI wiring + e2e tests (`--types tech-reference,feature-design`).
9. Dashboard AI settings + Doc types editor (enable/disable, templates, scope).
10. Dashboard Generate page + job runner + auth/CSRF.
11. CI `scribex-generate` job template (input: doc types).
12. Real-repo acceptance: consuming-app copy (with `id_policy: preserve`), then `scribex-ai` itself; second run must be a no-op.

Where: scribex-ai/pkg/{generate,llm,config,dashboard,ci}, cmd/scribex.
