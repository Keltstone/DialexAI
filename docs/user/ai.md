# Scribex AI Prose Rewriting

Scribex provides an optional AI-assisted prose rewriter (`scribex sync -ai`) that updates documentation prose when a code symbol signature changes.

## Threat Model and Guarantees

AI provider replies are treated as untrusted and potentially attacker-influenced (e.g. prompt injection embedded in repository code or comments, or compromised endpoints).

1. **Prose-Only Guarantee**: The model may only update plain prose within a single section. It cannot inject HTML, markdown fences, new anchors, or end markers.
2. **All-or-Nothing Sync**: If the model output violates any validation rule or fails, the anchor is left completely untouched (byte-identical, keeping the old signature hash), marked stale, and retried on the next run.
3. **AI Marker**: Every section updated by AI is marked with `<!-- @scribex:ai sig=<hash> -->` as the last line in the section so human reviewers can identify AI-generated drafts.

## Known Limits

- **Semantic Correctness**: Validation cannot detect a fluent but WRONG or refusal-style reply of plausible length; AI-drafted sections carry the `@scribex:ai` marker and the docs-PR/approval flow (Phase 1–2) is what makes a human check them.
- **Mechanical vs AI Sync**: Mechanical `sync` (without `-ai`) refreshes `sig=` while leaving prose untouched — Phase 1 reports those anchors as "prose not reviewed".
