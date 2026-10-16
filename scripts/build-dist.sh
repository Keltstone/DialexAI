#!/usr/bin/env bash
# ==============================================================================
# DialexAI Ecosystem Unified Build & Packaging Pipeline
# Builds Artix AI, Dialex AI, Kritix AI, Sentinix AI, and Socratix Engine
# Supports:
#   - Standalone CLI / Daemon binaries & release archives (Go: macOS, Linux, Windows)
#   - Desktop Applications (Compose Multiplatform: DMG, MSI, EXE, DEB, RPM)
#   - "With Engine" (bundled standalone) & "Without Engine" (client-only) variants
#   - Cryptographic SHA256 checksums
# ==============================================================================

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${APP_VERSION:-1.0.0}"
DIST_DIR="${ROOT_DIR}/dist"
TMP_DIR="${ROOT_DIR}/build/tmp-dist"

# Default flags
BUILD_CLI=true
BUILD_DESKTOP=false
CROSS_COMPILE=false
VARIANT="both" # "both", "with-engine", "without-engine"
TARGET_APP="all" # "all", "dialex", "artix", "kritix", "socratix", "sentinix"
OS_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_NAME="$(uname -m)"

case "${ARCH_NAME}" in
    x86_64) ARCH_NAME="amd64" ;;
    aarch64|arm64) ARCH_NAME="arm64" ;;
esac

usage() {
    cat <<EOF
DialexAI Ecosystem Build Pipeline (v${VERSION})

Usage:
  ./scripts/build-dist.sh [OPTIONS]

Options:
  --all                 Build both CLI binaries and Desktop application installers
  --cli                 Build CLI binaries only (default: true if --desktop not set)
  --desktop             Build Compose Desktop packages (DMG on macOS, MSI/EXE on Windows, DEB/RPM on Linux)
  --app <name>          Specify target application: 'all', 'dialex', 'artix', 'kritix', 'socratix', 'sentinix' (default: all)
  --cross-compile       Cross-compile Go CLI & Engine binaries for all platforms (macOS, Linux, Windows)
  --with-engine         Build ONLY variants with bundled Socratix engine
  --without-engine      Build ONLY variants without engine (lightweight client)
  --both-variants       Build BOTH 'with-engine' and 'without-engine' variants (default)
  --version <version>   Override build version (default: ${VERSION})
  --help, -h            Show this message

Outputs:
  dist/bin/                     Compiled host CLI & Engine binaries
  dist/archives/                Platform release archives (.tar.gz / .zip)
  dist/desktop/with-engine/     Desktop installers with embedded engine
  dist/desktop/without-engine/  Desktop installers without engine
  dist/SHA256SUMS               Cryptographic checksums for all release artifacts
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)
            BUILD_CLI=true
            BUILD_DESKTOP=true
            shift
            ;;
        --cli)
            BUILD_CLI=true
            shift
            ;;
        --desktop)
            BUILD_DESKTOP=true
            shift
            ;;
        --cross-compile)
            CROSS_COMPILE=true
            BUILD_CLI=true
            shift
            ;;
        --app)
            TARGET_APP="$2"
            shift 2
            ;;
        --with-engine)
            VARIANT="with-engine"
            shift
            ;;
        --without-engine)
            VARIANT="without-engine"
            shift
            ;;
        --both-variants)
            VARIANT="both"
            shift
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --help|-h)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            ;;
    esac
done

echo "=============================================================================="
echo "  🚀 Starting DialexAI Ecosystem Build Pipeline"
echo "  Version:       ${VERSION}"
echo "  Target App:    ${TARGET_APP}"
echo "  Build CLI:     ${BUILD_CLI}"
echo "  Build Desktop: ${BUILD_DESKTOP}"
echo "  Cross-Compile: ${CROSS_COMPILE}"
echo "  Variant Mode:  ${VARIANT}"
echo "  Platform:      ${OS_NAME}/${ARCH_NAME}"
echo "=============================================================================="

mkdir -p "${DIST_DIR}/bin"
mkdir -p "${DIST_DIR}/archives"
mkdir -p "${DIST_DIR}/desktop/with-engine"
mkdir -p "${DIST_DIR}/desktop/without-engine"
mkdir -p "${TMP_DIR}"

