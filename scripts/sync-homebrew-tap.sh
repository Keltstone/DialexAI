#!/usr/bin/env bash
# ==============================================================================
# Sync Homebrew Formulas and Casks to Standalone Homebrew Tap Repository
# Enables Homebrew shorthand syntax:
#   brew install kolta-labs/tap/sentinix
#   or:
#   brew install kolta-labs/dialexai/sentinix
# ==============================================================================

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DIR="${ROOT_DIR}/packaging/homebrew"
DEFAULT_ORG="Kolta-Labs"
TAP_REPO_NAME="${1:-homebrew-tap}"
TARGET_REPO="https://github.com/${DEFAULT_ORG}/${TAP_REPO_NAME}.git"
WORK_DIR="/tmp/homebrew-tap-sync-$$"

echo "=============================================================================="
echo "  🍺 Syncing Homebrew Tap: ${DEFAULT_ORG}/${TAP_REPO_NAME}"
echo "=============================================================================="

if [[ ! -d "${SOURCE_DIR}" ]]; then
    echo "❌ Error: Source directory ${SOURCE_DIR} not found."
    exit 1
fi

rm -rf "${WORK_DIR}"
mkdir -p "${WORK_DIR}"
trap 'rm -rf "${WORK_DIR}"' EXIT

# Clone or initialize tap repo
if git ls-remote "${TARGET_REPO}" &>/dev/null; then
    echo "• Cloning existing tap repository..."
    git clone --depth 1 "${TARGET_REPO}" "${WORK_DIR}"
else
    echo "• Initializing new tap repository..."
    cd "${WORK_DIR}"
    git init
    git branch -M main
fi

# Copy formulas and casks
mkdir -p "${WORK_DIR}/Formula"
mkdir -p "${WORK_DIR}/Casks"
cp -R "${SOURCE_DIR}/Formula/"* "${WORK_DIR}/Formula/" 2>/dev/null || true
cp -R "${SOURCE_DIR}/Casks/"* "${WORK_DIR}/Casks/" 2>/dev/null || true

# Generate Tap README
cat <<EOF > "${WORK_DIR}/README.md"
# Kolta Labs Homebrew Tap

Official Homebrew formulas and casks for the DialexAI Ecosystem:
- **Sentinix AI** (\`sentinix\`): Continuous Security Review, PR Gate & Vulnerability Posture Engine
- **Artix AI** (\`artix\`): Autonomous Coding & Dialectic Specification CLI
- **Kritix AI** (\`kritix\`): Autonomous QA & Testing Studio CLI
- **Socratix Deliberation Engine** (\`socratix\`): Multi-Agent Deliberation Daemon & Bench

## Quick Install (Shorthand — No URL Needed)

\`\`\`bash
# Install Sentinix Security Review CLI & Daemon
brew install kolta-labs/tap/sentinix

# Install Artix AI CLI
brew install kolta-labs/tap/artix

# Install Kritix AI CLI
brew install kolta-labs/tap/kritix

# Install Socratix Deliberation Engine
brew install kolta-labs/tap/socratix
\`\`\`

## Alternatively: Add Tap

\`\`\`bash
brew tap kolta-labs/tap
brew install sentinix
\`\`\`
EOF

cd "${WORK_DIR}"
git add -A

if git diff --cached --quiet; then
    echo "✓ Homebrew tap is already up to date. No changes to commit."
    exit 0
fi

git config user.name "Kolta Labs Bot"
git config user.email "bot@koltalabs.com"
git commit -m "feat(tap): synchronize formulas and casks from DialexAI suite"

echo ""
echo "🎉 Staged tap repository ready at: ${WORK_DIR}"
echo ""
echo "To push to GitHub, run:"
echo "  cd ${WORK_DIR}"
echo "  git remote add origin ${TARGET_REPO}"
echo "  git push -u origin main"
echo ""

if [[ -n "${GH_PAT:-}" || -n "${GITHUB_TOKEN:-}" ]]; then
    TOKEN="${GH_PAT:-${GITHUB_TOKEN}}"
    AUTH_REPO="https://x-access-token:${TOKEN}@github.com/${DEFAULT_ORG}/${TAP_REPO_NAME}.git"
    echo "• Pushing to ${DEFAULT_ORG}/${TAP_REPO_NAME} via token..."
    git push -f "${AUTH_REPO}" main
    echo "✓ Pushed successfully!"
fi
