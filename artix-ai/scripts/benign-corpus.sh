#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

OUTPUT_CSV="${1:-${REPO_ROOT}/docs/pilot/benign_corpus_results.csv}"

echo "=== Artix AI: Benign Corpus Evaluation Harness ==="
cd "${REPO_ROOT}"
go run ./cmd/artix-corpus/main.go --output "${OUTPUT_CSV}"