# ------------------------------------------------------------------------------
# 1. Build CLI & Engine Binaries
# ------------------------------------------------------------------------------
build_cli_binaries() {
    echo ""
    echo "📦 [1/3] Compiling Go Binaries..."
    cd "${ROOT_DIR}"

    # Target host build
    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "socratix" || "${TARGET_APP}" == "dialex" ]]; then
        echo "  • Compiling Socratix Engine (${OS_NAME}/${ARCH_NAME})..."
        CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${DIST_DIR}/bin/socratix" ./socratix-engine/cmd/socratix
        CGO_ENABLED=0 go build -ldflags="-s -w" -o "${DIST_DIR}/bin/socratixbench" ./socratix-engine/cmd/socratixbench
        chmod +x "${DIST_DIR}/bin/socratix" "${DIST_DIR}/bin/socratixbench"
        echo "    ✓ ${DIST_DIR}/bin/socratix"
        echo "    ✓ ${DIST_DIR}/bin/socratixbench"
    fi

    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "artix" ]]; then
        echo "  • Compiling Artix AI CLI & Daemon..."
        CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${DIST_DIR}/bin/artix" ./artix-ai/cli
        CGO_ENABLED=0 go build -ldflags="-s -w" -o "${DIST_DIR}/bin/artixd" ./artix-ai/cmd/artixd
        chmod +x "${DIST_DIR}/bin/artix" "${DIST_DIR}/bin/artixd"
        echo "    ✓ ${DIST_DIR}/bin/artix"
        echo "    ✓ ${DIST_DIR}/bin/artixd"
    fi

    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "kritix" ]]; then
        echo "  • Compiling Kritix AI CLI..."
        CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=${VERSION}" -o "${DIST_DIR}/bin/kritix" ./kritix-ai/cmd/kritix
        chmod +x "${DIST_DIR}/bin/kritix"
        echo "    ✓ ${DIST_DIR}/bin/kritix"
    fi

    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "sentinix" ]]; then
        echo "  • Compiling Sentinix AI CLI & Daemon..."
        CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${DIST_DIR}/bin/sentinix" ./sentinix-ai/cmd/sentinix
        CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${DIST_DIR}/bin/sentinixd" ./sentinix-ai/cmd/sentinixd
        chmod +x "${DIST_DIR}/bin/sentinix" "${DIST_DIR}/bin/sentinixd"
        echo "    ✓ ${DIST_DIR}/bin/sentinix"
        echo "    ✓ ${DIST_DIR}/bin/sentinixd"
    fi

    # Cross-compilation matrix if requested
    if [[ "${CROSS_COMPILE}" == "true" ]]; then
        echo ""
        echo "🌐 Cross-compiling Go binaries for distribution targets..."
        local platforms=(
            "darwin/arm64"
            "darwin/amd64"
            "linux/amd64"
            "linux/arm64"
            "windows/amd64"
        )

        for p in "${platforms[@]}"; do
            local goos="${p%/*}"
            local goarch="${p#*/}"
            local ext=""
            if [[ "${goos}" == "windows" ]]; then ext=".exe"; fi

            echo "  --> Platform: ${goos}/${goarch}"
            local stage_dir="${TMP_DIR}/stage-${goos}-${goarch}"
            rm -rf "${stage_dir}"
            mkdir -p "${stage_dir}"

            # Socratix
            if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "socratix" || "${TARGET_APP}" == "dialex" ]]; then
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w -X main.version=${VERSION}" -o "${stage_dir}/socratix${ext}" ./socratix-engine/cmd/socratix
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w" -o "${stage_dir}/socratixbench${ext}" ./socratix-engine/cmd/socratixbench
                
                local arc_name="socratix-v${VERSION}-${goos}-${goarch}"
                if [[ "${goos}" == "windows" ]]; then
                    (cd "${stage_dir}" && zip -q "${DIST_DIR}/archives/${arc_name}.zip" "socratix${ext}" "socratixbench${ext}")
                else
                    tar -czf "${DIST_DIR}/archives/${arc_name}.tar.gz" -C "${stage_dir}" "socratix${ext}" "socratixbench${ext}"
                fi
            fi

            # Artix
            if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "artix" ]]; then
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w -X main.version=${VERSION}" -o "${stage_dir}/artix${ext}" ./artix-ai/cli
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w" -o "${stage_dir}/artixd${ext}" ./artix-ai/cmd/artixd
                
                local arc_name="artix-v${VERSION}-${goos}-${goarch}"
                if [[ "${goos}" == "windows" ]]; then
                    (cd "${stage_dir}" && zip -q "${DIST_DIR}/archives/${arc_name}.zip" "artix${ext}" "artixd${ext}")
                else
                    tar -czf "${DIST_DIR}/archives/${arc_name}.tar.gz" -C "${stage_dir}" "artix${ext}" "artixd${ext}"
                fi
            fi

            # Kritix
            if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "kritix" ]]; then
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w -X main.Version=${VERSION}" -o "${stage_dir}/kritix${ext}" ./kritix-ai/cmd/kritix
                
                local arc_name="kritix-v${VERSION}-${goos}-${goarch}"
                if [[ "${goos}" == "windows" ]]; then
                    (cd "${stage_dir}" && zip -q "${DIST_DIR}/archives/${arc_name}.zip" "kritix${ext}")
                else
                    tar -czf "${DIST_DIR}/archives/${arc_name}.tar.gz" -C "${stage_dir}" "kritix${ext}"
                fi
            fi

            # Sentinix
            if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "sentinix" ]]; then
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w -X main.version=${VERSION}" -o "${stage_dir}/sentinix${ext}" ./sentinix-ai/cmd/sentinix
                GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
                    go build -ldflags="-s -w -X main.version=${VERSION}" -o "${stage_dir}/sentinixd${ext}" ./sentinix-ai/cmd/sentinixd
                
                local arc_name="sentinix-v${VERSION}-${goos}-${goarch}"
                if [[ "${goos}" == "windows" ]]; then
                    (cd "${stage_dir}" && zip -q "${DIST_DIR}/archives/${arc_name}.zip" "sentinix${ext}" "sentinixd${ext}")
                else
                    tar -czf "${DIST_DIR}/archives/${arc_name}.tar.gz" -C "${stage_dir}" "sentinix${ext}" "sentinixd${ext}"
                fi
            fi
        done
        echo "  ✓ Cross-compilation archives packaged in ${DIST_DIR}/archives/"
    fi

    echo "  ✓ All requested CLI binaries compiled successfully."
}

