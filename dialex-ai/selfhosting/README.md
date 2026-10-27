# Self-Hosting Dialex

Welcome to the official Dialex self-hosting handbook. Dialex is designed to be completely self-hosted, sovereign, and privacy-preserving.

This guide provides step-by-step instructions for hosting the **Dialex Engine Daemon** on your own server, VPS, Mac Mini, or Home Lab (TrueNAS, Unraid, Synology).

---

## Architecture Overview

```
                     +---------------------------------------+
                     |            DIALEX CLIENTS             |
                     |  Android Phone  •  iPhone  •  Laptop  |
                     +---------------------------------------+
                                         |
                                         | WireGuard / LAN / HTTPS
                                         v
+-----------------------------------------------------------------------------------+
| YOUR SERVER / VPS / NAS                                                           |
|                                                                                   |
|  +---------------------------+       +-----------------------------------------+  |
|  | Caddy / Nginx / Tailscale | ----> | Dialex Go Daemon (:7890)                |  |
|  | (Optional TLS/Proxy)      |       | - Web Admin Dashboard (//go:embed)      |  |
|  +---------------------------+       | - Multi-Agent Turn Loop & SSE Stream    |  |
|                                      | - AES-256-GCM Encrypted Key Vault       |  |
|                                      +-----------------------------------------+  |
|                                                           |                       |
|                                                           v                       |
|                                              [ Persistent Volume / Data ]         |
+-----------------------------------------------------------------------------------+
```

---

## Deployment Options

