# Feature List, USPs & Beneficiaries Catalog — Dialex AI

This document details all user-facing capabilities, unique selling propositions (USPs), architectural controls, target audiences, and operational workflows of **Dialex AI**.

---

## 1. Executive Summary of Key USPs

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                             DIALEX AI KEY ADVANTAGES                            │
├───────────────────────┬──────────────────────────┬───────────────────────────────┤
│ Adversarial Council   │ No Extra Per-Token Billing     │ 1-Click Native Desktop Apps   │
│ Up to 6 Frontier LLMs │ Run on local subshells   │ macOS (.dmg), Win (.msi),     │
│ eliminate blind spots │ with existing subscriptions│ Linux (.AppImage), Android    │
├───────────────────────┼──────────────────────────┼───────────────────────────────┤
│ Zero-Cloud Privacy    │ Deterministic State      │ Actionable Deliverables       │
│ AES-256 Vault +       │ Go turn state-machine    │ Automated ADRs, Matrices,     │
│ Tailscale Private Mesh│ with Instant Interrupts  │ Executive Memos, Code Diffs   │
└───────────────────────┴──────────────────────────┴───────────────────────────────┘
```

---

## 2. Multi-Agent Deliberation & Frontier Models

- **Up to 6 Simultaneous Frontier Models**: Convene Anthropic Claude, OpenAI (GPT / Codex), Google (Gemini / Antigravity), xAI Grok, DeepSeek (R1 / V3), Mistral Large, and local Ollama models in a single deliberation.
- **Designated Moderator Topology**: Seat 1 serves as the Primary Facilitator to frame topics, direct rounds, and synthesize consensus, accompanied by up to 5 competing peer models.
- **12-Color Member Palette**: Deterministic, accessible color coding mapped dynamically to council seats using Kolt theme tokens, ensuring crystal-clear speaker differentiation.
- **Dual Execution Modes (API vs CLI)**: Connect directly via encrypted cloud API keys or shell out to authenticated developer CLIs (`claude`, `codex`, `antigravity`/`agy`, `ollama`) with zero extra per-token billing.

---

## 3. Two-Tab Persona Registry & Style Modifiers

- **10 Universal Debate Archetypes**:
  1. 🏛️ **The Facilitator**: Structures debate, maintains topical focus, and synthesizes consensus.
  2. 😈 **Devil's Advocate**: Relentlessly stress-tests assumptions, exposing hidden failure modes.
  3. 🚀 **The Optimist**: Explores 10x upside, radical scalability, and architectural innovation.
  4. ⚖️ **The Pragmatist**: Grounds debates in engineering velocity, maintenance cost, and latency.
  5. ⚡ **The Contrarian**: Attacks groupthink and defends high-leverage unorthodox paradigms.
  6. 🔬 **The Expert**: Enforces protocol precision, mathematical validity, and rigorous correctness.
  7. 🛡️ **The Risk Analyst**: Uncovers edge cases, blast radiuses, and single points of failure.
  8. 🧭 **The Ethicist**: Audits alignment, user privacy, and organizational governance.
  9. 📜 **The Historian**: References software engineering precedents and historical post-mortems.
  10. 🔮 **The Futurist**: Assesses 5-to-10-year technology trajectories and obsolescence risks.
- **40+ Specialized Domain Personas**: Expandable domains covering Distributed Systems, Cybersecurity, AI/ML Infrastructure, Healthcare/HIPAA, FinTech Core, Legal Compliance, and Product Strategy.
- **Deliberation Style Modifiers**:
  - **Standard**: Deep dialectic rigor with code citations and nuance.
  - **Ponytail Mode**: Structured executive bullet points with trade-off matrices.

- **8-Layer Cognitive DNA Studio & MMOS Ingestion (v1.7)**:
  - **8 Cognitive Layers**:
    1. *Layer 1 (Core Identity)*: Professional title, background, credentials, and domain authority boundaries.
    2. *Layer 2 (Epistemic Bias)*: 5-D cognitive coordinate matrix (`theoryVsPractice`, `noveltyVsProvenance`, `safetyVsVelocity`, `rigorThreshold`) and primary reasoning modes (*First Principles*, *Empirical/Statistical*, *Historical Analogy*, *Pragmatic/Engineering*, *Formal Logical*).
    3. *Layer 3 (Communication Vector)*: Calibrated tone, formality level (1 to 5), sentence ceiling constraints, rhetorical devices, and syntax patterns.
    4. *Layer 4 (Heuristic Library)*: 10 standard mental model heuristics (*Gall's Law*, *Conway's Law*, *Chesterton's Fence*, *Amdahl's Law*, *Goodhart's Law*, *Occam's Razor*, *CAP Theorem*, *Second-Order Thinking*, *Hanlon's Razor*, *Inversion Principle*) with triggers and application directives.
    5. *Layer 5 (Taboo Space)*: Negative cognitive constraints defining forbidden arguments, rejected fallacies, and intolerable buzzwords with automated penalty actions.
    6. *Layer 6 (Domain Ontology)*: Mandatory RFCs, ISO standards, and specialized terminology with formal citation enforcement.
    7. *Layer 7 (Adversarial Posture)*: 5 combat stances (*Unyielding Dogmatic*, *Counter-Attacking*, *Socratic Inverter*, *Analytical Deconstructor*, *Pragmatic Accommodator*), tenacity scores (0.0 to 1.0), and concede conditions.
    8. *Layer 8 (Synthesis Preference)*: Consensus styles (*Seek Synthesis*, *Hold Minority Report*, *Conditional Compromise*), minority report triggers, and compromise envelopes.
  - **DNA Radar Matrix Visualizer**: Canvas 5-axis pentagonal spider chart rendering live cognitive bias profiles with interactive calibration sliders.
  - **Token-Efficient Prompt Compiler**: Generates mathematical `[COGNITIVE DNA MANDATE]` instruction blocks directly injected into agent context.
  - **MMOS v1.0 Standard Compatibility**: Bi-directional YAML/JSON import and export adhering to the Mind Matrix Open Standard.

<table align="center" width="100%">
  <tr>
    <td width="50%" align="center">
      <img src="screenshots/07_personas_registry_and_custom_library.png" alt="Two-Tab Persona Registry" width="100%" />
      <br/><em>Figure 3.1: Two-Tab Persona Registry with 10 Universal Roles, 40+ Domain Personas, and Custom Badges.</em>
    </td>
    <td width="50%" align="center">
      <img src="screenshots/08_persona_studio_ai_builder.png" alt="Persona Studio AI Builder" width="100%" />
      <br/><em>Figure 3.2: Persona Studio & AI Prompt Builder with Ponytail brevity controls.</em>
    </td>
  </tr>
</table>

---

## 4. 3-Tier Anti-Fluff & Topic Drift Guardrails

- **Human Dialogue Mode**: Enforces concise turns (2–4 sentences), eliminates sycophantic pleasantries (*"I agree with my esteemed colleague"*), and requires direct technical challenges.
- **Topic Drift Guardrail**: Forbids peripheral semantic debates, forcing agents to pull the deliberation back to the core question.
- **3-Tier Cascade Hierarchy**:
  1. *Global Settings*: Master organization-wide defaults.
  2. *Project Settings*: Workspace-level overrides.
  3. *Discussion Setup*: Per-session toggles.

---

## 5. Live Human Interjections & Instant Interrupts

- **Steering Queue**: Type guidance while models are speaking; Dialex AI queues and injects your steering directive before the next round begins without stalling active turns.
- **Instant Interrupt**: Immediately halt active model generation, safely finalize partial tokens, and force human guidance as the next turn without 409 concurrency conflicts.

---

## 6. In-Stream Deliverables & Generative Artifacts

- **Consensus Outcome Bubble**: Upon reaching agreement, the moderator generates a dedicated consensus outcome bubble with bottom-line verdict, consensus agreements, and critical caveats.
- **1-Tap Generative Deliverables**:
  - 📋 **ADR-042 (Architecture Decision Record)**
  - ⚖️ **Weighted Decision Matrix** (Multi-criteria comparative table)
  - 📊 **Pro/Con Trade-Off Breakdown**
  - 📄 **Executive Memorandum (HTML / Markdown)**
  - 💻 **Code Implementation Diff & Scaffolding**
  - ✨ **Custom Deliverable Format Prompt**
- **Chronological Anchoring**: Deliverables remain locked at their exact round timestamps in the chat stream.

<table align="center" width="100%">
  <tr>
    <td width="50%" align="center">
      <img src="screenshots/03_consensus_outcome_deliverables.png" alt="Moderator Consensus Outcome and Deliverables" width="100%" />
      <br/><em>Figure 6.1: Moderator Consensus Outcome Bubble & instant 1-tap generative deliverable action buttons.</em>
    </td>
    <td width="50%" align="center">
      <img src="screenshots/04_artifacts_sliding_drawer.png" alt="Saved Deliverables Sliding Drawer" width="100%" />
      <br/><em>Figure 6.2: Saved Deliverables Sliding Drawer (Cmd+Shift+A) with Markdown/HTML rendering and export.</em>
    </td>
  </tr>
</table>

---

## 7. Cross-Platform Native Clients

### Dialex AI Desktop (macOS, Windows, Linux)
- Standalone 1-click downloadables (`.dmg`, `.msi`, `.AppImage`, `.deb`).
- Auto-spawns managed background Go engine child process on startup.
- Fluid-responsive reading widths (`680.dp` to `880.dp`).
- Top bar double-click window maximization and macOS traffic light insets.

### Dialex AI Mobile (Android)
- Radial Stadium Arena visualizer with active speaker spotlight glows.
- Hold-to-speak voice dilemma dictation with automatic topic extraction.
- Multi-voice native Text-to-Speech (TTS) audio briefings.
- 1-Tap QR Companion Pairing over local LAN or Tailscale WireGuard mesh.

<div align="center">
  <img src="screenshots/09_general_settings_and_theming.png" alt="Settings and Theming" width="75%" />
  <p><em>Figure 7.1: Desktop Settings, dark/light appearance tokens, and legal notice.</em></p>
</div>

---

## 8. Deployment Flexibility: In-Device vs Remote Server

| Deployment Mode | Setup Complexity | Best Suited For | Topology |
|---|---|---|---|
| **In-Device Engine** (Default) | Zero (Automated) | Individual developers, solo architects, offline flights | Desktop app spawns local Go engine at `127.0.0.1:8080`. |
| **Self-Hosted Remote Server** | Low (Docker / Systemd) | Engineering teams, homelabs, dedicated office servers | Central Go engine on Linux server with shared SQLite database. |
| **Tailscale Private Mesh** (`tsnet`) | Zero Port Forwarding | Remote teams, secure mobile-to-desktop connectivity | Encrypted WireGuard MagicDNS at `http://dialex:8080`. |

