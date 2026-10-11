# Artix AI — Professional Platform Guide
### Sovereign Multi-Agent Software Engineering & Autonomous Coding

---

## 1. Architectural Philosophy: The Dialectic Engine

Most AI coding tools operate under the **"Solo Sycophant Problem"**: a single LLM attempts to act as product manager, architect, developer, and tester simultaneously. It invents missing requirements, validates its own assumptions, and generates unverified code directly into the developer's working files.

**Artix AI decouples software engineering into two separate checks-and-balances chambers:**

```
                                 ┌──────────────────────────────────────────────┐
                                 │              USER / DEVELOPER / PO           │
                                 └──────────────────────┬───────────────────────┘
                                                        │
                         Prompt / Story Idea            │  Target Spec / Review
                                                        ▼
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                       CHAMBER 1: STAKEHOLDER COUNCIL                                    │
│                                           (Requirements Planning)                                      │
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  • Product Owner Lead:    Value maximization, user story framing, scope boundary defense.             │
│  • Senior Architect:      Layer isolation, clean boundaries, ADR trade-off synthesis.                  │
│  • QA Testing Lead:       Adversarial edge-case modeling, negative scenarios, test command tailoring.  │
│  • Engineering Manager:   Delivery feasibility, complexity pruning, risk balancing.                   │
│                                                                                                        │
│  Output ────────► Canonical Story Spec with Gherkin Scenarios (docs/specs/STORY-<id>.md)               │
└───────────────────────────────────────────────────┬────────────────────────────────────────────────────┘
                                                    │
                                                    ▼
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                       CHAMBER 2: ENGINEERING LAB                                       │
│                                           (Grounded Execution)                                         │
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  • Shadow Git Worktree:   Isolated execution branch (.artix/worktrees/); zero workspace disruption.  │
│  • Domain Coder:          Selected specialist (Android, Backend, iOS) produces atomic unified diffs.   │
│  • Local Test Sandbox:    Deterministic process-group execution (./gradlew test, go test, cargo test). │
│  • Adversarial Reviewer:  Scrutinizes AST diffs against test results and Taboo Space constraints.       │
│                                                                                                        │
│  Loop (Rounds 1..N) ────► 100% Green Tests & Reviewer Sign-Off ────► Clean Merge / PR Creation         │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Installation & Quick Start

### 2.1 System Requirements & Platform Constraints
- **macOS** (Apple Silicon or Intel), **Linux** (x86_64 or aarch64), or **Windows** (WSL2).
- **Go 1.27+** (for native CLI and LSP server).
- **Java JDK 21+** (for Compose Multiplatform Desktop Cockpit App).
- **Git 2.30+** (with support for `git worktree`).

> [!IMPORTANT]
> **Platform Constraints & Driver Governance:**
> - **Supported Drivers & Rollout Allowlist:** By default, only `go` and `gradle` are allowed (`allowedDrivers: [go, gradle]`). Unallowed drivers (e.g. `cargo`, `npm`, `swiftpm`, `xcode`) are refused before any LLM spend unless explicitly enabled in policy.
> - **Offline Cache Prerequisite (`artix warm`):** Sandboxed builds run in complete network isolation (`--offline`). Dependencies must be pre-warmed outside the sandbox with `artix warm` before running the loop. A cold cache returns `OFFLINE_CACHE_MISS` naming the missing artifact.
> - **iOS Targets Unsupported on Linux (`SKIPPED_HOST_UNSUPPORTED`):** iOS simulator and compilation targets require macOS. On Linux, iOS targets are classified as `SKIPPED_HOST_UNSUPPORTED`, preventing false "all targets green" convergence claims and refusing autonomous execution.

### 2.2 Installing the Native CLI

To build and install the native binary locally:
```bash
cd artix-ai
go build -ldflags="-s -w" -o /usr/local/bin/artix ./cli/main.go
go build -ldflags="-s -w" -o /usr/local/bin/artixd ./cmd/artixd/main.go

