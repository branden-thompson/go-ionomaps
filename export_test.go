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

// Built is how many typical cells and backgrounds the library has worked
// out, for the tests of what it keeps (R-8.1).
func Built(l *Library) (cells, backgrounds int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.typical.mu.Lock()
	defer l.typical.mu.Unlock()
	for _, d := range l.typical.days {
		cells += int(d.filled.Load())
	}
	return cells, l.bg.built
}

// ReachExact is a reach whose every cell takes its own circuit, the
// measure the polar grid is held to (watchpost D-227).
func ReachExact(s Snapshot, origin LatLon, mhz float64) (ReachField, error) {
	return s.reach(origin, mhz, false)
}
