package ionomaps

import "time"

// HeldReadings is what the library holds from GIRO, for the tests: how many
// stations' readings, and the oldest - held, whether or not the field
// assimilates them.
func HeldReadings(l *Library) (n int, oldest time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, h := range l.giro.Readings() {
		if oldest.IsZero() || h.Reading.At.Before(oldest) {
			oldest = h.Reading.At
		}
	}
	return len(l.giro.Readings()), oldest
}

// Built is how many climatology hours ahead and backgrounds the library has
// worked out, for the tests of what it keeps (R-8.1).
func Built(l *Library) (aheadHours, backgrounds int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ahead.mu.Lock()
	defer l.ahead.mu.Unlock()
	return l.ahead.built, l.bg.built
}
