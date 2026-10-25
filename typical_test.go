package ionomaps

import (
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// TestTheTypicalDayIsKept is P-4: a library keeps a typical day once made -
// the same day at the same F10.7 mean is the same cells, so each is worked
// out once - and keeps no more than two, the oldest alone going for a new one; a snapshot made by
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
	next := kept.day(day.Add(24*time.Hour), 150)
	if kept.day(day, 160); len(kept.days) != 2 || kept.day(day.Add(24*time.Hour), 150) != next {
		t.Errorf("a new mean: %d days kept, the next day kept %v; want the oldest alone gone", len(kept.days), kept.day(day.Add(24*time.Hour), 150) == next)
	}
	// RP-2: past midnight, the day after next asked, the next day stays, so
	// a snapshot still answered for it is not made anew each time.
	kept.day(day.Add(48*time.Hour), 160)
	if kept.day(day.Add(24*time.Hour), 150) != next || len(kept.days) != 2 {
		t.Errorf("a third day emptied the cache: the next day kept %v, %d kept", kept.day(day.Add(24*time.Hour), 150) == next, len(kept.days))
	}
	// RP-2: a day older than every day kept - a late answer of a snapshot
	// from before midnight - is made for that answer alone, never evicting
	// today's or tomorrow's.
	var late typicalDays
	d1, d2 := late.day(day.Add(24*time.Hour), 150), late.day(day.Add(48*time.Hour), 150)
	if a, b := late.day(day, 150), late.day(day, 150); a == b || len(late.days) != 2 || late.day(day.Add(24*time.Hour), 150) != d1 || late.day(day.Add(48*time.Hour), 150) != d2 {
		t.Errorf("a day older than every kept one: cached %v, %d kept, today kept %v, tomorrow kept %v", a == b, len(late.days), late.day(day.Add(24*time.Hour), 150) == d1, late.day(day.Add(48*time.Hour), 150) == d2)
	}
	var none *typicalDays
	if none.day(day, 150) == none.day(day, 150) {
		t.Error("a snapshot made by hand shares a day")
	}
	var answered typicalDays // a keeper of its own: kept holds the days after this one now
	s := flatSnapshot(noonUTC, 7, 21)
	s.typical = &answered
	if _, err := s.Bands([]LatLon{kansas}, 400); err != nil {
		t.Fatal(err)
	}
	// An answer at one place reads each hour at a path's few points, so it
	// makes those rows of each hour, never the globe's (watchpost R2P-1).
	filled, rows, hours := 0, 0, 0
	for _, hour := range answered.days[typicalKey{day: day, f107: s.Inputs.F107Mean}].hours {
		if hour == nil {
			continue
		}
		hours++
		for r := range hour.rows {
			row := hour.rows[r].Load()
			if row == nil {
				continue
			}
			rows++
			for i := range row {
				if row[i].Load() != 0 {
					filled++
				}
			}
		}
	}
	if filled == 0 {
		t.Error("an answer filled no kept cell")
	}
	if hours == 0 || rows > hours*grid.Rows/10 {
		t.Errorf("an answer at one place made %d rows over %d hours; want at most a tenth of each hour's %d", rows, hours, grid.Rows)
	}
}
