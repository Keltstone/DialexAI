# Anchors

Anchors link Markdown documentation directly to symbols defined in source code files. When the source code changes, Scribex can verify that the documentation stays up-to-date and synchronizes the code signatures.

## Anchor Syntax

An anchor is represented by an HTML comment tag in Markdown:

```markdown
<!-- @scribex:anchor(path/to/file.go#SymbolName) -->
... documentation or snippet ...
<!-- /@scribex:anchor -->
```

Anchors may optionally store a cryptographic signature hash:

```markdown
<!-- @scribex:anchor(path/to/file.go#SymbolName,sig=0123456789abcdef) -->
```

## 16-Hex Anchor Hashes

Scribex uses a 16-character hexadecimal hash (64-bit cryptographic digest) to represent symbol signatures. This provides high collision resistance even in monorepos with hundreds of thousands of code symbols.

### Legacy 8-Char Hash Compatibility

Earlier versions of Scribex recorded signatures with 8-character hex hashes.

Scribex maintains full backward compatibility with legacy hashes:
- When checking an anchor, Scribex matches a stored 8-character hash if it forms a prefix of the target symbol's 16-character hash.
- Running `scribex sync` automatically updates any legacy 8-character hash to the full 16-character hash.
- Line endings (`\n` vs `\r\n`) in the document are preserved during sync.
