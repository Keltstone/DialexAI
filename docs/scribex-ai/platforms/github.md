# GitHub & GitHub Actions Setup Guide

**Module:** `scribex-ai`  
**Platform:** GitHub Cloud & GitHub Enterprise Server (GHES)  
**Audience:** DevOps Engineers, CI/CD Architects, Open Source Maintainers  
**Scope:** Streamlined setup: repo implementation, GitHub settings, PR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Scaffold Workflows Automatically
Run `scribex ci init github` in your repository root:
```bash
scribex ci init github
```

This automatically generates three pre-configured, hardened GitHub Actions workflows:
- `.github/workflows/scribex-pr.yml`: Fast PR validation and status comment bot.
- `.github/workflows/scribex-docs.yml`: Post-merge sync and companion PR creator.
- `.github/workflows/scribex-scavenge.yml`: Weekly scheduled scavenge and wiki linter.

### 1.2 Test Locally Before Committing
Verify your anchors against the base branch locally:
```bash
scribex check -base origin/main
```

---

## Step 2: GitHub Settings & Permissions

### 2.1 Enable Pull Request Creation in Repository Settings
To allow the post-merge workflow (`scribex-docs.yml`) to open documentation PRs automatically:
1. Navigate to **Settings** $\rightarrow$ **Actions** $\rightarrow$ **General**.
2. Scroll to **Workflow permissions**.
3. Enable: ☑️ **"Allow GitHub Actions to create and approve pull requests"**.
4. Select **Read and write permissions**.

### 2.2 Configure Repository Secrets
In **Settings** $\rightarrow$ **Secrets and variables** $\rightarrow$ **Actions**:
- `ANTHROPIC_API_KEY` (or `OPENAI_API_KEY`): Required if using `scribex sync -ai`.
- `ATLASSIAN_TOKEN`: Required if syncing with Confluence or Jira.

### 2.3 GitHub Enterprise Server (GHES) On-Premise Settings
For self-hosted GitHub Enterprise Server instances on private corporate networks:
```bash
export GITHUB_API_URL="https://github.internal.corp/api/v3"
export SCRIBEX_ALLOW_PRIVATE_HOSTS=1
```

---

## Step 3: CI/CD & PR Gate Settings

The scaffolded `.github/workflows/scribex-pr.yml` uses a secure two-job architecture for gating:

### 3.1 PR Gate Workflow
```yaml
name: Scribex PR Check
on: [pull_request]

jobs:
  check:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27.0'
      - name: Build Scribex
        run: go build -trimpath -o /usr/local/bin/scribex ./cmd/scribex
      - name: Run PR Gate
        run: scribex check -base "origin/${{ github.base_ref }}" -report "$RUNNER_TEMP/report.md"
      - name: Upload Report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: scribex-report
          path: ${{ runner.temp }}/report.md

  comment:
    needs: check
    if: always() && needs.check.result != 'skipped'
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: scribex-report
          path: ${{ runner.temp }}
      - name: Upsert PR Comment
        env:
          GITHUB_TOKEN: ${{ github.token }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
        run: |
          if [ -f "$RUNNER_TEMP/report.md" ]; then
            scribex comment -repo "$GITHUB_REPOSITORY" -pr "$PR_NUMBER" -body-file "$RUNNER_TEMP/report.md"
          fi
```

### 3.2 PR Comment Bot Behavior
- **Non-Spam Upsert:** Scribex identifies existing comments using an internal marker. It updates the comment in-place on new commits rather than creating duplicate notifications.
- **Fail-Closed on Dead Anchors:** If a PR deletes code symbols without updating docs, the gate fails and blocks merge.

---

## Step 4: Scribex Dashboard Items

When monitored via `scribex serve`, GitHub repository health appears in the central control plane:

### 4.1 Dashboard Overview Items
- **Repository Health Matrix:** Displays current anchor status (Total, Stale, Dead) for every tracked GitHub repository.
- **PR Check Failure Rate:** Identifies squads with frequent documentation drift at PR review time.
- **Scavenger Results:** Summarizes findings from the weekly cron scavenge.

### 4.2 Multi-Repo Team Mappings
Configure `/etc/scribex/repos.txt` to organize GitHub repos by squad:
```text
mobile-app /var/repos/mobile-app mobile
backend-api /var/repos/backend-api backend
```
Team members logging into the dashboard with their token only see metrics for their respective repositories.
