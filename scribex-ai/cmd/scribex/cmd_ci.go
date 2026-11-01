package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"scribex/pkg/fsx"
)

// installStepYAML installs scribex in a generated workflow. Inside the scribex repo it builds from
// source; everywhere else it downloads a pinned release and fails closed unless the URL is https and
// the sha256 matches. Inputs arrive through env (repository variables), never through run: interpolation.
const installStepYAML = `      - name: Install scribex
        env:
          SCRIBEX_RELEASE_URL: ${{ vars.SCRIBEX_RELEASE_URL }}
          SCRIBEX_SHA256: ${{ vars.SCRIBEX_SHA256 }}
          GOTOOLCHAIN: local
          CGO_ENABLED: '0'
          GOFLAGS: -mod=readonly
        run: |
          set -euo pipefail
          mkdir -p "$RUNNER_TEMP/bin"
          if [ -f cmd/scribex/main.go ] && head -n 1 go.mod 2>/dev/null | grep -qx 'module scribex'; then
            go build -trimpath -buildvcs=false -ldflags "-s -w" -o "$RUNNER_TEMP/bin/scribex" ./cmd/scribex
          else
            if [[ ! "${SCRIBEX_RELEASE_URL:-}" =~ ^https://[^[:space:]]+$ ]] || [[ ! "${SCRIBEX_SHA256:-}" =~ ^[0-9a-f]{64}$ ]]; then
              echo "scribex: set repository variables SCRIBEX_RELEASE_URL and SCRIBEX_SHA256 (https URL, 64-hex sha256)" >&2
              exit 2
            fi
            curl -fsSL --proto '=https' --max-time 60 -o "$RUNNER_TEMP/bin/scribex" "$SCRIBEX_RELEASE_URL"
            echo "$SCRIBEX_SHA256  $RUNNER_TEMP/bin/scribex" | sha256sum -c -
            chmod +x "$RUNNER_TEMP/bin/scribex"
          fi
          echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"`

var (
	ghPRWorkflow       = strings.ReplaceAll(ghPRWorkflowT, "@@INSTALL@@", installStepYAML)
	ghDocsWorkflow     = strings.ReplaceAll(ghDocsWorkflowT, "@@INSTALL@@", installStepYAML)
	ghScavengeWorkflow = strings.ReplaceAll(ghScavengeWorkflowT, "@@INSTALL@@", installStepYAML)
	ghGenerateWorkflow = strings.ReplaceAll(ghGenerateWorkflowT, "@@INSTALL@@", installStepYAML)
)

const ghPRWorkflowT = `name: Scribex PR Check
on:
  pull_request:

permissions: {}

concurrency:
  group: scribex-${{ github.workflow }}-${{ github.event.pull_request.number }}
  cancel-in-progress: true

jobs:
  check:
    name: Check Documentation
    runs-on: ubuntu-latest
    permissions:
      contents: read
    timeout-minutes: 10
    defaults:
      run:
        shell: bash
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@f111f3307d8850f501ac008e886eec1fd1932a34 # v5.3.0
        with:
          go-version: '1.27.0'
@@INSTALL@@
      - name: Run scribex check
        env:
          BASE_REF: ${{ github.base_ref }}
        run: |
          set -euo pipefail
          if [[ ! "$BASE_REF" =~ ^[A-Za-z0-9._/@~^-]{1,200}$ ]] || [[ "$BASE_REF" =~ ^- ]]; then
            echo "scribex: invalid base ref $BASE_REF" >&2
            exit 2
          fi
          scribex check -base "origin/$BASE_REF" -report "$RUNNER_TEMP/scribex-report.md"
      - name: Upload report artifact
        if: always()
        uses: actions/upload-artifact@4cec3d8aa04e39d1a68397de0c4cd6fb9dce8ec1 # v4.6.1
        with:
          name: scribex-report
          path: ${{ runner.temp }}/scribex-report.md
          if-no-files-found: ignore

  comment:
    name: Post PR Comment
    needs: check
    if: always() && needs.check.result != 'skipped'
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
    timeout-minutes: 5
    defaults:
      run:
        shell: bash
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          ref: ${{ github.base_ref }}
          persist-credentials: false
      - uses: actions/setup-go@f111f3307d8850f501ac008e886eec1fd1932a34 # v5.3.0
        with:
          go-version: '1.27.0'
@@INSTALL@@
      - name: Download report artifact
        uses: actions/download-artifact@cc203385981b70ca67e1cc392babf9cc229d5806 # v4.1.9
        with:
          name: scribex-report
          path: ${{ runner.temp }}
      - name: Post comment
        env:
          GITHUB_TOKEN: ${{ github.token }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
        run: |
          set -euo pipefail
          if [ -f "$RUNNER_TEMP/scribex-report.md" ]; then
            scribex comment -repo "$GITHUB_REPOSITORY" -pr "$PR_NUMBER" -body-file "$RUNNER_TEMP/scribex-report.md"
          fi
`

