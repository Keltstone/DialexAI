#!/usr/bin/env bash
# ==============================================================================
# Sentinix AI: Android Security & Policy Enforcement Interactive Demo
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
APP_DIR="${SCRIPT_DIR}/android-showcase"
SENTINIX="${ROOT_DIR}/dist/bin/sentinix"

# Colors for terminal presentation
BOLD="\033[1m"
GREEN="\033[0;32m"
BLUE="\033[0;34m"
YELLOW="\033[0;33m"
RED="\033[0;31m"
CYAN="\033[0;36m"
NC="\033[0m"

echo -e "${BOLD}${CYAN}==============================================================================${NC}"
echo -e "${BOLD}${CYAN}  🛡️  SENTINIX AI — ANDROID SECURITY & GOVERNANCE DEMONSTRATION${NC}"
echo -e "${BOLD}${CYAN}==============================================================================${NC}"
echo -e "Target Directory: ${BOLD}${APP_DIR}${NC}"
echo -e "Engine:           ${BOLD}Sentinix v1.0.0 (Deterministic RE2 Pattern Matcher)${NC}"
echo -e "Rule Sets:        ${BOLD}Android Pack (60 rules) + Custom Banking Rules${NC}"
echo ""

# Ensure CLI binary exists
if [[ ! -f "${SENTINIX}" ]]; then
    echo -e "${RED}Sentinix binary not found at ${SENTINIX}. Compiling...${NC}"
    "${ROOT_DIR}/scripts/build-dist.sh" --cli --app sentinix
fi

# Cleanup any previous demo artifacts
rm -f "${APP_DIR}/.sentinix-baseline.json" "${APP_DIR}/security-report.sarif" "${APP_DIR}/src/VulnerablePatch.kt"

# ------------------------------------------------------------------------------
# ACT 1: OUT-OF-THE-BOX SCAN & RULE PACK AUTODETECTION
# ------------------------------------------------------------------------------
echo -e "${BOLD}${BLUE}[ACT 1/4] Running Initial Codebase Security Scan...${NC}"
echo -e "Sentinix automatically detects AndroidManifest.xml and Kotlin source files,"
echo -e "activating the 60-rule Android MASVS security pack and team custom rules."
echo ""

set +e
"${SENTINIX}" scan \
    -rules "${APP_DIR}/.sentinix/custom-rules.json" \
    -policy "${APP_DIR}/.sentinix/policy.json" \
    -report-only \
    "${APP_DIR}"
set -e

echo ""
echo -e "${GREEN}✓ Act 1 Complete: Vulnerabilities detected and classified by policy:${NC}"
echo -e "  • ${RED}[high|block]${NC}   Exported components, Hardcoded AES keys, App Link hijack"
echo -e "  • ${YELLOW}[high|block]${NC} Custom banking rule: Direct KeyStore password load"
echo -e "  • ${BLUE}[low|notice]${NC}  AllowBackup flag marked as informational Notice per policy"
echo ""
read -p "Press Enter to proceed to Act 2 (Establishing Baseline)..." _
echo ""

# ------------------------------------------------------------------------------
# ACT 2: ESTABLISHING A SECURITY BASELINE
# ------------------------------------------------------------------------------
echo -e "${BOLD}${BLUE}[ACT 2/4] Establishing Security Baseline (.sentinix-baseline.json)...${NC}"
echo -e "Legacy codebases contain existing debt. Sentinix baselines legacy issues so"
echo -e "active developers are NEVER blocked by issues they did not introduce."
echo ""

"${SENTINIX}" scan \
    -rules "${APP_DIR}/.sentinix/custom-rules.json" \
    -policy "${APP_DIR}/.sentinix/policy.json" \
    -baseline "${APP_DIR}/.sentinix-baseline.json" \
    -update-baseline \
    "${APP_DIR}"

echo -e "${GREEN}✓ Baseline recorded in .sentinix-baseline.json with content-based fingerprints.${NC}"
echo ""

