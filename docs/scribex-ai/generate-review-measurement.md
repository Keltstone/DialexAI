# Socratix Review Measurement

## Summary
- **Review mechanism**: `socratix ask -mode redteam|premortem|tenthman -out <file> -`
- **Measurement status**: Unmeasured live (no external provider API key or local Ollama daemon available in the build environment).
- **Quality claim**: None made. As specified in the honesty model and `generate.md`, review benefit remains explicitly unmeasured; a `clean` verdict must never be treated as "verified".

## Verification & Hardening
1. **Binary invocation**:
   - `socratix-engine/bin/socratix ask` verified to compile and run.
   - Tested behavior when no provider credentials exist: correctly reports `dialex ask: no API key found: set one of ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY, XAI_API_KEY, DEEPSEEK_API_KEY, MISTRAL_API_KEY, or use --provider ollama for a local model`.
2. **Review parser hardening**:
   - `Review()` in `pkg/generate/review.go` previously treated any output lacking `UNSUPPORTED:` lines as clean.
   - Hardened to require that the memo is non-empty and explicitly references the draft (`strings.Contains(strings.ToLower(memo), "draft")`).
   - If the memo is empty or unrelated, `Review()` returns an error rather than false `clean`, causing `apply` to record `review=skipped:...` instead of falsely claiming clean.
   - Tested with `TestSocratixEmptyOrUnrelatedMemoErrors`.

## Measurement Protocol (to be executed when keys/Ollama configured)
1. Select real repo with derived documents (`runbook-ops`, `feature-design`, `requirements`).
2. Run generation pass 1: `ai.review: off`.
3. Run generation pass 2: `ai.review: socratix`, `ai.review_mode: redteam`.
4. Hand-audit 10 claims per section against source code:
   - Supported claims count.
   - Unsupported claims present.
   - Caught by council.
   - False alarms raised by council.
   - Call count overhead.
5. Record raw counts in this table. Until then, documentation must continue to declare review benefits unmeasured.

