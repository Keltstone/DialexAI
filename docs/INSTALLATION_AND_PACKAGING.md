# DialexAI Ecosystem: Installation, Packaging & Distribution Guide

Comprehensive guide for building and installing **Artix AI**, **Dialex AI**, and **Kritix AI** alongside or independent of the **Socratix Deliberation Engine**.

---

## 1. Overview of Ecosystem Components

| Component | Nature | Primary Role | Default Engine Mode |
|---|---|---|---|
| **Dialex AI** | Compose Desktop / Mobile App | Sovereign Multi-Agent Deliberation Platform & Persona Studio | Supports embedded Socratix or remote engine (port 7890) |
| **Artix AI** | CLI / Daemon + Desktop Cockpit | Autonomous Software Engineering & Spec-to-Patch Deliberation | Standalone CLI + optional Socratic Council integration |
| **Kritix AI** | CLI / Server + Desktop Studio | Autonomous QA Testing, Vision Driver & OWASP DAST Auditing | Headless studio (port 9090) + optional Socratic synthesis |
| **Sentinix AI** | CLI / Daemon + Web Dashboard | Continuous Security Review, PR Gate & Vulnerability Posture Engine | Standalone scanner + optional server / AI Council |
| **Socratix Engine** | Standalone Go Daemon & CLI | Multi-Agent Deliberation, Consensus Detection & SSE API | Core engine binary (`socratix`, `socratixbench`) |

---

## 2. Installation Channels

### Channel A: Homebrew Tap (macOS & Linux)

Install directly via the shorthand tap (Homebrew automatically taps on install):

```bash
# Install Sentinix Security Review CLI & Daemon
brew install kolta-labs/tap/sentinix

# Install Artix AI CLI
brew install kolta-labs/tap/artix

# Install Kritix AI CLI
brew install kolta-labs/tap/kritix

# Install Socratix Deliberation Engine
brew install kolta-labs/tap/socratix
```

Alternatively, tap the repository manually:

```bash
brew tap koltalabs/dialexai https://github.com/Kolta-Labs/DialexAI
brew install sentinix
```

#### 1. CLI Binaries & Engine Formulas
```bash
# Install Sentinix AI Security Review CLI & Server
brew install sentinix

# Install the Socratix Deliberation Engine
brew install socratix

# Install Artix AI CLI (without engine)
brew install artix

# Install Artix AI CLI with engine bundled
brew install artix --with-engine

# Install Kritix AI Testing CLI
brew install kritix
brew install kritix --with-engine
```

#### 2. Desktop GUI Applications (Casks)
```bash
# Dialex AI Desktop Application (Bundled with Socratix Engine)
brew install --cask dialex-ai-bundled

# Dialex AI Desktop Application (Lightweight Client - Without Engine)
brew install --cask dialex-ai

# Artix AI Engineering Cockpit (Bundled with Socratix Engine)
brew install --cask artix-ai-bundled

# Artix AI Engineering Cockpit (Lightweight Client - Without Engine)
brew install --cask artix-ai

# Kritix AI Autonomous Testing Studio (Bundled with Socratix Engine)
brew install --cask kritix-ai-bundled

# Kritix AI Autonomous Testing Studio (Lightweight Client - Without Engine)
brew install --cask kritix-ai
```

---

### Channel B: Universal Interactive Installer Script

Run the automated installer on macOS, Linux, or Windows (WSL/Git Bash):

```bash
# Interactive mode (prompts for products, CLI/Desktop, and engine variant):
./scripts/install.sh

# Or install directly via curl:
curl -fsSL https://raw.githubusercontent.com/Kolta-Labs/DialexAI/main/scripts/install.sh | bash
```

#### Script Options & Flags
| Flag | Description |
|---|---|
| `--all` | Install Artix, Dialex, Kritix, and Sentinix (default) |
| `--sentinix` | Install Sentinix AI only (Security Review CLI & Server) |
| `--artix` | Install Artix AI only |
| `--dialex` | Install Dialex AI only |
| `--kritix` | Install Kritix AI only |
| `--engine-only` | Install Socratix Deliberation Engine only |
| `--cli` | Install CLI binaries only (installs to `/usr/local/bin` or `~/.local/bin`) |
| `--desktop` | Install Desktop GUI applications only (`/Applications` on macOS) |
| `--both` | Install both CLI and Desktop applications |
| `--with-engine` | Request Socratix Engine to be included |
| `--without-engine` | Client-only installation (omits Socratix Engine) |
| `--update-engine` | Expressly grant permission to update existing engine without prompting |
| `--skip-engine` | Expressly skip updating existing engine without prompting |
| `--prefix <dir>` | Custom installation prefix for binaries (default: `/usr/local` or `~/.local`) |
| `--dry-run` | Preview actions without modifying system files |

---

### Channel C: Windows Installation (PowerShell)

For native Windows installations:

```powershell
# Run the PowerShell installer script:
.\scripts\install.ps1 -All -WithEngine

# Non-interactive CLI-only install:
.\scripts\install.ps1 -CliOnly -SkipEngine -NonInteractive
```

---

## 3. Engine Detection & Express Update Permission

