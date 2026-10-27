# Dialex AI Go Orchestration Engine — Server, Self-Hosting & Deployment Guide

This guide covers building, configuring, deploying, and maintaining the standalone **Dialex AI Go Orchestration Engine** (`socratix-engine/`). The engine coordinates multi-agent turns, evaluates consensus, manages local CLI subprocesses, persists discussions atomically, and streams real-time Server-Sent Events (SSE) to connected desktop, mobile, and web clients.

---

## 1. Deployment Topology: In-Device Engine vs Remote Host Engine

Dialex AI supports two complementary engine execution models:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                 DIALEX AI DEPLOYMENT MODES                              │
├───────────────────────────────────────────┬────────────────────────────────────────────┤
│ 1. IN-DEVICE ENGINE (Default / Solo)       │ 2. REMOTE HOST ENGINE (Team / Homelab)     │
├───────────────────────────────────────────┼────────────────────────────────────────────┤
│ • Zero configuration required             │ • Single central server on Linux/Docker    │
│ • Desktop app automatically spawns local  │ • Multiple desktop & mobile clients share  │
│   engine process at http://127.0.0.1:8080 │   discussions and debate transcripts       │
│ • Shuts down cleanly when app exits       │ • Connect via LAN IP or Tailscale WireGuard│
│ • Ideal for individual developers/offline │ • 1-Tap QR Companion pairing               │
└───────────────────────────────────────────┴────────────────────────────────────────────┘
```

### Switching to a Remote Host Engine:
- In the Desktop App: Set `DIALEX_ENGINE_URL="http://192.168.1.100:8080"` or configure under **Settings &gt; Server Connection**.
- In Dialex AI Mobile: Tap the QR scanner on the welcome screen to scan the pairing QR code from the server's Web Admin (`http://<server-ip>:8080/admin`).

<div align="center">
  <img src="screenshots/01_engine_connection_setup.png" alt="Deployment Topology Connection Setup" width="85%" />
  <p><em>Figure 1.1: Engine Connection Setup allowing seamless switching between local solo engine and self-hosted remote clusters.</em></p>
</div>

---

## 2. Prerequisites for Self-Hosting

- **Go**: Version `1.22` or higher (compatible with Go `1.24+`).
- **Operating System**: Linux (amd64, arm64), macOS (Apple Silicon arm64, Intel x86_64), or Windows (amd64).
- **Optional Tools**:
  - `docker` and `docker compose` (for containerized deployments).
  - Local AI CLI binaries in system `$PATH` (`claude`, `codex`, `antigravity`/`agy`, `ollama`) if using local CLI agent runners.
  - Tailscale account (for `tsnet` zero-port WireGuard mesh self-hosting).

---

## 3. Compiling from Source

Navigate to the `socratix-engine/` directory and compile the binary:

```bash
cd socratix-engine

# Download and verify dependencies
go mod download
go mod verify

# Run test suite with race condition detection
go test -race -v ./pkg/...

# Build production stripped binary
go build -ldflags="-s -w" -o socratix ./cmd/socratix

# Verify binary
./socratix --version
```

---

## 4. CLI Command Reference

The `dialex` binary contains subcommands for server execution, user authentication management, and background service integration.

### 4.1 `dialex serve`
Starts the HTTP REST API, Server-Sent Events (SSE) broadcaster, and turn orchestration hub.

```bash
# Default: Binds to 127.0.0.1:8080 with data stored in ~/.dialex
./socratix serve

# Run with custom port and custom data directory
./socratix serve --port 9090 --data-dir /var/lib/dialex

# Allow connections from local network (LAN)
./socratix serve --host 0.0.0.0 --allow-insecure-lan

# Enable TLS with existing certificates
./socratix serve --tls --cert /path/to/cert.pem --key /path/to/key.pem
```