# Verify installation:
artix version
# Output: artix version 1.0.0
```

### 2.3 Shell Autocompletion
Add to your `~/.zshrc` or `~/.bashrc`:
```bash
# Artix CLI aliases
alias k="artix"
alias kr="artix repl"
alias kp="artix plan"
alias kc="artix code"
alias kl="artix lsp"
```

---

## 3. Professional IDE Integration Guide

Artix operates a standard **Language Server Protocol (LSP 3.17)** server via `artix lsp`. This allows any modern editor to get real-time Taboo Space diagnostics, CodeLens triggers, and quick-fix actions without maintaining fragile custom plugins.

### 3.1 Visual Studio Code & Cursor Setup

#### Method: Using Generic LSP Client
1. Install the **Generic LSP Client** or **Language Server Protocol Inspector** extension from the VS Code Marketplace.
2. Open your repository's `.vscode/settings.json` and add:

```json
{
  "languageServerExample.trace.server": "verbose",
  "artix.lsp.enabled": true,
  "artix.lsp.path": "/usr/local/bin/artix",
  "artix.lsp.arguments": ["lsp"],
  
  // Custom editor integration
  "[kotlin]": {
    "editor.codeLens": true,
    "editor.codeActionsOnSave": {
      "source.fixAll": "explicit"
    }
  },
  "[go]": {
    "editor.codeLens": true
  }
}
```

#### What You Experience in VS Code / Cursor:
- **Real-Time Taboo Warnings:** If you type forbidden code (such as `android.database.sqlite` in presentation layers or `Thread.sleep` on the main thread), red squiggly lines appear instantly.
- **CodeLens Over Headings:** A clickable lens appears above class headers:  
  `⚡ Artix: Deliberate Story Spec` (click to run the Stakeholder Council).
- **Quick-Fix Lightbulb (`Cmd + .`):** Select `"Artix: Fix Taboo Space Violation"` to prompt the Domain Coder to generate an architectural refactor.

---

### 3.2 JetBrains IDEs (IntelliJ IDEA, Android Studio, GoLand)

1. Open **Settings / Preferences** (`Cmd + ,`) $\rightarrow$ **Plugins**.
2. Install the **LSP4IJ (Language Server Protocol for IntelliJ)** plugin.
3. Navigate to **Settings** $\rightarrow$ **Languages & Frameworks** $\rightarrow$ **Language Servers**.
4. Click **`+` Add Language Server**:
   - **Name:** `Artix AI`
   - **Executable Path:** `/usr/local/bin/artix`
   - **Arguments:** `lsp`
   - **File Types:** `Kotlin (.kt, .kts)`, `Java (.java)`, `Go (.go)`, `Rust (.rs)`, `TypeScript (.ts)`
5. Click **Apply** and **OK**.

You will now receive live Taboo Space squigglies, intentions, and CodeLens directly in your editor.

---

### 3.3 Zed Editor Setup

Add the following to `~/.config/zed/settings.json`:

```json
{
  "lsp": {
    "artix": {
      "binary": {
        "path": "/usr/local/bin/artix",
        "arguments": ["lsp"]
      }
    }
  },
  "languages": {
    "Kotlin": {
      "language_servers": ["artix", "!kotlin-language-server"]
    },
    "Go": {
      "language_servers": ["artix", "gopls"]
    }
  }
}
```

---

### 3.4 Neovim Setup

Add the following to your `init.lua`:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "kotlin", "go", "rust", "typescript" },
  callback = function()
    vim.lsp.start({
      name = "artix-lsp",
      cmd = { "artix", "lsp" },
      root_dir = vim.fs.dirname(vim.fs.find({ ".git", "go.mod", "build.gradle.kts" }, { upward = true })[1]),
    })
  end,
})
```

---

### 3.5 Provider & Model Configuration in GUI-Launched IDEs (macOS)

