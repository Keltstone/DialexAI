#!/usr/bin/env bash
# build.sh - Reproducible multi-platform release build for Scribex
#
# Usage: ./scripts/build.sh <VERSION>
# Example: ./scripts/build.sh 1.0.0
#
# Outputs:
#   dist/scribex_<os>_<arch>[.exe]
#   dist/SHA256SUMS
#
# Release Signing:
# TODO: Binary signing will be integrated via Cosign once release key infrastructure is provisioned.
# To sign a released binary artifact:
#   cosign sign-blob --key <key-path-or-kms> --output-signature dist/scribex_<os>_<arch>.sig dist/scribex_<os>_<arch>
# To verify:
#   cosign verify-blob --key <pubkey-path-or-kms> --signature dist/scribex_<os>_<arch>.sig dist/scribex_<os>_<arch>
# Note: Signed releases do not exist yet.

set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <VERSION>" >&2
  exit 2
fi

VERSION="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"

# Clean and prepare dist directory
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Honour SOURCE_DATE_EPOCH if set for reproducibility
if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
  export SOURCE_DATE_EPOCH
fi

TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

echo "Building Scribex AI release ${VERSION}..."

for target in "${TARGETS[@]}"; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  EXT=""
  if [ "$GOOS" = "windows" ]; then
    EXT=".exe"
  fi
  BIN_NAME="scribex_${GOOS}_${GOARCH}${EXT}"
  OUT_PATH="$DIST_DIR/$BIN_NAME"

  echo "  -> ${GOOS}/${GOARCH} -> ${BIN_NAME}"
  (
    cd "$ROOT_DIR"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOTOOLCHAIN=local \
      go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o "$OUT_PATH" ./cmd/scribex
  )
done

# Generate SHA256SUMS
echo "Generating SHA256SUMS..."
(
  cd "$DIST_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum scribex_* > SHA256SUMS
  else
    shasum -a 256 scribex_* > SHA256SUMS
  fi
)

echo "Build complete. Artifacts in dist/:"
cat "$DIST_DIR/SHA256SUMS"
