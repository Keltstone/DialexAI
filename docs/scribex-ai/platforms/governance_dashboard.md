# Centralized Documentation Governance Dashboard Guide

**Module:** `scribex-ai`  
**Platform:** Self-Hosted Daemon (`scribex serve`) & Enterprise Multi-Repo Cockpit  
**Audience:** Site Reliability Engineers (SRE), Platform Leads, Engineering Managers, DevSecOps Leads  
**Scope:** Multi-repository registry, SHA-256 RBAC authentication, TLS termination, reverse proxy setup, and systemd service management.

---

## 1. Overview: The Multi-Repo Governance Challenge

In enterprise organizations operating microservice architectures or distributed polyglot codebases, documentation drift is not isolated to a single repository. Different squads maintain dozens or hundreds of repositories with varying levels of documentation health.

The **Scribex Governance Daemon** (`scribex serve`) provides an enterprise-wide control plane:

```
┌────────────────────────────────────────────────────────┐
│               Scribex Governance Daemon                │
│                 (scribex serve :8080)                  │
├──────────────────────────┬─────────────────────────────┤
│      Web Dashboard       │         REST API            │
│  - Team-Scoped Repos     │  - GET /api/v1/health       │
│  - Stale Anchor Count    │  - GET /api/v1/repos        │
│  - Dead Symbol Alerts    │  - GET /api/v1/trace.csv    │
│  - Read-Only /config     │  - GET /api/v1/metrics      │
├──────────────────────────┴─────────────────────────────┤
│                RBAC Authentication Engine              │
│      HTTP Basic Auth: User + SHA-256 Hashed Token      │
│      Roles: admin | viewer (Scoped by team ownership)  │
└──────────────────────────┬─────────────────────────────┘
                           │
       ┌───────────────────┼───────────────────┐
       ▼                   ▼                   ▼
  Repository A        Repository B        Repository C
   (Team: Core)        (Team: Auth)       (Team: Billing)
```

### Core Daemon Capabilities
1. **Multi-Repo Federation:** Aggregates documentation freshness metrics across hundreds of git repositories.
2. **Team-Scoped RBAC:** Team members only see documentation metrics for their assigned repositories; administrators have global visibility.
3. **In-Memory Measurement Caching:** Caches anchor scans and drift calculations with configurable TTL (`-cache-ttl 30s`) to prevent filesystem saturation.
4. **Hardened Configuration View:** Displays AI provider status and secret presence (`set` vs `unset`) without ever displaying secret strings or permitting writes over HTTP.

---

## 2. Configuring the Daemon Files

Running `scribex serve` requires two simple configuration files: the **Repos Registry** and the **RBAC Control File**.

### 2.1 The Repos Registry File (`/etc/scribex/repos.txt`)

Each line defines a tracked repository in the format:  
`name path team`

```text
# Repository Registry: name path team
auth-service /var/repos/auth-service auth
billing-api /var/repos/billing-api billing
core-engine /var/repos/core-engine core
gateway /var/repos/gateway core
mobile-app /var/repos/mobile-app mobile
```

### 2.2 Generating Cryptographic RBAC Tokens

Users authenticate via HTTP Basic Auth. The username can be arbitrary; the password is a pre-hashed token.

Generate a secure token hash using the CLI:

```bash
# Generate a random high-entropy token:
TOKEN=$(openssl rand -hex 24)
echo "Your plain token is: $TOKEN"

# Compute the SHA-256 hash:
scribex hash-token "$TOKEN"
# Output:
# e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

### 2.3 The RBAC Control File (`/etc/scribex/rbac.txt`)

Each line maps a token hash to a role and team permissions:  
`sha256(token) role team1,team2,...`

```text
# RBAC Access Control
# Format: sha256_hash role teams (comma-separated)

# Global Admin Token (sees all teams and repositories)
8f3b20c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1 admin *

# Auth Team Lead (sees only auth repositories)
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 viewer auth

# Core Platform Engineer (sees core and billing)
a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef0 viewer core,billing
```

---

## 3. Starting the Daemon

### 3.1 Local Testing Command

```bash
scribex serve \
  -repos /etc/scribex/repos.txt \
  -rbac /etc/scribex/rbac.txt \
  -allowed-root /var/repos \
  -addr 127.0.0.1:8080 \
  -cache-ttl 30s
```

Navigate to `http://127.0.0.1:8080` in your browser. Enter your username and plain token when prompted by HTTP Basic Auth.

---

## 4. Production Deployment: systemd Service

Create a dedicated systemd service to run Scribex as a persistent daemon:

```ini
# /etc/systemd/system/scribexd.service
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
RestartSec=5s

# Security Hardening Directives
ProtectSystem=strict
ProtectHome=true
NoNewPrivileges=true
ReadOnlyPaths=/var/repos
ReadWritePaths=/var/log/scribex

[Install]
WantedBy=multi-user.target
```

Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now scribexd
sudo systemctl status scribexd
```

---

## 5. Reverse Proxy Configuration (Caddy & Nginx)

In production, terminate TLS and manage external traffic using a hardened reverse proxy.

### 5.1 Option A: Caddy Server (`/etc/caddy/Caddyfile`)

```caddyfile
docs-health.internal.corp {
    reverse_proxy 127.0.0.1:8080 {
        header_up X-Forwarded-Proto {scheme}
        header_up X-Forwarded-For {remote_host}
    }
}
```

### 5.2 Option B: Nginx (`/etc/nginx/sites-available/scribex.conf`)

```nginx
server {
    listen 443 ssl http2;
    server_name docs-health.internal.corp;

    ssl_certificate /etc/ssl/certs/scribex.crt;
    ssl_certificate_key /etc/ssl/private/scribex.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 6. REST API Reference for Monitoring & Dashboards

The daemon exposes JSON endpoints for integration with monitoring systems (Datadog, Prometheus, Grafana):

### `GET /api/v1/health`
Liveness probe.
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 84200
}
```

### `GET /api/v1/repos`
Returns team-scoped documentation health metrics:
```json
[
  {
    "repo": "auth-service",
    "team": "auth",
    "anchors_total": 42,
    "anchors_stale": 0,
    "anchors_dead": 0,
    "last_scanned_utc": "2026-10-28T18:00:00Z"
  },
  {
    "repo": "billing-api",
    "team": "billing",
    "anchors_total": 18,
    "anchors_stale": 2,
    "anchors_dead": 1,
    "last_scanned_utc": "2026-10-28T18:00:00Z"
  }
]
```

---

## 7. Troubleshooting & Security FAQs

### Q: `scribex: path /var/repos/auth-service is outside allowed-root /srv/repos`
- **Cause:** A repository in `/etc/scribex/repos.txt` points outside the designated `-allowed-root` directory.
- **Fix:** Ensure all repositories reside within the directory tree passed to `-allowed-root`.

### Q: `scribex: unauthorized`
- **Cause:** Token provided in the HTTP Basic Auth password field does not match any hash in `/etc/scribex/rbac.txt`.
- **Fix:** Recompute the SHA-256 hash using `scribex hash-token <plain_token>` and update the RBAC file.
