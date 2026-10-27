# Dialex AI Client: User Guide & Deliberation Manual

**Client:** `dialex-ai` (Desktop & Android Compose)  
**Author:** Keltstone Systems Product & Experience Design Group  
**Audience:** Architects, Engineering Leaders, Strategists, End Users  
**Scope:** Desktop & Mobile user guide, Council setup, live deliberation arena, steering, deliverable exports, and pairing.

---

## 1. Installation & Client Setup

### 1.1 Desktop Client (macOS, Windows, Linux)
Standalone native packages are available under the releases portal:
- **macOS**: `Dialex-AI-1.0.0.dmg` (Universal binary: Apple Silicon M-series & Intel). Drag to `Applications`.
- **Windows**: `Dialex-AI-Setup-1.0.0.msi` / `Dialex.exe`.
- **Linux**: `Dialex-AI-1.0.0.AppImage` or `.deb`.

### 1.2 Android Mobile Client
- Install `Dialex-AI-1.0.0.apk` onto an Android 12+ device.
- Supports hardware biometric unlocking (Fingerprint / Face ID) to decrypt the local keyring vault.

---

## 2. Setting Up a Council Deliberation

When launching Dialex AI, click **"New Deliberation"** from the workspace drawer:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│  NEW DELIBERATION SETUP                                                                │
├────────────────────────────────────────────────────────────────────────────────────────┤
│  Topic / Core Question:                                                                │
│  [ Should we migrate our high-throughput logging pipeline from Kafka to Apache Pulsar? ]│
│                                                                                        │
│  Council Archetype Preset:                                                             │
│  (●) Systems Architecture Council    ( ) Security Red-Team Audit   ( ) FinOps / Cost   │
│                                                                                        │
│  Council Seats:                                                                        │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐  │
│  │ Seat 1: Principal Architect  [ Anthropic: Claude 3.5 Sonnet   ]  [DNA: Strict]   │  │
│  │ Seat 2: Staff SRE            [ OpenAI: GPT-4o                 ]  [DNA: Paranoia] │  │
│  │ Seat 3: FinOps Lead          [ Google: Gemini 2.0 Flash       ]  [DNA: Cost]     │  │
│  │ Seat 4: Security Auditor     [ DeepSeek: DeepSeek-R1          ]  [DNA: RedTeam]  │  │
│  │ Moderator: Dialex Synthesizer[ Anthropic: Claude 3.5 Haiku    ]  [DNA: Impartial]│  │
│  └──────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                        │
│  Deliberation Mode:                                                                    │
│  [●] Fixed Rounds: [ 3 ]   [ ] Convergence Driven (Paraconsistent Stop)                │
│                                                                                        │
│  Attached Reference Context:                                                           │
│  [ 📎 Attach Architecture Spec (.pdf, .md, .proto) ]  [ 📁 Link Codebase Workspace ]   │
│                                                                                        │
│                                                          [ 🚀 Launch Deliberation ]    │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Configuration Options
1. **Model & Provider Selection**: Mix and match cloud models (Anthropic, OpenAI, Gemini) and local models (Ollama, local CLI seats) with zero per-token markup.
2. **Persona DNA Adjustments**: Click the DNA badge on any seat to tweak epistemic strictness, skepticism, and modularity sliders.
3. **Attached Reference Documents**: Attach PDFs, RFCs, OpenAPI specs, or protobuf files. The engine automatically token-optimizes and grounds participant arguments in these documents.

---

## 3. The Live Deliberation Arena

Once launched, the screen transitions to the **Arena**:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│  ARENA: Kafka vs Apache Pulsar Migration       [Round 2/3] [● Live]   [⏸ Pause] [⏹ Stop]│
├────────────────────────────────────────────────────────────────────────────────────────┤
│  Active Dialectic Tensions:                                                            │
│  • [⚠️ OPEN] ZooKeeper/BookKeeper Ops Overhead vs Native Geo-Replication Multi-Tenancy  │
│  • [🤝 SYNTHESIZED] Client-Side Buffering Backpressure handled via Tiered Storage      │
├────────────────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────────────────────────────────┐  │
│  │ 🏛️ Principal Architect (Claude 3.5 Sonnet)                              10:14:02 │  │
│  │ Pulsar's tiered storage directly resolves our S3 archival cost bottleneck. By    │  │
│  │ offloading segments to object storage, we eliminate Kafka's expensive EBS volumes.│  │
│  └──────────────────────────────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────────────────────────────┐  │
│  │ 🛠️ Staff SRE (GPT-4o)                                                   10:14:28 │  │
│  │ @PrincipalArchitect: While tiered storage saves disk costs, you are ignoring the  │  │
│  │ operational complexity of managing BookKeeper and ZooKeeper quorum nodes in      │  │
│  │ Kubernetes. Kafka with KRaft has 0 external metadata dependencies.               │  │
│  └──────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                        │
│  [🎙️ Human Steering Input: "Address our team's lack of BookKeeper operational experience..."]│
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Live Steering Controls
- **Pause / Resume**: Instantly freeze the debate at the current turn boundary.
- **Human Injections**: Type directly into the steering bar at the bottom. Your comment is highlighted as an authoritative constraint in the next round.
- **Emergency Wrap-Up**: Click **Stop**, and the moderator synthesizes a final conclusion from existing rounds.

---

## 4. 1-on-1 Socratic Interview Mode (`/grill-me`)

For individual deep-dive stress testing of a nascent idea:
1. Navigate to **Socratic Studio**.
2. Select an interrogation stance:
   - **Classic Elenchus**: Hunts for logical contradictions in your claims.
   - **Maieutic Architect**: Extracts unstated architectural invariants.
   - **Adversarial Red Team**: Attacks your security assumptions.
3. The interviewer asks 3 probing questions, updating a live **Epistemic Ledger** (`HARDENED`, `UNDER_SIEGE`, `CONCEDED`).
4. At the conclusion, the app generates a **Socratic Digest** summarizing hardened axioms and exposed blind spots.

---

## 5. Exporting Production Deliverables

From the deliberation summary screen, click **Export Deliverable**:

1. **Architecture Decision Record (ADR)**: Exports a clean Markdown file (`docs/adr/ADR-001.md`) with Gherkin acceptance criteria.
2. **Weighted Decision Matrix**: Exports a CSV or Markdown matrix comparing options against weighted criteria.
3. **Executive Memorandum**: Generates an interactive, standalone HTML memo with styling and radar chart visualizations.

---

## 6. Mobile Companion Pairing (Android $\leftrightarrow$ Desktop)

To control deliberations on your desktop from your mobile phone:
1. On Desktop: Open **Settings $\rightarrow$ Mobile Companion $\rightarrow$ Pair Device**.
2. A secure cryptographic QR code appears on the screen (containing host IP or Tailscale address and ephemeral session token).
3. On Android: Tap **Scan to Pair**.
4. The mobile app establishes an encrypted mTLS / WireGuard session, streaming live arena turns with real-time push notifications.

---

*Next: Continue to [Dialex AI Client: Developer Guide & Build Pipeline](../dialex-ai/04_DEVELOPER_GUIDE_AND_BUILD_PIPELINE.md).*
