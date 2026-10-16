# Sentinix AI: Complete Setup Scenarios & Deployment Guide

**Module:** `sentinix-ai`  
**Author:** Kolta Labs Security & Systems Engineering Group  
**Audience:** DevSecOps Engineers, Platform Teams, Cloud Architects, Developers  
**Scope:** Step-by-step instructions for every deployment scenario, from zero-server pipelines to multi-tenant Kubernetes clusters.

---

## Deployment Scenarios Matrix

| Scenario | Infrastructure Cost | Database Required | Central Server | Best For |
| :--- | :--- | :--- | :--- | :--- |
| **[Scenario 1: Serverless GitHub Actions](#scenario-1-serverless-github-actions)** | **$0** (Free CI tier) | None (Git Baseline) | No | Open-source or single-repo projects wanting fast PR checks |
| **[Scenario 2: Serverless GitLab CI](#scenario-2-serverless-gitlab-ci)** | **$0** (Free CI tier) | None (Git Baseline) | No | GitLab projects using native Security & Compliance dashboards |
| **[Scenario 3: Single-Node Linux VM](#scenario-3-single-node-linux-vm-sentinixd--sqlite)** | Low ($5/mo VPS) | Embedded SQLite | Yes | Small teams wanting a central web dashboard and webhook bot |
| **[Scenario 4: Production Docker Compose](#scenario-4-production-docker-compose-sentinixd--postgresql)** | Medium | PostgreSQL 14+ | Yes | Teams requiring high-concurrency scans and persistent trends |
| **[Scenario 5: Enterprise Kubernetes (Helm)](#scenario-5-enterprise-kubernetes-deployment-helm)** | Enterprise | Managed DB (RDS/CloudSQL) | Yes (K8s Cluster) | Large organizations with strict multi-tenancy and compliance |
| **[Scenario 6: Local Developer CLI & Pre-Commit](#scenario-6-developer-cli-installation--pre-commit-hooks)** | Free | None | Optional | Local developer machine verification before git commit/push |
| **[Scenario 7: Full AI Pipeline & Container Sandbox](#scenario-7-configuring-the-ai-council--container-sandbox)** | Depends on Models | SQLite / Postgres | Yes | Teams wanting automated false-positive triage and draft patches |

---

## Scenario 1: Serverless GitHub Actions

Run Sentinix without maintaining a 24/7 server. Scans execute directly on GitHub Actions runners, using Git itself to track the baseline and uploading findings to GitHub's native **Security** tab.

### Step 1: Establish the Baseline Locally
On your workstation, generate your initial baseline file and push it to GitHub:
```bash
# Install Sentinix via Homebrew shorthand
brew install kolta-labs/tap/sentinix

# Generate baseline on your main branch
sentinix scan -baseline .sentinix-baseline.json -update-baseline .

# Commit baseline into repository
git add .sentinix-baseline.json
git commit -m "chore(security): initialize Sentinix baseline"
git push origin main
```

### Step 2: Create the GitHub Actions Workflow
Create `.github/workflows/sentinix.yml`:

```yaml
name: Sentinix Security Gate

on:
  pull_request:
    branches: [ main, master ]
  push:
    branches: [ main, master ]

permissions:
  contents: read
  security-events: write  # Grants permission to upload SARIF to Security Tab
  pull-requests: write     # Grants permission to annotate PR diffs

jobs:
  sentinix-scan:
    name: Sentinix Vulnerability Scan
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0 # Full history ensures diffs can be evaluated

      - name: Install Sentinix CLI
        run: |
          # Install via Homebrew
          brew install kolta-labs/tap/sentinix
          
          # Verify installation
          sentinix version

      - name: Execute Scan Against Baseline
        run: |
          # Gate on high-severity findings; ignore issues already in .sentinix-baseline.json
          sentinix scan \
            -baseline .sentinix-baseline.json \
            -fail-on high \
            -sarif sentinix.sarif \
            .

      - name: Upload SARIF to GitHub Security Tab
        uses: github/codeql-action/upload-sarif@v3
        if: always() # Upload even if scan fails so alerts appear in PR
        with:
          sarif_file: sentinix.sarif
          category: sentinix-security-gate
```

### Step 3: Enforce as a Required Check
1. In your GitHub repository, navigate to **Settings > Branches**.
2. Under **Branch protection rules**, edit your `main` branch rule.
3. Check **Require status checks to pass before merging**.
4. Search for and check `Sentinix Vulnerability Scan`.
5. Save changes. PRs with new critical or high findings will now be blocked from merging.

---

## Scenario 2: Serverless GitLab CI

Run Sentinix inside GitLab CI runners, publishing findings directly to GitLab Merge Requests and the Security Dashboard.

Create `.gitlab-ci.yml`:

```yaml
stages:
  - test
  - security

sentinix:
  stage: security
  image: golang:1.23-alpine
  before_script:
    - apk add --no-cache git ca-certificates
    - go install github.com/Kolta-Labs/DialexAI/sentinix-ai/cmd/sentinix@main
  script:
    - sentinix scan -baseline .sentinix-baseline.json -fail-on high -sarif sentinix.sarif .
  artifacts:
    when: always
    paths:
      - sentinix.sarif
    reports:
      sast: sentinix.sarif
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

---

## Scenario 3: Single-Node Linux VM (`sentinixd` + SQLite)

Ideal for small engineering teams running a single virtual machine (Ubuntu / Debian / RHEL) with an embedded SQLite database and automatic TLS via Caddy.

### Step 1: Install Binary & Create Service User
```bash
sudo adduser --system --group --home /var/lib/sentinix sentinix
sudo mkdir -p /etc/sentinix /var/lib/sentinix/data
sudo chown -R sentinix:sentinix /var/lib/sentinix

# Build or download binary to /usr/local/bin
sudo cp sentinixd /usr/local/bin/sentinixd
sudo chmod +x /usr/local/bin/sentinixd
```

### Step 2: Configure Git Hosts (`/etc/sentinix/sentinix.json`)
```json
[
  {
    "name": "gh",
    "kind": "github",
    "secret": "$GH_WEBHOOK_SECRET",
    "token": "$GH_ACCESS_TOKEN"
  }
]
```

### Step 3: Configure Environment (`/etc/sentinix/sentinix.env`)
```bash
SENTINIX_SESSION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
GH_WEBHOOK_SECRET="your_shared_webhook_secret"
GH_ACCESS_TOKEN="ghp_your_github_personal_access_token"

# Optional OIDC Login for Web UI
OIDC_ISSUER="https://accounts.google.com"
OIDC_CLIENT_ID="your-client-id.apps.googleusercontent.com"
OIDC_CLIENT_SECRET="your-client-secret"
OIDC_REDIRECT_URL="https://sentinix.corp.example.com/auth/callback"
SENTINIX_OWNER_EMAIL="security-admin@corp.example.com"
```
Secure permissions:
```bash
sudo chown sentinix:sentinix /etc/sentinix/sentinix.env /etc/sentinix/sentinix.json
sudo chmod 600 /etc/sentinix/sentinix.env
```

### Step 4: Create Systemd Service (`/etc/systemd/system/sentinixd.service`)
```ini
[Unit]
Description=Sentinix AI Continuous Security Review Daemon
After=network.target

[Service]
Type=simple
User=sentinix
Group=sentinix
WorkingDirectory=/var/lib/sentinix
EnvironmentFile=/etc/sentinix/sentinix.env
ExecStart=/usr/local/bin/sentinixd \
  -config /etc/sentinix/sentinix.json \
  -db sqlite:/var/lib/sentinix/data/sentinix.db \
  -addr 127.0.0.1:8080 \
  -enforcement block \
  -fail-on high

Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

Enable and start service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sentinixd
sudo systemctl status sentinixd
```

### Step 5: Configure Reverse Proxy (Caddy)
Edit `/etc/caddy/Caddyfile`:
```caddy
sentinix.corp.example.com {
    reverse_proxy 127.0.0.1:8080
}
```
Reload Caddy: `sudo systemctl reload caddy`.

---

## Scenario 4: Production Docker Compose (`sentinixd` + PostgreSQL)

For production teams requiring PostgreSQL durability, automated container restarts, and isolated networks.

### Step 1: Clone Deployment Directory
In your operations repository, use [`sentinix-ai/deploy/docker-compose.yml`](file:///Users/arunelectra/Project/Tech/KMP/Kolta/DialexAI/sentinix-ai/deploy/docker-compose.yml):

```yaml
services:
  db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: sentinix
      POSTGRES_USER: sentinix
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sentinix -d sentinix"]
      interval: 5s
      timeout: 5s
      retries: 5

  sentinix:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      SENTINIX_SESSION_KEY: ${SENTINIX_SESSION_KEY}
      GH_SECRET: ${GH_SECRET}
      GH_TOKEN: ${GH_TOKEN}
      OIDC_ISSUER: ${OIDC_ISSUER}
      OIDC_CLIENT_ID: ${OIDC_CLIENT_ID}
      OIDC_CLIENT_SECRET: ${OIDC_CLIENT_SECRET}
      OIDC_REDIRECT_URL: ${OIDC_REDIRECT_URL}
      SENTINIX_OWNER_EMAIL: ${SENTINIX_OWNER_EMAIL}
    depends_on:
      db:
        condition: service_healthy
    command: >
      sentinixd
      -config /etc/sentinix/sentinix.json
      -db postgres://sentinix:${POSTGRES_PASSWORD}@db:5432/sentinix?sslmode=disable
      -addr :8080
    volumes:
      - ./sentinix.json:/etc/sentinix/sentinix.json:ro

volumes:
  pgdata:
```

### Step 2: Configure Secrets (`.env`)
```bash
POSTGRES_PASSWORD=$(openssl rand -hex 16)
SENTINIX_SESSION_KEY=$(openssl rand -hex 32)
GH_SECRET="webhook_shared_secret"
GH_TOKEN="ghp_token_with_repo_access"
OIDC_ISSUER="https://accounts.google.com"
OIDC_CLIENT_ID="xxx.apps.googleusercontent.com"
OIDC_CLIENT_SECRET="xxx"
OIDC_REDIRECT_URL="https://sentinix.example.com/auth/callback"
SENTINIX_OWNER_EMAIL="lead-security@example.com"
```

### Step 3: Start Stack
```bash
docker compose up -d
docker compose logs -f sentinix
```

---

## Scenario 5: Enterprise Kubernetes Deployment (Helm)

For high-availability clusters using the official Helm chart located at [`sentinix-ai/deploy/helm/sentinix`](file:///Users/arunelectra/Project/Tech/KMP/Kolta/DialexAI/sentinix-ai/deploy/helm/sentinix).

### Step 1: Create Kubernetes Secrets
```bash
kubectl create namespace sentinix

kubectl create secret generic sentinix-secrets \
  --namespace sentinix \
  --from-literal=SENTINIX_DB="postgres://user:password@pg-cluster.internal:5432/sentinix?sslmode=require" \
  --from-literal=SENTINIX_SESSION_KEY="$(openssl rand -hex 32)" \
  --from-literal=GH_SECRET="webhook_secret" \
  --from-literal=GH_TOKEN="ghp_token"
```

### Step 2: Configure Custom `values.yaml`
```yaml
replicaCount: 1 # Run single replica (scheduler executes internally)

image:
  repository: ghcr.io/kolta-labs/sentinix
  tag: "1.0.0"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: sentinix.corp.internal
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: sentinix-tls-cert
      hosts:
        - sentinix.corp.internal

resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 2048Mi

envFromSecret: sentinix-secrets
```

### Step 3: Install Helm Release
```bash
helm upgrade --install sentinix ./deploy/helm/sentinix \
  --namespace sentinix \
  -f values.yaml
```

---

## Scenario 6: Developer CLI Installation & Pre-Commit Hooks

Ensure developers catch vulnerabilities locally before committing code.

### Step 1: Install CLI
```bash
brew install kolta-labs/tap/sentinix
```

### Step 2: Add Git Pre-Commit Hook
In any local Git repository, create `.git/hooks/pre-commit`:

```bash
#!/usr/bin/env bash
set -e

echo "🔍 Running Sentinix Pre-Commit Security Guard..."

# Execute fast local scan on staged files
if ! sentinix scan -fail-on high .; then
    echo "❌ Commit rejected: High or Critical security finding detected."
    echo "Run 'sentinix scan .' to inspect evidence."
    exit 1
fi

echo "✓ Sentinix Security Check Passed."
```
Make executable: `chmod +x .git/hooks/pre-commit`.

---

## Scenario 7: Configuring the AI Council & Container Sandbox

Enable AI-powered false-positive triage, automated patch generation, and regression test verification.

### Step 1: Create `models.json`
Configure your frontier and local LLMs. Secrets formatted as `$NAME` are resolved from environment variables:

```json
[
  {
    "name": "gemini-flash",
    "url": "https://generativelanguage.googleapis.com/v1beta/openai/",
    "model": "gemini-1.5-flash",
    "key": "$GEMINI_API_KEY",
    "local": false
  },
  {
    "name": "deepseek-coder",
    "url": "https://api.deepseek.com/v1",
    "model": "deepseek-coder",
    "key": "$DEEPSEEK_API_KEY",
    "local": false
  },
  {
    "name": "local-ollama",
    "url": "http://localhost:11434/v1",
    "model": "qwen2.5-coder",
    "local": true
  }
]
```

### Step 2: Build the Hardened Scanner Sandbox Image
Build the container sandbox image from [`sentinix-ai/deploy/Dockerfile.scanner`](file:///Users/arunelectra/Project/Tech/KMP/Kolta/DialexAI/sentinix-ai/deploy/Dockerfile.scanner):

```bash
docker build -f deploy/Dockerfile.scanner -t sentinix-scanner:latest .
```

### Step 3: Launch Daemon with AI & Sandbox Flags
```bash
export GEMINI_API_KEY="AIza..."
export DEEPSEEK_API_KEY="sk-..."

sentinixd \
  -config sentinix.json \
  -db sqlite:sentinix.db \
  -ai models.json \
  -sandbox-image sentinix-scanner:latest \
  -addr :8080
```

### Step 4: Configure Project Privacy Policy via Web UI
1. Open `http://localhost:8080/#/settings`.
2. Select your repository.
3. Check **`[x] AI add-on`**.
4. Choose **`cloud-redacted`** in the **Model policy** dropdown.
5. Click **Save**.
6. When opening any finding in `#/findings`, the **"AI triage"** and **"Draft patch"** actions are now live. All API keys remain isolated on the server; code snippets traveling to cloud models have hardcoded tokens and secrets scrubbed beforehand.
