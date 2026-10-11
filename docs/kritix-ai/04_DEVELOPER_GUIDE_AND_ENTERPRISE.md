# Kritix AI Platform: Developer Guide & Enterprise Administration

**Platform:** `kritix-ai` (Go 1.27 & Compose Desktop)  
**Author:** Kolta Labs Security & Systems Architecture  
**Audience:** Platform Engineers, Security Administrators, SDET Contributors  
**Scope:** Custom workflow block authoring, Enterprise RBAC, cryptographic audit logs, and CI/CD integration.

---

## 1. Creating Custom DAG Workflow Blocks

The workflow engine executes pipelines composed of blocks implementing the `Block` interface (`pkg/workflow/block.go`):

```go
package workflow

import (
    "context"
)

type Block interface {
    ID() string
    Execute(ctx context.Context, input BlockInput) (BlockOutput, error)
}
```

### Implementing a Custom Prometheus Latency Assertion Block
```go
package workflow

type PrometheusLatencyBlock struct {
    MaxP99LatencyMs int
}

func (b *PrometheusLatencyBlock) ID() string {
    return "custom.assert-prometheus-p99"
}

func (b *PrometheusLatencyBlock) Execute(ctx context.Context, input BlockInput) (BlockOutput, error) {
    p99, err := queryPrometheusP99(ctx, input.TargetURL)
    if err != nil {
        return BlockOutput{Passed: false, Error: err.Error()}, err
    }
    
    passed := p99 <= b.MaxP99LatencyMs
    return BlockOutput{
        Passed: passed,
        Metrics: map[string]float64{"p99_latency_ms": float64(p99)},
    }, nil
}
```

---

## 2. Enterprise Role-Based Access Control (RBAC) (`pkg/auth/rbac.go`)

Kritix enforces five distinct enterprise personas:

| Persona Role | Capabilities & Authorizations |
|---|---|
| **`Admin`** | User provisioning, API key rotation, SOC 2 audit export, global policy mutation. |
| **`TestArchitect`** | Designs & publishes DAG blueprints, configures model routes, manages quarantine SLAs. |
| **`Developer`** | Executes blueprints, records journeys in Studio, inspects repros; cannot alter global blueprints. |
| **`Viewer / Auditor`** | Read-only access to test reports, JUnit/SARIF artifacts, and ROI summaries. |
| **`CIRunner`** | Headless token scoped strictly to automated CI/CD execution gates. |

### Issuing Cryptographic Session Tokens
Tokens are signed via **HMAC-SHA256**. The daemon requires `KRITIX_AUTH_SECRET` (minimum 32 bytes) at startup:

```bash
export KRITIX_AUTH_SECRET="super-secure-random-32-byte-secret-key-12345"

# Issue a Developer token with 8-hour expiration:
./bin/kritix auth token Developer --ttl 8h
# Output: KRITIX_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

---

## 3. Tamper-Evident Audit Ledger Administration

All grant decisions, blueprint runs, and role mutations append to a local hash-chained log (`KRITIX_AUDIT_LOG`, default: `.kritix/audit.log`):

### 3.1 Verifying Cryptographic Integrity
To verify that no log entries have been inserted, reordered, or edited by an attacker:

```bash
./bin/kritix auth verify-audit
# Output: [PASS] Audit log verified. 142 records checked, hash chain unbroken.
```

If an entry was tampered with:
```text
[FAIL] Audit log corrupted at sequence #84!
Expected prev_hash: 7f83b1657ff1...
Actual prev_hash:   000000000000...
Engine refuses to start until log integrity is restored.
```

### 3.2 Exporting Audit Trails to Enterprise SIEM
Export audit trails for Datadog, Splunk, Elastic, or AWS CloudTrail:

```bash
./bin/kritix auth export-audit \
  --format json \
  --out /var/log/siem/kritix-audit.json
```

---

## 4. Continuous Integration (GitHub Actions)

Add automated PR quality and security gating to `.github/workflows/kritix.yml`:

```yaml
name: Kritix Autonomous Quality Gate

on:
  pull_request:
    branches: [ main, develop ]

jobs:
  smoke-and-dast-guard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27'

      - name: Build Kritix
        run: |
          cd kritix-ai
          go build -o bin/kritix ./cmd/kritix

      - name: Run Smoke Guard Blueprint
        env:
          KRITIX_AUTH_SECRET: ${{ secrets.KRITIX_AUTH_SECRET }}
          KRITIX_ALLOWED_TARGETS: "localhost,127.0.0.1"
          KRITIX_HEAL_MODE: "strict"
        run: |
          ./kritix-ai/bin/kritix run pr-smoke-guard --target "http://localhost:3000"

      - name: Upload Test Results
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test-artifacts
          path: |
            .kritix/results.sarif
            .kritix/junit.xml
            .kritix/repro.spec.ts
```

---

## 5. Running the Go Test Suite

```bash
cd kritix-ai

# Run all unit tests with race detector:
go test -race -v -count=1 ./pkg/...

# Run DOM token compression benchmarks:
go test -bench=. ./pkg/optimizer/
```

---

*This concludes the Dialex Suite Core Wiki documentation.*  
*Return to [Wiki Master Navigation](../README.md).*