| Method | Best For | Prerequisites |
|---|---|---|
| **[Scenario C: Tailscale Mesh](#1-scenario-c-tailscale-private-mesh-recommended)** | Zero-config remote access from mobile/laptop anywhere in the world | Tailscale account (free) |
| **[Standard Docker Compose](#2-standard-docker-compose-local-lan)** | Home servers, Unraid, Synology, TrueNAS | Docker & Docker Compose |
| **[Native Systemd Service](#3-bare-metal--linux-vps-systemd)** | Dedicated Linux VPS (Debian, Ubuntu, Arch) | Go binary or compiled release |
| **[Caddy / Nginx Reverse Proxy](#4-domain--https-reverse-proxy)** | Public domain name with SSL | Domain name pointing to server |

---

## 1. Scenario C: Tailscale Private Mesh (Recommended)

This is the cleanest and most secure method. Dialex joins your private Tailnet directly via a sidecar container.
- **Zero port forwarding** on your router.
- **End-to-end WireGuard encryption** between your phone and the server.
- Seamlessly accessible over cellular or home Wi-Fi.

### Step 1: Generate an Auth Key
1. Go to your [Tailscale Admin Console → Keys](https://login.tailscale.com/admin/settings/keys).
2. Generate an **Auth Key** (Ephemeral or Reusable).

### Step 2: Configure and Run
```bash
cd selfhosting
cp .env.example .env

# Set your auth key in .env
echo "TS_AUTHKEY=tskey-auth-your-key-here" >> .env

# Launch the Tailscale mesh stack
docker compose -f docker-compose.tailscale.yml up -d
```

### Step 3: Access
From your phone or laptop connected to Tailscale:
* **Web Dashboard**: `http://dialex-engine.your-tailnet.ts.net:7890`
* **Mobile App Connection**: Enter `http://dialex-engine.your-tailnet.ts.net:7890` or scan the QR code from the Web Dashboard.

---

## 2. Standard Docker Compose (Local LAN)

If you just want to run Dialex on your local home network (e.g. at `192.168.1.50:7890`):

```bash
cd selfhosting
cp .env.example .env

# Start container
docker compose up -d

# View live logs
docker compose logs -f
```

Open `http://<your-server-ip>:7890` in your web browser to access the Admin Dashboard.

---

## 3. Bare Metal / Linux VPS (Systemd)

To run Dialex as a supervised system daemon:

```bash
# 1. Download or build the dialex binary
cd socratix-engine
go build -o /usr/local/bin/socratix ./cmd/socratix

# 2. Create service user and data directory
sudo useradd -r -s /bin/false dialex
sudo mkdir -p /var/lib/dialex
sudo chown -R dialex:dialex /var/lib/dialex
sudo chmod 700 /var/lib/dialex

# 3. Install and start systemd service
sudo cp selfhosting/systemd/dialex.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dialex

# 4. Check status
sudo systemctl status dialex
```

---

## 4. Domain & HTTPS (Reverse Proxy)

If exposing Dialex on a public domain (e.g. `https://debate.yourdomain.com`):

### Using Caddy (Recommended)
Caddy automatically provisions Let's Encrypt certificates and correctly handles SSE streaming:
```bash
sudo cp selfhosting/reverse-proxy/Caddyfile /etc/caddy/Caddyfile
# Edit your domain name in /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

### Using Nginx
Ensure `proxy_buffering off;` is set (see `selfhosting/reverse-proxy/nginx.conf`) so Server-Sent Events stream without delay.

---

## Web Admin Dashboard

The server embeds an administrative portal accessible via your browser at `http://<server-ip>:7890`:
- **Overview & Telemetry**: Monitor server uptime, active debates, and Go memory.
- **Device Pairing**: Displays a **One-Time QR Code** that pairs the Dialex Mobile App in 2 seconds.
- **Credentials**: Change admin password and manage accounts.
- **API Keys**: Add and update Anthropic, OpenAI, Gemini, Grok, DeepSeek, and Mistral keys.
- **Live Logs**: Watch real-time execution logs.

---

## Backups & Maintenance

### Creating a Backup
```bash
./selfhosting/scripts/backup.sh
```
Creates an encrypted tarball of your discussion history, accounts, and AES keys in `./backups/`.

### Restoring a Backup
```bash
./selfhosting/scripts/restore.sh ./backups/dialex_backup_YYYYMMDD.tar.gz
```

---

## 📄 License

Dialex AI is licensed under the [PolyForm Noncommercial License 1.0.0](../LICENSE) (dual-licensed: see [COMMERCIAL_LICENSE.md](../../COMMERCIAL_LICENSE.md)).  
Copyright (c) 2026 Keltstone Systems. Free for personal, academic, and noncommercial research. Commercial deployments require an enterprise license from Keltstone Systems.

---

## Security Model & Data Retention

**What the code does today**
- The engine binds to `127.0.0.1:7890` by default. Exposing it (`--host 0.0.0.0:...`, Docker, Tailscale) is your decision and your exposure.
- Data routes require authentication; passwords are hashed with bcrypt; tokens are signed, short-lived JWTs.
- **Roles:** `admin` and `user`. Only admins can list or create users, read logs, get the pairing code, or export the audit log. The first user and any account created before roles existed are admins; users created through the API default to `user`.
- **Audit log:** `audit.log` (JSON lines, `0600`) in the data directory records logins (ok, failed, blocked) and every state-changing request with user, path, status and IP. Admins export it with `GET /api/v1/admin/audit?limit=N`.
- **Login lockout:** 5 failures per username or 20 per IP in 15 minutes returns `429`.
- **SSO through a trusted proxy** (oauth2-proxy, Tailscale serve, Cloudflare Access, Authelia): set `DIALEX_SSO_USER_HEADER` (e.g. `X-Forwarded-Email`) and `DIALEX_SSO_TRUSTED_CIDR` (the proxy's address); optionally `DIALEX_SSO_ADMINS` (comma-separated identities). The header is believed only from that CIDR, and the proxy must be the only route to the engine. New identities are created as passwordless users. This is not native OIDC.
- Provider API keys are encrypted at rest with AES-256-GCM. The key is held in the OS keychain (macOS, Linux with `secret-tool`). On a headless server without one (typical Docker/VPS), it falls back to a `0600` file in the same data directory, so anyone who can read the whole directory can decrypt the keys.
- Transcripts, projects, personas and the knowledge graph are stored unencrypted in the data directory (SQLite/JSON). Use disk encryption if that matters to you.

**What it does not do**
- No native OIDC/SAML, no fine-grained permissions beyond admin/user, no SOC 2 or other certification.
- The audit log has no rotation and no tamper evidence; ship it to a log collector if you need that.
- `/api/v1/admin/stats`, `/api/v1/cli/status` and `/api/v1/cli/logins` are unauthenticated (counts and local CLI status, no content).
- Login lockout is in memory and per process; it resets on restart.
- No verified multi-tenant isolation: treat one deployment as one trust domain and assume every user on an instance can reach the same data.
- No independent security audit or penetration test.

**Recommended deployment**: Tailscale or another private network, or a reverse proxy (Caddy/nginx) with TLS. Never expose the raw port to the internet.

**Data retention**: Keltstone Systems holds none of your data. On your instance, data stays until you delete it in the app or remove the data directory; deleting a discussion removes it from the local store. Content sent to a model provider is retained under that provider's policy (see the privacy policy).
