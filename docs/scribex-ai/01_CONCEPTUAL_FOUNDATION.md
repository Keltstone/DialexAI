# Scribex AI: Conceptual Foundation & Epistemic Grounding

**Module:** `scribex-ai`  
**Author:** Keltstone Systems Core Engineering & Knowledge Architecture Group  
**Audience:** Software Architects, Engineering Directors, Technical Writers, DevSecOps Leads  
**Scope:** Core philosophy, documentation decay dynamics, AST grounding, zero-token gating, and deterministic truth verification.

---

## 1. The Universal Dilemma of Documentation Decay

In modern software engineering, code and documentation exist in permanent epistemic tension:

1. **Code is dynamic and strictly enforced:** Compilers, linters, unit tests, and CI/CD pipelines reject code changes that fail syntactic or semantic verification. Code moves fast because its execution is deterministic.
2. **Documentation is passive and advisory:** Specifications, READMEs, architecture decision records (ADRs), API references, runbooks, and enterprise wikis (Confluence, Notion) have no automated compiler. They depend entirely on human discipline.

### The Lifecycle of Documentation Decay ("The Documentation Graveyard")

Without deterministic coupling, all documentation follows an unavoidable degradation curve:

```
Day 1: Perfect Docs  ──► PRs Change Signatures ──► Docs Not Updated ──► Drift Accumulates ──► Complete Disregard
  (Author writes docs)    (Engineers skip doc edits)    (CI remains green)      (Mismatches cause bugs)  (Docs become harmful noise)
```

- **The Phantom Spec Problem:** Engineers read documentation describing function contracts, configuration flags, or error states that were refactored or deleted months earlier.
- **The Context Hoarding Bottleneck:** When documentation loses credibility, junior engineers and cross-functional partners are forced to interrupt senior engineers or reverse-engineer raw source code to understand system behavior.
- **The Audit Scramble:** During compliance audits (SOC2, ISO 27001, FedRAMP), organizations spend hundreds of engineering hours attempting to reconcile static Confluence architecture diagrams and Jira user stories with actual production code.

**Scribex AI** was engineered to resolve this fundamental decoupling. It transforms passive documentation into a **continuously verified, living contract**.

---

## 2. Core Epistemic Principles

Scribex AI operates under five sovereign design principles:

### 2.1 Deterministic Over Generative

```
                                  ┌────────────────────────┐
                                  │      Git PR Diff       │
                                  └───────────┬────────────┘
                                              │
                                              ▼
                                 ┌──────────────────────────┐
                                 │ Polyglot AST/Lex Scanner │
                                 └────────────┬─────────────┘
                                              │
                      ┌───────────────────────┴───────────────────────┐
                      ▼                                               ▼
         [Internal Logic Only]                           [Public Contract Changed]
                      │                                               │
                      ▼                                               ▼
           ┌─────────────────────┐                       ┌─────────────────────────┐
           │ Exit 0 (0 AI Tokens)│                       │ Deterministic Anchor    │
           │ Nothing to Update   │                       │ Graph Resolution        │
           └─────────────────────┘                       └────────────┬────────────┘
                                                                      │
                                                                      ▼
                                                         ┌─────────────────────────┐
                                                         │ AST Skeleton Extraction │
                                                         │ (~40 tokens signature)  │
                                                         └────────────┬────────────┘
                                                                      │
                                                                      ▼
                                                         ┌─────────────────────────┐
                                                         │ AI Prose Rewriter       │
                                                         │ (Bounded, Validated)    │
                                                         └─────────────────────────┘
```

Generative AI models are stochastic, prone to hallucinations, and sycophantic. Relying on an LLM to read an entire codebase and "guess" what documentation needs updating is prohibitively expensive, unreliable, and dangerous.

Scribex reverses this paradigm:
- **Deterministic parsers** (Go AST parser, Kotlin/Java/TypeScript/Python scanners) calculate exact symbol signatures and interface boundaries.
- **Deterministic cryptographic digests (16-hex SHA-256)** detect precisely whether a symbol's public surface has changed.
- **AI is invoked only at the final mile**: synthesizing precise, professional markdown prose for the localized paragraph anchored to that exact symbol.

### 2.2 The Zero-Token Impact Gate

In typical engineering repositories, over 70% of pull requests modify internal algorithms, private helper routines, test suites, or performance optimizations without modifying any public contracts or exported APIs.

Scribex enforces an impenetrable **Zero-Token Impact Gate**:
- When analyzing a PR (`scribex check -base origin/main`), Scribex calculates the AST delta between the head commit and the base branch.
- If no exported type, public function signature, configuration schema, or documented symbol has altered, Scribex exits immediately with code `0`.
- **Zero API calls** are made to LLM providers (Anthropic, OpenAI). No billable tokens are consumed, and no CI minutes are squandered.

### 2.3 Surgical AST Skeletons vs. Full Context Dumps

Conventional AI coding tools send entire multi-thousand-line files to LLMs, wasting millions of tokens and diluting model attention.

