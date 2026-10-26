# Hardened Dashboard (`scribex serve`)

The Scribex dashboard provides cross-repository documentation health visibility and administrative configuration oversight with team-scoped Role-Based Access Control (RBAC).

---

## Threat Model

The dashboard is designed to defend against:
1. **Unauthorized Access & Brute-Force Attacks**: Credentials are submitted exclusively as HTTP Basic authentication. Tokens are hashed via SHA-256 and evaluated using constant-work comparisons (`subtle.ConstantTimeCompare`) over all configured principals to prevent timing side channels.
2. **Denial of Service (DoS) & Resource Exhaustion**:
   - **IP-Level Failure Rate Limiter**: 5 consecutive authentication failures within 60 seconds triggers an immediate 15-minute IP lockout (`HTTP 429 Too Many Requests` with `Retry-After`). The limiter state is stored in an in-memory LRU cache bounded to 10,000 IPs and evaluates before cryptographic hashing.
   - **Cache & Singleflight**: Documentation health measurements are cached for a configurable TTL (default 30 seconds). Concurrent requests for the same repository coalesce via singleflight, executing only a single measurement pass.
   - **Concurrency Semaphore**: A global concurrency semaphore caps simultaneous repository scans to 4. Excessive requests queue for up to 5 seconds before failing cleanly with `503 Service Unavailable` (`Retry-After: 5`).
3. **Cross-Site Scripting (XSS) & Clickjacking**:
   - Strict `Content-Security-Policy: default-src 'none'; style-src 'sha256-...'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`.
   - `X-Content-Type-Options: nosniff`.
   - `X-Frame-Options: DENY`.
   - `Referrer-Policy: no-referrer`.
   - Dynamic user and repository strings are HTML-entity escaped in web views.
4. **Path Traversal & Symlink Attacks**:
   - Every repository path must resolve within the mandatory `-allowed-root` directory.
   - Symlinks pointing outside the allowed root or swapped after server startup are detected and refused, reporting `unavailable` rather than leaking files.
   - Absolute filesystem paths are never revealed in HTTP responses, templates, or error messages.

---

## Command Flags

```bash
scribex serve -repos <repos.txt> -rbac <rbac.txt> -allowed-root <dir> [options]
```

| Flag | Default | Description |
|---|---|---|
| `-repos <file>` | *(required)* | Path to repositories definition file |
| `-rbac <file>` | *(required)* | Path to RBAC definitions file |
| `-allowed-root <dir>` | *(required)* | Mandatory root directory confining all repositories |
| `-addr <host:port>` | `127.0.0.1:8080` | Bind address. Non-loopback bindings require TLS or `-behind-proxy` |
| `-cache-ttl <dur>` | `30s` | Documentation measurement cache duration |
| `-tls-cert <file>` | `""` | Path to TLS X.509 certificate |
| `-tls-key <file>` | `""` | Path to TLS private key |
| `-behind-proxy` | `false` | Enables reverse-proxy mode (sends HSTS; honors XFF from trusted proxies) |
| `-trusted-proxy <cidr>`| `""` | Comma-separated list of trusted proxy CIDRs (e.g. `10.0.0.0/8,172.16.0.0/12`) |

> [!IMPORTANT]
> Binding to a non-loopback interface (e.g. `0.0.0.0:8080` or a public IP) without either `-tls-cert`/`-tls-key` or `-behind-proxy` will cause the server to immediately refuse to start.

---

## File Formats

### 1. RBAC File (`rbac.txt`)

Lines contain 2 or 3 whitespace-separated fields:
```
<sha256_token_hex> <admin|viewer> [team1,team2,...]
```

- **`sha256_token_hex`**: Exactly 64 hexadecimal characters representing the SHA-256 hash of the bearer token.
- **`role`**: Strictly `admin` or `viewer`.
- **`teams`**: Comma-separated list of team identifiers matching `^[A-Za-z0-9_.-]{1,64}$`.

#### Generating Hashes:
```bash
scribex hash-token "my-super-secret-admin-token"
# Output:
# 3b0c... (64 hex characters)
```

Example `rbac.txt`:
```
# Admin with access to all repos and /config
9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 admin

# Team Core viewer
5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8 viewer core

# Multi-team viewer
4b227777d4dd1fc61c6f884f48641d02b4d121d3fd328cb08b5531fcacdabf8a viewer core,payments
```

### 2. Repos File (`repos.txt`)

Lines contain exactly 3 whitespace-separated fields:
```
<repo_name> <relative_or_absolute_path> <team_name>
```

- **`repo_name`**: Identifier matching `^[A-Za-z0-9_.-]{1,64}$`.
- **`path`**: Filesystem path. Must resolve (`filepath.EvalSymlinks`) within `-allowed-root`.
- **`team_name`**: Team identifier matching `^[A-Za-z0-9_.-]{1,64}$`.

Example `repos.txt`:
```
auth-service /var/repos/auth core
checkout-api /var/repos/checkout payments
frontend-web /var/repos/web frontend
```

---

## Production Deployment: Reverse Proxy (Nginx)

When deploying behind a TLS-terminating reverse proxy, specify `-behind-proxy` and declare the proxy's IP in `-trusted-proxy`:

```bash
scribex serve \
  -repos /etc/scribex/repos.txt \
  -rbac /etc/scribex/rbac.txt \
  -allowed-root /var/repos \
  -addr 127.0.0.1:8080 \
  -behind-proxy \
  -trusted-proxy 127.0.0.1/32
```

Example Nginx configuration:
```nginx
server {
    listen 443 ssl http2;
    server_name docs.internal.example.com;

    ssl_certificate /etc/ssl/certs/dashboard.crt;
    ssl_certificate_key /etc/ssl/private/dashboard.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
        proxy_send_timeout 60s;
    }
}
```

---

## Logging & Retention

Access logs are emitted to `stderr` formatted as structured JSON lines:

```json
{"ts":"2026-10-27T12:00:00Z","remote":"198.51.100.2","method":"GET","path":"/api/health","status":200,"dur_ms":14,"principal":"9f86d081"}
```

- **Logged Fields**: Timestamp, client IP, HTTP method, URL path (query strings stripped), HTTP status code, duration in milliseconds, first 8 characters of principal token hash.
- **Never Logged**: Authorization headers, plain tokens, query strings, or filesystem paths.
- **Retention**: The server maintains no persistent log files or database state on disk. Standard container or systemd log rotation should manage stderr capture.
