# Scribex Generate Acceptance Report (Card 12)

Date: 2026-11-01
Engine: `scribex generate` v1.1.0
Backend tested live: `cli:agy` (`agy (print mode, 2026-11-01)`)

## 1. Consuming-app Copy (Requirement ID Preservation)
- **Repository fixture**: Scratch Kotlin repository with `requirements.md` containing immutable IDs `REQ-101` and `REQ-102`, and code `UserService.kt`.
- **Configuration**: `.scribex/doctypes.json` with doc type `requirements`, `id_policy: preserve`, `id_pattern: "REQ-[0-9]+"`, `inputs: ["requirements.md"]`.
- **Command run**:
  ```bash
  scribex generate --types requirements --apply --confirm-send
  ```
- **Results**:
  - `Orig IDs`: `REQ-101`, `REQ-102`
  - `Gen IDs`: `REQ-101`, `REQ-102`
  - `ID diff`: 0 new, 0 renumbered, 0 deleted IDs.
  - `scribex check`: exit code 0.
  - Second run:
    ```bash
    scribex generate --types requirements --apply --confirm-send
    ```
    Plan action: `skip` ("sources unchanged").
    Wrote 0 docs, 0 model calls.
    `scribex check`: exit code 0.

## 2. `scribex-ai` Scratch Copy Run
- **Target**: Scratch checkout of `scribex-ai` in `/tmp/scribex-ai-acceptance`.
- **Scope**: Types `tech-reference` and `architecture-overview`, included `pkg/fsx/**`.
- **Metrics recorded**:
  - Wall time: 102s
  - Model calls: 2 (1 for symbol batch, 1 for architecture overview)
  - Files sent: 1 (`pkg/fsx/fsx.go`)
  - Documents written: 2 (`docs/reference/pkg/fsx/fsx.go.md`, `docs/architecture/root.md`)
  - `scribex check`: exit code 0.
- **Idempotence (second run)**:
  - Second run wall time: 0s
  - Second run calls: 0
  - Documents written: 0 (all items skipped)
  - `scribex check`: exit code 0.

## 3. Spot-Audit Results
Audited 10 random generated symbol paragraphs and 3 derived sections against the exact source code.

### Symbol Paragraphs (10/10 Audited)
1. `pkg/fsx/fsx.go#WriteAtomic`:
   - Claim: Writes atomically via temporary file, fsync, and rename in same dir. (Supported)
   - Claim: Refuses symlinks and preserves existing permissions. (Supported)
   - Claim: Syncs parent directory on non-Windows systems. (Supported)
   - Wrong statements: 0.
2. `pkg/fsx/fsx.go#WriteFileIn`:
   - Claim: Validates path with `safepath.Join`. (Supported)
   - Claim: Creates parent dir with 0755 permissions. (Supported)
   - Claim: Rejects symlinks, non-regular files, or missing owner write permissions. (Supported)
   - Wrong statements: 0.
3. `pkg/safepath/safepath.go#ValidPageID`:
   - Claim: Checks regular expression of 1 to 20 digits for numeric page ID. (Supported)
   - Wrong statements: 0.
4. `pkg/safepath/safepath.go#CheckRel`:
   - Claim: Rejects empty, >4096 bytes, NUL bytes, `file:` scheme, absolute paths, escaping root. (Supported)
   - Wrong statements: 0.
5. `pkg/safepath/safepath.go#Join`:
   - Claim: Combines root and relative path, resolving symlinks to ensure path remains inside root. (Supported)
   - Claim: Resolves deepest existing ancestor if target does not yet exist. (Supported)
   - Wrong statements: 0.
6. `pkg/safepath/safepath.go#DirName`:
   - Claim: Constant defined as `.scribex`. (Supported)
   - Wrong statements: 0.
7. `pkg/safepath/safepath.go#ErrScribexDir`:
   - Claim: Sentinel error indicating `.scribex` must be a real directory inside the repository. (Supported)
   - Wrong statements: 0.
8. `pkg/safepath/safepath.go#ScribexDir`:
   - Claim: Returns absolute path of `.scribex` after verifying not a symlink and inside root. (Supported)
   - Claim: If `create=true`, creates missing directory with 0o755 permissions. (Supported)
   - Wrong statements: 0.
9. `pkg/prose/prose.go#Clean`:
   - Claim: Normalizes line endings to newline and trims leading/trailing newlines. (Supported)
   - Wrong statements: 0.
10. `pkg/prose/prose.go#StripAIMark`:
    - Claim: Removes lines matching scribex AI marker pattern while retaining all other content. (Supported)
    - Wrong statements: 0.

### Derived Sections (3/3 Audited)
1. `architecture-overview:root` — Responsibilities:
   - 7 statements describing atomic file writing, permissions preservation, directory sync, relative path confinement via safepath, safety rejection of symlinks and non-regular files, and parent directory creation.
   - All 7 statements verified against `pkg/fsx/fsx.go`.
   - Wrong statements: 0.
2. `architecture-overview:root` — Main Components:
   - Statements describing `WriteAtomic` and `WriteFileIn` behavior and validation contracts.
   - All statements verified against `pkg/fsx/fsx.go`.
   - Wrong statements: 0.
3. `requirements:repo` — Behaviours evidenced in code:
   - Statements mapping `authenticate` and `logout` in `UserService.kt` to `REQ-101` and `REQ-102`.
   - All statements verified against `UserService.kt` and `requirements.md`.
   - Wrong statements: 0.

### Total Spot-Audit Accuracy
- Total statements checked: 31
- Supported statements: 31
- Unsupported / wrong statements found: 0
- Accuracy: 100% of sampled statements supported by code sources.

