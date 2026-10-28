#!/usr/bin/env bash
# perf-gen.sh - Deterministic synthetic repository generator for Scribex benchmarks
#
# Generates a synthetic multi-language git repository with mixed
# Go, TypeScript, Python, Kotlin, and Java files, along with 1 anchored doc
# per 100 source files.
#
# Usage:
#   ./scripts/perf-gen.sh <count> <target_dir>

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <count> <target_dir>" >&2
  exit 1
fi

COUNT="$1"
TARGET_DIR="$2"

if ! [[ "$COUNT" =~ ^[0-9]+$ ]] || [ "$COUNT" -le 0 ]; then
  echo "Error: count must be a positive integer, got: $COUNT" >&2
  exit 1
fi

mkdir -p "$TARGET_DIR"

# Fast deterministic generation using Python 3
python3 - "$COUNT" "$TARGET_DIR" <<'EOF'
import os
import sys

count = int(sys.argv[1])
target = sys.argv[2]

dirs = [
    os.path.join(target, "src", "go"),
    os.path.join(target, "src", "ts"),
    os.path.join(target, "src", "py"),
    os.path.join(target, "src", "kt"),
    os.path.join(target, "src", "java"),
    os.path.join(target, "docs"),
]
for d in dirs:
    os.makedirs(d, exist_ok=True)

anchors = []

for i in range(1, count + 1):
    lang_idx = (i - 1) % 5
    if lang_idx == 0:
        # Go
        path = os.path.join(target, "src", "go", f"file_{i}.go")
        with open(path, "w") as f:
            f.write(f"package p\n\n// Func{i} calculates a metric\nfunc Func{i}(x int) int {{\n\treturn x + {i}\n}}\n")
        if (i - 1) % 100 == 0:
            rel = f"src/go/file_{i}.go"
            sym = f"Func{i}"
            anchors.append((rel, sym, i))
    elif lang_idx == 1:
        # TypeScript
        path = os.path.join(target, "src", "ts", f"file_{i}.ts")
        with open(path, "w") as f:
            f.write(f"export function computeHandler{i}(item: number): number {{\n  return item * 2;\n}}\n")
    elif lang_idx == 2:
        # Python
        path = os.path.join(target, "src", "py", f"file_{i}.py")
        with open(path, "w") as f:
            f.write(f"def process_data_{i}(val: int) -> int:\n    return val * 10\n")
    elif lang_idx == 3:
        # Kotlin
        path = os.path.join(target, "src", "kt", f"file_{i}.kt")
        with open(path, "w") as f:
            f.write(f"package p\n\nfun executeAction{i}(id: Long): Long = id + 1\n")
    elif lang_idx == 4:
        # Java
        path = os.path.join(target, "src", "java", f"File_{i}.java")
        with open(path, "w") as f:
            f.write(f"package p;\n\npublic class File_{i} {{\n    public static int handleEvent{i}(int code) {{\n        return code + 1;\n    }}\n}}\n")

# Write 1 anchored doc per 100 files
for rel, sym, num in anchors:
    doc_path = os.path.join(target, "docs", f"doc_{num}.md")
    with open(doc_path, "w") as f:
        f.write(f"# Documentation {num}\n\n<!-- @scribex:anchor({rel}#{sym}) -->\nInitial documentation for {sym}.\n<!-- @scribex:end -->\n")

EOF

# Initialize git repository
(
  cd "$TARGET_DIR"
  git init -q
  git config user.name "Scribex Perf"
  git config user.email "perf@scribex.test"
  git add .
  git commit -q -m "initial synthetic codebase ($COUNT files)"
)

echo "Generated $COUNT synthetic source files in $TARGET_DIR"
