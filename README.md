# Dialex Suite

**By Kolta Labs.** Tools that make AI disagree with itself before you trust it.

Dialex Suite is four products on one deliberation engine:

| Product | Folder | What it is | Stack |
|---|---|---|---|
| **Dialex AI** | [`dialex-ai/`](./dialex-ai) | Desktop and Android app where several AI models debate a decision and a moderator synthesizes the result. | Kotlin Multiplatform, Compose |
| **Artix AI** | [`artix-ai/`](./artix-ai) | Autonomous engineering platform & spec-to-patch workflow for AI-written code (CLI and desktop cockpit). | Go, Compose Desktop |
| **Kritix AI** | [`kritix-ai/`](./kritix-ai) | Autonomous end-to-end testing platform with DOM pruning, self-healing locators, and DAST security audits. | Go, Compose Desktop |
| **Socratix Engine** | [`socratix-engine/`](./socratix-engine) | The shared deliberation daemon: consensus, orchestration, mesh networking, model runners. | Go 1.27 |

👉 **[Explore the Complete Technical Wiki Documentation Suite](docs/README.md)** — In-depth architectural guides, user manuals, and developer documentation for every engine.

---

## Architecture & Directory Layout

```text
DialexAI/
├── socratix-engine/       # Core Go Engine & Orchestration Daemon
│   ├── cmd/               # Daemon binaries (socratix, socratixbench)
│   ├── pkg/               # Engine packages (consensus, orchestrator, socratic, etc.)
│   └── go.mod             # Go module: socratix
│
├── dialex-ai/             # Kotlin Multiplatform Client Application
│   ├── shared/            # Common business logic, MVI architecture & UI components
│   ├── desktopApp/        # Compose Multiplatform Desktop client (macOS/Win/Linux)
│   ├── androidApp/        # Compose Android mobile client
│   └── build.gradle.kts   # Independent Gradle build & wrapper
│
├── artix-ai/              # Artix Autonomous Engineering Platform
│   ├── cli/               # artix CLI frontend (spec, code, review, steering)
│   ├── cmd/               # artixd remote server daemon & webhooks
│   ├── pkg/               # Core packages (coder, reviewer, spec, forge, steering)
│   ├── app/               # Standalone Compose Desktop Cockpit application
│   └── build.gradle.kts   # Independent Gradle build & wrapper
│
├── kritix-ai/             # Kritix Autonomous QA & Testing Studio
│   ├── cmd/               # kritix CLI binary
│   └── pkg/               # DOM pruner, vision driver, OWASP DAST, quarantine
│
├── sentinix-ai/           # Sentinix Security Review & Vulnerability Posture
│   ├── cmd/               # sentinix (CLI) & sentinixd (review server daemon)
│   ├── pkg/               # rule packs, secrets scanner, advisories, AI council
│   └── deploy/            # Dockerfile, docker-compose, Helm chart, CI templates
│
├── go.work                # Go Workspace uniting Go engines
└── .github/workflows/     # Unified CI/CD pipelines
```

---

## Quick Install & Build

See the full [**Installation & Packaging Guide**](docs/INSTALLATION_AND_PACKAGING.md) for complete details.

### 1. Homebrew (macOS & Linux)

Install via shorthand tap (auto-taps on install):
```bash
brew install kolta-labs/tap/sentinix # Sentinix security review & PR gate
brew install kolta-labs/tap/artix    # Artix coding CLI
brew install kolta-labs/tap/kritix   # Kritix QA studio CLI
brew install kolta-labs/tap/socratix # Deliberation engine
```

Alternatively, tap manually:
```bash
brew tap koltalabs/dialexai https://github.com/Kolta-Labs/DialexAI

# CLI Binaries
brew install sentinix            # Continuous security review & PR gate
brew install socratix            # Deliberation engine
brew install artix --with-engine # Artix coding CLI
brew install kritix              # Kritix QA studio CLI

# Desktop Apps (macOS)
brew install --cask dialex-ai-bundled # Dialex AI with bundled engine
brew install --cask dialex-ai         # Dialex AI client-only (without engine)
brew install --cask artix-ai          # Artix AI Cockpit
brew install --cask kritix-ai         # Kritix AI Testing Studio
```

