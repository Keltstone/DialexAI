package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var presets = map[string]string{"nightly": "0 2 * * *", "weekly": "0 3 * * 0"}

// Due reports whether spec (5-field cron, or nightly/weekly) fires in the minute containing t, in zone tz.
func Due(spec, tz string, t time.Time) (bool, error) {
	if p, ok := presets[spec]; ok {
		spec = p
	}
	if tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return false, err
		}
		t = t.In(loc)
	}
	f := strings.Fields(spec)
	if len(f) != 5 {
		return false, fmt.Errorf("cron needs 5 fields, got %q", spec)
	}
	lim := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	val := [5]int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	var ok [5]bool
	for i := range f {
		m, err := field(f[i], lim[i][0], lim[i][1], val[i], i == 4)
		if err != nil {
			return false, err
		}
		ok[i] = m
	}
	// like classic cron: when both day fields are restricted, either may match
	day := ok[2] && ok[4]
	if f[2] != "*" && f[4] != "*" {
		day = ok[2] || ok[4]
	}
	return ok[0] && ok[1] && ok[3] && day, nil
}

func field(s string, lo, hi, v int, dow bool) (bool, error) {
	match := false
	for _, part := range strings.Split(s, ",") {
		rng, step, hasStep := strings.Cut(part, "/")
		n := 1
		if hasStep {
			var err error
			if n, err = strconv.Atoi(step); err != nil || n < 1 {
				return false, fmt.Errorf("bad step in %q", s)
			}
		}
		a, b := lo, hi
		if rng != "*" {
			x, y, isRange := strings.Cut(rng, "-")
			var err1, err2 error
			a, err1 = strconv.Atoi(x)
			b = a
			if isRange {
				b, err2 = strconv.Atoi(y)
			} else if hasStep {
				b = hi
			}
			if err1 != nil || err2 != nil || a < lo || b > hi || a > b {
				return false, fmt.Errorf("bad field %q", s)
			}
		}
		for x := a; x <= b; x += n {
			if x == v || dow && x == 7 && v == 0 {
				match = true
			}
		}
	}
	return match, nil
}