const ghDocsWorkflowT = `name: Scribex Docs Sync
on:
  push:
    branches: [main]
  workflow_dispatch:

permissions: {}

jobs:
  docs:
    name: Sync Documentation
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    environment: scribex-ai
    timeout-minutes: 15
    defaults:
      run:
        shell: bash
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          fetch-depth: 0
          persist-credentials: true
      - uses: actions/setup-go@f111f3307d8850f501ac008e886eec1fd1932a34 # v5.3.0
        with:
          go-version: '1.27.0'
@@INSTALL@@
      - name: Validate config and secrets
        run: |
          set -euo pipefail
          scribex config validate
          scribex secrets check
      - name: Sync documentation
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
          ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
          ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
        run: |
          set -euo pipefail
          scribex sync -ai
      - name: Create PR for synced documentation
        env:
          GH_TOKEN: ${{ github.token }}
          COMMIT_SHA: ${{ github.sha }}
        run: |
          set -euo pipefail
          if git diff --quiet; then
            echo "No documentation changes produced by sync."
            exit 0
          fi
          SHORT_SHA="${COMMIT_SHA:0:12}"
          BRANCH="scribex/docs-${SHORT_SHA}"
          git config user.name "scribex-bot"
          git config user.email "scribex-bot@users.noreply.github.com"
          git checkout -b "$BRANCH"
          git commit -am "docs: scribex sync for ${SHORT_SHA}"
          git push origin "HEAD:refs/heads/scribex/docs-${SHORT_SHA}"
          BODY=$(printf "Automated documentation sync for commit %s.\\n\\nAI-drafted sections carry <!-- @scribex:ai -->.\\nNote: Repository setting 'Allow GitHub Actions to create and approve pull requests' is required.\\nHuman review required before merge; scribex-bot will not approve or merge." "$SHORT_SHA")
          gh pr create \
            --title "docs: scribex sync for ${SHORT_SHA}" \
            --body "$BODY" \
            --head "$BRANCH" \
            --base main
`

const ghScavengeWorkflowT = `name: Scribex Scavenge
on:
  schedule:
    - cron: "0 6 * * 1"
  workflow_dispatch:

permissions: {}

jobs:
  scavenge:
    name: Weekly Scavenge & Lint
    runs-on: ubuntu-latest
    permissions:
      contents: read
    timeout-minutes: 10
    defaults:
      run:
        shell: bash
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@f111f3307d8850f501ac008e886eec1fd1932a34 # v5.3.0
        with:
          go-version: '1.27.0'
@@INSTALL@@
      - name: Run scavenge and wiki lint
        run: |
          set -euo pipefail
          scribex scavenge
          scribex wiki lint
`

