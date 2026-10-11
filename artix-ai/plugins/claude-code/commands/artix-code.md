---
description: Run the Coder <-> Reviewer <-> Sandbox loop on a spec (supervised)
argument-hint: "--provider anthropic --model <name> [docs/specs/STORY-x.md]"
allowed-tools: Bash(artix code:*), Bash(git diff:*), Read, AskUser
---
1. Read the target story spec from docs/specs/ (or discover the active spec).
2. Extract and display the planned Test Commands from the spec to the user.
3. Explicitly ask the user for confirmation to execute these test commands before proceeding.
4. Only upon explicit user confirmation, execute: `artix code --json --autonomy supervised --confirm-tests $ARGUMENTS`.
   If confirmation is not granted, do not pass `--confirm-tests`.
   Plugins are strictly restricted to supervised mode; refuse if the user asks for unattended autonomous commits.
5. Parse the JSON result:
   - If status is `awaiting_approval` (or `awaitingApproval: true`), report that candidate commit `commitHash` was pushed to the PR branch and is awaiting human approval on the forge. Instruct the user to run `artix verify-approval --pr <pr> --sha <commitHash> --spec <specId>` after human approval is recorded on the forge.
   - If success is true (`status: "success"`), report convergence in roundsRun, show `git diff --stat`, and report commitHash / costReport totals.
   - If status is `rejected` or `unreviewed` (or success is false), report failure with `error` and `finalVerdict.blockingIssues/warnings`.


