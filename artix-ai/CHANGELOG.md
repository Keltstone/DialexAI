# Artix AI Changelog

All notable changes to **Artix AI** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Conventional Commits](https://www.conventionalcommits.org/).

---

## [0.11.0-enterprise] - 2026-10-18

### Added
- **Test Retention & Regression Gate**: Added CI check (`scripts/check-test-retention.sh`) enforcing 100% test retention against baseline release tags with `REMOVED_TESTS.md` justification tracking.
- **Adversarial AST Analysis Hardening**: Enforced `errgroup.Go` wait requirements, flagged tautological assertions, treated nil-channel select cases as dead code, preserved IEEE-754 `NaN` comparisons (`x != x`).
- **Taint Analysis & Egress Sinks**: Expanded detection to `net/smtp`, `net/rpc`, and `net/textproto`. Added secret exfiltration detection for `database/sql` external host DSNs.
- **Sandbox Child Isolation**: Allowlist-based execution environment stripping all provider, forge, and sensitive credentials from child processes.
- **Cryptographic Trust Anchor**: Published ED25519 public key and fingerprint in `.well-known/security.txt`. Standardized signed `SHA256SUMS` distribution.
- **Verify-Approval Matrix**: Implemented end-to-end integration test with compiled trust keys and root policies rejecting stale head SHAs, self-approval, and bot review overrides.
- **Three-Source Benign Corpus Evaluation**: Absolute FPR target gates (<5% on GOROOT, <5% on ecosystem modules).

---

## [0.10.0-enterprise] - 2026-10-10

### Added
- Multi-agent Stakeholder Council architecture exploring architectural tradeoffs prior to patch generation.
- Interactive Engineering Lab diff viewer with AST patch synthesis and rollback support.
- Model Context Protocol (MCP) toolchain and LSP server integration.

---

## [0.1.0-enterprise] - 2026-09-15

### Added
- Initial enterprise CLI (`artix`) and daemon (`artixd`) release with `plan`, `steer`, `persona`, and `replay` workflows.

---

[0.11.0-enterprise]: https://github.com/Keltstone/DialexAI/releases/tag/v0.11.0-enterprise
[0.10.0-enterprise]: https://github.com/Keltstone/DialexAI/releases/tag/v0.10.0-enterprise
[0.1.0-enterprise]: https://github.com/Keltstone/DialexAI/releases/tag/v0.1.0-enterprise
