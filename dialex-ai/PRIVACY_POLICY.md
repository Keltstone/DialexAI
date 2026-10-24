# Privacy Policy & Data Sovereignty Charter

**Effective Date:** September 21, 2026  
**Version:** 1.0.0  
**Maintained by:** Kolta Labs (<https://github.com/Keltstone/DialexAI>)  
**Data Protection Contact:** `privacy@koltalabs.com`

---

## 1. Core Commitment: Local-First, No Telemetry

At **Kolta Labs**, we believe developer tools should keep your data on your machine and be transparent about what leaves it. **Dialex AI is local-first and ships without telemetry.**

Unlike conventional cloud-hosted SaaS tools, Dialex AI does not require an account, does not route your data through Kolta Labs servers, and does not collect usage analytics. This is a statement about our software, not an independent audit; the source is available for you to verify.

---

## 2. No Telemetry

To the best of our knowledge, and as far as the published source shows:

1. **No Tracking Pixels or Analytics Beacons:** Dialex AI contains no tracking pixels, behavioral telemetry hooks, or third-party analytics SDKs (such as Google Analytics, PostHog, Mixpanel, Segment, or Amplitude).
2. **No Crash Reporting Data Leaks:** Dialex AI does not automatically transmit stack traces, system logs, memory dumps, or error reports to remote cloud servers. Diagnostic logs remain in-memory and on your local device only.
3. **No Centralized User Profiles:** Kolta Labs maintains no central database of Dialex AI users, session frequencies, active discussion topics, or device identifiers.
4. **No Model Training on User Prompts:** Kolta Labs does not intercept, harvest, retain, or use your prompts, attachments, debate transcripts, or synthesized deliverables to train or fine-tune artificial intelligence models.

---

## 3. Local-First Data Residence & Storage Architecture

All operational data created or used within Dialex AI resides strictly on your local host filesystem and local SQLite database:

| Data Category | Storage Location | Retention & Custody |
|---|---|---|
| **Discussion Transcripts & Chat History** | Local SQLite Database & JSON Files (`~/Library/Application Support/Dialex/` or local app storage) | 100% under user custody; deleted instantly upon user command. |
| **Workspace Context & Project Files** | Local Filesystem | Never uploaded to Kolta Labs cloud servers. |
| **Custom Persona Definitions** | Local SQLite / JSON files | Managed locally; exportable by user. |
| **Diagnostic & Network Logs** | In-Memory Ring Buffer (`AppLogStore`) | Cleared on application termination; never transmitted externally. |
| **API Keys & Credentials** | Local state file, encrypted with AES-256-GCM; the key is stored in the OS keychain (macOS Keychain, Linux `secret-tool`) or, where unavailable, in a `0600` key file beside the data | The keychain path protects a copied data directory; the file fallback protects only against a copied state file alone. |

---

## 4. Network Data Transmission & Direct-to-Provider Transport

When you execute deliberations using Dialex AI, network communication occurs strictly under two models:

### A. Cloud Model Providers (Bring-Your-Own-Key)
When configured with cloud AI endpoints (including Anthropic Claude, OpenAI GPT, Google Gemini, xAI Grok, DeepSeek, or Mistral AI):
1. **Direct TLS Transport:** HTTP/REST and Server-Sent Event (SSE) requests originate directly from your local machine to the official, verified TLS/HTTPS API endpoints of the respective AI provider.
2. **No Intermediary Proxies:** Requests do **NOT** route through any server, proxy, or relay operated by Kolta Labs.
3. **Provider Privacy Policies:** Your transmission of data to cloud AI providers is governed by your direct commercial agreements with those providers. We encourage you to review their respective privacy commitments regarding zero-retention API endpoints.

### B. Local Foundation Models (100% Offline Air-Gapped Operation)
When configured with local model providers (such as Ollama, llama.cpp, or local developer CLI scripts):
1. **No Outbound Model Traffic:** Deliberations execute entirely on your local machine using loopback IPC (`localhost` or Unix domain sockets).
2. **Offline Operation:** The application requires zero internet connectivity, keeping your content on your machine. Verify your own model and runner configuration before using it with sensitive material.

### C. Optional Features That Contact Third Parties

These are user-initiated or passive downloads, not telemetry. Kolta Labs operates none of the Telegram or GitHub endpoints.

1. **Bug report / feedback (opt-in):** Sending feedback posts your description and up to 2,500 characters of the current session transcript to the Telegram Bot API, using a bot token and chat ID that you supply. Nothing is sent unless you press Send.
2. **Persona gallery:** Opening the gallery downloads a public JSON file from `raw.githubusercontent.com` (GitHub) and links to `dialex.dev/gallery`. The host can see your IP address and request time.
3. **Artix integrations:** Artix AI calls GitHub/GitLab and MCP connector APIs (for example Figma) only when you configure and invoke them.

---

## 5. Security & Cryptographic Protection of Secrets

1. **At-Rest Encryption:** Sensitive API tokens and credentials are encrypted at rest using industry-standard algorithms (AES-256-GCM with a random per-install key held in the OS keychain where available, else a separate local key file; no hardware-backed storage).
2. **Subprocess Sanitization:** When invoking local CLI runners (such as `claude`, `codex`, or `antigravity`), Dialex AI sanitizes the child process environment to prevent credential leakage into ambient shell variables.
3. **Memory Safety:** Authentication secrets are held in memory only for the duration of active requests and are cleared when sessions end.

---

## 6. User Rights & Complete Data Erasure

Because all deliberation data is stored locally on your device:

1. **Immediate Deletion:** Deleting a project, discussion, or credential within Dialex AI immediately and permanently purges the records from your local storage.
2. **No Tombstoning or Remote Backups:** Kolta Labs does not maintain shadow copies, backups, or tombstoned records of your deleted data.
3. **Full Portability:** You can export discussions, transcripts, and synthesized deliverables to standard Markdown (`.md`), JSON, or HTML formats at any time.

---

## 7. Children's Privacy

Dialex AI is not directed to children under the age of 18, and we do not knowingly collect personal information from individuals under the age of majority.

---

## 8. International Data Privacy Compliance (GDPR, CCPA)

1. **Data Controller Status:** As the operator of the local instance, You are the sole Data Controller of any data entered into Dialex AI.
2. **No Data Processor Exposure:** Because Kolta Labs does not receive, host, or process your deliberation payloads, Kolta Labs does not act as a Data Processor for your deliberation data under the EU General Data Protection Regulation (GDPR).
3. **Zero Sub-processors:** Kolta Labs engages zero sub-processors for application execution.

---

## 9. Updates to this Privacy Charter

Kolta Labs may update this Privacy Policy & Data Sovereignty Charter from time to time. Any revisions will be published in the source repository with an updated effective date and version number. Significant revisions will be highlighted in the application's **About & Legal** section and may require re-acknowledgment.

---

## 10. Contact Information

If you have questions, feedback, or concerns regarding our privacy practices or data sovereignty commitments, contact:

**Kolta Labs — Privacy & Governance**  
Email: `privacy@koltalabs.com`  
Repository: <https://github.com/Keltstone/DialexAI>