> [!IMPORTANT]
> **Safety Invariant:** Whenever an installation is performed through any channel (Script, Homebrew, or Desktop installer), the installer checks whether the **Socratix Engine** is already present on the system.
> 
> If an engine is detected:
> 1. It compares the existing version against the incoming package version.
> 2. It **will NOT** silently overwrite or corrupt the existing engine.
> 3. It prompts for **express permission from the user** before making any modification:
>    - **User approves (`y`/`yes`):** The previous engine binary is backed up with a timestamp (e.g., `socratix.bak.1791722233`) and updated to the new version.
>    - **User declines (`n`/`no`):** The engine installation step is cleanly skipped, preserving the existing binary and user settings, and configuring Dialex / Artix / Kritix to use the existing engine.

### Automated CI / Script Usage:
- To grant express permission automatically: pass `--update-engine` (or `-UpdateEngine` on Windows).
- To preserve existing engine automatically: pass `--skip-engine` (or `-SkipEngine` on Windows).

---

## 4. "With Engine" vs. "Without Engine" Packages

Each desktop application is provided in two distinct native packaging variants:

| Variant | macOS (`.dmg`) | Windows (`.msi` / `.exe`) | Linux (`.deb` / `.rpm`) | Use Case |
|---|---|---|---|---|
| **With Engine** *(Standalone)* | `*AI-<version>-with-engine.dmg` | `*AI-<version>-with-engine.msi`<br>`*AI-<version>-with-engine.exe` | `*AI-<version>-with-engine.deb`<br>`*AI-<version>-with-engine.rpm` | Solo developers, offline air-gapped workstations, zero-config local runs. The app embeds and spawns `socratix` out of the box. |
| **Without Engine** *(Client)* | `*AI-<version>-without-engine.dmg` | `*AI-<version>-without-engine.msi`<br>`*AI-<version>-without-engine.exe` | `*AI-<version>-without-engine.deb`<br>`*AI-<version>-without-engine.rpm` | Corporate teams, fleet deployments, or users connecting to a centralized team deliberation server or Docker cluster. |

---

## 5. Version Logging & Synchronized Release Bumper

The repository maintains a centralized, single-source-of-truth version log across all 4 products in the master [`CHANGELOG.md`](../CHANGELOG.md).

To bump the version across all 4 products, Gradle properties, Homebrew formulas/casks, and changelogs:

```bash
# Preview changes without modifying files:
./scripts/bump-version.sh 1.1.0 --dry-run

# Execute synchronized version bump across all 4 products:
./scripts/bump-version.sh 1.1.0
```

---

## 6. Building from Source

All artifacts can be compiled from source with a single command.

### Prerequisites
- **Go 1.22+**
- **JDK 21+**
- **Gradle 8.10+** (managed via `./gradlew`)

### Build Commands

```bash
# Build host CLI & Engine binaries:
./scripts/build-dist.sh --cli

# Cross-compile standalone CLI & Engine archives for all platforms (macOS, Linux, Windows):
./scripts/build-dist.sh --cli --cross-compile

# Build Desktop Applications (DMG on macOS, MSI & EXE on Windows, DEB & RPM on Linux):
./scripts/build-dist.sh --desktop

# Build BOTH 'with-engine' and 'without-engine' variants for a specific app:
./scripts/build-dist.sh --desktop --app dialex --both-variants

# Build everything (CLI cross-compile + Desktop, all variants, with SHA256 checksums):
./scripts/build-dist.sh --all --cross-compile
```

All compiled binaries, DMGs, MSIs, EXEs, DEBs, RPMs, and cross-platform archives are staged cleanly in the `dist/` folder:

```text
dist/
├── SHA256SUMS                  # Cryptographic verification checksums
├── bin/                        # Standalone host Go binaries
│   ├── artix
│   ├── artixd
│   ├── kritix
│   ├── socratix
│   └── socratixbench
├── archives/                   # Platform release archives (.tar.gz / .zip)
│   ├── artix-v1.0.0-*.tar.gz / .zip
│   ├── kritix-v1.0.0-*.tar.gz / .zip
│   └── socratix-v1.0.0-*.tar.gz / .zip
└── desktop/
    ├── with-engine/            # Installers with bundled Socratix engine
    │   ├── ArtixAI-1.0.0-with-engine.dmg (.msi / .exe / .deb / .rpm)
    │   ├── DialexAI-1.0.0-with-engine.dmg (.msi / .exe / .deb / .rpm)
    │   └── KritixAI-1.0.0-with-engine.dmg (.msi / .exe / .deb / .rpm)
    └── without-engine/         # Lightweight client-only installers
        ├── ArtixAI-1.0.0-without-engine.dmg (.msi / .exe / .deb / .rpm)
        ├── DialexAI-1.0.0-without-engine.dmg (.msi / .exe / .deb / .rpm)
        └── KritixAI-1.0.0-without-engine.dmg (.msi / .exe / .deb / .rpm)
```

---

## 7. Verifying Installation

Run these verification commands after installation:

```bash
# 1. Verify Socratix Deliberation Engine
socratix --help
socratix serve --port 7890

# 2. Verify Artix AI CLI
artix version
artix persona

# 3. Verify Kritix AI Testing Studio
kritix version
kritix models
```
