# Artix AI Platform: User Guide & Cockpit Manual

**Platform:** `artix-ai` (CLI & Desktop Cockpit)  
**Author:** Keltstone Systems Developer Experience Group  
**Audience:** Software Engineers, Tech Leads, Product Owners  
**Scope:** CLI operations, TUI REPL, Compose Desktop Cockpit, IDE setup, and workflow playbooks.

---

## 1. Installation & Binary Setup

```bash
cd artix-ai

# Build the native CLI binary:
go build -ldflags="-s -w" -o /usr/local/bin/artix ./cli/main.go

# Build the webhook daemon:
go build -ldflags="-s -w" -o /usr/local/bin/artixd ./cmd/artixd/main.go

# Verify installation:
artix version
# Output: artix version 1.0.0
```

---

## 2. Interactive Terminal REPL (`artix repl`)

Launch the stateful developer shell:
```bash
artix repl
```

```
┌────────────────────────────────────────────────────────────────────────┐
│  ARTIX AI DEVELOPER REPL  v1.0.0                                       │
│  Connected: git@github.com:Keltstone/DialexAI.git [Branch: feature/auth]│
├────────────────────────────────────────────────────────────────────────┤
│  Type /help for slash commands. Use @file, @spec, @rule to attach context.│
│                                                                        │
│  artix> /grill-me "Add biometric FaceID authentication to mobile checkout"│
│                                                                        │
│  🎙️ Socratic Alignment Questions:                                     │
│  1. [PO Lead]: "What happens after 5 failed biometric attempts?"       │
│  2. [Architect]: "Where are biometric tokens isolated in memory?"     │
│  3. [QA Lead]: "What deterministic test commands verify bypass denial?" │
│                                                                        │
│  artix> Inspect @file:shared/src/auth/AuthManager.kt against @rule:taboo-sqlite│
└────────────────────────────────────────────────────────────────────────┘
```

### Context Mentions
- `@file:path/to/File.kt`: Injects exact file content into prompt context.
- `@spec:STORY-101`: Grounds the coder in a canonical specification.
- `@rule:taboo-sqlite`: Focuses verification on a specific architectural rule.
- `#symbol:RefreshToken`: Targets a specific function or class symbol.

### Slash Commands
- `/plan <story>`: Assembles Stakeholder Council to deliberate and generate `docs/specs/STORY-<id>.md`.
- `/code [spec]`: Executes Coder $\leftrightarrow$ Reviewer loop in an isolated shadow worktree.
- `/grill-me [story]`: Runs a 3-question alignment interview before planning.
- `/review`: Scans uncommitted working tree diffs against active Taboo Spaces.
- `/compact`: Prunes conversation history while preserving architectural invariants.
- `/undo`: Safely rolls back uncommitted modifications.

---

## 3. IDE Integration (VS Code, IntelliJ, Zed, Neovim)

Artix provides full real-time diagnostics via standard **Language Server Protocol (LSP 3.17)**:

### 3.1 Visual Studio Code Setup
In `.vscode/settings.json`:
```json
{
  "artix.lsp.enabled": true,
  "artix.lsp.path": "/usr/local/bin/artix",
  "artix.lsp.arguments": ["lsp"],
  "[kotlin]": {
    "editor.codeLens": true,
    "editor.codeActionsOnSave": {
      "source.fixAll": "explicit"
    }
  }
}
```

### 3.2 JetBrains IDEs (IntelliJ IDEA, Android Studio, GoLand)
1. Install the **LSP4IJ** plugin.
2. Under **Settings $\rightarrow$ Languages & Frameworks $\rightarrow$ Language Servers**, add:
   - **Name**: `Artix AI`
   - **Path**: `/usr/local/bin/artix`
   - **Arguments**: `lsp`
   - **File Types**: `.kt`, `.kts`, `.go`, `.java`, `.ts`

---

## 4. Compose Desktop Cockpit Application

For visual users, Artix provides a full-parity native Desktop Cockpit built with **Compose Multiplatform** and **KoltLibs**:

```bash
cd artix-ai
./gradlew :app:run
```

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│  ARTIX AI COCKPIT  v1.0.0            [● Council]  [Lab]  [Steering]  [Personas]  [Knowledge]    ⚙ Settings│
├────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  Story Prompt: [ Build user authentication flow with JWT refresh tokens                        ] [Plan]│
│  Style Vector: (●) Balanced (Standard)   ( ) Executive (Ponytail)   ( ) Technical (Caveman)            │
│  [🎙️ Toggle Socratic Alignment Interview ("Grill-Me")]                                                 │
│                                                                                                        │
│  Stakeholder Council Status:                                                                           │
│  [PO Lead: 95%]        [Architect: 90%]       [QA Lead: 85%]        [EM: 95%]                          │
│                                                                                                        │
│  ┌──────────────────────────────────────────────┬───────────────────────────────────────────────────┐  │
│  │ Live Stakeholder Deliberation Feed           │ Canonical Verified Story Spec (Markdown Preview)  │  │
│  │ • PO: "Tokens must refresh seamlessly."      │ # STORY-AUTH-001: JWT Authentication & Refresh    │  │
│  │ • Arch: "No JWT secrets in client storage."  │ ## Acceptance Scenarios (Gherkin)                 │  │
│  │ • QA: "Enforce expired token test suite."    │ Given valid refresh token When /api/refresh...    │  │
│  └──────────────────────────────────────────────┴───────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### The 5 Visual Workspaces
1. **Council Chamber (Chamber 1)**: Co-create specs with interactive `/grill-me` clarification drawers and live avatar influence streams.
2. **Engineering Lab (Chamber 2)**: Visual control center for autonomous code execution. Features an **Interactive Time-Travel Checkpoint Scrubber** to inspect diffs across rounds, a live test sandbox terminal, and a **1-Click Squash & Merge** button.
3. **Steering Studio**: Interactive grid binding Taboo rules to personas, plus a **PR-to-Rule Synthesizer** that turns PR review comments into permanent repo rules.
4. **Persona Studio**: Interactive radar chart visualizing 8-layer cognitive priors (*Strictness*, *Skepticism*, *Modularity*, *Velocity*, *Paranoia*).
5. **Knowledge Insights**: Explorer dashboard for self-evolving `.artix/knowledge/` items, breakthroughs, and learned architectural patterns.

---

*Next: Continue to [Artix AI Platform: Developer Guide & Extensibility](../artix-ai/04_DEVELOPER_GUIDE_AND_EXTENSIBILITY.md).*