if [[ "${BUILD_CLI}" == "true" ]]; then
    build_cli_binaries
fi

# ------------------------------------------------------------------------------
# 2. Package Desktop Applications (DMG, MSI, EXE, DEB, RPM)
# ------------------------------------------------------------------------------
prepare_engine_bundle_resource() {
    local target_res_dir="$1"
    mkdir -p "${target_res_dir}/engine"
    if [[ ! -f "${DIST_DIR}/bin/socratix" ]]; then
        echo "    Building socratix for bundled resource..."
        CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${DIST_DIR}/bin/socratix" "${ROOT_DIR}/socratix-engine/cmd/socratix"
    fi
    cp "${DIST_DIR}/bin/socratix" "${target_res_dir}/engine/socratix"
    chmod +x "${target_res_dir}/engine/socratix"
    echo "    ✓ Staged socratix engine binary into ${target_res_dir}/engine/"
}

package_desktop_apps() {
    echo ""
    echo "🖥️  [2/3] Packaging Desktop Native Distributables..."

    local gradle_tasks=()
    local file_exts=()

    case "${OS_NAME}" in
        darwin)
            gradle_tasks=("packageDmg")
            file_exts=("dmg")
            ;;
        linux)
            gradle_tasks=("packageDeb" "packageRpm")
            file_exts=("deb" "rpm")
            ;;
        msys*|mingw*|cygwin*|windows)
            gradle_tasks=("packageMsi" "packageExe")
            file_exts=("msi" "exe")
            ;;
        *)
            echo "Warning: Unrecognized OS '${OS_NAME}', defaulting to packageDmg"
            gradle_tasks=("packageDmg")
            file_exts=("dmg")
            ;;
    esac

    package_single_variant() {
        local app_name="$1"     # dialex-ai, artix-ai, kritix-ai
        local gradle_mod="$2"   # :desktopApp, :app, :app
        local out_name="$3"     # DialexAI, ArtixAI, KritixAI
        local variant_type="$4" # without-engine, with-engine
        local extra_props=()

        if [[ "${variant_type}" == "with-engine" ]]; then
            local res_dir="${TMP_DIR}/${app_name}-engine-res"
            rm -rf "${res_dir}"
            prepare_engine_bundle_resource "${res_dir}"
            extra_props+=("-Pengine.resources.dir=${res_dir}")
        fi

        echo "    --> Packaging [${variant_type}] for ${out_name}..."
        cd "${ROOT_DIR}/${app_name}"

        for i in "${!gradle_tasks[@]}"; do
            local task="${gradle_tasks[$i]}"
            local ext="${file_exts[$i]}"
            echo "        Task: ${gradle_mod}:${task} (Target: .${ext})"

            ./gradlew "${gradle_mod}:${task}" \
                -Papp.version="${VERSION}" \
                -Papp.package.name="${out_name}" \
                ${extra_props[@]+"${extra_props[@]}"} || {
                    echo "        ⚠️ Warning: ${task} could not complete (possible missing local tool on host). Skipping."
                    continue
                }

            # Locate built artifact
            local built_file
            built_file=$(find "${ROOT_DIR}/${app_name}" -path "*/compose/binaries/main/*" -name "*.${ext}" | head -n 1 || true)
            if [[ -n "${built_file}" && -f "${built_file}" ]]; then
                local dest="${DIST_DIR}/desktop/${variant_type}/${out_name}-${VERSION}-${variant_type}.${ext}"
                cp "${built_file}" "${dest}"
                echo "        ✓ Saved: ${dest}"
            fi
        done
    }

    package_app() {
        local app_name="$1"
        local gradle_mod="$2"
        local out_name="$3"

        echo "  📦 Building ${out_name} Native Packages..."
        if [[ "${VARIANT}" == "both" || "${VARIANT}" == "without-engine" ]]; then
            package_single_variant "${app_name}" "${gradle_mod}" "${out_name}" "without-engine"
        fi
        if [[ "${VARIANT}" == "both" || "${VARIANT}" == "with-engine" ]]; then
            package_single_variant "${app_name}" "${gradle_mod}" "${out_name}" "with-engine"
        fi
    }

    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "dialex" ]]; then
        package_app "dialex-ai" ":desktopApp" "DialexAI"
    fi

    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "artix" ]]; then
        package_app "artix-ai" ":app" "ArtixAI"
    fi

    if [[ "${TARGET_APP}" == "all" || "${TARGET_APP}" == "kritix" ]]; then
        package_app "kritix-ai" ":app" "KritixAI"
    fi
}

