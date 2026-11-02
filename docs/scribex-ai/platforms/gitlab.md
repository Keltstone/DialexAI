# GitLab CI/CD Setup Guide

**Module:** `scribex-ai`  
**Platform:** GitLab SaaS & Self-Hosted GitLab CE / EE  
**Audience:** DevOps Engineers, GitLab Administrators, Release Engineers  
**Scope:** Streamlined setup: repo implementation, GitLab settings, MR gating, and dashboard items.

---

## Step 1: Initial Implementation in the Repository

### 1.1 Scaffold the GitLab Pipeline
Run `scribex ci init gitlab` in your repository root:
```bash
scribex ci init gitlab
```
This generates `.gitlab-ci.scribex.yml`.

### 1.2 Include in Main `.gitlab-ci.yml`
Add the following line to your project's `.gitlab-ci.yml`:
```yaml
include:
  - local: '.gitlab-ci.scribex.yml'
```

### 1.3 Test Locally Before Committing
Verify your anchors against the target branch locally:
```bash
scribex check -base origin/main
```

---

## Step 2: GitLab Settings & CI/CD Variables

### 2.1 Configure Protected & Masked Variables
Navigate to **Settings** $\rightarrow$ **CI/CD** $\rightarrow$ **Variables**:
- `ANTHROPIC_API_KEY` (or `OPENAI_API_KEY`): AI provider token.
- `ATLASSIAN_TOKEN`: For Confluence/Jira sync.

**Attributes to Enable:**
- ☑️ **Masked:** Prevents secrets from being exposed in pipeline job logs.
- ☑️ **Protected:** Ensures variables are only passed to pipelines running on protected branches (e.g., `main`), protecting credentials from untrusted fork Merge Requests.

### 2.2 Configure Full Git History
In your pipeline settings, ensure `GIT_DEPTH: "0"` is configured so that git can compute commit diffs against the target branch.

### 2.3 Self-Hosted GitLab & Private Network Settings
If running on-premise GitLab instances with internal endpoints:
```yaml
variables:
  SCRIBEX_ALLOW_PRIVATE_HOSTS: "1"
```

---

## Step 3: CI/CD & MR Gate Settings

The scaffolded `.gitlab-ci.scribex.yml` gates Merge Requests automatically:

```yaml
stages:
  - test

scribex-check:
  stage: test
  image: golang:1.27.0@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
  variables:
    GIT_DEPTH: "0"
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  before_script:
    - |
      if [ -n "$SCRIBEX_RELEASE_URL" ] && [ -n "$SCRIBEX_SHA256" ]; then
        curl -fsSL "$SCRIBEX_RELEASE_URL" -o /usr/local/bin/scribex
        echo "$SCRIBEX_SHA256  /usr/local/bin/scribex" | sha256sum -c -
        chmod +x /usr/local/bin/scribex
      else
        go build -trimpath -ldflags "-s -w" -o /usr/local/bin/scribex ./cmd/scribex
      fi
  script:
    - scribex config validate
    - TARGET_BRANCH="$CI_MERGE_REQUEST_TARGET_BRANCH_NAME"
    - git fetch origin "$TARGET_BRANCH"
    - scribex check -base "origin/$TARGET_BRANCH"
    - scribex wiki lint
```

- **MR Gate Behavior:** If public signatures changed without updating docs, the MR pipeline fails and blocks merge.
- **Wiki Lint:** Catches broken wikilinks or orphan documentation pages.

---

## Step 4: Scribex Dashboard Items

When monitored via `scribex serve`, GitLab repositories provide continuous metrics:

### 4.1 Dashboard Overview Items
- **Repository Health Matrix:** Displays anchor freshness and dead anchor alerts across all tracked GitLab repos.
- **MR Failure Indicators:** Flags repositories with recurring documentation drift during MR reviews.
- **Wiki Integrity Status:** Displays the count of valid vs. broken wiki links.

### 4.2 Multi-Repo Team Mappings
Configure `/etc/scribex/repos.txt` to organize GitLab repos by squad:
```text
payment-gateway /var/repos/payment-gateway payments
user-service /var/repos/user-service auth
```
Team members logging into the dashboard with their token only see metrics for their respective repositories.