Scribex uses **AST Skeletonization**:
- Instead of feeding the whole function body (often 500–1,000 tokens), Scribex strips the implementation body and extracts only:
  - Exported symbol name
  - Parameter types and return values
  - Interface contracts and struct fields
  - Docstring/comments
- An AST skeleton typically consumes **less than 40 tokens**.
- The prompt provides the LLM with the new signature, the existing anchored prose, and an optional team style guide. The LLM's task is strictly confined to updating the explanatory prose to reflect the signature change.

### 2.4 In-Repo Sovereignty First, Enterprise Extended

- **In-Repo First:** The authoritative ground truth of any software system lives in the git repository. Scribex embeds non-intrusive HTML anchors (`<!-- @scribex:anchor(...) -->`) directly into standard Markdown files, READMEs, and wiki docs.
- **Enterprise Extended:** When organizations maintain external knowledge bases (Atlassian Confluence, Jira, Notion), Scribex projects repository truth outbound into these systems via declarative manifests (`.scribex/manifest.yaml`), creating draft revisions and audit matrices without making the external system the fragile master copy.

### 2.5 Epistemic Safety & All-or-Nothing Guarantees

AI-assisted prose rewriting (`scribex sync -ai`) treats LLM responses as untrusted data:
1. **No Instruction Following in Data:** Prompts strictly isolate source code and existing prose inside data delimiters (`<signature>`, `<prose>`). Prompt injections inside code comments cannot redirect model execution.
2. **Strict Structural Validation:** LLM output is parsed by a strict validator (`pkg/prose`). If the response introduces unauthorized Markdown fences, injects new HTML anchors, alters document boundaries, or leaks environment secrets, the rewrite is **instantly rejected**.
3. **All-or-Nothing Rollback:** If a single anchor fails validation, the document is kept bit-for-bit unchanged, keeping the old signature hash. The failure is reported as an issue, ensuring stale anchors are never silently masked.
4. **Transparent Audit Attribution:** Every AI-updated prose block is deterministically tagged with:
   ```markdown
   <!-- @scribex:ai sig=0123456789abcdef -->
   ```
   Human reviewers can instantly differentiate between human-written and machine-drafted documentation during pull request reviews.

---

## 3. The Code-to-Doc Topological Model

Scribex models documentation as an annotated dependency graph:

```
[Source Code AST]                [Cryptographic Link]           [Documentation Sinks]
pkg/auth/jwt.go                                                  docs/api/auth.md
└── func VerifyToken(t string) ──► sig=e3b0c44298fc1c14 ────────► <!-- @scribex:anchor(...) -->
                                                                 
                                                                 Confluence Cloud
                                                                 └── Page: "Auth Specification"
                                                                     (Draft Revision #42)
```

### The Anchor Grammar
Anchors link any Markdown block to an exact symbol in a source file:

```markdown
<!-- @scribex:anchor(pkg/auth/jwt.go#VerifyToken,sig=a1b2c3d4e5f67890) -->
### VerifyToken
Validates incoming JWT bearer tokens using the RS256 algorithm and verifies signature expiration against the authorization server clock.
<!-- /@scribex:anchor -->
```

- **File Path:** Clean relative path from repository root (`pkg/auth/jwt.go`).
- **Symbol:** Exact function, method, struct, interface, class, or type (`VerifyToken` or `AuthService.VerifyToken`).
- **Cryptographic Signature (`sig`):** 16-character hexadecimal SHA-256 digest of the normalized symbol signature.
- **Backward Compatibility:** Backward compatible with legacy 8-character hex hashes; `scribex sync` automatically upgrades legacy hashes to 16-character digests.

---

## 4. Scribex in the Dialex Suite Topology

Within the sovereign **Dialex Suite**, Scribex AI fulfills the mission of **Organizational Memory and Living Truth**:

| Engine | Primary Mission | Dialectic Role |
| :--- | :--- | :--- |
| **Socratix Engine** | Autonomous Deliberation & Epistemic Consensus | Multi-agent reasoning, thesis-antithesis synthesis, anti-sycophancy. |
| **Artix AI** | Autonomous Software Engineering & Sandboxed Patching | Multi-chamber spec deliberation and code generation in isolated worktrees. |
| **Kritix AI** | Autonomous Quality Assurance & Token-Optimized SDET | CDP browser automation, self-healing DOM locators, OWASP security fuzzing. |
| **Sentinix AI** | Continuous Vulnerability Review & Static Defense | PR change-delta security gating, credential leak defense, container sandboxing. |
| **Dialex AI Client** | Unified Visual Deliberation Cockpit | Desktop & mobile Compose application for human steering and governance. |
| **Scribex AI** | **Living Documentation & Knowledge Synchronization** | **AST grounding, zero-token doc drift prevention, enterprise wiki sync, SOC2 trace.** |

Together, these six engines form an unbroken circle of software creation, security, verification, and living documentation.
