# Socratix Engine Changelog

All notable changes to **Socratix Engine** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Conventional Commits](https://www.conventionalcommits.org/).

---

## [1.0.0] - 2026-10-15

### Added
- **Multi-Agent Consensus & Credence Engine**: Standalone Go daemon (`socratix`) serving high-throughput Server-Sent Events (SSE) APIs on port 7890.
- **Mathematical Deliberation Framework**: Turn state machines, tension detection, stance independence matrices, and Bayesian credence convergence tracking.
- **SQLite Deliberation Graph**: Local persistence of debate transcripts, model turns, and encrypted credential vault.
- **Benchmark Tool (`socratixbench`)**: Automated convergence rate and rubric scoring across model matrices.
- **System Service Automation**: One-line background login service installation (`socratix service install` via launchd on macOS and systemd on Linux).

---

## [0.8.0] - 2026-09-28

### Added
- Localhost HTTP/2 REST and SSE streaming endpoints.
- Support for Claude, GPT, Gemini, Grok, DeepSeek, Mistral, and local Ollama models.

---

[1.0.0]: https://github.com/Keltstone/DialexAI/releases/tag/socratix-v1.0.0
[0.8.0]: https://github.com/Keltstone/DialexAI/releases/tag/socratix-v0.8.0
