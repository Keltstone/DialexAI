package anchors

import (
	"errors"
	"strings"
	"testing"
)

func TestAnchorLookupError(t *testing.T) {
	// 1. Anchor into unparsable file
	parseErr := errors.New("unbalanced braces")
	look := func(file, sym string) (string, bool, error) {
		if file == "broken.kt" {
			return "", false, parseErr
		}
		if file == "x.rs" {
			return "", false, errors.New("symbols: unsupported language")
		}
		if file == "good.go" && sym == "Good" {
			return "func Good()", true, nil
		}
		return "", false, nil
	}

	docBroken := "<!-- @scribex:anchor(broken.kt#Foo) -->\n```signature\nfun old()\n```\nprose"
	issues, out := Process(docBroken, look, true, nil)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Stale {
		t.Errorf("issue Stale should be false, got true")
	}
	if !strings.Contains(issues[0].Msg, "cannot analyse broken.kt") {
		t.Errorf("expected 'cannot analyse broken.kt' in %q", issues[0].Msg)
	}
	if out != docBroken {
		t.Fatalf("sync should leave doc byte-identical on lookup error, got:\n%s\nwant:\n%s", out, docBroken)
	}

	// 2. Anchor into unsupported extension
	docUnsupported := "<!-- @scribex:anchor(x.rs#Foo) -->\n```signature\nfn old()\n```\nprose"
	issues, out = Process(docUnsupported, look, true, nil)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Stale {
		t.Errorf("issue Stale should be false, got true")
	}
	if !strings.Contains(issues[0].Msg, "cannot analyse x.rs") || !strings.Contains(issues[0].Msg, "unsupported language") {
		t.Errorf("expected 'cannot analyse x.rs' and 'unsupported language' in %q", issues[0].Msg)
	}
	if out != docUnsupported {
		t.Fatalf("sync should leave doc byte-identical on unsupported language, got:\n%s\nwant:\n%s", out, docUnsupported)
	}

	// 3. Healthy anchor in the same doc still syncs
	mixedDoc := "<!-- @scribex:anchor(broken.kt#Foo) -->\n```signature\nfun old()\n```\n<!-- @scribex:anchor(good.go#Good) -->\n"
	issues, out = Process(mixedDoc, look, true, nil)
	if len(issues) != 2 { // 1 error from broken.kt, 1 stale from good.go
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	// broken.kt anchor block must remain byte-identical
	if !strings.Contains(out, "<!-- @scribex:anchor(broken.kt#Foo) -->\n```signature\nfun old()\n```") {
		t.Errorf("broken anchor part was modified: %s", out)
	}
	// good.go anchor must have synced
	if !strings.Contains(out, "<!-- @scribex:anchor(good.go#Good) sig=") || !strings.Contains(out, "func Good()") {
		t.Errorf("healthy anchor was not synced: %s", out)
	}
}
