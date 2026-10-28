<div align="center">

<img src="logos/logo.png" alt="Dialex AI Logo" width="140" style="border-radius: 20px; margin-bottom: 16px;" />

# Dialex AI
### Sovereign Multi-AI Deliberation & Synthetic Advisory Platform

**Stop trusting a single AI for important decisions.**  
Dialex AI brings the world’s leading AI models into a structured debate council — stress-testing your ideas, exposing blind spots, and synthesizing balanced, evidence-backed conclusions.

<br/>

[![Kotlin](https://img.shields.io/badge/Kotlin-2.1+-7F52FF.svg?style=for-the-badge&logo=kotlin&logoColor=white)](https://kotlinlang.org)
[![Compose Multiplatform](https://img.shields.io/badge/Compose_Multiplatform-Desktop_%26_Mobile-4285F4.svg?style=for-the-badge&logo=jetpackcompose&logoColor=white)](https://www.jetbrains.com/lp/compose-multiplatform/)
[![Go Engine](https://img.shields.io/badge/Go_Engine-Fast_%26_Lightweight-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![Local Privacy](https://img.shields.io/badge/Privacy-100%25_Local_Vault-00C853.svg?style=for-the-badge&logo=lock&logoColor=white)](#privacy-first--data-sovereignty)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm_Noncommercial_1.0.0-orange.svg?style=for-the-badge)](LICENSE)

<br/>

[Why Dialex?](#the-problem-the-single-ai-echo-chamber) · [How It Works](#how-dialex-ai-works) · [Who It's For](#who-is-dialex-ai-for) · [Key Features](#what-makes-dialex-ai-different) · [Interface Tour](#interface-tour) · [Quick Start](#quick-start--installation) · [Documentation](#full-documentation-hub)

</div>

---

## 🚀 Release Details (v1.0.0)

- **Current Version:** `1.0.0`
- **Release Changelog:** [CHANGELOG.md](CHANGELOG.md)
- **Supported Platforms:** macOS (`.dmg`), Windows (`.msi`, `.exe`), Linux (`.deb`, `.rpm`), Android (`.apk`)
- **Quick Install via Homebrew:**
  ```bash
  # Dialex AI Desktop with bundled engine
  brew install --cask dialex-ai-bundled

  # Dialex AI Client only (connects to external engine)
  brew install --cask dialex-ai
  ```

---

## The Problem: The Single AI "Echo Chamber"

When you ask a standard AI chatbot for advice on a major decision, it usually acts like an agreeable assistant. It nods along, tells you what sounds exciting, and confirms your initial bias:

```text
The Single-AI Trap:
You:       "We are thinking of rebuilding our entire database architecture next month."
Single AI: "That sounds like a brilliant, forward-thinking move! Here are 5 ways to start..."
Reality:   Six months later — unexpected downtimes, soaring costs, and painful operational hurdles.
```

### Why relying on one AI model is risky:
* **The "Yes-Man" Tendency (Sycophancy)**: Mainstream AI models are trained to be helpful and polite, which often means validating your premise instead of pointing out critical flaws.
* **Hidden Blind Spots**: Every AI model has unique biases and gaps based on how it was trained. Consulting just one gives you an incomplete picture.
* **Unchallenged Mistakes**: If a single AI makes an error in turn one, it will confidently defend that error through the rest of the conversation.
* **Lack of Accountability**: You receive an opinion, but not a rigorous evaluation backed by healthy cross-examination.

---

## How Dialex AI Works

Dialex AI replaces single-model chat with **collaborative peer deliberation**. Instead of talking to just one AI, you convene a **Council of AIs** that review your question from different perspectives, challenge one another, and arrive at a well-reasoned consensus.

```text
                           ┌─────────────────────────────────────────┐
                           │            YOUR BIG QUESTION            │
                           │  "Should we launch this new strategy?"  │
                           └────────────────────┬────────────────────┘
                                                │
                     ┌──────────────────────────┴──────────────────────────┐
                     ▼                                                     ▼
        ┌─────────────────────────┐                           ┌─────────────────────────┐
        │   1. THE PROPOSAL       │                           │   2. THE CRITIQUE       │
        │   The Optimist / Expert │                           │   The Contrarian / SRE  │
        │   Outlines the upside,  │◄─────────────────────────►│   Attacks weak points,  │
        │   benefits, and best-   │      PEER SCRUTINY &      │   exposes hidden costs, │
        │   case opportunities.   │     CROSS-EXAMINATION     │   and stresses hazards. │
        └─────────────────────────┘                           └─────────────────────────┘
                     │                                                     │
                     └──────────────────────────┬──────────────────────────┘
                                                │
                                                ▼
                           ┌─────────────────────────────────────────┐
                           │            3. THE SYNTHESIS             │
                           │               The Moderator             │
                           │   • Separates proven facts from hype    │
                           │   • Highlights surviving arguments      │
                           │   • Delivers an objective verdict       │
                           └────────────────────┬────────────────────┘
                                                │
            ┌───────────────────┬───────────────┴───────────────┬───────────────────┐
            ▼                   ▼                               ▼                   ▼
       Action Plan       Decision Matrix                 Pro/Con Table          Exec Memo
```

### The 3 Stages of Deliberation:
1. **The Proposal**: Different AI debaters (such as *The Strategist*, *The Domain Expert*, or *The Optimist*) examine the situation and present initial arguments.
2. **The Cross-Examination**: Opposing AI personas (such as *The Contrarian*, *The Risk Auditor*, or *The Pragmatist*) rigorously interrogate those points, poking holes in weak assumptions and highlighting overlooked risks.
3. **The Synthesis**: A neutral AI *Moderator* weighs the competing viewpoints, filters out conversational fluff, measures which arguments held up to scrutiny, and distills an actionable, balanced consensus.

---

## Who Is Dialex AI For?

Dialex AI is built for anyone who needs to make sound, defensible decisions in high-stakes situations:

* 🎓 **Scholars & Researchers**: Stress-test research hypotheses, discover alternative perspectives in academic debates, critique literature, and avoid personal confirmation bias.
* 🏛️ **Executives, Founders & Strategists**: Assemble an on-demand "synthetic board of advisors" to evaluate capital investments, go-to-market strategies, and organizational policies before committing real resources.
* 💻 **Engineers & Architects**: Compare technical trade-offs (e.g., choosing between tech stacks, evaluating security architectures, or auditing complex code changes) with balanced pro/con analysis.
* ✍️ **Writers, Analysts & Thinkers**: Explore multifaceted topics, refine debate arguments, polish essays, and discover nuances you might otherwise miss.

---

## What Makes Dialex AI Different?

| Feature | Dialex AI | Standard Chat AI (ChatGPT / Claude) | Developer Frameworks (AutoGen / CrewAI) |
|---|:---:|:---:|:---:|
| **Debate Format** | **Multi-Model Council** (Claude vs GPT vs Gemini vs DeepSeek) | Single AI talking to itself | Code-heavy scripts |
| **Perspective Diversity** | **40+ specialized roles & debate personas** | Single default voice | Requires writing Python code |
| **Privacy & Sovereignty** | **100% Local Vault** (data never leaves your machine) | Data retained in provider clouds | Depends on hosting |
| **Cost Control** | **$0 extra fees** (use free developer CLIs or direct API keys) | Monthly subscriptions per model | High API usage costs |
| **Interactive Interruption** | **Live steering queue** (interject or redirect anytime) | Stop and re-type prompt | Terminal scripts crash or block |
| **Actionable Deliverables** | **1-Click Executive Memos, Decision Matrices & Plans** | Plain chat text | Raw JSON / text outputs |
| **Accessibility** | **Clean Desktop & Mobile App** (macOS, Windows, Linux, Android) | Web browser | Command-line only |

---

## Key Highlights

### 🤝 Multi-Model Intelligence Under One Roof
Connect multiple frontier models in the same room:
* **Anthropic** (Claude models)
* **OpenAI** (GPT models)
* **Google** (Gemini models)
* **DeepSeek** (reasoning and chat models)
* **xAI** (Grok)
* **Mistral** (Large & Codestral)
* **Local Offline Models** (via Ollama for 100% offline work)

### 🔒 Privacy-First & Data Sovereignty
Your questions, debate transcripts, and API keys are stored locally on your computer; API keys are encrypted at rest (the key file sits on the same machine; OS keychain support is planned). There is no Dialex cloud server in the path. Content you send to a cloud model provider is governed by that provider's terms.

### 💰 Direct Pricing, Zero Markup
Dialex AI is not a middleman selling expensive tokens. You can connect your existing free development CLIs (like `claude`, `codex`, or `antigravity`) or bring your own API keys. You only pay provider rates directly, with no hidden subscription markup.

> **Recommended path:** direct API keys or local Ollama. Driving a vendor CLI (`claude`, `codex`, ...) depends on that vendor's terms of service, which may restrict automated use of a consumer subscription, and the CLIs can change and break. Check your plan's terms before relying on it.

### 📑 1-Click Executive Deliverables
Once a deliberation concludes, you don't have to scroll through walls of text. Click one button to export:
* **Executive Memorandum**: A formal, publication-ready summary ready to share with stakeholders.
* **Decision Matrix**: A structured scoring table weighing competing options across your criteria.
* **Pros & Cons Comparison**: Clear trade-offs with risk levels identified.
* **Implementation Plan**: Step-by-step roadmap to put the conclusion into practice.

### 🧭 Two Modes of Inquiry
1. **Council Deliberation**: Multiple AI personas deliberate across synchronized rounds guided by an impartial moderator.
2. **Socratic Mode**: A focused, one-on-one session where the AI acts as a philosophical examiner, challenging your assumptions with brief, piercing questions to sharpen your reasoning.

---

## Interface Tour

<div align="center">
  <img src="docs/screenshots/02_deliberation_chat_transcript.png" alt="Dialex AI Deliberation Workspace" width="94%" style="border-radius: 12px; box-shadow: 0 8px 32px rgba(0,0,0,0.35);" />
  <p><em>The Live Deliberation Workspace: Color-coded council seats, round-by-round progress, and a live queue to interject with your own questions at any time.</em></p>
</div>

<br/>

<table align="center" width="100%">
  <tr>
    <td width="50%" align="center">
      <img src="docs/screenshots/03_consensus_outcome_deliverables.png" alt="Consensus Deliverables" width="100%" style="border-radius: 8px;" />
      <br/><em><strong>Consensus & Deliverables:</strong> Review the agreed outcome and generate formatted decision memos, matrices, or action plans with one click.</em>
    </td>
    <td width="50%" align="center">
      <img src="docs/screenshots/04_artifacts_sliding_drawer.png" alt="Artifacts Drawer" width="100%" style="border-radius: 8px;" />
      <br/><em><strong>Artifacts Drawer:</strong> Access, copy, or export generated reports, markdown documents, and code artifacts instantly.</em>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <img src="docs/screenshots/07_personas_registry_and_custom_library.png" alt="Persona Library" width="100%" style="border-radius: 8px;" />
      <br/><em><strong>Persona Library:</strong> Select from over 40 pre-built roles (e.g., The Optimist, Risk Auditor, Legal Analyst) or create your own custom expert.</em>
    </td>
    <td width="50%" align="center">
      <img src="docs/screenshots/05_mobile_qr_pairing_modal.png" alt="Mobile QR Pairing" width="100%" style="border-radius: 8px;" />
      <br/><em><strong>Mobile Companion:</strong> Pair your Android phone in seconds using a local QR code to follow debates and vote on outcomes on the go.</em>
    </td>
  </tr>
</table>

---

## Ready-to-Use Scenarios (Playbooks)

Dialex includes ready-to-run **Playbooks**—pre-configured debate templates with proven combinations of roles, questions, and deliverables:

* 🏛️ **Strategic Planning & Capital Allocation**: Pair an ambitious growth strategist with a conservative risk auditor to evaluate business investments and new market entries.
* 🛡️ **Risk Assessment & Threat Modeling**: Bring in a security red-team agent to expose vulnerabilities in software systems or organizational workflows.
* 🔬 **Scientific & Academic Inquiry**: Pit alternative scientific hypotheses against each other with a moderator tracking empirical evidence.
* ⚙️ **Architecture & Technology Choices**: Objectively evaluate competing technologies (e.g., cloud vs. on-premise, monolith vs. microservices) without vendor hype.

*Read more in our [Enterprise Use Cases & Playbooks Guide](docs/USE_CASES.md).*

---

## Quick Start & Installation

> **Status:** no prebuilt installers are published yet (no `.dmg`, `.msi`, `.deb`, `.rpm`, APK, Homebrew or winget package). Build from source below.


### Docker & Developer Builds

<details>
<summary><strong>🐳 Self-Hosting with Docker Compose</strong></summary>

```bash
# Clone the repository
git clone https://github.com/Keltstone/DialexAI.git && cd DialexAI

# Start the background engine with persistent storage
docker compose -f selfhosting/docker-compose.yml up -d
```
The server will be available on `http://localhost:8787` (or across your private network via Tailscale).
</details>

<details>
<summary><strong>🛠️ Building from Source (Developers)</strong></summary>

```bash
# 1. Clone Kolt and DialexAI side-by-side in your workspace
git clone https://github.com/keltstone/Kolt.git
git clone https://github.com/Keltstone/DialexAI.git

# 2. Run tests and start the Desktop application
cd DialexAI
./gradlew desktopTest
./gradlew :desktopApp:run
```
*For detailed setup instructions, see the [Developer Guide](docs/DEVELOPER_GUIDE.md).*
</details>

---

## Full Documentation Hub

Want to dive deeper into the design, mathematics, or technical guides? Explore the complete documentation library:

| Guide | Description | Audience |
|---|---|---|
| 📖 [**User Guide & Manual**](docs/USER_GUIDE.md) | Step-by-step walkthrough of council setup, debate controls, and mobile pairing. | Everyone |
| 🎯 [**Use Cases & Playbooks**](docs/USE_CASES.md) | Ready-to-use prompts, team compositions, and scenario templates. | Decision Makers & Scholars |
| 🧠 [**Product Philosophy**](docs/PRODUCT_PHILOSOPHY.md) | The reasoning principles, anti-fluff rules, and philosophy behind Dialex. | Scholars & Curious Thinkers |
| 💼 [**Executive Overview**](docs/EXECUTIVE_OVERVIEW.md) | Business justification, ROI, and risk reduction for teams and enterprises. | Executives & Leaders |
| 🏗️ [**System Architecture**](docs/ARCHITECTURE.md) | Detailed technical breakdown of the Kotlin UI, Go engine, and encryption vault. | Engineers & Architects |
| 🖥️ [**Server Setup Guide**](docs/SERVER_SETUP.md) | How to deploy the Go deliberation engine on servers, systemd, or Docker. | DevOps & SysAdmins |
| 💻 [**Client Setup Guide**](docs/CLIENT_SETUP.md) | Platform-specific instructions for Desktop and Android installations. | Everyone |
| 🛠️ [**Developer Guide**](docs/DEVELOPER_GUIDE.md) | How to contribute, add model providers, and customize the Kolt framework. | Contributors & Developers |
| 🔌 [**API Reference**](docs/API_REFERENCE.md) | Complete documentation of REST endpoints, SSE streams, and authentication. | Integrators & Developers |

---

## Community & Contributing

We welcome contributions from researchers, software developers, writers, and designers:
* **Report Bugs & Suggest Features**: Open an issue on our GitHub repository.
* **Contribute Code**: Check out our [Developer Guide](docs/DEVELOPER_GUIDE.md) and submit a pull request.
* **Support Our Work**: If Dialex AI helps you make better decisions, consider starring the repository ⭐ or [sponsoring on GitHub](https://github.com/sponsors/Keltstone).

---

## License & Usage

Dialex AI is **source-available and dual-licensed** by Keltstone Systems. It is not OSI open source.

* ✅ **Noncommercial use (free)**: personal, academic, educational, research and non-profit use under the **PolyForm Noncommercial License 1.0.0**.
* 💼 **Commercial use**: using it in or for a business, or offering it to customers, requires a commercial license. See [COMMERCIAL_LICENSE.md](../COMMERCIAL_LICENSE.md) or email `licensing@keltstone.com`.

*For full legal terms, see the [LICENSE](LICENSE) file.*

---

<div align="center">
  <sub>Copyright &copy; 2026 <strong>Keltstone Systems</strong> · Empowering thoughtful, sovereign decisions.</sub>
</div>