#### Flags Reference:
| Flag | Environment Variable | Default | Description |
|---|---|---|---|
| `--host` | `DIALEX_HOST` | `127.0.0.1` | Network interface to bind to |
| `--port` | `DIALEX_PORT` | `8080` | Port for the HTTP and SSE server |
| `--data-dir` | `DIALEX_DATA_DIR` | `~/.dialex` | Path where discussions, keystore, and logs are persisted |
| `--allow-insecure-lan` | `DIALEX_ALLOW_LAN` | `false` | Permits non-localhost client connections |
| `--tls` | `DIALEX_ENABLE_TLS` | `false` | Enables HTTPS termination |
| `--cert` | `DIALEX_TLS_CERT` | `""` | Path to TLS certificate |
| `--key` | `DIALEX_TLS_KEY` | `""` | Path to TLS private key |
| `--verbose` | `DIALEX_VERBOSE` | `false` | Enables detailed debug and audit logging |

---

### 4.2 User & Account Management (`dialex users`)
# Reset a user's password
./socratix users reset-password admin

# List active users
./socratix users list

# Delete a user
./socratix users remove username
```

---

### 4.3 Operating System Background Service (`dialex service`)
The engine includes native integration with Linux `systemd` and macOS `launchd` to run automatically on boot.

```bash
# Install and register the background service
sudo ./socratix service install --data-dir /var/lib/dialex

# Start the background service
sudo ./socratix service start

# Check service status
sudo ./socratix service status

# Uninstall the service
sudo ./socratix service uninstall
```

---

## 5. Containerized Deployment (Docker & Compose)

The `selfhosting/` directory includes production-grade container specifications with persistent SQLite data volumes:

```bash
cd selfhosting
docker compose up -d
```

### `docker-compose.yml`:
```yaml
version: '3.8'

services:
  dialex-engine:
    image: ghcr.io/dialex/engine:latest
    container_name: dialex-engine
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - DIALEX_DATA_DIR=/data
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - GEMINI_API_KEY=${GEMINI_API_KEY}
    volumes:
      - dialex_data:/data

volumes:
  dialex_data:
```

---

## 6. Private Tailscale Mesh Stack (`tsnet`)

Deploy Dialex AI privately across all your devices with **zero open router ports** using embedded Tailscale:

```bash
cd selfhosting
TS_AUTHKEY="tskey-auth-kXXXXX-XXXXXX" docker compose -f docker-compose.tailscale.yml up -d
```

The engine will join your Tailnet as `http://dialex:8080`, protected by mutual WireGuard authentication.

<div align="center">
  <img src="screenshots/05_mobile_qr_pairing_modal.png" alt="Mobile Tailscale QR Pairing" width="75%" />
  <p><em>Figure 6.1: Pairing mobile clients directly with Tailscale tsnet instances using 1-tap QR codes.</em></p>
</div>

---

## 7. Reverse Proxy Configuration (Nginx & Caddy)

When exposing Dialex AI behind a reverse proxy, **Server-Sent Events (SSE) buffering must be disabled**. Otherwise, agent streaming responses will be buffered and delivered in jarring lumps rather than real-time streams.

### Nginx Configuration:
```nginx
server {
    listen 443 ssl http2;
    server_name dialex.example.com;

    ssl_certificate /etc/letsencrypt/live/dialex.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/dialex.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # CRITICAL: Disable proxy buffering for real-time SSE streaming
        proxy_buffering off;
        proxy_cache off;
        proxy_set_header Connection '';
        chunked_transfer_encoding off;

        # Keep alive long-running deliberation turns (up to 15 minutes)
        proxy_read_timeout 900s;
        proxy_send_timeout 900s;
    }
}
```

---

## 8. Health Checks & Diagnostics

### `GET /api/health`
Returns system status, active deliberations count, and memory consumption:
```json
{
  "status": "healthy",
  "version": "1.2.0",
  "active_discussions": 2,
  "uptime_seconds": 184920,
  "memory_alloc_mb": 42.6
}
```

### `GET /admin`
The embedded Web Admin dashboard provides an in-browser interface to inspect active debates, review token consumption across models, manage API keys, and pair mobile devices via QR code.

---

## 📄 License

Dialex AI is licensed under the [PolyForm Noncommercial License 1.0.0](file:///LICENSE).  
Copyright (c) 2026 Keltstone Systems. Free for personal, academic, and noncommercial research. Commercial deployments require an enterprise license from Keltstone Systems.
