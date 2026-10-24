# Changelog

All notable changes to **Dialex AI** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Conventional Commits](https://www.conventionalcommits.org/).

---

## [Unreleased]

### Added
- Automated GitHub Actions Release workflow with cross-platform matrix artifact building.
- Automated Conventional Commits changelog extractor and release publisher.
- Single source of truth versioning in `gradle.properties` with dynamic binding to Compose Desktop and Android.

---

## [1.0.0] - 2026-09-21

### 🚀 Major Features & Capabilities
- **Hegelian Multi-Agent Adversarial Deliberation**:
  - Pits up to 6 competing frontier AI models (Anthropic Claude, OpenAI GPT, Google Gemini, xAI Grok, DeepSeek, Mistral Large, and local Ollama) in multi-round structured dialectic debates.
  - Designated Moderator (Seat 1) who frames topic agendas, leads round-by-round syntheses, and generates the final consensus outcome.
- **No Extra Per-Token Billing via Local CLIs or Ollama**:
  - Direct execution through local authenticated developer subshells (`claude`, `codex`, `antigravity`/`agy`, `ollama`) to eliminate metered enterprise API bills.
- **Two-Tab Persona Registry & Style Modifiers**:
  - 10 Universal General Debate Archetypes (*The Facilitator*, *Devil's Advocate*, *The Optimist*, *The Pragmatist*, *The Contrarian*, *The Expert*, *The Risk Analyst*, *The Ethicist*, *The Historian*, *The Futurist*).
  - 40+ Domain-Specific Personas spanning Distributed Systems, Cybersecurity, AI Infrastructure, FinTech, Legal Compliance, and Product Strategy.
  - Interactive `PersonaBadge` chip system with inline edit sheet, `(Custom)` persona tags, and *Ponytail* style modifiers.
- **3-Tier Anti-Fluff & Topic Drift Cascade Guardrails**:
  - Global Settings &rarr; Project Settings &rarr; Discussion Setup hierarchy.
  - Human Dialogue Mode enforcing 2–4 crisp sentences and eliminating sycophantic conversational pleasantries.
  - Anti-Rabbit-Hole guardrails that mandate immediate steering back to the core dilemma.
- **Live Human Steering & Instant Interrupts**:
  - Queue steering prompts for the next round without interrupting active streams.
  - Instant Interrupt to halt running generation, cleanly finalize partial tokens, and inject human directives without 409 concurrency conflicts.
- **Actionable Generative Deliverables**:
  - Moderator Consensus Outcome Bubble with bottom-line verdict, consensus agreements, and critical caveats.
  - 1-Tap downstream synthesis: **Architecture Decision Records (ADR-042)**, **Weighted Decision Matrices**, **Pro/Con Breakdowns**, **Executive Memorandums (HTML / Markdown)**, and **Code Diffs**.

### 🏛️ Cross-Platform Architecture & Topologies
- **Kotlin Multiplatform & Compose Multiplatform Frontend**:
  - Decoupled unidirectional Model-View-Intent (MVI) architecture using `Contract.kt` and `kotlinx.collections.immutable`.
  - Obsidian Nebula design system powered by the **Kolt Ecosystem (`KoltLibs`)** composite build (`compose-kmp`, `utils`, `logutils`).
  - Fluid-responsive desktop window management (680–880dp) with traffic light insets and sliding artifacts drawer (`Cmd+Shift+A`).
  - Dialex AI Mobile (Android) with radial stadium arena, hold-to-speak voice dilemma intake, and native TTS audio summaries.
- **High-Throughput Go Orchestration Engine**:
  - Mutex-locked turn state machine with Server-Sent Events (SSE) streaming hub.
  - Dual deployment support: **In-Device Engine** (auto-spawned local child process at `127.0.0.1:8080`) and **Remote Host Engine** (Docker Compose, systemd, Tailscale `tsnet` WireGuard mesh).
  - AES-256-GCM encrypted API-key storage (key file on disk; OS keychain planned).
  - In-memory diagnostics ring buffer (`AppLogStore`) capturing the last 1,000 transport events.

---

[Unreleased]: https://github.com/Keltstone/DialexAI/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/Keltstone/DialexAI/releases/tag/v1.0.0
