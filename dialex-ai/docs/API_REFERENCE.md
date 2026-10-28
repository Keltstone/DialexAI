# Dialex AI REST & Server-Sent Events (SSE) API Reference

The **Dialex AI Go Orchestration Engine** exposes a high-performance HTTP REST API and real-time Server-Sent Events (SSE) stream on port `8080` (by default).

---

## 1. Authentication & Headers

Protected endpoints require a JSON Web Token (JWT) passed in the `Authorization` header:

```http
Authorization: Bearer <jwt_token>
Content-Type: application/json
```

---

## 2. Authentication Endpoints

### 2.1 Login & Obtain Token
`POST /api/auth/login`

#### Request:
```json
{
  "username": "admin",
  "password": "your_password"
}
```

#### Response (`200 OK`):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": 1758153600,
  "user": {
    "id": "u_01j9a",
    "username": "admin",
    "role": "administrator"
  }
}
```

---

## 3. Deliberation Management Endpoints

### 3.1 List Deliberations
`GET /api/debates`

Returns all active and archived debate discussions.

#### Response (`200 OK`):
```json
[
  {
    "id": "disc_01j9a123",
    "name": "Microservices vs Modular Monolith",
    "status": "RUNNING",
    "created_at": 1758067200,
    "updated_at": 1758067400,
    "round_count": 2,
    "max_rounds": 5
  }
]
```

---

### 3.2 Create Deliberation
`POST /api/debates`

#### Request:
```json
{
  "name": "Kafka vs Pulsar Architecture",
  "topic": "Should we migrate from Kafka to Apache Pulsar for real-time telemetry?",
  "context": "Current throughput: 50k msgs/sec. Need multi-tenancy and tiered S3 storage.",
  "config": {
    "maxRounds": 4,
    "primary": {
      "provider": "ANTHROPIC",
      "model": "claude-3-7-sonnet",
      "mode": "API",
      "role": "The Facilitator",
      "personaId": "sys_facilitator"
    },
    "secondary": {
      "provider": "OPENAI",
      "model": "gpt-4o",
      "mode": "API",
      "role": "Devil's Advocate",
      "personaId": "sys_devils_advocate"
    },
    "tertiary": {
      "provider": "GOOGLE",
      "model": "gemini-2.0-flash",
      "mode": "API",
      "role": "The Pragmatist",
      "personaId": "sys_pragmatist"
    },
    "humanDialogue": true,
    "topicDriftGuard": true
  }
}
```

#### Response (`201 Created`):
```json
{
  "id": "disc_01j9a123",
  "status": "DRAFT",
  "message": "Discussion created successfully"
}
```

---

### 3.3 Start / Pause / Stop Deliberation
- `POST /api/debates/{id}/start`: Initiates or resumes the debate turn loop.
- `POST /api/debates/{id}/pause`: Pauses deliberation cleanly after the current turn completes.
- `POST /api/debates/{id}/stop`: Immediately stops deliberation and requests final summary synthesis.

---

## 4. Live Human Interventions

### 4.1 Queue Comment for Next Round
`POST /api/debates/{id}/comment`

Queues a comment into the orchestrator's thread-safe interjection queue. It will be drained and injected as a user prompt turn before the next round begins.

#### Request:
```json
{
  "content": "Assume our infrastructure budget is capped at $5,000/month."
}
```

#### Response (`200 OK`):
```json
{
  "status": "QUEUED",
  "position": 1
}
```

---

### 4.2 Immediate Interrupt
`POST /api/debates/{id}/interrupt`

Immediately cancels the in-flight generation context of the active agent, safely finalizes the partial message, and forces the user's interjection to be processed immediately.

#### Request:
```json
{
  "content": "Stop. Do not consider Cassandra; we have already selected PostgreSQL."
}
```

#### Response (`200 OK`):
```json
{
  "status": "INTERRUPTED",
  "turn_finalized": true
}
```

---

## 5. Alternative Deliverables

### 5.1 Generate Alternative Deliverable
`POST /api/debates/{id}/deliverable`

Prompts the moderator model to synthesize an alternative deliverable format based on the full debate transcript.

#### Request:
```json
{
  "format": "DECISION_MATRIX"
}
```

*Valid formats: `ACTION_PLAN`, `DECISION_MATRIX`, `PRO_CON_LIST`, `EXECUTIVE_BRIEF`, `DECISION_SUMMARY`, `CUSTOM_FORMAT`.*

#### Response (`202 Accepted`):
```json
{
  "status": "SYNTHESIZING",
  "format": "DECISION_MATRIX"
}
```

---

## 6. Real-Time Server-Sent Events (SSE) Stream

### `GET /api/debates/{id}/stream`
Streams real-time events from the turn orchestrator.

#### Headers Required:
```http
Accept: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

### Event Specifications

#### 1. `event: debate_started`
Fired when the debate state machine initializes.
```
event: debate_started
data: {"id":"disc_01j9a123","timestamp":1758067200}
```

