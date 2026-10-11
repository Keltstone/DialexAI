# Artix AI Test Removal Registry

This registry tracks any test functions intentionally removed, refactored, or deprecated across release versions. Any deletion of a `Test*` function that existed in a previous tag must be recorded here with an architectural and security justification. The CI test retention gate (`scripts/check-test-retention.sh`) strictly enforces this registry.

## Registry Format
```markdown
- `TestName`: [REASON] Justification for removal / superseded by `NewTestName`.
```

## Removed Tests
*(No tests removed in v0.11.0-enterprise)*
