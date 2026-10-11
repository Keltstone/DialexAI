# Dialex Suite: Distributed Topology & Mesh Networking

**Author:** Kolta Labs Core Architecture Group  
**Audience:** Site Reliability Engineers, Enterprise Network Architects, DevOps Engineers  
**Scope:** Distributed deployment modes, network protocols, streaming event architectures, and zero-trust mesh integration.

---

## 1. Network Topology Overview

Dialex Suite supports three operational topologies:
1. **Local Embedded Desktop Topology**: Single developer workstation where client apps and engine daemons run locally on loopback interfaces (`127.0.0.1`) or in-process.
2. **On-Premise / Enterprise Private Cloud Topology**: Engine daemons run centrally on bare-metal servers or private Kubernetes clusters with PostgreSQL and SQLite WAL stores, serving multiple client workstations over mTLS or internal VPN.
3. **Zero-Trust Sovereign Mesh (`tsnet`) Topology**: Client mobile devices and remote laptops connect directly to on-premise daemons over an encrypted, peer-to-peer WireGuard mesh powered by embedded Tailscale (`tailscale.com/tsnet`), requiring **zero open public firewall ports** and **zero reverse proxy certificates**.

```mermaid
graph TB
    subgraph ZeroTrustMesh ["Tailscale Sovereign WireGuard Mesh (tsnet)"]
        Mobile["Dialex Mobile App<br/>(Android / iOS Remote Client)"]
        Laptop["Developer Laptop<br/>(macOS Desktop Client / CLI)"]
        MeshNode["Socratix Tailscale Node<br/>(socratix.tailnet-name.ts.net)"]
    end

    subgraph OnPremServer ["Private On-Premise Host / Homelab"]
        Socratix["socratix daemon<br/>Port 8080 (REST + SSE)"]
        Artix["artixd webhook daemon<br/>Port 8081 (HMAC Webhooks)"]
        Kritix["kritix daemon<br/>Port 9090 (Studio / DAST)"]
        LocalLLM["Local GPU Box<br/>(vLLM / Ollama: Port 11434)"]
    end

    Mobile -->|WireGuard P2P Tunnel| MeshNode
    Laptop -->|WireGuard P2P Tunnel| MeshNode
    MeshNode --> Socratix
    Socratix --> Artix
    Socratix --> Kritix
    Socratix --> LocalLLM
```

---

## 2. Communication Protocols

| Channel | Protocol | Transport | Serialization | Primary Use Case |
|---|---|---|---|---|
| **Client $\leftrightarrow$ Socratix** | HTTP/1.1 & HTTP/2 | TCP / TLS / UNIX Domain Socket | JSON (UTF-8) | Discussion creation, config mutation, project queries |
| **Socratix $\rightarrow$ Client** | Server-Sent Events (SSE) | HTTP/1.1 Persistent Stream | `text/event-stream` | Streaming tokens, turn transitions, live consensus updates, tension notifications |
| **Client $\leftrightarrow$ Artix** | Language Server Protocol (LSP 3.17) | JSON-RPC 2.0 over Stdio | JSON-RPC | Real-time Taboo Space diagnostics, CodeLens, in-editor quick-fixes |
| **GitHub/GitLab $\rightarrow$ Artixd** | HTTP Webhooks | HTTPS | JSON (HMAC-SHA256 signed) | Automated CI/CD PR triggers, issue autonomous task execution |
| **Client $\leftrightarrow$ Kritix** | HTTP / SSE / WebSocket | TCP | JSON / Binary frames | Studio recording telemetry, CDP DOM manipulation, live test stream |
| **Engine $\leftrightarrow$ Local Models** | OpenAI-compatible REST | HTTP (Loopback) | JSON streaming | High-throughput local token generation via Ollama / vLLM |
| **Engine $\leftrightarrow$ Subshells** | OS Process I/O | Stdin / Stdout / Stderr | Text / Structured JSON | Driving authenticated developer CLIs (`claude`, `codex`, `agy`) |

---

## 3. Server-Sent Events (SSE) Protocol Specification

The real-time deliberative lifecycle is streamed over SSE endpoints (e.g. `POST /api/v1/debates/{id}/stream` or `GET /api/v1/debates/{id}/events`).

### SSE Event Stream Anatomy

