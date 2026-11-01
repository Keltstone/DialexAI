# `scribex generate`: whole-repo documentation

On-demand scan that creates or refreshes docs for the whole repository, reviewed as a normal PR. It is an occasional job, not a daemon. The CLI, the dashboard and the CI workflow all call the same engine (`pkg/generate`).

## Honesty model (read this first)

Every doc type has a `grounding`:

| grounding | what Scribex writes | what `check` guarantees |
|---|---|---|
| `symbol` | one anchored section per public symbol, prose from the model | signature drift is gated like any anchor; **prose accuracy is not verified** |
| `derived` | summaries (feature design, test requirements, PO summary, runbook...) with `sources=path@hash`, an "AI draft, confirm" banner | reports `stale-source` / `dead-source` when a source file changes; never claims the content is correct |
| `human` | a skeleton with the required headings only | nothing; people write the prose |

Hand-written prose is never modified: only sections carrying `<!-- @scribex:ai -->` and derived blocks are regenerated.

## Quick start

```bash
scribex generate                                   # plan only, no model call; writes .scribex/runs/<id>/plan.md
scribex generate -resume <id> -apply -confirm-send # write docs from that plan
scribex generate -types tech-reference,feature-design -include 'pkg/**' -apply -confirm-send -branch
scribex check                                      # must exit 0 afterwards
```

A second `-apply` on an unchanged repo makes zero model calls and zero diff.

Flags: `-types a,b` `-include g` `-exclude g` `-backend cli|ollama|anthropic|openai` `-model m` `-cli claude|codex|gemini|agy` `-max-calls N` `-max-wall 30m` `-apply` `-prune` `-confirm-send` `-branch` `-push` `-resume <id>` `-json`. Exit 0 ok, 1 problems (ceiling hit, a symbol rejected, refused), 2 usage.

## Backends

Default is a vendor CLI (`ai.provider: cli`, `ai.cli_command: claude`). Alternatives: `ollama` (local; the only backend that needs no `-confirm-send`), `anthropic`, `openai`. Set with `scribex config set ai.provider ...` or the dashboard. Model names are config (`ai.model`; empty = the CLI's own default). See `scribex config models`.

Any backend except `ollama` sends source files off the machine. The plan lists exactly which files; apply fails closed without `-confirm-send`.

## Doc types

`scribex doctypes list|show <id>|validate`; edit `.scribex/doctypes.json` or use the dashboard. Built-ins (only `tech-reference` is on by default): tech-reference, architecture-overview, api-reference, feature-design, test-requirements, requirements, po-feature-summary, runbook-ops. Custom types need `title`, `audience`, `granularity`, `output`, `grounding`.

`id_policy: preserve` + `id_pattern` (e.g. `REQ-[0-9]+`): the model may only reuse IDs found in the sources/inputs/existing doc; any invented ID rejects the document.

"Feature" granularity needs a rule: `features` (name -> globs) or `"discover": "auto"` in `doctypes.json`. The plan prints the discovered list before anything is sent.

## Safety rules the engine enforces

- Output confined to the repo (`safepath`/`fsx`), `.scribex/runs/` kept out of git via `.git/info/exclude`.
- Source is wrapped as untrusted data; delimiter look-alikes and secret-shaped lines are neutralised before sending; all model text passes `prose.Validate` (no HTML, headings, links, markers, secret shapes) and a rejected symbol is simply not written.
- Refuses: detached HEAD, uncommitted changes in the target doc files, HEAD moved since the plan.
- Ceilings: `ai.max_calls` (or `-max-calls`), `-max-wall`. On a ceiling the run stops cleanly; remaining items stay `pending` and `-resume` continues.
- Publish (`-branch`/`-push`) commits only the written files to `<ai.branch_prefix>/docs-<sha>`; it never touches your branch and never opens a PR itself (use the printed compare link, or CI).

## Optional Socratix review (derived docs only)

`scribex config set ai.review socratix` (or `"review": "socratix"` on one type). Runs `socratix ask -mode <ai.review_mode>` over each derived draft + its sources; unsupported claims it lists (`UNSUPPORTED:` lines) are shown as a `> Disputed:` banner and `review=...:disputed` on the section. `check -strict` fails on disputed sections. If `socratix` is not installed the review is recorded as `review=skipped:not-configured`. The verdict is a text heuristic and its benefit has **not been measured**; do not read "clean" as "verified".

## Dashboard

`scribex serve ... -enable-generate` adds admin-only pages: AI settings, Doc types, Generate docs. Off by default (the dashboard is otherwise read-only). Forms are CSRF-bound per action, same-origin only, no JavaScript. Apply runs `scribex generate -resume <id> -apply` as a child process, one job per repo. CLI backends then run under the server user's login: prefer localhost or the CI route on shared servers.

## CI

`scribex ci init github` writes `scribex-generate.yml` (manual `workflow_dispatch`, protected `scribex-ai` environment, inputs validated, pushes a branch and opens a PR with `gh`). `scribex ci init gitlab` adds a manual `scribex-generate` job (web pipelines only; needs a protected, masked `SCRIBEX_PUSH_TOKEN`). Hosted runners have no vendor-CLI login: use `anthropic`/`openai` (secrets in the environment) or a self-hosted runner with Ollama.

## Known limits

- `test-requirements` also reads test files that sit beside the unit's sources or share a file-name stem with them (heuristic, may miss tests elsewhere); other types never read tests.
- Removed symbols leave dead anchors that `check` reports; with `-prune`, `scribex generate -apply -prune` removes AI-managed orphan sections. Hand-written orphan sections are reported but never removed automatically.
- Derived sections are capped at about 2 KB each by prose validation.
- Kotlin/Java/TS/Python symbol lists come from lexical scanners; the plan shows `ast` vs `lexical` per file.
- `codex` and `gemini` CLIs are not installed on this machine and remain unverified (set `SCRIBEX_ALLOW_UNVERIFIED_CLI=1`). `claude` flags were verified against `claude 2.1.283` `--help`; live calls require active OAuth credentials. `agy` was verified live.
