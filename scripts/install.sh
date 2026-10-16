#!/usr/bin/env bash
# ==============================================================================
# DialexAI Ecosystem Universal Installer
# Installs Artix AI, Dialex AI, Kritix AI, and Socratix Deliberation Engine
# Supports:
#   - macOS, Linux, and Windows (via bash / WSL)
#   - CLI binaries & Desktop GUI Applications (DMG / DEB / App)
#   - With Engine & Without Engine variants
#   - Engine detection with EXPRESS PERMISSION check before updating or skipping
# ==============================================================================

set -euo pipefail

VERSION="1.0.0"
REPO_OWNER="Kolta-Labs"
REPO_NAME="DialexAI"
GITHUB_RELEASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${VERSION}"

# Color codes (if terminal supports it)
if [[ -t 1 ]]; then
    BOLD="\033[1m"
    GREEN="\033[0;32m"
    YELLOW="\033[0;33m"
    CYAN="\033[0;36m"
    RED="\033[0;31m"
    RESET="\033[0m"
else
    BOLD=""
    GREEN=""
    YELLOW=""
    CYAN=""
    RED=""
    RESET=""
fi

# Detect platform
OS_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_NAME="$(uname -m)"
case "${ARCH_NAME}" in
    x86_64) ARCH_NAME="amd64" ;;
    aarch64|arm64) ARCH_NAME="arm64" ;;
esac

# Options
INSTALL_TARGET="all"       # all, artix, dialex, kritix, sentinix, engine
INSTALL_MODE="both"         # both, cli, desktop
ENGINE_CHOICE="prompt"      # prompt, with, without
EXPRESS_ENGINE_ACTION=""    # update, skip, or empty (prompt)
PREFIX=""
APPS_DIR=""
DRY_RUN=false
NON_INTERACTIVE=false

# Root of repo if running inside checkout
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || echo "")"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." 2>/dev/null && pwd || echo "")"

usage() {
    cat <<EOF
${BOLD}DialexAI Ecosystem Installer (v${VERSION})${RESET}

Usage:
  ./scripts/install.sh [OPTIONS]
  curl -fsSL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/main/scripts/install.sh | bash -s -- [OPTIONS]

Options:
  --all                 Install all products: Artix AI, Dialex AI, Kritix AI, and Sentinix AI (default)
  --artix               Install Artix AI only (Autonomous Coding Cockpit & CLI)
  --dialex              Install Dialex AI only (Deliberation Platform & Studio)
  --kritix              Install Kritix AI only (Autonomous QA & Testing Studio)
  --sentinix            Install Sentinix AI only (Continuous Security Review & PR Gate)
  --engine-only         Install Socratix Deliberation Engine only

Component Modes:
  --both                Install both CLI binaries and Desktop GUI apps (default)
  --cli                 Install CLI binaries only
  --desktop             Install Desktop GUI apps only

Engine Options:
  --with-engine         Install with Socratix Engine (bundled or CLI engine)
  --without-engine      Install client only (without Socratix Engine)
  --update-engine       Expressly approve updating existing engine if detected (non-interactive)
  --skip-engine         Expressly skip updating existing engine if detected (non-interactive)

Target Paths:
  --prefix <dir>        Directory prefix for CLI binaries (default: /usr/local or ~/.local)
  --apps-dir <dir>      Directory for desktop applications (default: /Applications on macOS)
  --non-interactive     Run without interactive prompts (uses flags or defaults)
  --dry-run             Simulate installation without modifying system files
  --help, -h            Show this message
EOF
    exit 0
}

