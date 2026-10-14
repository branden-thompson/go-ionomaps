package ionomaps

import "time"

// HeldReadings is what the library holds from GIRO, for the tests: how many
// stations' readings, and the oldest. The snapshot says them once G5
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
