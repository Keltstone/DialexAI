package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const ghWorkflow = `name: Scribex
on:
  pull_request:
  push: { branches: [main] }
  schedule: [{ cron: "0 6 * * 1" }]   # weekly stale-doc scavenge
permissions: { contents: write, pull-requests: write }
env:   # Settings > Secrets and variables > Actions. Secrets are env-only here; never commit them.
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}   # name must match ai.api_key_env in .scribex/config.yaml
  ATLASSIAN_URL: ${{ vars.ATLASSIAN_URL }}
  ATLASSIAN_USER: ${{ vars.ATLASSIAN_USER }}
  ATLASSIAN_TOKEN: ${{ secrets.ATLASSIAN_TOKEN }}
jobs:
  scribex:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - run: go install ${{ vars.SCRIBEX_MODULE }}@latest   # repo variable, e.g. github.com/you/dialexai/scribex-ai/cmd/scribex
      - run: scribex config validate
      - if: github.event_name == 'pull_request'
        run: |
          scribex comment -repo "$GITHUB_REPOSITORY" -pr "${{ github.event.number }}" -base "origin/${{ github.base_ref }}"
          scribex check -base "origin/${{ github.base_ref }}"
      - if: github.event_name == 'push'
        run: |
          scribex secrets check
          scribex sync -ai -commit
          git push
      - if: github.event_name == 'schedule'
        run: scribex scavenge && scribex wiki lint
`

const glJob = `scribex:
  image: golang:latest
  variables: { GIT_DEPTH: "0" }   # secrets: Settings > CI/CD > Variables (masked): ANTHROPIC_API_KEY, ATLASSIAN_TOKEN, ...
  script:
    - go install "$SCRIBEX_MODULE@latest"
    - export PATH="$PATH:$(go env GOPATH)/bin"
    - scribex config validate
    - git fetch origin "$CI_DEFAULT_BRANCH"
    - scribex check -base "origin/$CI_DEFAULT_BRANCH"
    - scribex wiki lint
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_PIPELINE_SOURCE == "schedule"
`

func ciCmd(root string, a []string) error {
	if len(a) != 2 || a[0] != "init" {
		return fmt.Errorf("ci init github|gitlab")
	}
	path, body := "", ""
	switch a[1] {
	case "github":
		path, body = ".github/workflows/scribex.yml", ghWorkflow
	case "gitlab":
		path, body = ".gitlab-ci.scribex.yml", glJob
	default:
		return fmt.Errorf("ci init github|gitlab")
	}
	p := filepath.Join(root, path)
	if _, err := os.Stat(p); err == nil && !*force {
		return fmt.Errorf("%s exists (use -force)", path)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return os.WriteFile(p, []byte(body), 0o644)
}