# Parse flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)
            INSTALL_TARGET="all"
            shift
            ;;
        --artix)
            INSTALL_TARGET="artix"
            shift
            ;;
        --dialex)
            INSTALL_TARGET="dialex"
            shift
            ;;
        --kritix)
            INSTALL_TARGET="kritix"
            shift
            ;;
        --sentinix)
            INSTALL_TARGET="sentinix"
            INSTALL_MODE="cli"
            shift
            ;;
        --engine-only|--engine)
            INSTALL_TARGET="engine"
            shift
            ;;
        --both)
            INSTALL_MODE="both"
            shift
            ;;
        --cli)
            INSTALL_MODE="cli"
            shift
            ;;
        --desktop)
            INSTALL_MODE="desktop"
            shift
            ;;
        --with-engine)
            ENGINE_CHOICE="with"
            shift
            ;;
        --without-engine)
            ENGINE_CHOICE="without"
            shift
            ;;
        --update-engine)
            EXPRESS_ENGINE_ACTION="update"
            shift
            ;;
        --skip-engine|--no-engine)
            EXPRESS_ENGINE_ACTION="skip"
            shift
            ;;
        --prefix)
            PREFIX="$2"
            shift 2
            ;;
        --apps-dir)
            APPS_DIR="$2"
            shift 2
            ;;
        --non-interactive|-y)
            NON_INTERACTIVE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help|-h)
            usage
            ;;
        *)
            echo "Unknown argument: $1" >&2
            usage
            ;;
    esac
done

# Determine default prefix
if [[ -z "${PREFIX}" ]]; then
    if [[ -w "/usr/local/bin" ]]; then
        PREFIX="/usr/local"
    elif [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
        PREFIX="/usr/local"
    else
        PREFIX="${HOME}/.local"
    fi
fi

BIN_DIR="${PREFIX}/bin"

# Determine default desktop apps directory
if [[ -z "${APPS_DIR}" ]]; then
    if [[ "${OS_NAME}" == "darwin" ]]; then
        if [[ -w "/Applications" ]]; then
            APPS_DIR="/Applications"
        else
            APPS_DIR="${HOME}/Applications"
        fi
    else
        APPS_DIR="${HOME}/.local/share/applications"
    fi
fi

# TTY prompt helper
prompt_user() {
    local prompt_msg="$1"
    local default_val="$2"
    local response=""

    if [[ "${NON_INTERACTIVE}" == "true" ]]; then
        echo "${default_val}"
        return
    fi

    # Read from /dev/tty if available, otherwise stdin
    if [[ -c /dev/tty ]]; then
        printf "%s" "${prompt_msg}" > /dev/tty
        read -r response < /dev/tty
    else
        printf "%s" "${prompt_msg}"
        read -r response
    fi

    if [[ -z "${response}" ]]; then
        echo "${default_val}"
    else
        echo "${response}"
    fi
}

echo ""
echo -e "${BOLD}${CYAN}==============================================================================${RESET}"
echo -e "${BOLD}${CYAN}   🌟 DialexAI Ecosystem Installer: Artix, Dialex & Kritix (v${VERSION})${RESET}"
echo -e "${BOLD}${CYAN}==============================================================================${RESET}"
echo -e "  Platform:    ${BOLD}${OS_NAME}/${ARCH_NAME}${RESET}"
echo -e "  Bin Prefix:  ${BOLD}${BIN_DIR}${RESET}"
if [[ "${INSTALL_MODE}" != "cli" ]]; then
    echo -e "  Apps Dir:    ${BOLD}${APPS_DIR}${RESET}"
fi
echo ""

# ------------------------------------------------------------------------------
# Interactive Menu (when running interactively with default options)
# ------------------------------------------------------------------------------
if [[ "${NON_INTERACTIVE}" != "true" && -t 0 ]]; then
    echo -e "${BOLD}Select components to install:${RESET}"
    echo "  1) All Products (Artix AI, Dialex AI, Kritix AI, Sentinix AI)"
    echo "  2) Artix AI (Autonomous Coding Cockpit & CLI)"
    echo "  3) Dialex AI (Deliberation Desktop App & Studio)"
    echo "  4) Kritix AI (Autonomous QA & Testing Studio)"
    echo "  5) Sentinix AI (Continuous Security Review & PR Gate)"
    echo "  6) Socratix Deliberation Engine Only"
    ans=$(prompt_user "Enter choice [1-6, default 1]: " "1")
    case "$ans" in
        1) INSTALL_TARGET="all" ;;
        2) INSTALL_TARGET="artix" ;;
        3) INSTALL_TARGET="dialex" ;;
        4) INSTALL_TARGET="kritix" ;;
        5) INSTALL_TARGET="sentinix"; INSTALL_MODE="cli" ;;
        6) INSTALL_TARGET="engine" ;;
        *) INSTALL_TARGET="all" ;;
    esac

    if [[ "${INSTALL_TARGET}" != "engine" && "${INSTALL_TARGET}" != "sentinix" ]]; then
        echo ""
        echo -e "${BOLD}Select format:${RESET}"
        echo "  1) Both CLI Binaries and Desktop GUI Apps"
        echo "  2) CLI Binaries Only"
        echo "  3) Desktop GUI Apps Only"
        ans_mode=$(prompt_user "Enter choice [1-3, default 1]: " "1")
        case "$ans_mode" in
            1) INSTALL_MODE="both" ;;
            2) INSTALL_MODE="cli" ;;
            3) INSTALL_MODE="desktop" ;;
            *) INSTALL_MODE="both" ;;
        esac

        if [[ "${ENGINE_CHOICE}" == "prompt" ]]; then
            echo ""
            echo -e "${BOLD}Include Socratix Deliberation Engine?${RESET}"
            echo "  (Socratix coordinates multi-agent consensus deliberations)"
            echo "  1) Yes - Include Socratix Engine (Standalone / Bundled)"
            echo "  2) No  - Lightweight client only (Connect to existing / remote engine)"
            ans_eng=$(prompt_user "Enter choice [1-2, default 1]: " "1")
            if [[ "$ans_eng" == "2" ]]; then
                ENGINE_CHOICE="without"
            else
                ENGINE_CHOICE="with"
            fi
        fi
    fi
