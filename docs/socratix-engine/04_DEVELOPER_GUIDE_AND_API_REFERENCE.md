# Socratix Engine: Developer Guide & API Reference

**Engine:** `socratix-engine` (Go 1.27)  
**Author:** Kolta Labs Core Systems Architecture  
**Audience:** Backend Developers, Integrators, Custom Runner Authors  
**Scope:** REST endpoints, SSE payloads, custom runner development, persona compilation, and testing.

---

## 1. REST & SSE HTTP API Reference

The Socratix daemon exposes standard REST endpoints alongside real-time Server-Sent Events.

### 1.1 Discussion Management Endpoints

#### `POST /api/v1/debates`
Creates and initializes a new deliberation debate session.

**Request Body (`application/json`):**
```json
{
  "topic": "Should we migrate from REST to gRPC for internal service communication?",
  "commonContext": "High-throughput microservices architecture handling 150k req/sec.",
  "roundMode": "FIXED",
  "maxRounds": 3,
  "independence": "ROUND1_ISOLATED",
  "primary": {
    "id": "seat_architect",
    "provider": "ANTHROPIC",
    "model": "claude-3-5-sonnet-20241022",
    "role": "Principal Systems Architect",
    "systemPrompt": "Focus on API contract stability, schema evolution, and transport throughput."
  },
  "secondaries": [
    {
      "id": "seat_sre",
      "provider": "OPENAI",
      "model": "gpt-4o",
      "role": "Staff Site Reliability Engineer",
      "systemPrompt": "Focus on load balancing, HTTP/2 connection pooling, and debugging complexity."
    }
  ],
  "moderation": {
    "enabled": true,
    "detectTopicDrift": true,
    "detectRepetition": true,
    "persona": "IMPARTIAL_SYNTHESIZER"
  }
}
```

**Response (`201 Created`):**
```json
{
  "id": "deb_9f3b12ce",
  "status": "INITIALIZED",
  "createdAtMs": 1773229800000
}
```

---

#### `POST /api/v1/debates/{id}/start`
Triggers active round-robin turn execution. Supports streaming via `Accept: text/event-stream`.

**SSE Events Emitted:**
- `turn_start`: `{ "seatId": "...", "round": 1 }`
- `token_chunk`: `{ "seatId": "...", "delta": "..." }`
- `turn_end`: `{ "seatId": "...", "agreed": false, "tokensIn": 1400, "tokensOut": 320 }`
- `tension_update`: `{ "tensions": [ ... ] }`
- `credence_snapshot`: `{ "roundIndex": 1, "entropy": 0.94, ... }`
- `consensus_reached`: `{ "round": 2, "ratio": 1.0, "summary": "..." }`

---

#### `POST /api/v1/debates/{id}/pause`
Freezes execution cleanly at the current turn boundary.

**Response (`200 OK`):**
```json
{
  "id": "deb_9f3b12ce",
  "status": "PAUSED",
  "lastCompletedRound": 1,
  "lastCompletedSeat": "seat_architect"
}
```

---

#### `POST /api/v1/debates/{id}/inject`
Injects an authoritative human remark or external steering constraint into the active context.

**Request Body (`application/json`):**
```json
{
  "content": "Notice: WebSockets are already supported on the mobile client. Do not treat mobile gRPC as a blocker.",
  "authorDisplayName": "Engineering Director"
}
```

---

### 1.2 Knowledge Graph Endpoints

#### `GET /api/v1/projects/{projectId}/graph`
Fetches the active semantic knowledge graph for a project, applying exponential decay to node weights.

**Query Parameters:**
- `minWeight`: Minimum decayed weight threshold (default: `0.10`).

**Response (`200 OK`):**
```json
{
  "nodes": [
    {
      "id": "node_grpc_proto",
      "type": "CONCEPT",
      "title": "Protocol Buffers Binary Serialization",
      "content": "Strongly typed binary encoding reducing JSON deserialization CPU costs by 60%.",
      "current_weight": 0.88,
      "last_accessed_at": 1773229800
    }
  ],
  "edges": [
    {
      "id": "edge_e1a2",
      "source_id": "node_grpc_proto",
      "target_id": "node_low_latency",
      "relation": "SUPPORTS",
      "strength": 0.92
    }
  ]
}
```

---

## 2. Implementing a Custom `AgentRunner`

To connect a proprietary LLM gateway or internal company model endpoint, implement the `runner.AgentRunner` interface in Go:

```go
package runner

import (
    "context"
    "socratix/pkg/model"
)

type CustomInternalRunner struct {
    Endpoint string
    APIKey   string
}

func NewCustomInternalRunner(endpoint, apiKey string) *CustomInternalRunner {
    return &CustomInternalRunner{
        Endpoint: endpoint,
        APIKey:   apiKey,
    }
}

func (r *CustomInternalRunner) Respond(
    ctx context.Context,
    agent model.Agent,
    topic, commonContext, commonInstructions string,
    transcript []model.DebateMessage,
    modelOverride string,
) (AgentReply, error) {
    // 1. Format payload according to your gateway schema
    // 2. Transmit HTTP POST request with context timeout
    // 3. Parse JSON response and token metrics
    return AgentReply{
        Content:      "Generated response text...",
        TokensIn:     1250,
        TokensOut:    280,
        TokensCached: 640,
    }, nil
}
```

Register your runner in `pkg/orchestrator/wiring.go`:
```go
func BuildRunnerFor(provider model.Provider) runner.AgentRunner {
    switch provider {
    case "INTERNAL_GATEWAY":
        return runner.NewCustomInternalRunner(cfg.Endpoint, cfg.Key)
    default:
        return runner.NewApiRunner(...)
    }
}
```

---

## 3. 8-Layer Persona DNA Compilation

Personas are defined in structured JSON files under `.dialex/personas/` or declared programmatically:

```json
{
  "id": "dist_systems_skeptic",
  "name": "Dr. Leslie (Distributed Systems Skeptic)",
  "provider": "ANTHROPIC",
  "model": "claude-3-5-sonnet-20241022",
  "dna": {
    "epistemicStrictness": 0.95,
    "adversarialSkepticism": 0.90,
    "architecturalModularity": 0.85,
    "executionVelocity": 0.30,
    "securityParanoia": 0.80,
    "cognitiveEntropy": 0.20,
    "empiricalThreshold": 0.90,
    "styleVector": "caveman"
  },
  "tabooDirectives": [
    "Never accept unproven eventual consistency for financial balances",
    "Reject single points of failure in leader election"
  ]
}
```

The compiler (`pkg/persona/compiler.go`) compiles this DNA into an optimized system prompt that conditions the model's logits and steering directives.

---

## 4. Running the Go Test Suite

Socratix Engine adheres to strict unit and race-condition testing standards:

```bash
cd socratix-engine

# Run all unit tests with race detection:
go test -race -v -count=1 ./pkg/...

# Run benchmark suites:
go test -bench=. ./pkg/credence/
go test -bench=. ./pkg/consensus/
```

---

*Next: Continue to [Dialex AI Client: Conceptual Foundation](../dialex-ai/01_CONCEPTUAL_FOUNDATION.md).*
