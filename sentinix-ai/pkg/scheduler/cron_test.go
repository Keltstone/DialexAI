package scheduler

import (
	"testing"
	"time"
)

func TestDue(t *testing.T) {
	at := time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC) // Wednesday
	for spec, want := range map[string]bool{
		"nightly": true, "weekly": false, "*/15 2 * * *": true, "1 2 * * *": false,
		"0 2 14 * 1":  true, // dom OR dow when both restricted
		"0 2 * * 1-3": true, "0 2 * * 7": false, "0 1,2 * * *": true,
	} {
		got, err := Due(spec, "", at)
		if err != nil || got != want {
			t.Errorf("%q: got %v %v want %v", spec, got, err, want)
		}
	}
	if _, err := Due("bogus", "", at); err == nil {
		t.Error("bad spec accepted")
	}
	// 02:00 UTC is 22:00 the day before in New York
	if ok, _ := Due("0 22 * * *", "America/New_York", time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)); !ok {
		t.Error("timezone ignored")
	}
}