fi

# ------------------------------------------------------------------------------
# ENGINE DETECTION & EXPRESS PERMISSION CHECK (MANDATORY REQUIREMENT)
# ------------------------------------------------------------------------------
ENGINE_ACTION="install"

check_and_handle_engine() {
    echo -e "\n${BOLD}${CYAN}------------------------------------------------------------------------------${RESET}"
    echo -e "${BOLD}🔍 Inspecting Socratix Deliberation Engine Installation...${RESET}"

    local found_engine=""
    local candidates=(
        "$(command -v socratix 2>/dev/null || true)"
        "${BIN_DIR}/socratix"
        "/usr/local/bin/socratix"
        "/opt/homebrew/bin/socratix"
        "${HOME}/.local/bin/socratix"
    )

    for cand in "${candidates[@]}"; do
        if [[ -n "${cand}" && -x "${cand}" ]]; then
            found_engine="${cand}"
            break
        fi
    done

    if [[ -n "${found_engine}" ]]; then
        local installed_version
        installed_version=$("${found_engine}" --version 2>&1 | head -n 1 || echo "unknown")
        echo -e "  ${YELLOW}⚠️  Existing Socratix engine detected!${RESET}"
        echo -e "     Location:          ${BOLD}${found_engine}${RESET}"
        echo -e "     Installed Version: ${BOLD}${installed_version}${RESET}"
        echo -e "     Incoming Version:  ${BOLD}v${VERSION}${RESET}"
        echo ""

        if [[ "${EXPRESS_ENGINE_ACTION}" == "update" ]]; then
            echo -e "  ${GREEN}✓ Update permission granted via --update-engine flag.${RESET}"
            ENGINE_ACTION="update"
            EXISTING_ENGINE_PATH="${found_engine}"
        elif [[ "${EXPRESS_ENGINE_ACTION}" == "skip" ]]; then
            echo -e "  ${CYAN}✓ Skipping engine installation via --skip-engine flag.${RESET}"
            ENGINE_ACTION="skip"
            EXISTING_ENGINE_PATH="${found_engine}"
        else
            # Request express permission from user
            echo -e "${BOLD}Engine Update Decision:${RESET}"
            echo "The deliberation engine is already installed on your system."
            echo "Updating will replace the existing binary with v${VERSION} (a backup will be preserved)."
            echo "Skipping will keep your existing engine untouched."
            echo ""
            local perm_answer
            perm_answer=$(prompt_user "Do you give express permission to UPDATE the existing engine? [y/N]: " "n")
            case "${perm_answer}" in
                y|Y|yes|YES)
                    echo -e "  ${GREEN}✓ Express permission granted: Existing engine will be updated.${RESET}"
                    ENGINE_ACTION="update"
                    EXISTING_ENGINE_PATH="${found_engine}"
                    ;;
                *)
                    echo -e "  ${CYAN}✓ Express permission declined: Skipping engine install.${RESET}"
                    echo -e "  ${CYAN}  Existing engine at ${found_engine} will be preserved.${RESET}"
                    ENGINE_ACTION="skip"
                    EXISTING_ENGINE_PATH="${found_engine}"
                    ;;
            esac
        fi
    else
        echo -e "  ${GREEN}✓ No existing Socratix engine found. Will perform clean installation.${RESET}"
        ENGINE_ACTION="install"
    fi
    echo -e "${BOLD}${CYAN}------------------------------------------------------------------------------${RESET}\n"
}

