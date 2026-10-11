# DialexAI Ecosystem Changelog

All notable changes across the **DialexAI Suite** will be documented in this file.

The suite comprises four sovereign software systems:
1. **Dialex AI**: Sovereign Multi-Agent Deliberation Platform & Persona Studio (Compose Desktop, Android, & Server)
2. **Artix AI**: Autonomous Dialectic Software Engineering & Spec-to-Patch Cockpit (Compose Desktop & Go CLI/Daemon)
3. **Kritix AI**: Autonomous QA Testing Studio & Enterprise Verification Engine (Compose Desktop & Go CLI/Server)
4. **Socratix Engine**: Multi-Agent Consensus, Credence Tracking & Socratic Deliberation Daemon (Go CLI & Daemon)

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Conventional Commits](https://www.conventionalcommits.org/).

---

## [Unreleased]

### Added
- Cross-platform packaging automation for all 4 software products (macOS DMG, Windows MSI & EXE, Linux DEB & RPM, Android APK).
- Dual distribution packaging variants: **With Embedded Engine** (`*-with-engine`) and **Without Engine** (`*-without-engine`).
- Homebrew formulas and casks tap for Dialex AI (`dialex-ai`, `dialex-ai-bundled`), Artix AI (`artix`, `artix-ai`, `artix-ai-bundled`), Kritix AI (`kritix`, `kritix-ai`, `kritix-ai-bundled`), and Socratix Engine (`socratix`).
- Single source of truth versioning and synchronized suite bumper (`scripts/bump-version.sh`).

---

## [1.0.0] - 2026-10-11

### 🌟 Release Overview
Initial general release of the unified **DialexAI Suite**, establishing the sovereign multi-agent deliberation, autonomous engineering, and automated QA verification ecosystem.

All desktop applications are provided in two packaging variants:
- **With Engine (`*-with-engine`)**: Bundles the standalone Socratix deliberation daemon inside the application package. Spawns and manages engine lifecycle automatically out-of-the-box for offline and solo workstations.
- **Without Engine (`*-without-engine`)**: Lightweight client application configured to attach to an external Socratix engine on port 7890 or a central enterprise deliberation server.

---

### 1. Dialex AI (v1.0.0)
- **Hegelian Multi-Agent Adversarial Deliberation**:
  - Pits up to 6 competing frontier AI models (Claude, GPT, Gemini, Grok, DeepSeek, Mistral, Ollama) in multi-round structured dialectic debates.
  - Designated Moderator (Seat 1) who frames topic agendas, leads round-by-round syntheses, and generates consensus verdicts.
- **No Extra Per-Token Billing via Local CLIs or Ollama**:
  - Direct execution through local authenticated developer subshells (`claude`, `codex`, `antigravity`/`agy`, `ollama`) eliminating metered API bills.
- **Persona Registry & Style Modifiers**:
  - 10 Universal General Debate Archetypes (*Facilitator*, *Devil's Advocate*, *Optimist*, *Pragmatist*, *Contrarian*, *Expert*, *Risk Analyst*, *Ethicist*, *Historian*, *Futurist*).
  - 40+ Domain-Specific Personas spanning Distributed Systems, Cybersecurity, AI Infrastructure, FinTech, and Legal Compliance.
- **Generative Deliverables**:
  - One-tap synthesis: Architecture Decision Records (ADR-042), Weighted Decision Matrices, Executive Memorandums, and Code Diffs.
- **Multiplatform Clients**:
  - Compose Multiplatform Desktop (macOS DMG, Windows MSI/EXE, Linux DEB/RPM).
  - Android Mobile application with radial stadium arena and voice dilemma intake.

### 2. Artix AI (v1.0.0)
- **Autonomous Software Engineering & Spec-to-Patch Deliberation**:
  - Multi-agent Stakeholder Council exploring competing architectural paths and trade-offs before code modifications.
  - Interactive Engineering Lab with diff viewer, patch synthesizer, and AST verification.
- **CLI & Background Daemon**:
  - High-performance Go CLI (`artix`) and daemon (`artixd`) with `plan`, `steer`, `persona`, and `replay` workflows.
  - Integration with Git, Forge APIs, LSP servers, and Model Context Protocol (MCP) toolchains.
- **Packaging & Variants**:
  - macOS DMG (with-engine & without-engine), Windows MSI/EXE, Linux DEB/RPM.
  - Standalone Go CLI binaries for macOS (arm64, amd64), Linux (amd64, arm64), and Windows (amd64).
  - Homebrew tap: `artix` (formula with `--with-engine`), `artix-ai` (cask), `artix-ai-bundled` (cask).

### 3. Kritix AI (v1.0.0)
- **Autonomous QA Testing Studio & Cockpit**:
  - Visual driver and semantic DOM accessibility exploration.
  - Dynamic invariant checking and synthetic crash reproduction.
- **OWASP DAST & Security Auditing**:
  - Automated security fuzzing across SQL injection, Cross-Site Scripting (XSS), and PII leak detection boundaries.
  - Real-time token compression and ROI metrics calculator.
- **Enterprise Verification CLI**:
  - Standalone Go CLI (`kritix`) with headless test execution, model configuration, and truth audits.
- **Packaging & Variants**:
  - macOS DMG (with-engine & without-engine), Windows MSI/EXE, Linux DEB/RPM.
  - Standalone Go CLI binaries for macOS (arm64, amd64), Linux (amd64, arm64), and Windows (amd64).
  - Homebrew tap: `kritix` (formula with `--with-engine`), `kritix-ai` (cask), `kritix-ai-bundled` (cask).

### 4. Socratix Engine (v1.0.0)
- **Multi-Agent Consensus & Credence Engine**:
  - Standalone Go daemon (`socratix`) serving high-throughput SSE APIs on port 7890.
  - Mathematical consensus detection, tension resolution, and Bayesian credence tracking across turn state machines.
  - SQLite deliberation graph persistence and encrypted credential vault.
- **Benchmark Suite**:
  - `socratixbench` CLI tool for measuring convergence, stance independence, and rubric scoring across model matrices.
- **Packaging**:
  - Standalone distribution archives (`.tar.gz` / `.zip`) for macOS (arm64, amd64), Linux (amd64, arm64), and Windows (amd64).
  - Homebrew tap: `socratix` (formula).

---

### 📦 Release Distribution Matrix

| Software | Variant | macOS (`.dmg` / `.tar.gz`) | Windows (`.msi` / `.exe` / `.zip`) | Linux (`.deb` / `.rpm` / `.tar.gz`) | Homebrew Target |
|---|---|---|---|---|---|
| **Dialex AI** | With Engine | `DialexAI-1.0.0-with-engine.dmg` | `DialexAI-1.0.0-with-engine.msi`<br>`DialexAI-1.0.0-with-engine.exe` | `DialexAI-1.0.0-with-engine.deb`<br>`DialexAI-1.0.0-with-engine.rpm` | `brew install --cask dialex-ai-bundled` |
| **Dialex AI** | Without Engine | `DialexAI-1.0.0-without-engine.dmg` | `DialexAI-1.0.0-without-engine.msi`<br>`DialexAI-1.0.0-without-engine.exe` | `DialexAI-1.0.0-without-engine.deb`<br>`DialexAI-1.0.0-without-engine.rpm` | `brew install --cask dialex-ai` |
| **Artix AI** | With Engine | `ArtixAI-1.0.0-with-engine.dmg` | `ArtixAI-1.0.0-with-engine.msi`<br>`ArtixAI-1.0.0-with-engine.exe` | `ArtixAI-1.0.0-with-engine.deb`<br>`ArtixAI-1.0.0-with-engine.rpm` | `brew install --cask artix-ai-bundled` |
| **Artix AI** | Without Engine | `ArtixAI-1.0.0-without-engine.dmg` | `ArtixAI-1.0.0-without-engine.msi`<br>`ArtixAI-1.0.0-without-engine.exe` | `ArtixAI-1.0.0-without-engine.deb`<br>`ArtixAI-1.0.0-without-engine.rpm` | `brew install --cask artix-ai` |
| **Artix AI CLI** | Standalone CLI | `artix-v1.0.0-darwin-*.tar.gz` | `artix-v1.0.0-windows-amd64.zip` | `artix-v1.0.0-linux-*.tar.gz` | `brew install artix` (`--with-engine`) |
| **Kritix AI** | With Engine | `KritixAI-1.0.0-with-engine.dmg` | `KritixAI-1.0.0-with-engine.msi`<br>`KritixAI-1.0.0-with-engine.exe` | `KritixAI-1.0.0-with-engine.deb`<br>`KritixAI-1.0.0-with-engine.rpm` | `brew install --cask kritix-ai-bundled` |
| **Kritix AI** | Without Engine | `KritixAI-1.0.0-without-engine.dmg` | `KritixAI-1.0.0-without-engine.msi`<br>`KritixAI-1.0.0-without-engine.exe` | `KritixAI-1.0.0-without-engine.deb`<br>`KritixAI-1.0.0-without-engine.rpm` | `brew install --cask kritix-ai` |
| **Kritix AI CLI** | Standalone CLI | `kritix-v1.0.0-darwin-*.tar.gz` | `kritix-v1.0.0-windows-amd64.zip` | `kritix-v1.0.0-linux-*.tar.gz` | `brew install kritix` (`--with-engine`) |
| **Socratix Engine** | Daemon & Bench | `socratix-v1.0.0-darwin-*.tar.gz` | `socratix-v1.0.0-windows-amd64.zip` | `socratix-v1.0.0-linux-*.tar.gz` | `brew install socratix` |

---

[Unreleased]: https://github.com/Kolta-Labs/DialexAI/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/Kolta-Labs/DialexAI/releases/tag/v1.0.0
