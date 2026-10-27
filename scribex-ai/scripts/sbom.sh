#!/usr/bin/env bash
# sbom.sh - Generate Software Bill of Materials (SBOM) for Scribex
#
# Usage:
#   ./scripts/sbom.sh [--cyclonedx]
#
# Behavior:
#   If cyclonedx-gomod is installed:
#     Generates CycloneDX SBOM into dist/sbom.cdx.json
#   Else:
#     Dumps `go version -m` and `go list -m all` into dist/sbom.txt
#     If --cyclonedx was explicitly requested, exits non-zero with:
#     "install cyclonedx-gomod for CycloneDX"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"

mkdir -p "$DIST_DIR"

WANT_CYCLONEDX=false
for arg in "$@"; do
  case "$arg" in
    --cyclonedx)
      WANT_CYCLONEDX=true
      ;;
    *)
      echo "Unknown flag: $arg" >&2
      echo "Usage: $0 [--cyclonedx]" >&2
      exit 2
      ;;
  esac
done

if command -v cyclonedx-gomod >/dev/null 2>&1; then
  echo "Generating CycloneDX SBOM via cyclonedx-gomod..."
  (
    cd "$ROOT_DIR"
    cyclonedx-gomod app -main cmd/scribex -output "$DIST_DIR/sbom.cdx.json" -json .
  )
  echo "CycloneDX SBOM written to dist/sbom.cdx.json"
else
  echo "cyclonedx-gomod not found, generating fallback text SBOM into dist/sbom.txt..."
  (
    cd "$ROOT_DIR"
    {
      echo "# Scribex AI SBOM"
      echo "# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
      echo ""
      echo "## Go Toolchain and Compiler Modules"
      go version -m ./cmd/scribex 2>/dev/null || true
      echo ""
      echo "## Direct and Indirect Dependencies"
      go list -m all
    } > "$DIST_DIR/sbom.txt"
  )
  echo "Fallback text SBOM written to dist/sbom.txt"

  if [ "$WANT_CYCLONEDX" = true ]; then
    echo "install cyclonedx-gomod for CycloneDX" >&2
    exit 1
  fi
fi