On macOS, GUI applications launched from the Dock, Spotlight, or Finder do not inherit shell profile environment variables (such as those in `~/.zshrc` or `~/.bash_profile`). Consequently, `ARTIX_PROVIDER`, `ARTIX_MODEL`, and API keys are not automatically available unless explicitly configured.

To ensure `ARTIX_PROVIDER` reaches GUI-launched IDEs:

#### Option 1: VS Code Settings (Recommended for VS Code)
Configure the provider directly in your user or workspace settings (`.vscode/settings.json`):
```json
{
  "artix.binary": "artix",
  "artix.provider": "anthropic",
  "artix.model": "claude-3-5-sonnet-20241022"
}
```

#### Option 2: Launch IDE from Terminal
Launch your IDE from an active terminal session where your environment variables are already exported:
```bash
# For VS Code:
code .

# For IntelliJ IDEA:
idea .
# Or via open:
open -a "IntelliJ IDEA" .
```

#### Option 3: System-Wide GUI Environment via launchctl
To make environment variables available to all GUI applications launched by `launchd`:
```bash
launchctl setenv ARTIX_PROVIDER anthropic
launchctl setenv ANTHROPIC_API_KEY "your-api-key-here"
```

#### Option 4: IntelliJ Custom VM Options / Environment Plugin
In IntelliJ IDEA, navigate to **Help** $\rightarrow$ **Edit Custom VM Options** or configure environment variables in the EnvFile plugin.

---

## 4. Product Owner & Manager Guide: The Art of Verified Requirements

As a non-technical Product Owner or Engineering Manager, your primary challenge is ensuring that AI agents do not write code based on hallucinated assumptions or ambiguous requirements.

```
Idea ──► /grill-me (Alignment Interview) ──► /plan (Deliberation) ──► docs/specs/STORY-101.md
```

### Step 1: Conduct an Alignment Interview (`/grill-me`)
Before creating specifications, run the Socratic interview in the terminal shell:

```bash
artix repl
artix> /grill-me "Add biometric FaceID authentication to mobile checkout"
```

The Stakeholder Council immediately challenges you with 3 targeted questions:
1. **[Product Owner Lead]**: *"What happens when the biometric hardware is locked after 5 failed attempts? Should the system fall back to PIN, SMS OTP, or password re-entry?"*
2. **[Senior Architect]**: *"Which layer encapsulates biometric keychain tokens, and how are secrets scrubbed from memory after checkout completion?"*
3. **[QA Testing Lead]**: *"What deterministic test scenarios must pass to ensure biometric bypass is impossible on rooted/jailbroken devices?"*

Answering these questions upfront eliminates 90% of downstream rework and bug cycles.

### Step 2: Deliberate and Generate the Story Spec (`/plan`)
Once aligned, trigger the Stakeholder Council debate:

```bash
artix> /plan "Implement FaceID biometric authentication with PIN fallback and Keychain isolation"
```

The Council deliberates across 3 structured rounds and writes a verified spec to:  
`docs/specs/STORY-<id>.md`.

### Step 3: Understanding the Story Spec Anatomy

Every generated spec contains:
1. **User Story Statement**: Standard business objective.
2. **Strict In-Scope vs. Out-of-Scope Manifest**: Explicit fences against scope creep.
3. **Acceptance Criteria in Gherkin Syntax**:
   ```gherkin
   Scenario: Successful Biometric Authorization
     Given the user has enrolled FaceID in device settings
     When checkout total exceeds $50.00
     Then prompt system biometric dialog
     And proceed to order placement upon cryptographic signature confirmation

   Scenario: Biometric Failure Fallback
     Given biometric sensor fails or times out after 10 seconds
     When the fallback button is tapped
     Then display secure 6-digit PIN pad
     And lock session after 3 invalid attempts
   ```
4. **Architecture Decision Records (ADRs)**: Documents why a pattern was chosen and its consequences.
5. **Deterministic Verification Commands**: Project-tailored test scripts (e.g. `./gradlew testDebugUnitTest`, `go test -v ./...`).

