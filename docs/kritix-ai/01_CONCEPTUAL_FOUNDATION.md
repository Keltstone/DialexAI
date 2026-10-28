# Kritix AI Platform: Conceptual Foundation & Architecture

**Platform:** `kritix-ai` (Go 1.27, Web Studio, Compose Desktop)  
**Author:** Keltstone Systems Autonomous Testing & Quality Engineering Group  
**Audience:** QA Architects, SDET Leads, CISOs, Performance Engineers  
**Scope:** Deterministic-first token optimization, structural DOM pruning, self-healing locators, and enterprise QA reliability.

---

## 1. The Core Philosophy: Deterministic-First Token Optimization

Most AI testing platforms dump **full raw HTML DOMs** and network event streams into massive LLM context windows. This approach causes:
- **Runaway Token Costs**: \$10 to \$50 per test suite run ($200k+ annually for small teams).
- **Crippling Latency**: 30–60 second round-trips for a single click assertion.
- **Flaky Inconsistency**: Non-deterministic LLM reasoning leads to erratic test verdicts.

```
       TRADITIONAL AI TESTING AGENTS                     KRITIX AI ARCHITECTURE
  ┌─────────────────────────────────────┐         ┌─────────────────────────────────────┐
  │ Full HTML DOM (500,000+ characters) │         │ Full HTML DOM (500,000+ characters) │
  └──────────────────┬──────────────────┘         └──────────────────┬──────────────────┘
                     │                                               │
                     ▼                                               ▼
  ┌─────────────────────────────────────┐         ┌─────────────────────────────────────┐
  │  Dump 150,000 Tokens into Frontier  │         │ Deterministic Native Go Algorithms  │
  │  Cloud LLM ($1.50 per assertion)    │         │ (Pruning, Hashing, TOTP, Diffing)   │
  │  Latency: 20-45 seconds             │         │ Execution: <1ms | Cost: 0 TOKENS    │
  └──────────────────┬──────────────────┘         └──────────────────┬──────────────────┘
                     │                                               │
                     ▼                                               ▼
  ┌─────────────────────────────────────┐         ┌─────────────────────────────────────┐
  │  Non-Deterministic, Flaky Verdict   │         │ Compact Semantic Accessibility Tree │
  │                                     │         │ (<8,000 tokens) -> 95% SAVINGS!     │
  └─────────────────────────────────────┘         └─────────────────────────────────────┘
```

**Kritix reverses this model:**
1. **Deterministic Native Go First**: DOM pruning, state hashing, regex assertions, TOTP MFA calculations, and AST diffing run natively in Go at $<1\text{ ms}$ and **0 LLM tokens**.
2. **Structural DOM Pruning Engine**: Transforms full HTML DOMs into compact semantic accessibility trees before model submission, stripping decorative styling, SVGs, and non-interactive nodes (achieving up to **95% token compression**).
3. **Model & Provider Agnostic**: Can run against commercial APIs (GPT-4o, Claude 3.5), flat-rate developer CLI subshells (`claude`, `codex`, `agy`), or air-gapped on-premise local models (Ollama/vLLM with Qwen 2.5 Coder or Gemma 2).

---

## 2. Enterprise Governance Guarantees

To satisfy CISO and QA Architect requirements, Kritix enforces four enterprise guarantees:

### 2.1 Vault-Grade Session Management (`pkg/auth`)
- **Zero Plaintext Token Leaks**: Replaces static Playwright `storageState.json` files with **AES-256-GCM encrypted state** and an in-memory **EphemeralSessionStore** with automatic process wipe.
- **Strict TTL Expiration**: Session cookies and tokens automatically expire after configured TTL (default: 4 hours), preventing session hijacking on shared CI runners.

### 2.2 "Alert & Propose" Self-Healing Governance (`pkg/sdet`)
Traditional auto-healing silently clicks whatever element is near the broken selector, masking genuine UI regressions and generating false passes. Kritix enforces three governance modes:
- **`strict` (Default)**: If an exact locator (`test-id`, `id`) is broken, Kritix **does NOT silently click a shifted element**. It marks the regression and generates a proposed selector patch and diff.
- **`advisory`**: Executes against the best heuristic candidate but emits an audit warning for team review.
- **`permissive`**: Permissive auto-healing for rapid exploratory crawling.

> [!IMPORTANT]
> **Healed Pass Merging Rule**: A broken locator is only ever healed onto an element that shares its original visible text. Healed passes (`PASSED_WITH_HEALING`) **fail the CLI merge gate** unless `--allow-healed-override` is passed.

### 2.3 Quarantine SLA & Tech-Debt Prevention (`pkg/quarantine`)
- Intermittent / flaky tests isolated in quarantine are tagged with squad ownership and Jira/Linear ticket IDs.
- **Build-Failing SLA**: If a quarantined test exceeds its resolution SLA (e.g. 7 days), it **automatically fails the CI build**, preventing permanent accumulation of untested code.

### 2.4 Active-Scan Scope (Default-Deny)
Every block that sends traffic to a target (`exec.fuzz`, `exec.owasp-dast`, `perf.latency`, `exec.matrix`, `agent.vision-crawl`) refuses any host not explicitly listed in `KRITIX_ALLOWED_TARGETS`. If unset, all network requests are denied.

---

*Next: Continue to [Kritix AI Platform: Core Technical Architecture](../kritix-ai/02_CORE_TECHNICAL_ARCHITECTURE.md).*