---

## 9. Target Beneficiaries & Value Matrix

| Target Persona | Key Workflows in Dialex AI | Measurable ROI & Advantage |
|---|---|---|
| 👨‍💻 **Staff & Principal Architects** | Stress-testing backend migrations (Kafka vs Monolith, Postgres vs CockroachDB). | Automated ADR generation; prevents multi-month architectural rewrites. |
| 👔 **CTOs & Tech Executives** | Roadmap validation, cloud infrastructure spend evaluations. | Executive Memos with decision matrices for board and investor review. |
| 🛡️ **Cybersecurity Red-Teams** | Adversarial threat modeling and zero-trust auth vulnerability audits. | Exposes attack surfaces before production rollout. |
| 🔬 **AI / ML Researchers** | Cross-LLM evaluation benchmarks and hallucination elimination. | Direct consensus scoring across competing AI model families. |
| ⚖️ **Legal & Compliance Officers** | Multi-jurisdiction policy audits (GDPR, HIPAA, SOC2). | 100% on-premise sovereign execution with zero cloud data leaks. |
| 🚀 **Startup Founders & PMs** | Synthetic advisory board for product-market fit and pricing strategy. | Eliminates expensive $1,000/hr external advisory retainers. |

---

## 10. Advanced Epistemic Features & Roadmap