### Step 4: Tailoring Communication with Style Vectors
- **For Executives & Non-Technical Stakeholders (`--style ponytail`):**  
  `artix plan --style ponytail "..."` produces executive summaries, trade-off tables, and business ROI justifications.
- **For Deep Technical Teams (`--style caveman`):**  
  `artix plan --style caveman "..."` produces dense, high-signal, zero-fluff code commands.

---

## 5. Software Engineer Guide: Zero-Disruption Coding

### 5.1 The Danger of Other Coding Agents
Traditional AI tools execute edits directly on your active working files. If the AI makes a syntax error or breaks tests, your git tree is dirty, your local compile fails, and multitasking is impossible.

### 5.2 The Artix Solution: Isolated Shadow Worktrees
When you run `artix code`, Artix calls `git worktree add` to create an ephemeral, isolated workspace (`.artix/worktrees/<task-id>`).
- You can continue typing in your active branch uninterrupted.
- The Coder generates atomic patches in the shadow worktree.
- The Sandbox runs test commands inside the shadow worktree.
- The Adversarial Reviewer tests the patch against Taboo Spaces.
- **Only when tests pass 100% does Artix prompt you to merge the result.**

```
Your Active Workspace (Clean & Intact) ───────────► Keep coding without interruption
                                                     ▲
                                                     │ 1-Click Fast-Forward / Squash Merge
                                                     ▼
Isolated Shadow Worktree               ───────────► Coder modifies files
(.artix/worktrees/task-101)                         Sandbox executes ./gradlew test
                                                     Reviewer evaluates diffs & sign-off
```

### 5.3 Interactive Terminal REPL (`artix repl`)
Launch the stateful developer shell:
```bash
artix repl
```

#### Context Mentions (`@` and `#`)
Inject targeted context into prompts without copying and pasting:
- `@file:src/auth/AuthManager.kt` — attaches file contents directly into context.
- `@spec:STORY-101` — grounds the Coder in a specific Story Spec.
- `@rule:taboo-storage` — enforces a specific steering invariant.
- `#symbol:RefreshToken` — focuses context extraction on a specific symbol.

```bash
artix> Inspect @file:src/auth/AuthManager.kt against @spec:STORY-101 and fix timeout retries
```

#### Slash Commands
- `/plan <story>` — assemble Stakeholder Council.
- `/code [spec]` — run convergence loop in shadow worktree.
- `/grill-me [story]` — trigger 3-question alignment interview.
- `/review` — evaluate current uncommitted diffs against Taboo Spaces.
- `/compact` — prune conversation history while preserving ADRs.
- `/undo` — cleanly roll back working tree modifications.
- `/exit` — quit shell.

---

## 6. Steering Studio & Institutional Governance

### 6.1 What are Taboo Spaces?
A **Taboo Space** is a non-negotiable negative constraint. While ordinary prompts offer suggestions, Taboo Spaces are hard architectural barriers enforced by the Adversarial Reviewer.

| Example Taboo Constraint | Why It Exists |
| :--- | :--- |
| `Do not allow raw sqlite imports in UI layer` | Enforces Clean Architecture / prevents UI coupling to disk. |
| `Do not block main thread with Thread.sleep` | Prevents Application Not Responding (ANR) crashes on Android. |
| `Do not commit hardcoded secrets or JWTs` | Prevents security leakage in git history. |
| `Do not use java.util.Date in commonMain` | Enforces Kotlin Multiplatform cross-platform portability. |

### 6.2 Managing Steering Configuration (`.artix/steering.json`)
Manage rules and persona bindings with the CLI:

```bash
# List all discovered steering documents (local, standard symlinks, remote):
artix steering list

# Bind a taboo rule to a specific persona:
artix steering bind android_engineer taboo-sqlite
artix steering bind backend_engineer taboo-storage

# Sync external standards feeds from Git or remote URLs:
artix steering sync
```

