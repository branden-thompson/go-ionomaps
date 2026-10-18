package ionomaps

import "testing"

// BenchmarkReachFirst is a reach's cost from a snapshot whose typical day
// nothing has read yet: the climatology at each cell its paths read is
// worked out (watchpost D-205: "its cost is measured").
func BenchmarkReachFirst(b *testing.B) {
	for b.Loop() {
		s := flatSnapshot(noonUTC, 7, 21)
		s.typical = &typicalDays{}
		_, _ = s.Reach(kansas, 14)
	}
}

// BenchmarkReachKept is a reach's cost once the library keeps the typical
// day: a host's next reach that day, at another frequency or origin.
func BenchmarkReachKept(b *testing.B) {
	s := flatSnapshot(noonUTC, 7, 21)
	s.typical = &typicalDays{}
	_, _ = s.Reach(kansas, 14)
	for b.Loop() {
		_, _ = s.Reach(kansas, 10)
	}
}

// BenchmarkAnHourAheadFirst is an hour ahead's cost on a day nothing has
// read: its cells are the typical day's, worked out once and shared with the
// answers (R-8.1, watchpost D-222).
func BenchmarkAnHourAheadFirst(b *testing.B) {
	for b.Loop() {
		s := flatSnapshot(noonUTC, 7, 21)
		s.typical = &typicalDays{}
		_, _ = s.Ahead(3)
	}
}