The following advanced capabilities have been implemented or are planned:

### Completed Features (v1.1 – v1.6)
1. **Self-Organizing Knowledge Graph & Drag-and-Drop Workspace Organization (COMPLETED)**:
   - Local-first pure-Go SQLite + FTS5 graph storage with activation energy mathematical decay ($W(t) = W_0 \cdot 2^{-\Delta t / t_{\text{half}}}$) and Hebbian edge reinforcement ($\Delta W = \eta (1 - W)$).
   - Interactive 2D force-directed canvas with pan/zoom and activation filters.
   - Smooth drag-and-drop workspace reorganization with auto-expanding drop targets.
2. **Pre-Debate Multi-Perspective Problem Decomposition (COMPLETED)**:
   - Divergent dual-mind breakdown of user dilemmas into orthogonal sub-axes (Technical/Structural vs Product/Strategic) before the main council convenes.
   - Interactive `DecompositionModal` with one-click injection into discussion agenda contexts.
3. **Explicit Contradiction & Tension Pair Detection Engine (COMPLETED)**:
   - Automated real-time extraction and tracking of dialectic tension pairs (thesis vs antithesis) grounded in paraconsistent logic ($C_n$ systems).
   - Real-time slide-out `TensionMatrixDrawer` tracking severity metrics, quote citations, and synthesis resolution.