# Run engine check if engine is targeted or requested
if [[ "${INSTALL_TARGET}" == "engine" || "${ENGINE_CHOICE}" == "with" || "${INSTALL_TARGET}" == "all" ]]; then
    check_and_handle_engine
fi

# ------------------------------------------------------------------------------
# Binary Acquisition: Local Checkout or Build or Prebuilt Download
# ------------------------------------------------------------------------------
acquire_binary() {
    local bin_name="$1" # socratix, artix, kritix, etc.
    local src_file=""

    # 1. Check local dist directory
    if [[ -n "${REPO_ROOT}" && -f "${REPO_ROOT}/dist/bin/${bin_name}" ]]; then
        src_file="${REPO_ROOT}/dist/bin/${bin_name}"
    # 2. Check if we can build it from local source
    elif [[ -n "${REPO_ROOT}" && -d "${REPO_ROOT}/socratix-engine" && -f "${REPO_ROOT}/go.work" ]]; then
        echo "  Compiling ${bin_name} from source..."
        mkdir -p "${REPO_ROOT}/dist/bin"
        case "${bin_name}" in
            socratix)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${REPO_ROOT}/dist/bin/socratix" ./socratix-engine/cmd/socratix)
                ;;
            socratixbench)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "${REPO_ROOT}/dist/bin/socratixbench" ./socratix-engine/cmd/socratixbench)
                ;;
            artix)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${REPO_ROOT}/dist/bin/artix" ./artix-ai/cli)
                ;;
            artixd)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "${REPO_ROOT}/dist/bin/artixd" ./artix-ai/cmd/artixd)
                ;;
            kritix)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=${VERSION}" -o "${REPO_ROOT}/dist/bin/kritix" ./kritix-ai/cmd/kritix)
                ;;
            sentinix)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${REPO_ROOT}/dist/bin/sentinix" ./sentinix-ai/cmd/sentinix)
                ;;
            sentinixd)
                (cd "${REPO_ROOT}" && CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o "${REPO_ROOT}/dist/bin/sentinixd" ./sentinix-ai/cmd/sentinixd)
                ;;
        esac
        src_file="${REPO_ROOT}/dist/bin/${bin_name}"
    fi

    echo "${src_file}"
}

install_binary_file() {
    local src="$1"
    local dest_name="$2"
    local dest_path="${BIN_DIR}/${dest_name}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        echo -e "  [DRY-RUN] Install ${src} -> ${dest_path}"
        return
    fi

    mkdir -p "${BIN_DIR}"
    cp -f "${src}" "${dest_path}"
    chmod +x "${dest_path}"
    echo -e "  ${GREEN}✓ Installed:${RESET} ${dest_path}"
}

