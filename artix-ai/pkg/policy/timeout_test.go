package policy

import (
	"testing"
	"time"
)

func TestPerDriverTimeouts_ConfigChangesBehavior(t *testing.T) {
	// 1. Default timeout
	def := GetTimeout("gradle", "test")
	if def != 360*time.Second {
		t.Errorf("expected default gradle test timeout 360s, got: %v", def)
	}

	// 2. Custom policy configuration changes behavior
	p := &Policy{
		Timeouts: TimeoutsConfig{
			Gradle: DriverTimeoutConfig{
				Test: 42 * time.Second,
			},
		},
	}
	custom := p.GetDriverTimeout("gradle", "test")
	if custom != 42*time.Second {
		t.Errorf("expected custom gradle test timeout 42s, got: %v", custom)
	}

	// 3. Compile phase timeout
	pCompile := &Policy{
		Timeouts: TimeoutsConfig{
			Go: DriverTimeoutConfig{
				Compile: 15 * time.Second,
			},
		},
	}
	customGo := pCompile.GetDriverTimeout("go", "compile")
	if customGo != 15*time.Second {
		t.Errorf("expected custom go compile timeout 15s, got: %v", customGo)
	}
}
