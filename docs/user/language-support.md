# Language Support Matrix

Scribex extracts public contracts deterministically across languages using a fast, bounded-memory O(N) lexical scanner and AST parsing (stdlib only, zero cgo).

## Construct Support Matrix

| Construct | Kotlin | Java | TS / JS | Python | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Functions / Methods** | ✔ | ✔ | ✔ | ✔ | ✔ |
| **Overloads** | ✔ | ✔ | ✔ | ✔ | ✘ *(N/A)* |
| **Extension Members** | ✔ | ✘ | ✘ | ✘ | ✘ |
| **Properties / Fields** | ✔ | ✔ | ✔ | ✔ | ✔ |
| **Constants (Value Tracked)** | ✔ (`const val`) | ✔ (`static final`) | ✔ (`const` literal) | ✔ (`UPPER_SNAKE`) | ✔ (`const`) |
| **Nested Types** | ✔ | ✔ | ✔ (`namespace`/`class`) | ✔ | ✔ |
| **Enums** | ✔ | ✔ | ✔ | ✘ *(stdlib enum)* | ✘ *(iota group)* |
| **Generics** | ✔ | ✔ | ✔ | ✔ | ✔ |
| **Annotations / Decorators** | ✔ | ✔ | ✔ | ✔ | ✘ *(N/A)* |
| **Visibility Rules** | ✔ | ✔ | ✔ | ✔ | ✔ |
| **Re-exports** | ✘ | ✘ | ✔ | ✔ (`__all__`) | ✘ |
| **Multi-line Headers** | ✔ | ✔ | ✔ | ✔ | ✔ |

---

## Important Rules & Specifications

1. **Parameter Names are Part of the Signature**:
   Renaming a parameter is treated as a contract change across all languages because callers in Kotlin, Python, and JavaScript/TypeScript can use named arguments, and documentation displays parameter names.

2. **JavaScript Types**:
   Plain JavaScript (`.js`, `.jsx`, `.mjs`, `.cjs`) contains no type annotations. Signatures capture parameter names and export shapes.

3. **TypeScript Local Re-exports**:
   `export { local }` forms for locally declared symbols do not resolve local declarations; the signature records the export statement itself (`export { local }`).

4. **Kotlin Visibility**:
   `protected` members in Kotlin are treated as non-public (only `public` and interface members without explicit visibility restrictions are exported).

5. **Python Visibility and `__all__`**:
   - Symbols beginning with a single underscore `_` are excluded as private.
   - Dunder names (e.g. `__init__`, `__call__`, `__len__`) are treated as public.
   - If a module defines a static `__all__` list or tuple of string literals, only symbols listed in `__all__` are considered public at module level.
   - Any dynamic assignment to `__all__` (e.g. list comprehension or dynamic function call) is flagged as an error (`dynamic __all__`).

6. **Java Annotations**:
   Annotations are retained in method and type signatures, with the exception of `@Override`, `@SuppressWarnings`, and `@Generated`, which are ignored as implementation-level details.

7. **Known Unsupported Constructs**:
   - Kotlin anonymous objects inside function bodies (never scanned).
   - Java anonymous inner classes and local classes.
   - TypeScript complex computed property names in class declarations.
   - Python dynamic attribute attachments (`setattr`, `globals()`).
