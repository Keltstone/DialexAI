# Language Support and Contract Accuracy

Scribex extracts public API contracts across five supported languages using deterministic, bounded parsers (Go AST for Go; lexical tokenizers for TypeScript, Python, Kotlin, and Java).

---

## Per-Language Accuracy Table

Evaluated against the tracked contract mutation and no-change corpus ([`pkg/symbols/corpus_test.go`](../../pkg/symbols/corpus_test.go)) and the 23-case gate test suite ([`eval-probes/r8/zz8_gate.sh`](../../../.dev/scribex-ai/eval-probes/r8/zz8_gate.sh)):

| Language | Parser Implementation | Tracked Corpus Tests | Accuracy | False "No-Change" | False Change (Body/Fmt) |
|:---|:---|:---:|:---:|:---:|:---:|
| **Go** | Standard `go/parser` & `go/ast` | 14 / 14 | **100.0%** | 0 | 0 |
| **TypeScript / JS** | Lexical tokenizer (`pkg/symbols/scan/ts.go`) | 24 / 24 | **100.0%** | 0 | 0 |
| **Python** | Lexical tokenizer (`pkg/symbols/scan/python.go`) | 17 / 17 | **100.0%** | 0 | 0 |
| **Kotlin** | Lexical tokenizer (`pkg/symbols/scan/kotlin.go`) | 26 / 26 | **100.0%** | 0 | 0 |
| **Java** | Lexical tokenizer (`pkg/symbols/scan/java.go`) | 19 / 19 | **100.0%** | 0 | 0 |

---

## Supported Constructs Matrix

| Construct | Go | TypeScript / JS | Python | Kotlin | Java |
|:---|:---:|:---:|:---:|:---:|:---:|
| **Functions / Methods** | Supported | Supported | Supported | Supported | Supported |
| **Parameter Types & Counts** | Supported | Supported | Supported | Supported | Supported |
| **Return Types** | Supported | Supported | Supported | Supported | Supported |
| **Default Parameter Values** | N/A | Supported | Supported | Supported | N/A |
| **Multiline Signatures** | Supported | Supported | Supported | Supported | Supported |
| **Visibility Filtering** | Exported (`[A-Z]`) | `export` only | Non-`_` / `__all__` | `public` (default) | `public` (+ interfaces) |
| **Type Definitions / Classes** | Supported | Supported | Supported | Supported | Supported |
| **Generics / Type Parameters** | Supported | Supported | Supported | Supported | Supported |
| **Extension Functions** | N/A | N/A | N/A | Supported | N/A |
| **Companion Objects** | N/A | N/A | N/A | Supported | N/A |
| **Throws / Exception Clauses** | N/A | N/A | N/A | N/A | Supported |
| **Constants & Variables** | Value & Type | N/A | N/A | N/A | N/A |
| **BOM / CRLF Normalization** | Supported | Supported | Supported | Supported | Supported |
| **Unicode Identifiers** | Supported | Supported | Supported | Supported | Supported |

---

## Fail-Closed Gate Invariants

1. **Zero False "No Change"**: Any modification to a public signature (renamed parameter, altered return type, added parameter, visibility change, or deleted declaration) is detected as a delta.
2. **Zero False Positive**: Changes strictly confined to function bodies, comments, docstrings, or formatting/indentation produce zero contract deltas.
3. **Fail-Closed on Parse Failure**: If a supported file is syntactically invalid or cannot be parsed, Scribex does not silently skip it; it treats the file as changed and marks the impact gate dirty.
