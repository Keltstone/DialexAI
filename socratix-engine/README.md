# Socratix Engine

**High-Performance Multi-Agent Consensus, Credence Tracking & Socratic Deliberation Daemon**

Socratix is the core deliberation and consensus engine powering the DialexAI Ecosystem. Operating locally as a lightweight, zero-latency Go daemon (`socratix`) on port 7890, it orchestrates structured dialectic debates between competing frontier AI models, calculates mathematical consensus convergence, tracks Bayesian credence shifts across debate rounds, and manages encrypted credential vaults on developer workstations.

---

## 🚀 Release Details (v1.0.0 — Engine GA)

- **Current Version:** `1.0.0`
- **Release Changelog:** [CHANGELOG.md](CHANGELOG.md)
- **License:** Apache-2.0
- **Supported Platforms:** macOS (`darwin/arm64`, `darwin/amd64`), Linux (`linux/amd64`, `linux/arm64`), Windows (`windows/amd64`)
- **Quick Install via Homebrew:**
  ```bash
  brew install keltstone/tap/socratix
  ```
- **Universal One-Line Install:**
  ```bash
  curl -fsSL https://raw.githubusercontent.com/Keltstone/DialexAI/main/scripts/install.sh | bash -s -- --engine-only
  ```

---

## ⚡ Quick Start

### 1. Run the Deliberation Daemon
```bash
# Start local SSE daemon on 127.0.0.1:7890
socratix serve

# Or run with HTTPS / custom port:
socratix serve --port 8787 --allow-insecure-lan
```

### 2. One-Shot Quick Council (`socratix ask`)
Run an instant multi-persona deliberation against your plan or architectural dilemma without launching a GUI:
```bash
# Red-team an architectural migration:
socratix ask --mode red-team "Migrate customer database from MySQL to DynamoDB"

# Pre-mortem risk analysis:
socratix ask --mode pre-mortem "Release v1 without end-to-end load testing"
```

### 3. Background Login Service
```bash
# Register engine as persistent user service (launchd on macOS, systemd on Linux)
socratix service install

# Check status:
socratix service status
```

### 4. Convergence & Model Benchmarking
```bash
# Benchmark consensus convergence across models:
socratixbench --matrix default --rounds 3
```

---

## 🏗️ Architecture

- **Protocol**: Localhost HTTP/2 REST & Server-Sent Events (SSE) Stream.
- **Persistence**: Embedded SQLite deliberation graph (`~/.dialex/deliberation.db` or custom `--dir`).
- **Cryptographic Vault**: AES-256-GCM encrypted API key storage.
- **Model Integrations**: Anthropic Claude, OpenAI GPT, Google Gemini, xAI Grok, DeepSeek, Mistral, and local Ollama (`ollama run llama3.3`).
