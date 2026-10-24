#!/usr/bin/env bash
# ==============================================================================
# DialexAI Ecosystem Suite — Synchronized Version Bumper & Release Logger
# Updates:
#   - Root gradle.properties
#   - dialex-ai/gradle.properties
#   - artix-ai/gradle.properties
#   - kritix-ai/gradle.properties
#   - packaging/homebrew/ Formula & Casks
#   - scripts/build-dist.sh, scripts/install.sh, scripts/install.ps1
#   - Master CHANGELOG.md (promotes [Unreleased] to [new_version] - date)
# ==============================================================================

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ $# -lt 1 ]; then
    echo "❌ Error: Version argument required."
    echo "Usage: $0 <new-version> [--dry-run] [--no-git]"
    echo "Example: $0 1.1.0"
    exit 1
fi

NEW_VERSION="$1"
DRY_RUN=false
NO_GIT=false

shift
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --no-git)
            NO_GIT=true
            shift
            ;;
        *)
            echo "Unknown flag: $1"
            exit 1
            ;;
    esac
done

# Strip leading 'v'
NEW_VERSION="${NEW_VERSION#v}"

# Validate SemVer pattern
if [[ ! "${NEW_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    echo "❌ Error: Version '${NEW_VERSION}' does not match Semantic Versioning (X.Y.Z)."
    exit 1
fi

CURRENT_VERSION_CODE=$(grep -E "^app.versionCode=" "${ROOT_DIR}/gradle.properties" 2>/dev/null | cut -d'=' -f2 || echo "1")
NEW_VERSION_CODE=$((CURRENT_VERSION_CODE + 1))
TODAY=$(date +"%Y-%m-%d")

echo "=============================================================================="
echo "  🚀 Bumping DialexAI Suite Version Across All 5 Systems"
echo "  • New Version:      ${NEW_VERSION}"
echo "  • New Version Code: ${NEW_VERSION_CODE}"
echo "  • Release Date:     ${TODAY}"
echo "=============================================================================="

if [ "${DRY_RUN}" = true ]; then
    echo "🔍 [DRY RUN] Would update all properties, formulas, casks, and changelogs."
    exit 0
fi

# 1. Update gradle.properties files
update_gradle_props() {
    local file="$1"
    if [[ -f "${file}" ]]; then
        if grep -q "app.version=" "${file}"; then
            sed -i.bak -e "s/^app.version=.*/app.version=${NEW_VERSION}/" "${file}"
        else
            echo "app.version=${NEW_VERSION}" >> "${file}"
        fi
        if grep -q "app.versionCode=" "${file}"; then
            sed -i.bak -e "s/^app.versionCode=.*/app.versionCode=${NEW_VERSION_CODE}/" "${file}"
        else
            echo "app.versionCode=${NEW_VERSION_CODE}" >> "${file}"
        fi
        rm -f "${file}.bak"
        echo "  ✓ Updated ${file}"
    fi
}

update_gradle_props "${ROOT_DIR}/gradle.properties"
update_gradle_props "${ROOT_DIR}/dialex-ai/gradle.properties"
update_gradle_props "${ROOT_DIR}/artix-ai/gradle.properties"
update_gradle_props "${ROOT_DIR}/kritix-ai/gradle.properties"

# 2. Update Homebrew formulas and casks
for f in "${ROOT_DIR}/packaging/homebrew/Formula"/*.rb; do
    if [[ -f "$f" ]]; then
        sed -i.bak -e "s/version \"[0-9.]*\"/version \"${NEW_VERSION}\"/" "$f"
        sed -i.bak -e "s|/v[0-9.]*\.tar\.gz|/v${NEW_VERSION}.tar.gz|" "$f"
        rm -f "${f}.bak"
        echo "  ✓ Updated Formula: $(basename "$f")"
    fi
done

for c in "${ROOT_DIR}/packaging/homebrew/Casks"/*.rb; do
    if [[ -f "$c" ]]; then
        sed -i.bak -e "s/version \"[0-9.]*\"/version \"${NEW_VERSION}\"/" "$c"
        rm -f "${c}.bak"
        echo "  ✓ Updated Cask: $(basename "$c")"
    fi
done

# 3. Update installer and build scripts
sed -i.bak -e "s/^VERSION=\"[0-9.]*\"/VERSION=\"${NEW_VERSION}\"/" "${ROOT_DIR}/scripts/install.sh"
rm -f "${ROOT_DIR}/scripts/install.sh.bak"
echo "  ✓ Updated scripts/install.sh"

sed -i.bak -e "s/\\\$Version = \"[0-9.]*\"/\$Version = \"${NEW_VERSION}\"/" "${ROOT_DIR}/scripts/install.ps1"
rm -f "${ROOT_DIR}/scripts/install.ps1.bak"
echo "  ✓ Updated scripts/install.ps1"

sed -i.bak -e "s/VERSION=\"\${APP_VERSION:-[0-9.]*}\"/VERSION=\"\${APP_VERSION:-${NEW_VERSION}}\"/" "${ROOT_DIR}/scripts/build-dist.sh"
rm -f "${ROOT_DIR}/scripts/build-dist.sh.bak"
echo "  ✓ Updated scripts/build-dist.sh"

# 4. Update CHANGELOG.md (Master and Dialex)
update_changelog() {
    local changelog="$1"
    if [[ -f "${changelog}" ]] && grep -q "## \[Unreleased\]" "${changelog}"; then
        local temp_file
        temp_file=$(mktemp)
        awk -v ver="${NEW_VERSION}" -v date="${TODAY}" '
        /## \[Unreleased\]/ {
            print "## [Unreleased]\n"
            print "### Added\n"
            print "---\n"
            print "## [" ver "] - " date
            next
        }
        { print }
        ' "${changelog}" > "${temp_file}"
        mv "${temp_file}" "${changelog}"
        echo "  ✓ Promoted [Unreleased] in $(basename "${changelog}")"
    fi
}

update_changelog "${ROOT_DIR}/CHANGELOG.md"
update_changelog "${ROOT_DIR}/dialex-ai/CHANGELOG.md"

# 5. Git Commit & Tag
if [ "${NO_GIT}" = false ]; then
    cd "${ROOT_DIR}"
    git add gradle.properties dialex-ai/gradle.properties artix-ai/gradle.properties kritix-ai/gradle.properties \
            packaging/homebrew/ scripts/ CHANGELOG.md dialex-ai/CHANGELOG.md
    git commit -m "chore(release): bump suite version to v${NEW_VERSION}"
    git tag -a "v${NEW_VERSION}" -m "Release v${NEW_VERSION} — DialexAI Suite"
    echo "  ✓ Created git commit and tag: v${NEW_VERSION}"
    echo ""
    echo "💡 To publish release, push git tag:"
    echo "   git push origin main && git push origin v${NEW_VERSION}"
fi

echo "🎉 Synchronized version bump to v${NEW_VERSION} completed successfully!"
