#!/usr/bin/env bash
# ==============================================================================
# scripts/ledger-check.sh
# Verifies that .dev/kritix-ai/ledger/round<n>.md exists and every open finding
# has a valid fix SHA, failing-before run, passing-after run, and mutation-proof
# (or explicit BLOCKED: <prereq>).
#
# R8-1 Ledger Honesty Gate:
# Parses each row's status and re-runs the row's "passing-after" command.
# A row may say RESOLVED only if that command exits 0 now at HEAD.
# Rows whose acceptance test is red or blocked must say OPEN / BLOCKED: <prereq>
# (never RESOLVED). Mismatch = gate failure.
# Any statement about an agent/model/time without saved raw artifact = fail.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KRITIX_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(git -C "${KRITIX_DIR}" rev-parse --show-toplevel)"

ROUND="${1:-8}"
LEDGER_FILE="${REPO_ROOT}/.dev/kritix-ai/ledger/round${ROUND}.md"
if [ ! -f "${LEDGER_FILE}" ]; then
  # Also check inside kritix-ai/.dev/
  LEDGER_FILE="${KRITIX_DIR}/.dev/kritix-ai/ledger/round${ROUND}.md"
fi

echo "📋 [Ledger Check] Verifying ledger for Round ${ROUND} at ${LEDGER_FILE}..."

if [ ! -f "${LEDGER_FILE}" ]; then
  echo "⛔ LEDGER CHECK FAILED: Ledger file ${LEDGER_FILE} does not exist!" >&2
  exit 1
fi

# 1. Check for fabricated provenance (R8-1)
if [ -f "${REPO_ROOT}/.dev/kritix-ai/corpus-author/MODEL_METADATA.json" ] || [ -f "${KRITIX_DIR}/.dev/kritix-ai/corpus-author/MODEL_METADATA.json" ]; then
  echo "⛔ LEDGER CHECK FAILED: Fabricated provenance detected: MODEL_METADATA.json present without raw author output." >&2
  exit 1
fi

CONTENT=$(cat "${LEDGER_FILE}")

# 2. Parse table rows containing findings
FINDING_ROWS=$(echo "${CONTENT}" | grep -E '^\|[[:space:]]*\*\*FINDING-[^|]+\|' || true)

if [ -z "${FINDING_ROWS}" ]; then
  echo "⛔ LEDGER CHECK FAILED: No finding rows found matching '| **FINDING-...' in ledger." >&2
  exit 1
fi

# 3. Check for forbidden NOT STARTED status inside table rows
if echo "${FINDING_ROWS}" | grep -i "NOT STARTED" >/dev/null; then
  echo "⛔ LEDGER CHECK FAILED: Found 'NOT STARTED' findings in ledger." >&2
  echo "All findings must be fixed with a commit SHA, failing-before, passing-after, and mutation-proof (or explicit BLOCKED: <prereq>)." >&2
  exit 1
fi

ROW_COUNT=0
cd "${KRITIX_DIR}"

while IFS='|' read -r _ id desc commit failing passing proof status _; do
  id=$(echo "${id}" | tr -d '*' | xargs)
  commit=$(echo "${commit}" | tr -d '`' | xargs)
  failing=$(echo "${failing}" | xargs)
  passing=$(echo "${passing}" | xargs)
  proof=$(echo "${proof}" | xargs)
  status=$(echo "${status}" | tr -d '*' | xargs)

  if [ -z "${id}" ]; then
    continue
  fi

  ROW_COUNT=$((ROW_COUNT + 1))
  echo "  • Checking ${id} (Status: ${status})..."

  # Check commit SHA (must be valid 7-40 hex or BLOCKED or -)
  if ! [[ "${commit}" =~ ^[0-9a-f]{7,40}(,[[:space:]]*[0-9a-f]{7,40})*$ ]] && ! [[ "${status}" =~ BLOCKED ]] && [ "${commit}" != "-" ]; then
    echo "    ⛔ Invalid or missing commit SHA for ${id}: '${commit}'" >&2
    exit 1
  fi

  # Check failing-before run
  if [ -z "${failing}" ] || [ "${failing}" = "-" ]; then
    echo "    ⛔ Missing failing-before run description for ${id}" >&2
    exit 1
  fi

  # Check passing-after run
  if [ -z "${passing}" ] || [ "${passing}" = "-" ]; then
    echo "    ⛔ Missing passing-after run description for ${id}" >&2
    exit 1
  fi

  # Check mutation-proof or BLOCKED prerequisite
  if ! echo "${proof}" | grep -E "(scripts/mutations/|scripts/mutation-proof\.sh|BLOCKED:|unit test|mutation test|go test)" >/dev/null; then
    echo "    ⛔ Missing mutation proof or BLOCKED prerequisite specification for ${id}: '${proof}'" >&2
    exit 1
  fi

  # R8-1: Honesty verification of Status
  if [[ "${status}" =~ ^RESOLVED ]]; then
    # Must NOT be blocked
    if [[ "${status}" =~ BLOCKED ]]; then
      echo "    ⛔ Gate failure: ${id} cannot be marked both RESOLVED and BLOCKED: '${status}'" >&2
      exit 1
    fi

    # Extract command from passing-after column
    cmd=""
    if [[ "${passing}" =~ \`([^\`]+)\` ]]; then
      cmd="${BASH_REMATCH[1]}"
    elif [[ "${passing}" =~ (go[[:space:]]+test[[:space:]]+[^;|]+) ]]; then
      cmd="${BASH_REMATCH[1]}"
    elif [[ "${passing}" =~ (bash[[:space:]]+[^;|]+) ]]; then
      cmd="${BASH_REMATCH[1]}"
    fi

    if [ -n "${cmd}" ]; then
      echo "    -> Re-running acceptance command at HEAD: ${cmd}"
      set +e
      CMD_OUT=$(eval "${cmd}" 2>&1)
      CMD_EXIT=$?
      set -e

      if [ ${CMD_EXIT} -ne 0 ]; then
        echo "    ⛔ Gate failure: Finding ${id} claims RESOLVED, but passing-after command exited ${CMD_EXIT}:" >&2
        echo "       Command: ${cmd}" >&2
        echo "       Output: ${CMD_OUT}" >&2
        exit 1
      fi
      echo "    -> ✓ Acceptance command verified (exited 0)"
    else
      echo "    ⛔ Gate failure: Finding ${id} marked RESOLVED but passing-after has no executable command in backticks" >&2
      exit 1
    fi

  elif [[ "${status}" =~ ^OPEN ]]; then
    # Open row: verify it is not marked RESOLVED
    echo "    -> Verified OPEN row (correctly kept open)"
  elif [[ "${status}" =~ ^BLOCKED ]]; then
    # Blocked row: verify it specifies prerequisite
    if ! [[ "${status}" =~ BLOCKED:[[:space:]]*[a-zA-Z0-9_,-]+ ]]; then
      echo "    ⛔ Gate failure: Blocked status must specify missing prerequisite (e.g. BLOCKED: docker)" >&2
      exit 1
    fi
    echo "    -> Verified BLOCKED row (${status})"
  else
    echo "    ⛔ Invalid status for ${id}: '${status}'. Status must be RESOLVED, OPEN, or BLOCKED: <prereq>" >&2
    exit 1
  fi

done <<< "${FINDING_ROWS}"

echo "✓ Ledger Check PASSED: ${ROW_COUNT} findings verified with honest statuses, fix SHAs, and executable acceptance checks."