### 6.3 Reviewer Scan Modes & Multi-Language Coverage
The Adversarial Reviewer applies deterministic pre-filtering and taboo verification before invoking model critics:
- **Go diffs:** Native Go AST parsing (`scan mode: diff-literal | AST`), identifying syntax structures, call graphs, import graphs, and package aliases deterministically.
- **Non-Go languages (Kotlin, Swift, Python, TypeScript/JavaScript):** Semantic alias, reflection, and de-concatenation pattern scanning (`scan mode: heuristic`). Because native compilers for these languages are not embedded in the engine, Taboo enforcement for Kotlin and Swift runs in heuristic mode, scrutinizing import aliases (`import java.lang.Runtime as EvilRuntime`), type aliases (`typealias TaskRunner = Process`), dynamic reflection (`Class.forName(...)`), and string concatenations (`"NS" + "Task"`).

---

### 6.4 PR-to-Rule Synthesizer (Learning from Human Reviews)
Artix includes an automatic synthesizer in `artixd` that ingests human PR review comments and synthesizes structured rules:
- A reviewer comments on a pull request:  
  `"Never use GlobalScope.launch in our viewmodels, always use viewModelScope!"`
- The Synthesizer parses and sanitizes the comment into a **PROPOSED** rule:
  - **Category:** Taboo Constraint.
  - **Target Persona:** `android_engineer`, `adversarial_code_reviewer`.
  - **Status:** Starts as `proposed` (stored in `.artix/knowledge/`).
  - **Governance:** Rules start `proposed` and require human CODEOWNER ratification via `artix knowledge ratify <ruleId> --approver <username>` before becoming active.
- Once ratified by a CODEOWNER, the rule activates and permanently prevents the regression across all subsequent agent runs.

---

## 7. Autonomous Server & GitHub CI/CD Pipeline

### 7.1 Running the Headless Daemon (`artixd`)
For centralized servers or CI machines:

```bash
# Start the webhook daemon on port 8080:
artixd --addr :8080 --gh-secret $GITHUB_WEBHOOK_SECRET
```

#### Available Endpoints:
- `GET  /healthz` — healthcheck probe.
- `POST /webhook/github` — receives GitHub Issue and PR webhooks (validates HMAC-SHA256 signature).
- `POST /webhook/gitlab` — receives GitLab Issue events (validates `X-Gitlab-Token`).
- `GET  /jobs` — inspect status of active and completed autonomous background runs.

---

### 7.2 GitHub Actions Automated CI/CD Workflow
Artix includes a complete continuous integration pipeline at `.github/workflows/artix.yml`:

```yaml
name: Artix AI Verification Pipeline

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]

jobs:
  go-verification:
    name: Go Test Suite & Static Analysis
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22.x"
      - run: go vet ./pkg/... ./cli/...
      - run: go test -v -race -cover ./pkg/...
      - run: go build -o bin/artix ./cli/main.go

  cockpit-kmp-build:
    name: Desktop Cockpit Compilation
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "21"
      - uses: gradle/actions/setup-gradle@v4
      - run: ./gradlew :artix:app:compileKotlinJvm --no-daemon

  adversarial-pr-check:
    name: Adversarial Reviewer PR Check
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22.x"
      - run: |
          go build -o bin/artix ./cli/main.go
          git diff origin/${{ github.base_ref }}...HEAD > pr.diff
          ./bin/artix review
```

---

## 8. Standalone Desktop Cockpit App (KMP + KoltLibs)

**YES! Artix AI features a 100% full-parity Graphical User Interface (GUI).**

For Product Owners, Engineering Managers, and developers who prefer a modern visual workbench over terminal commands, Artix provides a standalone Desktop Cockpit built natively with **Compose Multiplatform** and **KoltLibs** (`io.github.koltalabs.kolt:compose-kmp`, `utils`, `logutils`).

Every capability available in the CLI and LSP—from Socratic alignment interviews to shadow worktree time-travel scrubbers—is accessible visually with clicks, sliders, and toggles.

