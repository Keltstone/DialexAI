#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

echo "=== Artix AI: Test Retention & Regression Gate ==="

# Determine baseline tag
BASELINE_TAG="${1:-}"
if [ -z "${BASELINE_TAG}" ]; then
    # Default to latest enterprise tag
    BASELINE_TAG=$(git tag -l "*enterprise" | sort -V | tail -n 1)
fi

if [ -z "${BASELINE_TAG}" ]; then
    echo "No baseline tag found. Test retention check skipped."
    exit 0
fi

echo "Comparing test suite against baseline tag: ${BASELINE_TAG}"

PREV_TESTS_TMP=$(mktemp)
CURR_TESTS_TMP=$(mktemp)
trap 'rm -f "${PREV_TESTS_TMP}" "${CURR_TESTS_TMP}"' EXIT

# Extract Test* functions from baseline tag
git grep -h "^func Test" "${BASELINE_TAG}" -- "*_test.go" | sed -E 's/^func (Test[A-Za-z0-9_]+).*/\1/' | sort -u > "${PREV_TESTS_TMP}"

# Extract Test* functions from current working tree
git grep -h "^func Test" HEAD -- "*_test.go" | sed -E 's/^func (Test[A-Za-z0-9_]+).*/\1/' | sort -u > "${CURR_TESTS_TMP}"

TOTAL_PREV=$(wc -l < "${PREV_TESTS_TMP}" | tr -d ' ')
TOTAL_CURR=$(wc -l < "${CURR_TESTS_TMP}" | tr -d ' ')

echo "Total tests in baseline (${BASELINE_TAG}): ${TOTAL_PREV}"
echo "Total tests in current commit (HEAD): ${TOTAL_CURR}"

# Find any tests in previous tag that are missing in current
MISSING_TESTS=$(comm -23 "${PREV_TESTS_TMP}" "${CURR_TESTS_TMP}")

if [ -z "${MISSING_TESTS}" ]; then
    echo "SUCCESS: 100% test retention verified (0 tests removed vs ${BASELINE_TAG})."
    exit 0
fi

# If there are missing tests, verify they are in REMOVED_TESTS.md with a valid justification
REMOVED_FILE="REMOVED_TESTS.md"
if [ ! -f "${REMOVED_FILE}" ] && [ -f "artix-ai/REMOVED_TESTS.md" ]; then
    REMOVED_FILE="artix-ai/REMOVED_TESTS.md"
fi

UNAUTHORIZED_DELETIONS=0
while read -r test_name; do
    if [ -z "${test_name}" ]; then
        continue
    fi
    if [ -f "${REMOVED_FILE}" ] && grep -q "${test_name}" "${REMOVED_FILE}"; then
        echo "  [EXEMPT] ${test_name} documented in ${REMOVED_FILE}"
    else
        echo "  [UNAUTHORIZED REMOVAL] Test function missing: ${test_name}"
        UNAUTHORIZED_DELETIONS=$((UNAUTHORIZED_DELETIONS + 1))
    fi
done <<< "${MISSING_TESTS}"

if [ "${UNAUTHORIZED_DELETIONS}" -gt 0 ]; then
    echo "ERROR: ${UNAUTHORIZED_DELETIONS} test(s) removed without entry in ${REMOVED_FILE}." >&2
    exit 1
fi

echo "SUCCESS: All removed tests are documented and reviewed in ${REMOVED_FILE}."