const ghGenerateWorkflowT = `name: Scribex Generate
on:
  workflow_dispatch:
    inputs:
      types:
        description: "Doc type ids, comma-separated (empty = all enabled)"
        required: false
        default: ""
      include:
        description: "Include globs, comma-separated"
        required: false
        default: ""
      backend:
        description: "Model backend (a vendor CLI is not available on hosted runners)"
        type: choice
        options: [anthropic, openai, ollama]
        default: anthropic
      model:
        description: "Model override (empty = ai.model)"
        required: false
        default: ""
      max_calls:
        description: "Model-call ceiling (empty = ai.max_calls)"
        required: false
        default: ""
      confirm_send:
        description: "I confirm the files in the plan may be sent to the backend"
        type: boolean
        default: false

permissions: {}

jobs:
  generate:
    name: Generate documentation
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    environment: scribex-ai
    timeout-minutes: 60
    defaults:
      run:
        shell: bash
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          fetch-depth: 0
          persist-credentials: true
      - uses: actions/setup-go@f111f3307d8850f501ac008e886eec1fd1932a34 # v5.3.0
        with:
          go-version: '1.27.0'
@@INSTALL@@
      - name: Generate
        env:
          IN_TYPES: ${{ inputs.types }}
          IN_INCLUDE: ${{ inputs.include }}
          IN_BACKEND: ${{ inputs.backend }}
          IN_MODEL: ${{ inputs.model }}
          IN_MAX_CALLS: ${{ inputs.max_calls }}
          IN_CONFIRM: ${{ inputs.confirm_send }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
        run: |
          set -euo pipefail
          [[ "$IN_TYPES$IN_INCLUDE" =~ ^[A-Za-z0-9_.,*/{}-]*$ ]] || { echo "scribex: invalid types or include" >&2; exit 2; }
          [[ "$IN_MODEL" =~ ^[A-Za-z0-9._:/-]{0,100}$ ]] && [[ ! "$IN_MODEL" =~ ^- ]] || { echo "scribex: invalid model" >&2; exit 2; }
          [[ "$IN_MAX_CALLS" =~ ^[0-9]{0,4}$ ]] || { echo "scribex: invalid max_calls" >&2; exit 2; }
          args=(generate -backend "$IN_BACKEND" -apply -branch)
          [ -n "$IN_TYPES" ] && args+=(-types "$IN_TYPES")
          [ -n "$IN_INCLUDE" ] && args+=(-include "$IN_INCLUDE")
          [ -n "$IN_MODEL" ] && args+=(-model "$IN_MODEL")
          [ -n "$IN_MAX_CALLS" ] && args+=(-max-calls "$IN_MAX_CALLS")
          [ "$IN_CONFIRM" = "true" ] && args+=(-confirm-send)
          scribex config validate
          git config user.name "scribex-bot"
          git config user.email "scribex-bot@users.noreply.github.com"
          scribex "${args[@]}"
      - name: Push branch and open PR
        env:
          GH_TOKEN: ${{ github.token }}
          BASE_BRANCH: ${{ github.ref_name }}
        run: |
          set -euo pipefail
          BRANCH=$(git rev-parse --abbrev-ref HEAD)
          if [ "$BRANCH" = "$BASE_BRANCH" ]; then
            echo "No documentation changes."
            exit 0
          fi
          git push origin "HEAD:refs/heads/${BRANCH}"
          gh pr create \
            --title "docs: scribex generate" \
            --body "AI-drafted documentation from scribex generate. Derived sections carry an AI-draft banner; review before merging. scribex-bot will not approve or merge." \
            --head "$BRANCH" \
            --base "$BASE_BRANCH"
`

const glJobT = `# Scribex GitLab CI Pipeline
# Phase 3 Note: Automated docs flow on default branch with GitLab MR creation is not implemented yet.
# Secrets (ANTHROPIC_API_KEY, ATLASSIAN_TOKEN) must be configured in GitLab CI/CD Settings as Protected + Masked
# so MR pipelines from forks and unprotected branches never receive them.

stages:
  - test

scribex-check:
  stage: test
  image: golang:1.27.0@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
  variables:
    GIT_DEPTH: "0"
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
@@GLINSTALL@@  script:
    - scribex config validate
    - TARGET_BRANCH="$CI_MERGE_REQUEST_TARGET_BRANCH_NAME"
    - git fetch origin "$TARGET_BRANCH"
    - scribex check -base "origin/$TARGET_BRANCH"
    - scribex wiki lint
`

const glInstallYAML = `  before_script:
    - |
      if [ -n "$SCRIBEX_SRC" ] && [ -d "$SCRIBEX_SRC" ]; then
        (cd "$SCRIBEX_SRC" && go build -trimpath -buildvcs=false -ldflags "-s -w" -o /usr/local/bin/scribex ./cmd/scribex)
      elif [ -n "$SCRIBEX_RELEASE_URL" ] && [ -n "$SCRIBEX_SHA256" ]; then
        case "$SCRIBEX_RELEASE_URL" in https://*) ;; *) echo "scribex: SCRIBEX_RELEASE_URL must be https" >&2; exit 2;; esac
        echo "$SCRIBEX_SHA256" | grep -Eq '^[0-9a-f]{64}$' || { echo "scribex: SCRIBEX_SHA256 must be 64 lowercase hex chars" >&2; exit 2; }
        curl -fsSL --proto '=https' --max-time 60 "$SCRIBEX_RELEASE_URL" -o /usr/local/bin/scribex
        echo "$SCRIBEX_SHA256  /usr/local/bin/scribex" | sha256sum -c -
        chmod +x /usr/local/bin/scribex
      else
        echo "Error: Set SCRIBEX_SRC (source directory) or SCRIBEX_RELEASE_URL and SCRIBEX_SHA256" >&2
        exit 1
      fi
`

var glJob = strings.ReplaceAll(glJobT+glGenerateJob, "@@GLINSTALL@@", glInstallYAML)

