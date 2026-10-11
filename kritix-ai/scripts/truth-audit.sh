#!/usr/bin/env bash
# ==============================================================================
# Kritix AI Enterprise Truth & Integrity Audit Gate v3 (Structural, AST-based)
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ -d "$REPO_ROOT/kritix-ai" ]; then
  KRITIX_DIR="$REPO_ROOT/kritix-ai"
else
  KRITIX_DIR="$REPO_ROOT"
fi

echo "🔍 [Truth Audit v3] Running structural mechanical verification..."

# 1. Dependency Hygiene: go mod tidy must leave no diff and make direct imports
echo "📦 [1/4] Checking dependency hygiene (go mod tidy)..."
(cd "$KRITIX_DIR" && go mod tidy)
if ! git -C "$KRITIX_DIR" diff --quiet -- go.mod go.sum; then
  echo "⛔ [TRUTH AUDIT FAILED]: go mod tidy resulted in diff (dependencies must be tidy and direct)" >&2
  git -C "$KRITIX_DIR" diff -- go.mod go.sum >&2
  exit 1
fi

# 2. Mechanical Ledger Check
ROUND="${1:-8}"
echo "📋 [2/4] Verifying per-round mechanical ledger (Round ${ROUND})..."
bash "$KRITIX_DIR/scripts/ledger-check.sh" "${ROUND}"

# 3. Integration Vet
echo "🔬 [3/4] Verifying integration-tagged source compilation..."
(cd "$KRITIX_DIR" && go vet -tags integration ./...)

# 4. Structural AST Truth Audit
echo "🔍 [4/4] Executing structural AST truth audit gates..."
(cd "$KRITIX_DIR" && go run ./cmd/truth-audit)
