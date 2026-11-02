# Centralized Documentation Governance Dashboard Guide

**Module:** `scribex-ai`  
**Platform:** Self-Hosted Daemon (`scribex serve`) & Multi-Repo Dashboard  
**Audience:** Site Reliability Engineers (SRE), Platform Leads, Engineering Managers  
**Scope:** Streamlined setup: repo implementation, daemon settings, production deployment, and dashboard UI/API items.

---

## Step 1: Initial Implementation in Your Environment

### 1.1 Quickstart Local Launch
Test the dashboard locally using minimal registry and RBAC files:
```bash
# 1. Create a repos file mapping repository names to paths and teams:
echo "core-service $(pwd) core" > repos.txt

# 2. Generate a secure token and its SHA-256 hash:
TOKEN="secret-team-token"
HASH=$(scribex hash-token "$TOKEN")

# 3. Create an rbac file: sha256_hash role teams
echo "$HASH admin *" > rbac.txt

# 4. Start the server:
scribex serve -repos repos.txt -rbac rbac.txt -allowed-root $(pwd) -addr 127.0.0.1:8080
```
Open `http://127.0.0.1:8080` in your browser. Enter any username and your token (`secret-team-token`) at the HTTP Basic prompt.

---

## Step 2: Daemon Settings & RBAC Configuration

### 2.1 The Repos Registry (`/etc/scribex/repos.txt`)
Lists all tracked repositories in the format `name path team`:
```text
auth-service /var/repos/auth-service auth
billing-api /var/repos/billing-api billing
core-engine /var/repos/core-engine core
gateway /var/repos/gateway core
```

### 2.2 The RBAC Access Control File (`/etc/scribex/rbac.txt`)
Maps SHA-256 hashed tokens to roles and assigned teams: `sha256(token) role team1,team2`:
```text
# Global Admin (sees all repositories)
8f3b20c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1 admin *

# Auth Squad (sees only auth repositories)
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 viewer auth
```
*(Compute token hashes using `scribex hash-token <plain_token>`)*.

### 2.3 Filesystem Containment (`-allowed-root`)
To prevent path traversal, `-allowed-root <dir>` ensures that the daemon only scans repositories contained within the designated directory tree.

---

## Step 3: Production Deployment & Proxy Settings

### 3.1 systemd Service (`/etc/systemd/system/scribexd.service`)
```ini
[Unit]
Description=Scribex AI Documentation Governance Daemon
After=network.target

[Service]
Type=simple
User=scribex
Group=scribex
WorkingDirectory=/var/repos
ExecStart=/usr/local/bin/scribex serve \
  -repos /etc/scribex/repos.txt \
  -rbac /etc/scribex/rbac.txt \
  -allowed-root /var/repos \
  -addr 127.0.0.1:8080 \
  -cache-ttl 60s \
  -behind-proxy \
  -trusted-proxy 127.0.0.1/32,10.0.0.0/8
Restart=always

[Install]
WantedBy=multi-user.target
```

### 3.2 Reverse Proxy Configuration (Caddy Example)
```caddyfile
docs-health.internal.corp {
    reverse_proxy 127.0.0.1:8080
}
```

---

## Step 4: Dashboard UI & API Items

### 4.1 Dashboard UI Overview Items
- **Team-Scoped Repositories:** Developers only see documentation metrics for their assigned teams; administrators see the global view.
- **Real-Time Stale Anchors Counter:** Pinpoints how many anchors are out of sync with code across all repositories.
- **Dead Symbol Alerts:** Instantly highlights any documentation anchor pointing to a deleted symbol or file.
- **Read-Only `/config` View:** Displays AI provider and secret presence status (`set` vs `unset`) without revealing secret strings or allowing modifications over HTTP.

### 4.2 REST API Endpoints for Monitoring
- **`GET /api/v1/health`**: Liveness probe returning daemon uptime and version.
- **`GET /api/v1/repos`**: JSON array of repository health metrics (total, stale, dead anchors) for ingestion into Prometheus/Datadog.