```text
event: turn_start
data: {"seatId":"seat_claude_1","provider":"ANTHROPIC","round":2,"timestampMs":1773229801000}

event: token_chunk
data: {"seatId":"seat_claude_1","delta":"While I acknowledge the performance arguments presented by "}

event: token_chunk
data: {"seatId":"seat_claude_1","delta":"GPT-4o regarding distributed caching, "}

event: token_chunk
data: {"seatId":"seat_claude_1","delta":"we must consider the network partition guarantees of the Raft cluster."}

event: turn_end
data: {"seatId":"seat_claude_1","round":2,"agreed":false,"tokensIn":1240,"tokensOut":340,"tokensCached":810}

event: tension_update
data: {"tensionId":"tension_a4f1","status":"OPEN","severity":0.85,"underlyingConflict":"CAP Consistency vs Cache Stale Window"}

event: credence_snapshot
data: {"roundIndex":2,"entropy":0.82,"dominantHypothesis":"hyp_raft_quorum","aggregatedCredence":{"hyp_raft_quorum":0.68,"hyp_eventual_cache":0.32}}

event: consensus_reached
data: {"round":3,"ratio":1.0,"exitReason":"CONSENSUS","summary":"Consensus converged on Raft consensus with client-side read replicas."}
```

### Heartbeats & Client Reconnection
- The server emits a keepalive comment (`: keepalive\n\n`) every 15 seconds.
- If a connection drops, the client sends `Last-Event-ID` upon reconnection. The Go daemon resumes streaming from the in-memory circular event buffer without dropping turn state or restarting the round.

---

## 4. Tailscale Mesh Integration (`tsnet`)

For sovereign remote access across mobile and desktop clients without public IP exposure or port forwarding, `socratix-engine` includes native Tailscale integration via Go's `tailscale.com/tsnet` library.

### Architecture

```mermaid
sequenceDiagram
    autonumber
    actor User as Mobile / Remote User
    participant Android as Dialex Android App
    participant Tsnet as Embedded tsnet Server (socratix)
    participant Engine as Deliberation Core

    Note over Tsnet: Starts ephemeral WireGuard listener<br/>Registered as 'dialex-daemon' on Tailnet
    User->>Android: Launches App outside local WiFi
    Android->>Tsnet: Connects via 100.x.y.z (Tailscale IP)
    Tsnet->>Tsnet: Verifies WireGuard Node Identity & AuthKey
    Tsnet->>Engine: Forwards authenticated HTTP/SSE requests
    Engine-->>Android: Streams real-time debate turns & deliberation
```

### Configuration Flags
When launching `socratix`:
```bash
socratix \
  --tsnet \
  --tsnet-hostname "dialex-daemon" \
  --tsnet-auth-key "tskey-auth-kXXXXX-XXXXXXXXXXXX" \
  --tsnet-dir "/var/lib/socratix/tsnet"
```
Once initialized, the engine joins your private Tailnet and acquires an internal MagicDNS address (e.g. `https://dialex-daemon.your-tailnet.ts.net`), terminating TLS automatically using Let's Encrypt certificates managed internally by Tailscale.

---

## 5. High-Throughput Local Model Clustering

For enterprise and defense environments requiring 100% sovereign, air-gapped execution, `socratix-engine` can orchestrate multiple local model backends concurrently:

```mermaid
graph LR
    subgraph Host ["Socratix Engine Daemon"]
        Runner["pkg/runner/api_runner.go"]
    end

    subgraph GPU1 ["NVIDIA GPU Node 1"]
        VLLM1["vLLM Instance A<br/>Qwen 2.5 Coder 32B<br/>Port 8001"]
    end

    subgraph GPU2 ["NVIDIA GPU Node 2"]
        VLLM2["vLLM Instance B<br/>DeepSeek-R1-Distill-Llama-70B<br/>Port 8002"]
    end

    subgraph Workstation ["Local Workstation"]
        Ollama["Ollama Daemon<br/>Gemma 2 27B<br/>Port 11434"]
    end

    Runner -->|Provider: OLLAMA| Ollama
    Runner -->|Provider: OPENAI_COMPATIBLE| VLLM1
    Runner -->|Provider: OPENAI_COMPATIBLE| VLLM2
```

Every participant seat in a deliberation can point to a distinct endpoint and port, allowing a 3-agent council to run across heterogeneous physical GPU nodes in the local network with zero cloud egress.

---

*Next: Continue to [03. Sovereign Vault & Zero-Cloud Security](../suite-architecture/03_SECURITY_VAULT_AND_SOVEREIGNTY.md).*
