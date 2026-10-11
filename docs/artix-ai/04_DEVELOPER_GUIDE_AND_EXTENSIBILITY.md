# Artix AI Platform: Developer Guide & Extensibility

**Platform:** `artix-ai` (Go 1.27 & Compose Desktop)  
**Author:** Kolta Labs Core Systems Architecture  
**Audience:** Core Engine Contributors, Language Driver Authors, DevSecOps Leads  
**Scope:** Go package extension points, custom build drivers, taboo rule definitions, and running adversarial security test suites.

---

## 1. Implementing a Custom Language Build Driver

All compilation and test execution is abstracted behind the `BuildDriver` interface (`pkg/plugins/build_driver.go`):

```go
package plugins

import (
    "context"
    "artix/pkg/sandbox"
)

type BuildDriver interface {
    Name() string
    Detect(workspacePath string) bool
    Warm(ctx context.Context, workspacePath string) error
    TestCommand() []string
    ParseTestOutput(stdout, stderr string, exitCode int) (TestResult, error)
}
```

### Implementing a Rust (`cargo`) Driver
```go
package plugins

type CargoDriver struct{}

func (d *CargoDriver) Name() string { return "cargo" }

func (d *CargoDriver) Detect(workspace string) bool {
    return fileExists(filepath.Join(workspace, "Cargo.toml"))
}

func (d *CargoDriver) Warm(ctx context.Context, workspace string) error {
    // Pre-warm local cargo crates cache outside sandbox
    return execCommand(ctx, workspace, "cargo", "fetch")
}

func (d *CargoDriver) TestCommand() []string {
    // Enforce offline execution inside sandbox
    return []string{"cargo", "test", "--offline"}
}

func (d *CargoDriver) ParseTestOutput(stdout, stderr string, exitCode int) (TestResult, error) {
    passed := exitCode == 0
    return TestResult{
        Passed:   passed,
        ExitCode: exitCode,
        Failures: extractCargoFailures(stdout + stderr),
    }, nil
}
```

---

## 2. Defining Custom Taboo Rules

Taboo rules are specified in JSON format under `.artix/rules/<rule-id>.json` or bound via `artix steering bind`:

```json
{
  "id": "taboo-global-scope",
  "name": "Banned Coroutine GlobalScope Launch",
  "description": "Never launch coroutines in GlobalScope; use viewModelScope or structured concurrency.",
  "targetLanguages": ["kotlin"],
  "bannedPatterns": [
    "GlobalScope.launch",
    "GlobalScope.async"
  ],
  "suggestedAlternative": "viewModelScope.launch { ... }",
  "severity": "CRITICAL"
}
```

---

## 3. The PR-to-Rule Synthesizer

When human engineers write review comments on pull requests, `artixd` automatically synthesizes candidates:

1. Reviewer comments: `"Never use Thread.sleep in our unit tests; use runTest with advanceTimeBy!"`
2. Synthesizer parses comment via `pkg/knowledge/synthesizer.go`.
3. Candidate rule generated in `.artix/knowledge/` in status `PROPOSED`.
4. A repository `CODEOWNER` ratifies the rule:
   ```bash
   artix knowledge ratify rule_time_travel --approver "senior_lead"
   ```
5. Once ratified, the rule activates across all local IDEs and CI/CD runs.

---

## 4. Running the Adversarial Security Test Suite

Evaluators can verify all daemon security controls, prompt injection defenses, and sandbox isolation using the bundled test suite:

```bash
cd artix-ai

# Run all G6 daemon attack suite tests with race detector:
go test -race -v -count=1 ./pkg/forge -run "TestG6|TestR2_2|TestWebhook"

# Run all sandbox confinement and timeout tests:
go test -race -v -count=1 ./pkg/sandbox/...

# Run all Coder <-> Reviewer loop tests:
go test -race -v -count=1 ./pkg/coder/...
```

---

*Next: Continue to [Kritix AI Platform: Conceptual Foundation](../kritix-ai/01_CONCEPTUAL_FOUNDATION.md).*
