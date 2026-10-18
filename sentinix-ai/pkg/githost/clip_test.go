package githost

import (
	"strings"
	"testing"
)

func TestClipKeepsStatusUnderHostLimit(t *testing.T) {
	long := strings.Repeat("some/long/path/File.kt, ", 50)
	if got := []rune(clip(long)); len(got) > 140 {
		t.Fatalf("len %d", len(got))
	}
	if clip("2 new finding(s)") != "2 new finding(s)" {
		t.Fatal("short description altered")
	}
}
