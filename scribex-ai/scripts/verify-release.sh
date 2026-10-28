#!/usr/bin/env bash
# verify-release.sh - Verify release assets, cosign signatures, and checksums
#
# Usage:
#   ./scripts/verify-release.sh <TAG> [REPO]
#
# Example:
#   ./scripts/verify-release.sh scribex-v1.0.0
#   ./scripts/verify-release.sh scribex-v0.0.0-rc1 Keltstone/DialexAI

set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <TAG> [REPO]" >&2
  exit 2
fi

TAG="$1"
REPO="${2:-${GH_REPO:-}}"

if [ -z "$REPO" ]; then
  if command -v gh >/dev/null 2>&1; then
    REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || true)"
  fi
fi

if [ -z "$REPO" ]; then
  REPO="Keltstone/DialexAI"
fi

echo "Verifying release $TAG from repository $REPO..."

# Ensure required tools are installed
for tool in gh cosign; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "FAIL: Required tool '$tool' is not installed or not in PATH" >&2
    exit 1
  fi
done

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [ -n "${RELEASE_DIR:-}" ] && [ -d "$RELEASE_DIR" ]; then
  echo "Using assets from local directory: $RELEASE_DIR"
  cp -r "$RELEASE_DIR"/* "$TMP_DIR"/
else
  echo "Downloading release assets for tag $TAG..."
  if ! gh release download "$TAG" --repo "$REPO" --dir "$TMP_DIR"; then
    echo "FAIL: Could not download release assets for tag $TAG from $REPO" >&2
    exit 1
  fi
fi

# 1. Verify SHA256SUMS
if [ ! -f "$TMP_DIR/SHA256SUMS" ]; then
  echo "FAIL: SHA256SUMS file missing from release assets" >&2
  exit 1
fi

echo "Verifying SHA256 checksums..."
(
  cd "$TMP_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c SHA256SUMS
  else
    shasum -a 256 -c SHA256SUMS
  fi
) || {
  echo "FAIL: SHA256SUMS verification failed" >&2
  exit 1
}

# 2. Verify Cosign signatures
EXPECTED_ISSUER="https://token.actions.githubusercontent.com"
EXPECTED_IDENTITY_REGEXP="^https://github\.com/${REPO}/\.github/workflows/scribex\.yml@refs/tags/${TAG}$"

echo "Verifying Cosign keyless signatures (GitHub OIDC)..."
VERIFIED_COUNT=0

for bundle in "$TMP_DIR"/*.bundle; do
  [ -e "$bundle" ] || continue
  target="${bundle%.bundle}"
  if [ ! -f "$target" ]; then
    continue
  fi
  base_target="$(basename "$target")"
  echo "  verifying $base_target with $(basename "$bundle")..."
  if ! cosign verify-blob \
    --bundle "$bundle" \
    --certificate-identity-regexp "$EXPECTED_IDENTITY_REGEXP" \
    --certificate-oidc-issuer "$EXPECTED_ISSUER" \
    "$target" >/dev/null 2>&1; then
    echo "FAIL: Cosign verification failed for $base_target" >&2
    exit 1
  fi
  VERIFIED_COUNT=$((VERIFIED_COUNT + 1))
done

if [ "$VERIFIED_COUNT" -eq 0 ]; then
  echo "FAIL: No verified signature bundles found in release assets" >&2
  exit 1
fi

echo "PASS: Successfully verified $VERIFIED_COUNT signed assets and SHA256SUMS for release $TAG"
echo "PASS"
