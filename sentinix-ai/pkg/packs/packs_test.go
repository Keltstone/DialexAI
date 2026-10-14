package packs

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinPacksPassTheirOwnTests(t *testing.T) {
	ps, err := Builtin()
	if err != nil || len(ps) != 3 {
		t.Fatalf("%d packs, %v", len(ps), err)
	}
}

func TestSignatureAndMissingTest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	body := []byte(`{ "name":"x","version":"1","rules":[{"id":"r","severity":"low","files":["*.kt"],"pattern":"bad"}],"tests":[{"rule":"r","file":"a.kt","bad":"bad","good":"ok"}]}`)
	signed, _ := Sign(body, priv)
	if _, err := Load(signed, []ed25519.PublicKey{pub}); err != nil {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if _, err := Load(signed, []ed25519.PublicKey{other}); err == nil {
		t.Fatal("untrusted key accepted")
	}
	if _, err := Parse([]byte(`{"name":"x","version":"1","rules":[{"id":"r","severity":"low","files":["*.kt"],"pattern":"bad"}]}`)); err == nil {
		t.Fatal("rule without test accepted")
	}
}

func TestEngineDetectsStackAndScans(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "app"), 0o755)
	os.WriteFile(filepath.Join(d, "app", "AndroidManifest.xml"), []byte("<application android:debuggable=\"true\">"), 0o644)
	ps, _ := Builtin()
	fs, err := Engine{Packs: ps}.Scan(d)
	if err != nil || len(fs) != 1 || fs[0].Rule != "android/debuggable" || fs[0].Line != 1 {
		t.Fatalf("%+v %v", fs, err)
	}
	fs, _ = Engine{Packs: ps, Disabled: map[string]bool{"android/debuggable": true}}.Scan(d)
	if len(fs) != 0 {
		t.Fatal("disabled rule still fired")
	}
}