#### 2. `event: agent_turn_started`
Fired when an agent begins formulating a turn.
```
event: agent_turn_started
data: {"seat_index":0,"provider":"ANTHROPIC","model":"claude-3-7-sonnet","role":"The Facilitator","round":1}
```

#### 3. `event: agent_token_chunk`
Fired for every raw text delta streamed from the model or CLI.
```
event: agent_token_chunk
data: {"seat_index":0,"delta":"When evaluating Apache Pulsar, "}
```

#### 4. `event: agent_turn_completed`
Fired when an agent turn finishes.
```
event: agent_turn_completed
data: {"seat_index":0,"message_id":"msg_01j9a","token_count":342,"duration_ms":1820}
```

#### 5. `event: consensus_reached`
Fired when agents reach consensus or when the final round completes.
```
event: consensus_reached
data: {"consensus_score":0.92,"synthesizer_seat":0}
```

#### 6. `event: deliverable_generated`
Fired when the Moderator Consensus Outcome Bubble or an alternative deliverable is produced.
```
event: deliverable_generated
data: {"format":"CONSENSUS_OUTCOME","markdown_content":"# Consensus Outcome\n..."}
```

#### 7. `event: error`
Fired when a provider returns a rate limit, authentication error, or timeout.
```
event: error
data: {"code":"RATE_LIMIT_EXCEEDED","provider":"OPENAI","message":"RPM limit reached. Retrying in 12s."}
```

---

## 5. Benchmarking & Quantitative Evaluation Endpoints

### 5.1 List Benchmark Dilemmas
`GET /api/v1/benchmarks/cases`

Returns all bundled canonical cases (`DB01`–`DB10`) and custom-authored cases.

#### Response (`200 OK`):
```json
[
  {
    "id": "DB01",
    "title": "Event-Driven vs CQRS in High-Throughput Financial Ledger",
    "domain": "Distributed Systems",
    "dilemma": "...",
    "constraints": ["Zero data loss", "<50ms p99 write latency"],
    "groundTruthTraps": ["Two-phase commit failure cascades"],
    "requiredTradeOffAxes": ["Eventual Consistency vs Immediate Read-After-Write"],
    "isBundled": true
  }
]
```

### 5.2 Create Custom Dilemma
`POST /api/v1/benchmarks/cases`

#### Request:
```json
{
  "id": "custom-kafka-vs-pulsar",
  "title": "Kafka vs Apache Pulsar for Tier-1 Telemetry",
  "domain": "Messaging & Streaming",
  "dilemma": "Architectural trade-off analysis under 5M msg/sec.",
  "constraints": ["Multi-tenancy", "Tiered cloud storage"],
  "groundTruthTraps": ["ZooKeeper vs KRaft migration overhead"],
  "requiredTradeOffAxes": ["Operational Simplicity vs Storage Decoupling"]
}
```

### 5.3 Trigger Dual-Arm Benchmark Run
`POST /api/v1/benchmarks/run`

Concurrently executes Arm A (Solo Frontier Model) and Arm B (Dialex AI Multi-Agent Council), followed by double-blind position-swapped LLM judge scoring.

#### Request:
```json
{
  "caseId": "DB01",
  "rounds": 2
}
```

#### Response (`200 OK`):
```json
{
  "id": "run_01j9a100",
  "caseId": "DB01",
  "caseTitle": "Event-Driven vs CQRS in High-Throughput Financial Ledger",
  "timestamp": 1758153600000,
  "soloResult": {
    "armType": "SOLO_BASELINE",
    "modelOrCouncil": "claude-3-7-sonnet",
    "deliverable": "...",
    "tokensUsed": 1820,
    "durationMs": 4200
  },
  "councilResult": {
    "armType": "COUNCIL",
    "modelOrCouncil": "Dialex 3-Agent Council",
    "deliverable": "...",
    "tokensUsed": 4600,
    "durationMs": 9800
  },
  "evaluations": [
    {
      "passNumber": 1,
      "order": "AB",
      "soloScores": [{"dimension": "FACTUALITY", "score": 7.5}],
      "councilScores": [{"dimension": "FACTUALITY", "score": 9.0}],
      "overallVerdict": "Council wins due to superior trade-off depth."
    }
  ],
  "soloTotalScore": 7.4,
  "councilTotalScore": 9.1,
  "deltaQ": 1.7,
  "winner": "COUNCIL"
}
```

### 5.4 List Historical Benchmark Runs
`GET /api/v1/benchmarks/runs`

### 5.5 Get Benchmark Statistical Summary
`GET /api/v1/benchmarks/summary`

Returns aggregated win rates, mean $\Delta Q$, and paired Student's $t$-test $p$-value.

#### Response (`200 OK`):
```json
{
  "totalRuns": 12,
  "councilWins": 11,
  "soloWins": 1,
  "ties": 0,
  "councilWinRate": 91.67,
  "meanDeltaQ": 1.82,
  "pValue": 0.0034,
  "isStatSignificant": true,
  "avgFactualityDelta": 1.5,
  "avgBlindSpotDelta": 2.1,
  "avgTradeOffDelta": 1.9,
  "avgActionDelta": 1.8
}
```