# ------------------------------------------------------------------------------
# Installation Steps
# ------------------------------------------------------------------------------

# 1. Install Socratix Engine
install_engine() {
    echo -e "${BOLD}▶ Installing Socratix Deliberation Engine...${RESET}"
    if [[ "${ENGINE_ACTION}" == "skip" ]]; then
        echo -e "  ${CYAN}• Skipping engine installation per express user permission.${RESET}"
        echo -e "  • Dialex and Artix will use existing engine at: ${EXISTING_ENGINE_PATH}"
        return
    fi

    local src_socratix
    src_socratix=$(acquire_binary "socratix")
    local src_bench
    src_bench=$(acquire_binary "socratixbench")

    if [[ -n "${src_socratix}" && -f "${src_socratix}" ]]; then
        if [[ "${ENGINE_ACTION}" == "update" && -n "${EXISTING_ENGINE_PATH:-}" ]]; then
            local backup_path="${EXISTING_ENGINE_PATH}.bak.$(date +%s)"
            echo "  Backing up previous engine to: ${backup_path}"
            if [[ "${DRY_RUN}" != "true" ]]; then
                cp -f "${EXISTING_ENGINE_PATH}" "${backup_path}"
            fi
            install_binary_file "${src_socratix}" "$(basename "${EXISTING_ENGINE_PATH}")"
        else
            install_binary_file "${src_socratix}" "socratix"
        fi
        if [[ -n "${src_bench}" && -f "${src_bench}" ]]; then
            install_binary_file "${src_bench}" "socratixbench"
        fi
        echo -e "  ${GREEN}✓ Socratix Deliberation Engine successfully configured!${RESET}"
    else
        echo -e "  ${YELLOW}⚠️  Pre-compiled socratix binary not found; skipping binary copy.${RESET}"
    fi
}

# 2. Install Artix AI CLI
install_artix_cli() {
    echo -e "${BOLD}▶ Installing Artix AI CLI & Daemon...${RESET}"
    local src_artix
    src_artix=$(acquire_binary "artix")
    local src_artixd
    src_artixd=$(acquire_binary "artixd")

    if [[ -n "${src_artix}" && -f "${src_artix}" ]]; then
        install_binary_file "${src_artix}" "artix"
        if [[ -n "${src_artixd}" && -f "${src_artixd}" ]]; then
            install_binary_file "${src_artixd}" "artixd"
        fi
        echo -e "  ${GREEN}✓ Artix AI CLI successfully installed!${RESET}"
    else
        echo -e "  ${YELLOW}⚠️  Artix binary not found.${RESET}"
    fi
}

# 3. Install Kritix AI CLI
install_kritix_cli() {
    echo -e "${BOLD}▶ Installing Kritix AI CLI...${RESET}"
    local src_kritix
    src_kritix=$(acquire_binary "kritix")

    if [[ -n "${src_kritix}" && -f "${src_kritix}" ]]; then
        install_binary_file "${src_kritix}" "kritix"
        echo -e "  ${GREEN}✓ Kritix AI CLI successfully installed!${RESET}"
    else
        echo -e "  ${YELLOW}⚠️  Kritix binary not found.${RESET}"
    fi
}

# 4. Install Sentinix AI CLI & Daemon
install_sentinix_cli() {
    echo -e "${BOLD}▶ Installing Sentinix AI CLI & Daemon...${RESET}"
    local src_sentinix
    src_sentinix=$(acquire_binary "sentinix")
    local src_sentinixd
    src_sentinixd=$(acquire_binary "sentinixd")

    if [[ -n "${src_sentinix}" && -f "${src_sentinix}" ]]; then
        install_binary_file "${src_sentinix}" "sentinix"
        if [[ -n "${src_sentinixd}" && -f "${src_sentinixd}" ]]; then
            install_binary_file "${src_sentinixd}" "sentinixd"
        fi
        echo -e "  ${GREEN}✓ Sentinix AI CLI & Daemon successfully installed!${RESET}"
    else
        echo -e "  ${YELLOW}⚠️  Sentinix binary not found.${RESET}"
    fi
}

