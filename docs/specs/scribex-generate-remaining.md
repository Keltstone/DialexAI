# Prompt: finish `scribex generate` (remaining work, v1.1.0 -> release)

Paste everything below the line into the implementing AI. Work in `DialexAI/scribex-ai` (Go, stdlib only, `go.mod` has zero dependencies; keep it that way).

---

## Context

`scribex generate` is implemented and green (`go vet ./... && go test -race ./...`). Read first, in this order:
1. `docs/specs/scribex-generate.md` (design) and `docs/specs/scribex-generate-cards.md` (cards 1-12, with acceptance tests).
2. `scribex-ai/docs/user/generate.md` (what exists, honesty model, known limits).
3. Code: `pkg/generate/{scan,units,plan,apply,review,publish}.go`, `pkg/derived`, `pkg/doctypes`, `pkg/llm/{cli,ollama,llm}.go`, `pkg/dashboard/generate.go`, `cmd/scribex/{cmd_generate,cmd_ci}.go`.
4. Repo rules: `DialexAI/CLAUDE.md` and `rules.md` (caveman/ponytail style, never delete files or run destructive commands without confirmation, no commit unless asked, pre-commit exposure scan).

Do not redesign. Each task below says what "done" means; add a failing test first wherever a test is possible. Do not weaken or delete existing tests. Keep `go test -race ./...`, `go vet ./...`, `gofmt -l .` clean after every task.

## Non-negotiable invariants (must stay true)

- Hand-written prose is never modified; only `<!-- @scribex:ai -->` sections and `@scribex:derived` blocks are regenerated.
- Second `generate -apply` on an unchanged repo: 0 model calls, 0 diff, `scribex check` exits 0.
- Cloud/CLI backends fail closed without `-confirm-send`; only `ollama` is local.
- All writes go through `safepath`/`fsx`; all model text passes `prose.Validate`; repo content is delimited as untrusted data and secret-shaped lines are redacted before sending.
- CLI table is code, never config. No arbitrary command from repo config.
- No engine-created PRs. Publish only commits to `<prefix>/docs-<sha>`.
- Never state quality claims that were not measured.

## Tasks (do in this order; commit nothing, report diffs)

### T1. Dead-anchor handling (`pkg/generate`, `cmd/scribex/cmd_generate.go`)
Problem: when a symbol is removed from code, generated sections stay as dead anchors; `check` reports them but nothing fixes them.
Do:
- Plan: for each output doc of a `symbol` type, list anchors whose symbol no longer exists AND whose section is AI-managed (has `@scribex:ai`). Add `Item.Orphans []string` (refs). Hand-written (no ai marker) orphans are only reported, never listed for removal.
- Apply: new flag `-prune` (default off). With it, remove exactly those AI-managed orphan sections (heading line immediately above the anchor through the `@scribex:end` line, plus one blank line). Without it, plan.md shows them under "Orphans (use -prune to remove)".
- Tests: orphan AI section removed with `-prune`, kept without; hand-written orphan never removed; doc stays valid (`docs.Check` clean); idempotent second run; plan lists orphans deterministically.
Done: tests above pass; `generate.md` documents `-prune`; spec "known limits" updated.