```bash
# Launch the desktop app:
./gradlew :artix:app:run
```

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│  ARTIX AI COCKPIT  v1.0.0               [● Council Chamber]  [Lab]  [Steering]  [Personas]  [Knowledge]     ⚙ Settings│
├────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  Story Prompt: [ Build user authentication flow with JWT refresh tokens                        ]  [⚡ Deliberate Spec] │
│  Style Vector: (●) Balanced (Standard)   ( ) Executive (Ponytail)   ( ) Technical (Caveman)                           │
│  [🎙️ Toggle Socratic Alignment Interview ("Grill-Me")]                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │ 🎙️ Socratic Alignment Interview (Interactive Clarifications)                                                      │  │
│  │  • Product Owner:  [ "Should failed token refreshes trigger immediate logout or silent retry?"                 ] │  │
│  │  • Lead Architect: [ "Store tokens in HTTP-only Secure Cookies or OS Keychain Encrypted SharedPreferences?"     ] │  │
│  │  • QA Testing Lead:[ "Do we test clock drift of +/- 300 seconds on expired refresh tokens?"                    ] │  │
│  └──────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘  │
│  Stakeholder Council:                                                                                                  │
│  [PO Lead 90%]         [Architect 95%]        [QA Lead 85%]         [Eng Manager 90%]                                  │
│  ┌──────────────────────────────────────────────┬───────────────────────────────────────────────────────────────────┐  │
│  │ Stakeholder Deliberation Feed:               │ Canonical Verified Story Spec (Markdown Live Preview):            │  │
│  │ • PO: "Tokens must refresh seamlessly."      │ # STORY-AUTH-001: JWT Authentication & Refresh                    │  │
│  │ • Arch: "No JWT secrets in client storage."  │ ## Gherkin Acceptance Scenarios                                   │  │
│  │ • QA: "Enforce expired token test suite."    │ Given valid refresh token When /api/refresh Then return 200 OK    │  │
│  └──────────────────────────────────────────────┴───────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### The 5 Visual Workspaces:

#### 1. 🏛️ Chamber 1: Council Chamber (Requirements & Planning)
Designed for Product Owners and Engineers to co-create bulletproof specifications:
- **Interactive Socratic Alignment Drawer (`/grill-me` GUI):** Click the microphone button to expose questions asked by PO, Architect, and QA agents. Type your answers directly into the fields to resolve ambiguities before generation.
- **Style Vector Toggle Pills:** Choose between **Balanced (Standard)**, **Executive (Ponytail)**, or **Technical (Caveman)** tone with a single click.
- **Live Avatar Influence Stream:** Watch stakeholders deliberate, defend architectural boundaries, and reach consensus in real time.
- **Export & Approve:** Preview the rendered Markdown spec and save it directly to `docs/specs/STORY-<id>.md`.

#### 2. 🔬 Chamber 2: Engineering Lab (Isolated Execution & Scrubber)
The visual control center for autonomous code generation:
- **🛡️ Shadow Worktree Indicator:** Displays active background worktree path (`.artix/worktrees/<task-id>`) ensuring zero risk to your current branch or uncommitted files.
- **⏪ Interactive Time-Travel Checkpoint Scrubber:** Scrub back and forth across execution rounds (`[Round 1]`, `[Round 2]`, `[Round 3]`) to inspect the AST diff evolution and test outcomes at each step.
- **Side-by-Side Diff Inspector:** Unified and split-screen diff view with chunk-by-chunk Accept/Reject buttons.
- **Live Sandbox Terminal:** Displays streaming test execution output (`./gradlew test`, `go test`, `cargo test`) with process exit codes.
- **One-Click Squash & Merge:** When tests pass 100% and the Reviewer signs off, click **Squash & Merge** to incorporate the verified changes into your active branch.

