#!/usr/bin/env bash
# repro-check.sh - Verify build reproducibility across isolated temporary environments
#
# Builds scribex in two separate temporary directories with separate GOCACHE and TMPDIR
# settings, then compares SHA-256 digests. Exits 0 if identical, non-zero if different.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION="${1:-1.0.0}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-1700000000}"
export SOURCE_DATE_EPOCH

DIR1="$(mktemp -d)"
DIR2="$(mktemp -d)"

cleanup() {
  rm -rf "$DIR1" "$DIR2"
}
trap cleanup EXIT

echo "=== Build 1 (isolated cache 1) ==="
BIN1="$DIR1/scribex"
CACHE1="$DIR1/gocache"
mkdir -p "$CACHE1"
(
  cd "$ROOT_DIR"
  GOCACHE="$CACHE1" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$BIN1" ./cmd/scribex
)

if command -v sha256sum >/dev/null 2>&1; then
  HASH1="$(sha256sum "$BIN1" | awk '{print $1}')"
else
  HASH1="$(shasum -a 256 "$BIN1" | awk '{print $1}')"
fi
echo "Build 1 SHA-256: $HASH1"

echo "=== Build 2 (isolated cache 2) ==="
BIN2="$DIR2/scribex"
CACHE2="$DIR2/gocache"
mkdir -p "$CACHE2"
(
  cd "$ROOT_DIR"
  GOCACHE="$CACHE2" CGO_ENABLED=0 \
    go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$BIN2" ./cmd/scribex
)

if command -v sha256sum >/dev/null 2>&1; then
  HASH2="$(sha256sum "$BIN2" | awk '{print $1}')"
else
  HASH2="$(shasum -a 256 "$BIN2" | awk '{print $1}')"
fi
echo "Build 2 SHA-256: $HASH2"

echo "=== Comparison ==="
if [ "$HASH1" != "$HASH2" ]; then
  echo "FAIL: Reproducibility check failed! Hashes differ:" >&2
  echo "  Build 1: $HASH1" >&2
  echo "  Build 2: $HASH2" >&2
  exit 1
fi

echo "PASS: Reproducible build verified! Identical SHA-256: $HASH1"
