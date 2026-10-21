package store

import "testing"

func TestSoDAliasUnicode(t *testing.T) {
	base := "alice@corp.test"
	for _, v := range []string{
		"ＡＬＩＣＥ@corp.test",
		"al​ice@corp.test",
		"A.l.i.c.e+x@Corp.Test",
		"\ufeffalice@corp.test",
		"alice‮@corp.test",
		"\u0430lice@corp.test",
	} {
		if !isSameActorOrAlias(base, v) {
			t.Errorf("%q not recognised as alias of %q", v, base)
		}
	}
	if isSameActorOrAlias(base, "bob@corp.test") {
		t.Error("distinct users treated as same")
	}
}