### 5.6 Export Benchmark Report
`GET /api/v1/benchmarks/export?format=markdown`

Supports query parameter `format=markdown`, `format=csv`, or `format=json`.

---

## 6. 8-Layer Cognitive DNA & MMOS Endpoints

### 6.1 Get Persona DNA Matrix
`GET /api/v1/personas/{id}/dna`

Returns the complete 8-layer cognitive DNA structure for a given persona identifier.

#### Response (`200 OK`):
```json
{
  "schemaVersion": "dialex.dna/v1.0",
  "id": "the-risk-analyst",
  "name": "The Risk Analyst",
  "role": "Failure Mode Specialist",
  "category": "Reliability & Risk",
  "coreIdentity": {
    "title": "Principal Reliability & Failure Mode Analyst",
    "background": "Deep systems engineering & formal analysis",
    "domainAuthority": "Distributed systems, fault tolerance, reliability engineering",
    "credentials": ["Formal Methods", "Fault Tolerance", "RFC Author"]
  },
  "epistemicBias": {
    "primaryMode": "FIRST_PRINCIPLES",
    "theoryVsPractice": 0.6,
    "noveltyVsProvenance": 0.8,
    "safetyVsVelocity": 0.85,
    "rigorThreshold": 0.9
  },
  "communicationVector": {
    "tone": "CONCISE_INCISIVE",
    "formalityLevel": 4,
    "targetSentenceCeiling": 4,
    "rhetoricalDevices": ["reductio-ad-absurdum", "inversion"],
    "syntaxPattern": "structured-bulleted"
  },
  "heuristicLibrary": [
    {
      "id": "heur_gall",
      "name": "Gall's Law",
      "formulaOrMaxime": "A complex system that works is invariably found to have evolved from a simple system that worked.",
      "triggerCondition": "System Architecture",
      "applicationDirective": "Start with a working simple system."
    }
  ],
  "tabooSpace": {
    "forbiddenArguments": ["hand-waving", "appeal-to-popularity"],
    "rejectedFallacies": ["sunk-cost", "false-dichotomy"],
    "intolerableBuzzwords": ["synergy", "paradigm-shift", "turnkey"],
    "penaltyAction": "IMMEDIATE_REFUTATION"
  },
  "domainOntology": {
    "mandatoryStandards": ["RFC 793", "CAP Theorem", "PACELC"],
    "authoritativeRFCs": ["RFC-1122", "RFC-7540"],
    "specializedLexicon": ["failure-domain", "split-brain", "backpressure"],
    "enforceFormalCitations": true
  },
  "adversarialPosture": {
    "stance": "SOCRATIC_INVERTER",
    "tenacityScore": 0.8,
    "counterAttackMethod": "IDENTIFY_HIDDEN_AXIOM_AND_DISPROVE",
    "concedeCondition": "RIGOROUS_EMPIRICAL_OR_FORMAL_PROOF"
  },
  "synthesisPreference": {
    "style": "CONDITIONAL_COMPROMISE",
    "allowMinorityReport": true,
    "minorityReportCriteria": "UNMITIGATED_CATASTROPHIC_FAILURE_MODE",
    "compromiseCondition": "BOUNDED_RISK_ENVELOPE"
  }
}
```

### 6.2 Update Persona DNA Matrix
`POST /api/v1/personas/{id}/dna`

Saves or updates the 8-layer cognitive DNA structure for a persona.

### 6.3 List Built-in Mental Model Heuristics
`GET /api/v1/personas/heuristics`

Returns canonical mental model heuristics (*Gall's Law*, *Conway's Law*, *Chesterton's Fence*, *Amdahl's Law*, *CAP Theorem*, etc.).

### 6.4 Compile DNA Prompt Mandate
`POST /api/v1/personas/dna/compile`

Compiles a raw `PersonaDNA` schema into a dense, token-efficient `[COGNITIVE DNA MANDATE]` mathematical prompt block.

#### Request:
```json
{
  "dna": { ... }
}
```

#### Response (`200 OK`):
```json
{
  "compiledPrompt": "[COGNITIVE DNA MANDATE: The Risk Analyst]\n..."
}
```

### 6.5 Import MMOS Specification
`POST /api/v1/personas/dna/import?format=yaml`

Parses and validates a Mind Matrix Open Standard (MMOS v1.0) YAML or JSON string payload into a `PersonaDNA` structure.

### 6.6 Export Persona DNA MMOS
`GET /api/v1/personas/{id}/dna/export?format=yaml`

Exports a persona's complete 8-layer DNA as valid MMOS v1.0 YAML (`format=yaml`) or JSON (`format=json`).

---

## 📄 License

Dialex AI is licensed under the [PolyForm Noncommercial License 1.0.0](file:///LICENSE).  
Copyright (c) 2026 Keltstone Systems. Free for personal, academic, and noncommercial research. Commercial deployments require an enterprise license from Keltstone Systems.
