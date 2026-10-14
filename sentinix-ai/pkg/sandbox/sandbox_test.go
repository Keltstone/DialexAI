package sandbox

import (
	"strings"
	"testing"
)

func TestScanCmdIsLockedDown(t *testing.T) {
	c := Docker{Image: "scanner:1"}.ScanCmd("/tmp/wt", "semgrep", []string{"scan", "--sarif"})
	line := strings.Join(c.Args, " ")
	for _, want := range []string{"--network none", "--cap-drop ALL", "no-new-privileges", "--read-only", "-v /tmp/wt:/src:ro", "scanner:1 semgrep scan --sarif /src", "--user 65534:65534"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "docker.sock") {
		t.Error("docker socket mounted")
	}
}
