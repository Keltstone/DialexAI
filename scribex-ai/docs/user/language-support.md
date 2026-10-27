# Language Support and Contract Accuracy

Scribex extracts public API contracts across five supported languages using deterministic, bounded parsers (Go standard AST for Go; lexical tokenizers for TypeScript, Python, Kotlin, and Java).

---

## Per-Language Accuracy Table

Evaluated against the tracked contract mutation and no-change corpus ([`pkg/symbols/corpus_test.go`](../../pkg/symbols/corpus_test.go)) and real Git integration tests ([`cmd/scribex/main_test.go`](../../cmd/scribex/main_test.go)):

| Language | Parser Implementation | Tracked Corpus Tests | Accuracy | False "No-Change" | False Change (Body/Fmt) |
|:---|:---|:---:|:---:|:---:|:---:|
| **Go** | Standard `go/parser` & `go/ast` | 20 / 20 | **100.0%** | 0 | 0 |
| **TypeScript / JS** | Lexical tokenizer (`pkg/symbols/scan/ts.go`) | 27 / 27 | **100.0%** | 0 | 0 |
| **Python** | Lexical tokenizer (`pkg/symbols/scan/python.go`) | 20 / 20 | **100.0%** | 0 | 0 |
| **Kotlin** | Lexical tokenizer (`pkg/symbols/scan/kotlin.go`) | 29 / 29 | **100.0%** | 0 | 0 |
| **Java** | Lexical tokenizer (`pkg/symbols/scan/java.go`) | 21 / 21 | **100.0%** | 0 | 0 |

> **Note on Scope**: Accuracy metrics reflect performance against the tracked corpus in `scribex-ai`. Real-world accuracy depends on language construct coverage.

---

## Anchor Scoping & Class Member Rules

1. **Gate is Anchor-Scoped**: The `check -base` impact gate evaluates changes strictly for symbols referenced by documentation anchors within the repository. Unanchored symbol changes in the codebase do not trip the gate.
2. **Class Anchors Do Not Cover Members**: A class-level anchor (e.g. `<!-- @scribex:anchor(Service.java#Service) -->`) monitors only the class declaration header itself (such as class modifiers `open`/`final`, class generics, interfaces, or superclasses). **Class-level anchors do not cover member changes.**
3. **Member-Level Anchoring**: To monitor methods, fields, properties, or constructors, docs must reference the qualified member symbol explicitly:
   - Java / Kotlin: `<!-- @scribex:anchor(Service.java#Service.doWork) -->`
   - TypeScript: `<!-- @scribex:anchor(service.ts#Service.doWork) -->`
   - Python: `<!-- @scribex:anchor(service.py#Service.do_work) -->`

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

1. **Zero False "No Change"**: Any modification to an anchored public signature (renamed parameter, altered return type, added parameter, visibility change, or deleted declaration) is detected as a contract delta.
2. **Zero False Positive**: Changes strictly confined to function bodies, comments, docstrings, or formatting/indentation produce zero contract deltas.
3. **Fail-Closed on Parse Failure**: If a supported file contains syntax errors or fails parsing during `check -base`, Scribex does not silently skip it or assume no change; it treats the file as changed and marks the impact gate dirty.