#### 3. 🧭 Steering Studio (Dynamic Governance & Synthesizer)
Manage repository rules, taboo spaces, and team standards visually:
- **Persona-to-Rule Binding Matrix:** Interactive grid with toggle switches to assign rules (e.g., `clean-architecture-invariants`, `no-reflection`) to specific personas.
- **🪄 PR-to-Rule Synthesizer Drawer:** Paste any PR comment or review feedback into the input field and click **"Synthesize & Bind Rule"** to automatically turn PR reviews into permanent repo governance rules.
- **🧪 Live Taboo Space Sandbox Simulator:** Paste candidate code snippets to instantly verify whether they trigger taboo AST violations (e.g., forbidden reflection or banned imports) in real time.
- **1-Click Remote Standards Sync:** Pull team steering standards down from GitHub/GitLab org repositories.

#### 4. 🧬 Persona Studio (SWE Cognitive Priors)
Inspect and fine-tune the AI personas running inside your repo:
- **Interactive Radar Chart:** Canvas-rendered 8-layer cognitive visualization of priors: *Strictness*, *Skepticism*, *Modularity*, *Velocity*, and *Paranoia*.
- **Persona Switcher:** Inspect predefined personas (Android, iOS, Backend, Security Auditor, QA Lead, Architect) and examine their prompt baselines and tooling permissions.

#### 5. 💡 Knowledge Insights (Repository Memory Explorer)
Visual dashboard of the self-evolving `.artix/knowledge/` store:
- **Breakthrough Cards:** Browse learned testing heuristics, architectural invariants, and debugging breakthroughs auto-extracted when hard loops converge.
- **Full Text Search & Category Filter:** Filter by Architecture, Testing, Performance, and Security.
- **Markdown Detail Viewer:** Read complete context, reproduction steps, and suggested remedies for any documented pattern.

---

### Interface Modality Comparison: Choose What Fits You

| Feature | Desktop Cockpit (GUI) | Interactive CLI (TUI / REPL) | Universal LSP (VS Code / Zed) |
| :--- | :---: | :---: | :---: |
| **Primary Audience** | Product Owners, Tech Leads, Visual Devs | Terminal Power Users | In-Editor Developers |
| **Stakeholder Deliberation** | Live Avatars & Chat Transcript | Rich ANSI Colored Stream | In-editor Spec generation |
| **Socratic Alignment ("Grill-Me")** | Interactive Visual Drawer | Interactive TUI REPL (`/grill-me`) | QuickPick Prompts |
| **Execution Safety** | Shadow Worktree Badge | Isolated Worktree Subshell | Isolated Worktree Background Task |
| **Time-Travel Checkpoints** | Visual Clickable Scrubber | Git Checkpoint Tree | Git History Lens |
| **Diff Review** | Chunk-by-chunk Visual Accept/Reject | ANSI Unified Diff Pager | Editor Diff Tab |
| **Taboo Space Violations** | Interactive Simulator | Stderr Rule Report | Real-time Inline Squigglies & Diagnostics |
| **PR-to-Rule Synthesizer** | Visual Paste & 1-Click Bind | `artix steering synthesize` | CodeAction: "Synthesize Rule from Diff" |
| **Knowledge Base Explorer** | Interactive Card & Detail View | `artix knowledge list` | Auto-injected Prompt Context |

---

## 9. CLI Command Reference & Cheat Sheet

```text
Usage:
  artix [command] [options] [arguments]

Core Commands:
  repl                     Launch interactive TUI shell with @mentions and /grill-me
  lsp                      Launch Language Server Protocol backend for IDEs (VS Code, Zed, etc.)
  plan, spec <story>       Deliberate with Stakeholder Council and produce docs/specs/STORY-<id>.md
  code [spec-file]         Execute Domain Coder <-> Reviewer convergence loop in shadow worktree
  review                   Run Adversarial Reviewer against current uncommitted diffs
  steering [list|sync|bind] Manage dynamic steering rules and persona bindings
  persona [list]           Inspect and manage SWE Personas
  daemon [--addr :8080]    Launch headless server daemon for GitHub/GitLab automation
  version                  Display version

Options for 'plan':
  --style standard         Default balanced specification format
  --style ponytail         Executive summary bullet points for leadership
  --style caveman          Dense, zero-fluff code commands for technical engineers

Options for 'code':
  --domain <domain>        Target SWE persona (default: backend_engineer, android_engineer)
  --autonomy <gate>        supervised (confirm diff), interactive (pause per round), autonomous (auto-commit; requires verified, cryptographically signed policy with allowAutonomous=true)
  --rounds <n>             Maximum convergence rounds (default: 3)
```

