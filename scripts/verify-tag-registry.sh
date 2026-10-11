#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== Verifying Git Tag Immutability Registry against committed registry & remote refs ==="
cd "${REPO_ROOT}"

REGISTRY_FILE="${REPO_ROOT}/docs/release/TAG_IMMUTABILITY_REGISTRY.json"
if [ ! -f "${REGISTRY_FILE}" ]; then
    REGISTRY_FILE="${REPO_ROOT}/artix-ai/docs/release/TAG_IMMUTABILITY_REGISTRY.json"
fi

if [ ! -f "${REGISTRY_FILE}" ]; then
    echo "ERROR: Committed tag registry file not found: ${REGISTRY_FILE}" >&2
    exit 1
fi

# Query remote tags - FAIL CLOSED if offline or empty
set +e
REMOTE_TAGS=$(git ls-remote --tags origin 2>&1)
EXIT_CODE=$?
set -e

if [ ${EXIT_CODE} -ne 0 ] || [ -z "${REMOTE_TAGS}" ]; then
    echo "ERROR: Failed to query remote tags from origin (exit code ${EXIT_CODE}). Fail-closed policy requires reachable remote tags registry." >&2
    echo "${REMOTE_TAGS}" >&2
    exit 1
fi

echo "Loaded committed registry: ${REGISTRY_FILE}"

# Read and verify each tag entry from the JSON registry
python3 -c '
import json, sys, subprocess

registry_path = sys.argv[1]
remote_tags = sys.argv[2]

with open(registry_path, "r") as f:
    data = json.load(f)

tags = data.get("tags", [])
if not tags:
    print("ERROR: Registry contains no tag entries!", file=sys.stderr)
    sys.exit(1)

for entry in tags:
    tag_name = entry["tag"]
    tag_obj = entry["tag_object"]
    commit_obj = entry["commit"]

    print(f"Verifying {tag_name} (tag_obj: {tag_obj}, commit: {commit_obj})...")

    # 1. Check remote tag object
    expected_tag_ref = f"{tag_obj}\trefs/tags/{tag_name}"
    expected_tag_ref_space = f"{tag_obj} refs/tags/{tag_name}"
    if expected_tag_ref not in remote_tags and expected_tag_ref_space not in remote_tags:
        # Check if line contains tag_obj and refs/tags/tag_name
        lines = [line.strip() for line in remote_tags.splitlines()]
        matched = False
        for line in lines:
            parts = line.split()
            if len(parts) >= 2 and parts[0] == tag_obj and parts[1] == f"refs/tags/{tag_name}":
                matched = True
                break
        if not matched:
            print(f"ERROR: Remote tag object for {tag_name} does not match committed registry ({tag_obj})!", file=sys.stderr)
            sys.exit(1)

    # 2. Check remote target commit dereference
    matched_commit = False
    for line in remote_tags.splitlines():
        parts = line.split()
        if len(parts) >= 2 and parts[0] == commit_obj and parts[1] == f"refs/tags/{tag_name}^{{}}":
            matched_commit = True
            break
    if not matched_commit:
        print(f"ERROR: Remote commit target for {tag_name} does not match committed registry ({commit_obj})!", file=sys.stderr)
        sys.exit(1)

    # 3. Check local tag object if local tag exists
    try:
        local_tag_obj = subprocess.check_output(["git", "rev-parse", f"refs/tags/{tag_name}"], text=True).strip()
        if local_tag_obj != tag_obj:
            print(f"ERROR: Local tag object for {tag_name} ({local_tag_obj}) does not match committed registry ({tag_obj})!", file=sys.stderr)
            sys.exit(1)
        local_commit_obj = subprocess.check_output(["git", "rev-parse", f"refs/tags/{tag_name}^{{commit}}"], text=True).strip()
        if local_commit_obj != commit_obj:
            print(f"ERROR: Local commit target for {tag_name} ({local_commit_obj}) does not match committed registry ({commit_obj})!", file=sys.stderr)
            sys.exit(1)
    except Exception as e:
        print(f"Warning: Local tag {tag_name} check: {e}")

print("SUCCESS: All committed tag registry entries match remote refs and local git tag objects.")
' "${REGISTRY_FILE}" "${REMOTE_TAGS}"