// glGenerateJob is manual-only: it runs from "Run pipeline" in the web UI, never on push or MR events.
var glGenerateJob = `
scribex-generate:
  stage: test
  image: golang:1.27.0@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
  variables:
    GIT_DEPTH: "0"
    SCRIBEX_TYPES:
      value: ""
      description: "Comma-separated doc type ids (empty = all enabled in .scribex/doctypes.json)"
    SCRIBEX_INCLUDE:
      value: ""
      description: "Comma-separated include globs"
    SCRIBEX_BACKEND:
      value: "anthropic"
      description: "anthropic | openai | ollama (a vendor CLI is not available on shared runners)"
    SCRIBEX_CONFIRM_SEND:
      value: "no"
      description: "Type yes to allow the planned files to be sent to the backend"
  rules:
    - if: $CI_PIPELINE_SOURCE == "web"
      when: manual
  environment: scribex-ai
@@GLINSTALL@@  script:
    - |
      set -eu
      case "$SCRIBEX_BACKEND" in anthropic|openai|ollama) ;; *) echo "scribex: invalid backend" >&2; exit 2;; esac
      echo "$SCRIBEX_TYPES$SCRIBEX_INCLUDE" | grep -Eq '^[A-Za-z0-9_.,*/{}-]*$' || { echo "scribex: invalid types or include" >&2; exit 2; }
      [ -n "${SCRIBEX_PUSH_TOKEN:-}" ] || { echo "scribex: set a protected, masked SCRIBEX_PUSH_TOKEN (write_repository)" >&2; exit 2; }
      scribex config validate
      CONFIRM=""; [ "$SCRIBEX_CONFIRM_SEND" = "yes" ] && CONFIRM="-confirm-send"
      git config user.name scribex-bot
      git config user.email scribex-bot@users.noreply.gitlab.com
      git checkout -q -B "$CI_COMMIT_REF_NAME" "$CI_COMMIT_SHA"
      scribex generate -backend "$SCRIBEX_BACKEND" -types "$SCRIBEX_TYPES" -include "$SCRIBEX_INCLUDE" -apply $CONFIRM -branch
      BRANCH=$(git rev-parse --abbrev-ref HEAD)
      if [ "$BRANCH" = "$CI_COMMIT_REF_NAME" ]; then echo "No documentation changes."; exit 0; fi
      git push -o merge_request.create -o merge_request.target="$CI_DEFAULT_BRANCH" -o merge_request.title="docs: scribex generate" "https://oauth2:${SCRIBEX_PUSH_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git" "HEAD:refs/heads/$BRANCH"
`

func writeProtectedFile(root, path, body string) error {
	// Check if any parent component (e.g. .github, .github/workflows) is a symlink
	curr := filepath.Dir(path)
	for curr != "." && curr != "/" && curr != "" {
		parentP := filepath.Join(root, curr)
		if fi, err := os.Lstat(parentP); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symlink %s", curr)
		}
		curr = filepath.Dir(curr)
	}

	p := filepath.Join(root, path)
	if fi, err := os.Lstat(p); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to overwrite symlink %s", path)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing to overwrite non-regular file %s", path)
		}
		if !*force {
			return fmt.Errorf("%s exists (use -force)", path)
		}
	}

	if err := fsx.WriteFileIn(root, path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

func ciCmd(root string, a []string) error {
	if len(a) != 2 || a[0] != "init" {
		return fmt.Errorf("ci init github|gitlab")
	}
	switch a[1] {
	case "github":
		files := []struct {
			path string
			body string
		}{
			{".github/workflows/scribex-pr.yml", ghPRWorkflow},
			{".github/workflows/scribex-docs.yml", ghDocsWorkflow},
			{".github/workflows/scribex-scavenge.yml", ghScavengeWorkflow},
			{".github/workflows/scribex-generate.yml", ghGenerateWorkflow},
		}
		for _, f := range files {
			if err := writeProtectedFile(root, f.path, f.body); err != nil {
				return err
			}
		}
		fmt.Println("set repository variables SCRIBEX_RELEASE_URL and SCRIBEX_SHA256 (see scripts/build.sh SHA256SUMS)")
		fmt.Println("scribex-generate.yml is manual (workflow_dispatch); it needs the scribex-ai environment with ANTHROPIC_API_KEY or OPENAI_API_KEY")
		return nil
	case "gitlab":
		return writeProtectedFile(root, ".gitlab-ci.scribex.yml", glJob)
	default:
		return fmt.Errorf("ci init github|gitlab")
	}
}