# Verify zero-findings scan against baseline
echo -e "${BOLD}Verifying codebase against established baseline:${NC}"
set +e
"${SENTINIX}" scan \
    -rules "${APP_DIR}/.sentinix/custom-rules.json" \
    -policy "${APP_DIR}/.sentinix/policy.json" \
    -baseline "${APP_DIR}/.sentinix-baseline.json" \
    -report-only \
    "${APP_DIR}"
set -e

echo ""
echo -e "${GREEN}✓ Act 2 Complete: 0 new findings reported. Baseline protects the active sprint.${NC}"
echo ""
read -p "Press Enter to proceed to Act 3 (Simulating a PR Regression)..." _
echo ""

# ------------------------------------------------------------------------------
# ACT 3: PR REGRESSION SIMULATION & CI GATE BLOCK
# ------------------------------------------------------------------------------
echo -e "${BOLD}${BLUE}[ACT 3/4] Simulating a Pull Request Introducing a New Vulnerability...${NC}"
echo -e "A developer opens a PR introducing obsolete DES cryptography in a new file:"
echo -e "  -> ${BOLD}src/VulnerablePatch.kt${NC}"
echo ""

cat << 'EOF' > "${APP_DIR}/src/VulnerablePatch.kt"
package com.enterprise.banking

import javax.crypto.Cipher

class VulnerablePatch {
    fun insecureCrypto() {
        // Introduced in PR: Obsolete DES cipher (CWE-327)
        val cipher = Cipher.getInstance("DES/CBC/PKCS5Padding")
    }
}
EOF

echo -e "Scanning PR branch with policy gate (-fail-on high):"
echo ""

set +e
"${SENTINIX}" scan \
    -rules "${APP_DIR}/.sentinix/custom-rules.json" \
    -policy "${APP_DIR}/.sentinix/policy.json" \
    -baseline "${APP_DIR}/.sentinix-baseline.json" \
    -sarif "${APP_DIR}/security-report.sarif" \
    -fail-on high \
    "${APP_DIR}"
EXIT_CODE=$?
set -e

echo ""
if [[ ${EXIT_CODE} -ne 0 ]]; then
    echo -e "${RED}🛑 CI GATE FAILED (Exit Code: ${EXIT_CODE})${NC}"
    echo -e "Sentinix caught the newly introduced regression and blocked the pull request,"
    echo -e "while completely ignoring the baselined legacy debt!"
else
    echo -e "${YELLOW}Notice: Scan finished with code ${EXIT_CODE}${NC}"
fi

echo ""
read -p "Press Enter to proceed to Act 4 (Compliance Artifacts)..." _
echo ""

# ------------------------------------------------------------------------------
# ACT 4: COMPLIANCE ARTIFACTS & SARIF
# ------------------------------------------------------------------------------
echo -e "${BOLD}${BLUE}[ACT 4/4] Inspecting Generated Compliance Artifacts...${NC}"
if [[ -f "${APP_DIR}/security-report.sarif" ]]; then
    SARIF_SIZE=$(wc -c < "${APP_DIR}/security-report.sarif" | tr -d ' ')
    echo -e "${GREEN}✓ SARIF v2.1.0 generated: ${APP_DIR}/security-report.sarif (${SARIF_SIZE} bytes)${NC}"
    echo -e "  Ready for upload to GitHub Code Scanning, GitLab Security, or SonarQube."
fi

# Cleanup patch to leave tree clean
rm -f "${APP_DIR}/src/VulnerablePatch.kt"

echo ""
echo -e "${BOLD}${GREEN}==============================================================================${NC}"
echo -e "${BOLD}${GREEN}  🎉 DEMO COMPLETED SUCCESSFULLY!${NC}"
echo -e "${BOLD}${GREEN}==============================================================================${NC}"
echo -e "Key Takeaways Demonstrated:"
echo -e "  1. Automatic stack detection (60 Android rules + custom team rules)"
echo -e "  2. Content-fingerprinted baselines (never block on pre-existing debt)"
echo -e "  3. Strict differential PR gating (fails ONLY on newly introduced regressions)"
echo -e "  4. Granular policy enforcement (Block / Warn / Notice / Ignore)"
echo -e "  5. Standard SARIF export for CI/CD integration"
echo ""
