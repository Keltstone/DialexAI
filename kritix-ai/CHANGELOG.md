# Kritix AI Changelog

All notable changes to **Kritix AI** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Conventional Commits](https://www.conventionalcommits.org/).

---

## [0.3.0-enterprise-hardened] - 2026-10-08

### Added
- **Structural Truth Audit (12 Gates)**: Built-in truth auditor verifying provenance, label honesty, event integrity, self-certifications, AST secrets, paths, and hygiene.
- **Enterprise Vault-Grade Session Security**: AES-256-GCM encryption with ephemeral in-memory session stores, automatic wipe on exit, and strict TTL enforcement.
- **Deterministic Token Optimization**: Structural DOM pruning engine reducing HTML DOMs to semantic accessibility trees prior to model submission.
- **OWASP DAST & Security Fuzzing**: Real-time SQLi, XSS, and PII leak detection boundaries.
- **Self-Healing Locator Registry**: Zero-false-pass safety guarantees refusing to heal destructive actions (e.g. transfer vs delete) across UI mutations.

---

## [0.2.0] - 2026-09-20

### Added
- Visual driver and semantic DOM accessibility exploration.
- Headless test execution, replay logs, and multi-tenant isolation.

---

## [0.1.0] - 2026-09-01

### Added
- Initial autonomous testing studio and CLI prototype.

---

[0.3.0-enterprise-hardened]: https://github.com/Keltstone/DialexAI/releases/tag/kritix-v0.3.0-enterprise-hardened
[0.2.0]: https://github.com/Keltstone/DialexAI/releases/tag/kritix-v0.2.0
[0.1.0]: https://github.com/Keltstone/DialexAI/releases/tag/kritix-v0.1.0
