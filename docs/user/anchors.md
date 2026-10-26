# Scribex Anchors & Docs Confinement

Scribex links markdown documentation to code signatures via comment anchors. This document outlines anchor syntax, symbol naming rules, path confinement guarantees, and link scavenging.

## Anchor Syntax

An anchor is declared on a single line using an HTML comment:

```markdown
<!-- @scribex:anchor(path/to/file.ext#SymbolKey) -->
```

When synchronized (`scribex sync`), Scribex computes a 4-byte hex hash of the symbol's public signature and appends `sig=<hash>` to the anchor line, immediately followed by a ` ```signature ` fence:

```markdown
<!-- @scribex:anchor(path/to/file.ext#SymbolKey) sig=1234abcd -->
```signature
func SymbolKey() string
```
Document prose describing the symbol follows.
<!-- @scribex:end -->
```

### Overload & Generic Keys

Scribex supports overloaded methods and generic types emitted by the lexical scanners:
- Overloaded methods: `<!-- @scribex:anchor(pkg/service.kt#Service.execute(int,String)) -->`
- Generics: `<!-- @scribex:anchor(pkg/box.ts#Box<T>.get) -->`

**Rules:**
- **Whitespace**: Symbol keys cannot contain whitespace (`\S+?`). Typed parameter lists must not include spaces between types (e.g. `(int,String)` rather than `(int, String)`).
- **Format**: Exactly one `#` separates the repo-relative file path from the symbol key.

### Fenced Examples Ignored

Examples of anchors inside fenced code blocks (such as ` ```markdown ` or ` ~~~ `) are never processed as real anchors:
- They are ignored by `scribex check`, `scribex sync`, and `scribex scavenge`.
- They do not increment anchor counts (`Count`).
- Delimiters and content inside code fences remain completely untouched during sync.
- Scribex's own ` ```signature ` fence following an anchor line is processed as part of the anchor block, preserving round-trip idempotency.

## Path Confinement Guarantees

All file paths specified in anchors, markdown links, or git change sets are strictly confined to the repository root via `safepath`:

1. **No Directory Traversal**:
   - `..` path segments are rejected anywhere in relative paths.
   - Absolute paths (e.g. `/etc/passwd` or `C:\Windows`) are rejected.
   - URL schemes like `file://` are rejected.
2. **Symlinks Not Followed**:
   - Symlinks pointing outside the repository root (both file symlinks and directory symlinks) are strictly rejected.
   - Symlinked `.md` files are skipped during doc discovery and reported to stderr as `skipped symlink: <path>` (informational).
   - Git-tracked symlinks pointing outside the repository are flagged as `Unknown` in `docs.Deltas` and never read.
   - When updating documents during `sync`, Scribex re-verifies that target paths have not been swapped with symlinks before writing.
3. **Repository Root as Symlink**:
   - Scribex transparently resolves symlinks in the root directory itself (e.g. macOS `/var` -> `/private/var`), ensuring confinement checks remain accurate.
4. **Length and Character Bounds**:
   - Relative paths are capped at 4096 bytes and cannot contain NUL bytes or backslash-newline escapes.
   - Symbol keys are capped at 512 bytes.

## Scavenge & Link Checking

Running `scribex scavenge` validates all anchors and repo-relative markdown links (`[text](target)`):

- **Target Resolution**:
  - Anchors and queries (`?query`, `#fragment`) on link targets are stripped prior to resolution.
  - URL percent-encoding (e.g. `a%20b.md`) is decoded.
  - Pure fragments (`#heading`) and external URLs (`https://`, `mailto:`, `//cdn`) are ignored.
  - A leading slash (e.g. `[guide](/docs/guide.md)`) resolves relative to the repository root.
  - Relative links (e.g. `[sibling](sibling.md)`) resolve relative to the document's directory.
- **Reporting**:
  - Links that resolve outside the repository or through escaping symlinks are reported as:
    `link escapes repository`
  - Targets that do not exist in the repository are reported as:
    `broken link`
  - Case sensitivity: link resolution uses exact path case. Note that case-insensitive filesystems (such as default macOS APFS or Windows NTFS) may mask case mismatches that fail on case-sensitive CI environments (Linux).
