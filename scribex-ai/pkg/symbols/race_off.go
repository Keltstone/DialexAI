//go:build !race

package symbols

// isRaceDetector is false when compiled without -race.
const isRaceDetector = false