# 4. Install Desktop Apps (DMG / DEB)
install_desktop_app() {
    local app_name="$1" # DialexAI, ArtixAI, KritixAI
    local variant_folder="without-engine"
    if [[ "${ENGINE_CHOICE}" == "with" ]]; then
        variant_folder="with-engine"
    fi

    echo -e "${BOLD}▶ Installing ${app_name} Desktop (${variant_folder})...${RESET}"

    if [[ "${OS_NAME}" == "darwin" ]]; then
        local dmg_file=""
        if [[ -n "${REPO_ROOT}" ]]; then
            dmg_file=$(find "${REPO_ROOT}/dist/desktop/${variant_folder}" -name "${app_name}*.dmg" | head -n 1 || true)
        fi

        if [[ -n "${dmg_file}" && -f "${dmg_file}" ]]; then
            echo "  Mounting DMG: ${dmg_file}..."
            local mount_point="/Volumes/${app_name}_Install_$$"
            hdiutil attach "${dmg_file}" -mountpoint "${mount_point}" -quiet -nobrowse
            local app_bundle
            app_bundle=$(find "${mount_point}" -maxdepth 2 -name "*.app" | head -n 1 || true)

            if [[ -n "${app_bundle}" && -d "${app_bundle}" ]]; then
                local target_app="${APPS_DIR}/$(basename "${app_bundle}")"
                echo "  Copying to ${target_app}..."
                if [[ "${DRY_RUN}" != "true" ]]; then
                    mkdir -p "${APPS_DIR}"
                    rm -rf "${target_app}"
                    cp -R "${app_bundle}" "${target_app}"
                fi
                echo -e "  ${GREEN}✓ Successfully installed ${target_app}${RESET}"
            fi
            hdiutil detach "${mount_point}" -quiet || true
        else
            echo "  ℹ️  ${app_name} DMG not found in dist/. To build it locally, run:"
            echo "     ./scripts/build-dist.sh --desktop --app $(echo "${app_name}" | tr '[:upper:]' '[:lower:]' | sed 's/ai//') --${variant_folder}"
        fi
    elif [[ "${OS_NAME}" == "linux" ]]; then
        local deb_file=""
        local rpm_file=""
        if [[ -n "${REPO_ROOT}" ]]; then
            deb_file=$(find "${REPO_ROOT}/dist/desktop/${variant_folder}" -name "${app_name}*.deb" | head -n 1 || true)
            rpm_file=$(find "${REPO_ROOT}/dist/desktop/${variant_folder}" -name "${app_name}*.rpm" | head -n 1 || true)
        fi
        if [[ -n "${deb_file}" && -f "${deb_file}" && "$(command -v dpkg 2>/dev/null)" ]]; then
            echo "  Installing DEB package: ${deb_file}..."
            sudo dpkg -i "${deb_file}" || true
            echo -e "  ${GREEN}✓ Installed ${app_name} via dpkg.${RESET}"
        elif [[ -n "${rpm_file}" && -f "${rpm_file}" && "$(command -v rpm 2>/dev/null)" ]]; then
            echo "  Installing RPM package: ${rpm_file}..."
            sudo rpm -Uvh "${rpm_file}" || true
            echo -e "  ${GREEN}✓ Installed ${app_name} via rpm.${RESET}"
        elif [[ -n "${deb_file}" || -n "${rpm_file}" ]]; then
            echo "  Package available at: ${deb_file:-${rpm_file}}. Please install with your distribution package manager."
        else
            echo "  ℹ️  ${app_name} package not found in dist/. Run ./scripts/build-dist.sh --desktop"
        fi
    elif [[ "${OS_NAME}" == msys* || "${OS_NAME}" == mingw* || "${OS_NAME}" == cygwin* ]]; then
        local msi_file=""
        local exe_file=""
        if [[ -n "${REPO_ROOT}" ]]; then
            msi_file=$(find "${REPO_ROOT}/dist/desktop/${variant_folder}" -name "${app_name}*.msi" | head -n 1 || true)
            exe_file=$(find "${REPO_ROOT}/dist/desktop/${variant_folder}" -name "${app_name}*.exe" | head -n 1 || true)
        fi
        if [[ -n "${msi_file}" && -f "${msi_file}" ]]; then
            echo "  Launching MSI Installer: ${msi_file}..."
            msiexec.exe /i "$(cygpath -w "${msi_file}" 2>/dev/null || echo "${msi_file}")"
        elif [[ -n "${exe_file}" && -f "${exe_file}" ]]; then
            echo "  Launching Windows Installer: ${exe_file}..."
            cmd.exe /c start "" "${exe_file}"
        else
            echo "  ℹ️  ${app_name} Windows installer not found in dist/. Run ./scripts/build-dist.sh --desktop"
        fi
    fi
}