### T2. Verify the unverified vendor CLIs (`pkg/llm/cli.go`)
On a machine that has them installed:
- `claude`: confirm `claude -p --tools "" --no-session-persistence --output-format text --model <m>` returns text for a stdin prompt (the build machine's login had expired, so this was never run live). Record the `claude --version` in `Verified`.
- `codex`, `gemini`: read `<bin> --help`, confirm the headless/read-only/stdin flags in `cliTable` really exist and that the tool cannot edit files or run commands in that mode. Fix `Args`/`PromptArg`/`Agentic` to match reality; set `Verified` to the version string and `Checked` to today. If a CLI cannot be run read-only, set `Agentic: true` (it then needs `SCRIBEX_ALLOW_AGENTIC_CLI=1`).
- Add `cli_test.go` cases using the existing fake-binary pattern for any argv change.
Done: each `cliTable` entry has `Verified` set from a real run, or a documented reason it cannot be; `scribex config models` output matches; generate.md "Known limits" updated accordingly. If a CLI is not installed, leave it unverified and say so; do not guess flags.

### T3. Socratix review: real run and measurement
- Install/build `socratix-engine` (`socratix-engine/bin/socratix`), confirm `socratix ask -mode redteam -out f -` works with a real provider key or Ollama. Fix `Review()` in `pkg/generate/review.go` if the real memo format does not contain parsable `UNSUPPORTED:` lines (the prompt asks for them; real councils may ignore this). If the memo format is unreliable, change the parser to a documented, tested one; never treat "no lines parsed" as "clean" without checking the memo is non-empty and mentions the draft.
- Measurement (required before any quality claim): pick a real repo, generate 3+ derived docs twice (review off / on). Hand-count unsupported claims in each draft (open the sources, mark each claim supported/unsupported). Record counts, how many the council caught, false alarms, cost in calls, in `docs/scribex-ai/` (new `generate-review-measurement.md`). State plainly if review did not help.
Done: measurement doc exists with raw counts; `generate.md` "Optional Socratix review" cites it or still says "unmeasured".

### T4. Acceptance runs (card 12)
1. Consuming-app copy: take a copy of a real app repo that has immutable requirement IDs (e.g. `REQ-*`). Add `.scribex/doctypes.json` with `requirements` enabled, `id_policy: preserve`, `id_pattern` for that scheme, `inputs` pointing at the existing requirements doc. Run plan, apply with a real backend. Verify: no new/renumbered/deleted IDs anywhere (diff the ID set before/after), `scribex check` exit 0, second run no-op.
2. `scribex-ai` itself (scratch copy, never the working tree): `tech-reference` + one derived type, real backend, `-max-calls` high enough to finish. Verify `check` exit 0 and the second run makes 0 calls. Record wall time, calls, and files sent.
3. Spot-audit: open 10 random generated symbol paragraphs and 3 derived sections against the code; list every wrong statement. Put the results in `docs/scribex-ai/generate-acceptance.md`. Do not claim accuracy beyond what you counted.
Done: acceptance doc committed-ready; any bug found gets a test + fix.

### T5. Hardening tests not yet written
Add, each failing first if it reveals a bug:
- `pkg/generate`: `doctypes.json` `discover: auto` and explicit `features` map produce expected units on a Kotlin/TS/Python fixture (lexical parsers); a file with a syntax the lexical parser mishandles does not crash the plan (goes to `Skips` or yields fewer symbols, never panics).
- `pkg/generate`: output path template containing `..`, absolute path, or symlinked `docs/` dir is refused by apply (no write outside root).
- `pkg/generate`: model reply that echoes the system prompt, returns 1 MB, returns empty, or duplicates a section heading: item fails cleanly, nothing partial written for that symbol.
- `pkg/generate`: `-resume` after a HEAD change is refused; after a partial run only `pending` items are processed and `done` items are not re-called.
- `pkg/dashboard`: repo name / run id / doc-type id containing HTML or traversal never reaches output unescaped (XSS test on every page); POST with wrong Content-Type; oversized body (>64 KB) rejected; non-admin POST rejected; token for repo A rejected on repo B.
- `cmd/scribex`: `-push` pushes only the new branch (use a local bare remote), never the current branch; `-branch` with nothing written prints "nothing to publish".
- `pkg/llm`: Ollama non-loopback http URL rejected; CLI with prompt > `cliMaxPrompt` rejected; model name starting with `-` rejected.

### T6. Release gate (card 12)
- `scripts/repro-check.sh 1.1.0` and `scripts/build.sh 1.1.0` succeed; update version examples in `scripts/*.sh` and any docs that say 1.0.0 for Scribex only.
- Run `govulncheck ./...` and `osv-scanner` if available; zero new findings. Confirm `.github/workflows/scribex.yml` still passes `actionlint` and still covers the new packages.
- Windows: `GOOS=windows go build ./...` must compile; if `pkg/generate` or `pkg/llm` use anything non-portable (signals, `/dev/null` paths), fix or document.
- Update `scribex-ai/README.md` (feature list + link to generate.md), suite `README.md`/`CHANGELOG.md` only if facts changed.
Done: all green, results listed.

### T7. Hygiene before any commit
- Run the pre-commit exposure scan from `DialexAI/CLAUDE.md` rule 18 on the staged diff and untracked files. Report `file:line` for any hit; do not commit on a blanket approval.
- Nine docs files under `docs/scribex-ai/` (`04_SETUP_SCENARIOS_AND_PLATFORMS_GUIDE.md`, `platforms/*.md`) were modified before this work and are NOT part of it. Keep them out of the generate commit.
- Do not commit unless asked. If asked: suggested split = (1) engine+CLI+tests, (2) dashboard, (3) CI workflows, (4) docs/spec/changelog.

## Output format

For each task: files changed, tests added (names), commands run with pass/fail, anything you could not verify and why. Stop and ask if a task is ambiguous or you are several attempts into the same failure; do not widen scope.