---

## 10. Frequently Asked Questions (FAQ)

#### Q: How does Artix prevent breaking my active branch?
**A:** Artix executes coder/reviewer iterations in an isolated **Shadow Worktree** (`git worktree add`). Your local unstaged edits and working directory are completely untouched until you review and approve the final result.

#### Q: What happens if tests fail during the Coder loop?
**A:** The Adversarial Reviewer captures the exact test exit code and error trace, rolls back the broken patch, provides constructive feedback to the Domain Coder, and prompts for an amended patch in Round 2. If tests do not pass after $N$ rounds, the worktree is safely discarded without polluting your git history.

#### Q: Can I run Artix in an air-gapped or private enterprise network?
**A:** Yes. Artix can run entirely against local sandboxes and supports local model backends (via Ollama or llama.cpp) as well as self-hosted GitHub Enterprise / GitLab instances.

#### Q: Can our team share steering rules across multiple repositories?
**A:** Yes. You can register a central Git repository or HTTP endpoint in `.artix/steering.json` using `artix steering sync`. All team members inherit the organization's architectural and taboo standards automatically.

---

## 11. Daemon Deployment & Adversarial Attack Verification (R3-8 / G6)

The Artix daemon (`artixd`) provides webhook integration for automated continuous software engineering on GitHub and GitLab.

### 11.1 Security Controls and Threat Defenses

| Threat Vector | Defense Mechanism | Fail-Closed Behavior |
|---|---|---|
| **Forged Webhook Sender** | GitHub HMAC-SHA256 (`X-Hub-Signature-256`) & GitLab Token (`X-Gitlab-Token`) | `401 Unauthorized` / `403 Forbidden` |
| **Unauthorized Triggers** | `author_association` verified (`OWNER`, `MEMBER`, `COLLABORATOR`) or `--allowed-users` allowlist | `403 Forbidden` (rejected before parsing prompt) |
| **Replay Attacks** | Delivery cache tracking (`X-GitHub-Delivery`, `X-Gitlab-Event-UUID`) with time-window deduplication | `409 Conflict` (ignored without execution) |
| **Prompt Injection in Issues** | AST prompt sanitizer (`sanitizer.SanitizePrompt`) strips injection keywords and jailbreaks | `400 Bad Request` |
| **Denial of Service (Oversized Payloads)** | Request body capped by `--max-body-length` (default: 32 KB) | `413 Request Entity Too Large` |
| **Policy Tampering** | Issue body text cannot modify policy, steering bindings, or bypass taboos | Hard-rejected by deterministic policy validator |

### 11.2 Launching the Daemon

```bash
artixd \
  --addr ":8080" \
  --github-secret "$GITHUB_WEBHOOK_SECRET" \
  --gitlab-token "$GITLAB_WEBHOOK_TOKEN" \
  --jobs-token "$JOBS_AUTH_BEARER_TOKEN" \
  --work-dir "/var/artix/workspaces" \
  --max-concurrent-jobs 4 \
  --max-body-length 32768
```

### 11.3 Running the Adversarial Attack Suite Against the Build

Evaluators and hostile reviewers can reproduce and verify all daemon security controls using the automated adversarial suite:

```bash
# Run all G6 daemon attack suite tests with race detector:
go test -race -v -count=1 ./pkg/forge -run "TestG6|TestR2_2|TestWebhook"

# Run daemon entrypoint and server binding authorization tests:
go test -race -v -count=1 ./cmd/artixd -run "Test"
```

