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

func TestInContainerSandbox(t *testing.T) {
	s := InContainer{}
	c := s.ScanCmd("/tmp/wt", "semgrep", []string{"scan", "--sarif"})
	line := strings.Join(c.Args, " ")
	if c.Dir != "/tmp/wt" {
		t.Errorf("expected working directory /tmp/wt, got %s", c.Dir)
	}
	if !strings.HasPrefix(line, "semgrep scan --sarif") {
		t.Errorf("unexpected command line: %s", line)
	}
	if strings.Contains(line, "docker") || strings.Contains(line, "docker.sock") {
		t.Errorf("in-container sandbox must not reference docker or docker.sock: %s", line)
	}
}
