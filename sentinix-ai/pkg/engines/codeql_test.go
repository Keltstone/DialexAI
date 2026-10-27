package engines

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeCodeQL = `#!/bin/sh
case "$2" in
  create) mkdir -p "$3" ;;
  analyze) for a in "$@"; do case "$a" in --output=*) out="${a#--output=}";; esac; done
    echo '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"CodeQL","rules":[{"id":"java/path-injection","properties":{"security-severity":"8.8"}}]}},"results":[{"ruleId":"java/path-injection","level":"error","message":{"text":"tainted path"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"A.kt"},"region":{"startLine":3}}}]}]}]}' > "$out" ;;
esac
`

func TestCodeQLEngine(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "A.kt"), []byte("class A"), 0o644)
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "codeql"), []byte(fakeCodeQL), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	fs, err := CodeQL{}.Scan(context.Background(), root)
	if err != nil || len(fs) != 1 || fs[0].Engine != "codeql" || fs[0].File != "A.kt" || fs[0].Line != 3 {
		t.Fatalf("%+v %v", fs, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (CodeQL{}).Scan(context.Background(), root); !errors.Is(err, ErrEngineUnavailable) || !strings.Contains(err.Error(), "codeql") {
		t.Fatalf("missing binary must be a coverage gap: %v", err)
	}
}