# ------------------------------------------------------------------------------
# Execute Installation Plan
# ------------------------------------------------------------------------------

# CLI Install step
if [[ "${INSTALL_MODE}" == "cli" || "${INSTALL_MODE}" == "both" ]]; then
    if [[ "${INSTALL_TARGET}" == "all" || "${INSTALL_TARGET}" == "engine" || "${ENGINE_CHOICE}" == "with" ]]; then
        install_engine
    fi
    if [[ "${INSTALL_TARGET}" == "all" || "${INSTALL_TARGET}" == "artix" ]]; then
        install_artix_cli
    fi
    if [[ "${INSTALL_TARGET}" == "all" || "${INSTALL_TARGET}" == "kritix" ]]; then
        install_kritix_cli
    fi
fi

# Desktop Install step
if [[ "${INSTALL_MODE}" == "desktop" || "${INSTALL_MODE}" == "both" ]]; then
    if [[ "${INSTALL_TARGET}" == "all" || "${INSTALL_TARGET}" == "dialex" ]]; then
        install_desktop_app "DialexAI"
    fi
    if [[ "${INSTALL_TARGET}" == "all" || "${INSTALL_TARGET}" == "artix" ]]; then
        install_desktop_app "ArtixAI"
    fi
    if [[ "${INSTALL_TARGET}" == "all" || "${INSTALL_TARGET}" == "kritix" ]]; then
        install_desktop_app "KritixAI"
    fi
fi

# ------------------------------------------------------------------------------
# Final Verification & Post-Install Message
# ------------------------------------------------------------------------------
echo ""
echo -e "${BOLD}${GREEN}==============================================================================${RESET}"
echo -e "${BOLD}${GREEN}  🎉 DialexAI Ecosystem Installation Complete!${RESET}"
echo -e "${BOLD}${GREEN}==============================================================================${RESET}"
echo "Summary of Installed Channels:"
if [[ "${INSTALL_MODE}" == "cli" || "${INSTALL_MODE}" == "both" ]]; then
    echo -e "  • Binaries in:       ${BOLD}${BIN_DIR}${RESET}"
    if [[ ":$PATH:" != *":${BIN_DIR}:"* ]]; then
        echo -e "    ${YELLOW}⚠️  Notice: ${BIN_DIR} is not currently in your \$PATH.${RESET}"
        echo -e "    Add it by running: ${CYAN}export PATH=\"${BIN_DIR}:\$PATH\"${RESET}"
    fi
fi

if [[ "${INSTALL_MODE}" == "desktop" || "${INSTALL_MODE}" == "both" ]]; then
    echo -e "  • Applications in:   ${BOLD}${APPS_DIR}${RESET}"
fi

echo ""
echo "Quick Commands:"
echo "  socratix serve        Start Socratix deliberation daemon (port 7890)"
echo "  artix plan \"topic\"    Start dialectic specification with Stakeholder Council"
echo "  kritix studio         Launch autonomous QA testing studio"
echo "=============================================================================="