### 2. Universal One-Line Installer
```bash
# Automated installer (detects existing engine, asks express permission to update or skip)
curl -fsSL https://raw.githubusercontent.com/Kolta-Labs/DialexAI/main/scripts/install.sh | bash

# Or from clone:
./scripts/install.sh --all --with-engine
```

### 3. Build Everything from Source
```bash
# Build CLI binaries (socratix, artix, kritix):
make build

# Package Desktop apps (DMG / MSI / DEB) in both 'with-engine' and 'without-engine' variants:
./scripts/build-dist.sh --all
```

---

## Developer Quick Start

### 1. Socratix Engine (Go Daemon)
```bash
cd socratix-engine
go test ./pkg/...
go run ./cmd/socratix
```

### 2. Dialex AI (Compose Desktop & Android)
```bash
cd dialex-ai
# Run Desktop Application
./gradlew :desktopApp:run

# Compile Android Client
./gradlew :androidApp:assembleDebug
```

### 3. Artix AI (CLI & Desktop Cockpit)
```bash
cd artix-ai
# Build CLI binary
go build -o bin/artix ./cli/main.go
./bin/artix help

# Run Artix Cockpit Desktop App
./gradlew :app:run
```

### 4. Kritix AI (CLI & Testing Studio)
```bash
cd kritix-ai
# Build CLI binary
go build -o bin/kritix ./cmd/kritix/main.go
./bin/kritix help

# Run Kritix Studio Desktop App
./gradlew :app:run
```

---

## Status & Limitations

Run the tests yourself:

```bash
(cd socratix-engine && go test ./pkg/...)
(cd artix-ai        && go test ./pkg/...)
(cd dialex-ai       && ./gradlew :shared:allTests)   # includes real-engine integration tests
```

**Feature maturity**

| Tier | Features |
|---|---|
| **Core** | Council deliberation, moderator synthesis, live steering, deliverables (ADR, decision matrix, memo) |
| **Experimental** | Knowledge graph, problem decomposition, contradiction detection, dynamic retrieval, Socratic mode, benchmarking harness, persona DNA, mobile parity, Bayesian credence |
| **Planned** | Autonomous artifact sandbox |

**Known limitations**

| Area | Status |
|---|---|
| Debate vs single model | A benchmark harness and a 10-case set (`Dialex-Bench-10`) exist in `socratix-engine/pkg/benchmark`. **No results are published yet.** |
| Artix `plan` and review | `plan` is template-based and the reviewer is rule-based; neither calls a model. See [artix-ai/README.md](artix-ai/README.md). `artix code` writes patches with a model only when you pass `--provider` and `--model`. |
| Artix execution isolation | macOS: commands run under `sandbox-exec` with writes limited to the workspace, temp and tool caches, and network denied by default (tested). Linux: same via `bwrap` if installed (not yet tested in CI). Windows or no tool: process-group only. Reads are not restricted, so it is **not** a defence against a hostile repository. Each result reports its `isolation` level. |
| API key storage | AES-256-GCM; the key is kept in the OS keychain (macOS Keychain, Linux `secret-tool`) with a `0600` key-file fallback (Windows, or `DIALEX_KEY_STORAGE=file`). |
| Third-party CLI runners | Driving vendor CLIs (`claude`, `codex`, ...) depends on each vendor's terms and may break. Direct API keys or Ollama are the stable path. |

## Contributing

We want real contributors on scoped pieces of work (docs, a runner, a platform port, benchmark cases). See [CONTRIBUTING.md](CONTRIBUTING.md). Contributors are credited for what they build.

---

## Development & Engineering Standards

All modules adhere to the [Kolta KMP & Go Engineering Standards](https://github.com/Kolta-Labs/Kolt). Personas, agents, and tooling are governed by rules symlinked in `.standards/`.