4. **Round-Aware Dynamic Graph Retrieval & Evidence Grounding (COMPLETED)**:
   - Dynamic per-round query expansion based on disputed claims from round $N$ to retrieve empirical evidence from knowledge graph nodes and attached project files.
   - Automated prompt injection of `[DYNAMIC GROUNDING EVIDENCE FOR ROUND N+1]` grounding blocks and slide-out `RoundEvidenceDrawer`.
5. **Dedicated 1-on-1 Socratic Interview Mode & Live Epistemic Ledger (COMPLETED)**:
   - Targeted 1-on-1 forensic interrogation of individual expert personas without spinning up a full 6-agent council.
   - 5 deep Socratic stances (*Classic Elenchus*, *Maieutic Architecture*, *Radical First Principles*, *Adversarial Red-Team*, *Aporia Boundary-Pusher*).
   - *Brevis Interrogatio* rule ($\le 2$ sentences) forcing high-leverage tension and concise dialectic dialogue.
   - Live Epistemic Ledger tracking green **Hardened Invariants** vs red strike-through **Surrendered Concessions**, reactive Dialogue Assist Chips, structured **Socratic Digest**, and 1-click **Council Elevation**.
6. **Null Hypothesis Benchmarking & Quantitative Evaluation Suite (COMPLETED)**:
   - Automated testing framework validating multi-agent debate superiority against single-model baselines across hallucination, blind spot coverage, trade-off depth, and actionability metrics with paired two-tailed Student's $t$-test ($p < 0.05$).
   - Full-stack execution: Native Desktop KMP UI (Sidebar "⚔️ Arena & Benchmarks" with spider/radar chart and side-by-side deliverable comparison) + Web Admin (`:8080/admin/benchmarks`) + CLI runner (`socratixbench`).
   - Includes embedded **DialexBench-10** canonical dilemma dataset plus custom dilemma authoring and Markdown/CSV/JSON export.

7. **8-Layer DNA Mental & Structured Persona Ingestion (COMPLETED - v1.7)**:
   - Standardized 8-layer cognitive schema (*Core Identity*, *Epistemic Bias*, *Communication Vector*, *Heuristic Library*, *Taboo Space*, *Domain Ontology*, *Adversarial Posture*, *Synthesis Preference*) with MMOS v1.0 YAML/JSON export/import and interactive desktop/mobile Persona Studio.
8. **Mobile Epistemic Parity & Gap Closure Suite (COMPLETED - v1.8)**:
   - Full mobile parity across Socratic Interview launcher, Benchmark Arena mobile viewport & 260dp canvas radar, Project workspace hierarchy with knowledge graph links, and touch-optimized bottom sheets for Dynamic RAG evidence citations and Tension Matrix drawers.
   - Zero-Wait Autonomous Autopilot: 1-tap presets immediately initiate autonomous deliberation without user input or setup pauses.

9. **Bayesian Credence Tracking & Quantitative Epistemic Uncertainty Network (COMPLETED - v1.9)**:
   - Mathematical Bayesian decision engine tracking prior and posterior probability distributions ($P(H | E)$) across 2 to 4 competing hypotheses per debate.
   - Computes real-time Shannon Epistemic Entropy ($\mathcal{S}$) in bits and evidence Likelihood Ratios ($\Lambda$) to isolate decisive tipping points.
   - Interactive Credence Ribbon Canvas with fluid Bézier area flows and live certainty pills in desktop/mobile workspace headers.
   - Embeds Bayesian Epistemic Decision Matrices into generated ADRs and Executive Memos.

### Active Roadmap Capabilities (v2.0)
10. **Autonomous Artifact Sandbox & Code Verification Engine (Planned - v2.0)**:
    - Multi-language isolated micro-sandbox (Go, Python, TypeScript, Rust, SQLite, Bash) with process group cancellation and ephemeral scrubbed runtimes.
    - Automated closed-loop verification: Compiler errors and test panics feed directly back into council rounds as empirical challenges.
    - Interactive Sandbox Terminal Drawer (`Cmd+Shift+X` / `Ctrl+Shift+X`) in Desktop and Mobile with live stdout/stderr streams and runtime selector.
    - Proof-of-execution verification badges embedded into exported ADRs and Executive Memorandums.

---

## 📄 License

Dialex AI is licensed under the [PolyForm Noncommercial License 1.0.0](file:///LICENSE).  
Copyright (c) 2026 Keltstone Systems. Free for personal, academic, and noncommercial research. Commercial deployments require an enterprise license from Keltstone Systems.
