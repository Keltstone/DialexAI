package scheduler

import "testing"

func TestCheckRefRejectsOptionLikeRefs(t *testing.T) {
	for _, bad := range []string{"", "-x", "--upload-pack=id", "a b", "a..b", "a:b", "a\nb"} {
		if checkRef(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, ok := range []string{"main", "feature/x-1", "refs/pull/12/head", "0123456789abcdef0123456789abcdef01234567"} {
		if err := checkRef(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
}
