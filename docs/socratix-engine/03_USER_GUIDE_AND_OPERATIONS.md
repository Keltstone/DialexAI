# Socratix Engine: User Guide & Operations Manual

**Engine:** `socratix-engine` (Go 1.27)  
**Author:** Kolta Labs Operations & Site Reliability Engineering  
**Audience:** Platform Engineers, System Administrators, Power Users  
**Scope:** Installation, daemon configuration, CLI flags, Socratic interactive sessions, benchmarking, and troubleshooting.

---

## 1. Installation & Binary Compilation

### 1.1 Prerequisites
- **Go 1.27+** installed.
- SQLite development libraries (optional; pure Go SQLite driver is bundled).
- Git 2.30+.

### 1.2 Building the Binaries
From the repository root:

```bash
cd socratix-engine

# Build the core daemon
go build -ldflags="-s -w" -o bin/socratix ./cmd/socratix

# Build the benchmark evaluator
go build -ldflags="-s -w" -o bin/socratixbench ./cmd/socratixbench

# Build the terminal roundtable REPL
go build -ldflags="-s -w" -o bin/roundtable ./cmd/roundtable

# Verify build
./bin/socratix --version
```

---

## 2. Launching the Core Daemon (`socratix`)

The daemon exposes a REST and Server-Sent Events (SSE) API on port 8080 by default.

### 2.1 Basic Local Mode
```bash
./bin/socratix --addr "127.0.0.1:8080" --db "$HOME/.dialex/socratix.db"
```

### 2.2 Complete Flag Reference

| Flag | Default Value | Environment Variable | Description |
|---|---|---|---|
| `--addr` | `127.0.0.1:8080` | `SOCRATIX_ADDR` | TCP address and port for the HTTP server. |
| `--db` | `~/.dialex/socratix.db` | `SOCRATIX_DB` | Path to the SQLite WAL database file. |
| `--cors` | `*` | `SOCRATIX_CORS` | Allowed CORS origins for browser/desktop clients. |
| `--auth-token` | `""` | `SOCRATIX_AUTH_TOKEN` | Optional static bearer token for API authorization. |
| `--tsnet` | `false` | `SOCRATIX_TSNET` | Enable Tailscale zero-trust WireGuard mesh listener. |
| `--tsnet-hostname` | `socratix` | `SOCRATIX_TSNET_HOSTNAME` | Hostname on the user's Tailscale Tailnet. |
| `--tsnet-auth-key` | `""` | `TS_AUTHKEY` | Tailscale pre-authorized node enrollment key. |
| `--tsnet-dir` | `/var/lib/socratix` | `SOCRATIX_TSNET_DIR` | Tailscale state directory for node credentials. |
| `--log-level` | `info` | `SOCRATIX_LOG_LEVEL` | Log verbosity: `debug`, `info`, `warn`, `error`. |

### 2.3 Running as a Background Systemd Service (Linux)
Create `/etc/systemd/system/socratix.service`:

```ini
[Unit]
Description=Socratix Deliberation Daemon
After=network.target

[Service]
Type=simple
User=dialex
WorkingDirectory=/var/lib/dialex
ExecStart=/usr/local/bin/socratix --addr 0.0.0.0:8080 --db /var/lib/dialex/socratix.db
Restart=always
RestartSec=5
LimitNOFILE=65536
Environment="SOCRATIX_LOG_LEVEL=info"

[Install]
WantedBy=multi-user.target
```

Reload and enable:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now socratix
```

---

## 3. Interactive Terminal Roundtable (`roundtable`)

For developers who want immediate terminal deliberations without starting a graphical UI:

```bash
cd socratix-engine
./bin/roundtable \
  --topic "Should we migrate from PostgreSQL to ScyllaDB for telemetry ingestion?" \
  --primary "ANTHROPIC:claude-3-5-sonnet-20241022" \
  --secondary "OPENAI:gpt-4o" \
  --tertiary "GEMINI:gemini-2.0-flash" \
  --rounds 3
```

During execution, ANSI-colored streams render each participant's turn in real time. The user can press `Ctrl+C` to pause or inject steering directives on the fly.

---

## 4. Running Benchmarks (`socratixbench`)

The `socratixbench` utility runs the **Dialex-Bench-10** test suite to scientifically evaluate multi-model debate convergence against single-model baselines:

```bash
cd socratix-engine
./bin/socratixbench \
  --suite "Dialex-Bench-10" \
  --cases "all" \
  --runs-per-case 3 \
  --output "benchmark_results.json"
```

### Metrics Evaluated
1. **Convergence Ratio**: Percentage of rounds ending in paraconsistent consensus.
2. **Entropy Delta ($\Delta S$)**: Reduction in Shannon entropy from Round 0 prior to final round.
3. **Contradiction Resolution Rate**: Percentage of identified dialectic tensions resolved or agreed as trade-offs.
4. **Token Cost per Decision**: Total input, output, and cached tokens consumed per converged verdict.

---

## 5. Live Human Steering & Interventions

Socratix Engine allows the human user to act as the supreme arbiter:

1. **Pause / Resume**: The client can toggle `isStopped()`. The engine freezes state cleanly at the exact turn boundary; no partial turns are committed.
2. **Human Turn Injection (`getInjected()`)**: A user can insert an authoritative remark or constraint into the active debate context:
   ```json
   {
     "seatId": "human_operator",
     "authorDisplayName": "Lead Architect (Human)",
     "content": "Notice: We have a strict $2,000/month AWS budget cap. Solutions requiring 5 multi-region nodes are disqualified.",
     "isUserComment": true
   }
   ```
   All participants in subsequent rounds must acknowledge and adapt to this constraint.
3. **Emergency Wrap-Up**: If time runs out, the user can trigger immediate moderator synthesis based on existing rounds.

---

*Next: Continue to [Socratix Engine: Developer Guide & API Reference](../socratix-engine/04_DEVELOPER_GUIDE_AND_API_REFERENCE.md).*
