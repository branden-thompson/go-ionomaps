package ionomaps

import (
	"testing"
	"time"
)

// TestTheTypicalDayIsKept is P-4: a library keeps a typical day once made -
// the same day at the same F10.7 mean is the same cells, so each is worked
// out once - and keeps no more than two days or one mean; a snapshot made by
// hand gets a day of its own each time.
func TestTheTypicalDayIsKept(t *testing.T) {
	var kept typicalDays
	day := noonUTC.Truncate(24 * time.Hour)
	a := kept.day(day, 150)
	if b := kept.day(day, 150); b != a {
		t.Error("the same day and mean made anew")
	}
	if b := kept.day(day.Add(24*time.Hour), 150); b == a || len(kept.days) != 2 {
		t.Errorf("the next day: the same %v, %d kept", b == a, len(kept.days))
	}
	if kept.day(day, 160); len(kept.days) != 1 {
		t.Errorf("a new mean: %d days kept; the old ones are not asked again", len(kept.days))
	}
	var none *typicalDays
	if none.day(day, 150) == none.day(day, 150) {
		t.Error("a snapshot made by hand shares a day")
	}
	s := flatSnapshot(noonUTC, 7, 21)
	s.typical = &kept
	if _, err := s.Bands([]LatLon{kansas}, 400); err != nil {
		t.Fatal(err)
	}
	filled := 0
	for h := range kept.days[typicalKey{day: day, f107: 150}].hours {
		for i := range kept.days[typicalKey{day: day, f107: 150}].hours[h] {
			if kept.days[typicalKey{day: day, f107: 150}].hours[h][i].Load() != 0 {
				filled++
			}
		}
	}
	if filled == 0 {
		t.Error("an answer filled no kept cell")
	}
}