if [[ "${BUILD_DESKTOP}" == "true" ]]; then
    package_desktop_apps
fi

# ------------------------------------------------------------------------------
# 3. Generate SHA256 Checksums
# ------------------------------------------------------------------------------
echo ""
echo "🔒 [3/3] Generating SHA256 Verification Checksums..."
cd "${DIST_DIR}"
rm -f SHA256SUMS

if command -v sha256sum >/dev/null 2>&1; then
    find . -type f ! -name "SHA256SUMS" -exec sha256sum {} + | sort -k2 > SHA256SUMS
elif command -v shasum >/dev/null 2>&1; then
    find . -type f ! -name "SHA256SUMS" -exec shasum -a 256 {} + | sort -k2 > SHA256SUMS
else
    echo "Warning: Neither sha256sum nor shasum available to generate checksums."
fi

if [[ -f SHA256SUMS ]]; then
    echo "✓ Generated ${DIST_DIR}/SHA256SUMS"
fi

echo ""
echo "=============================================================================="
echo "  🎉 DialexAI Ecosystem Build Completed Successfully!"
echo "  Artifacts staged in: ${DIST_DIR}"
echo "=============================================================================="
ls -lh "${DIST_DIR}/bin" 2>/dev/null || true
ls -lh "${DIST_DIR}/archives" 2>/dev/null || true
ls -lh "${DIST_DIR}/desktop/with-engine" 2>/dev/null || true
ls -lh "${DIST_DIR}/desktop/without-engine" 2>/dev/null || true
