package hybrid

import (
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

var now = time.Date(2026, 10, 14, 12, 5, 0, 0, time.UTC)

// held is a station's one reading, taken ago before now.
func held(lat, lon float64, ago time.Duration, fo float32) giro.Held {
	return giro.Held{Station: giro.Station{Lat: lat, Lon: lon},
		Reading: giro.Reading{At: now.Add(-ago), FoF2: fo, M3000: 3, MUF3000: 3 * fo, HasM300: true}}
}

// TestOnlyFreshReadingsAreAssimilated is D-64: a reading taken at or before
// now and no more than an hour before is assimilated; one older, or one
// dated after now - which a recorded day replayed can hold - is not.
func TestOnlyFreshReadingsAreAssimilated(t *testing.T) {
	rs, oldest, newest := Readings([]giro.Held{
		held(40, -100, 0, 9),
		held(41, -101, For, 10),
		held(42, -102, For+time.Second, 11),
		held(43, -103, -time.Minute, 12),
	}, now)
	if len(rs) != 2 || rs[0].FoF2 != 9 || rs[1].FoF2 != 10 {
		t.Fatalf("assimilated %+v", rs)
	}
	if !oldest.Equal(now.Add(-For)) || !newest.Equal(now) {
		t.Errorf("oldest %v, newest %v", oldest, newest)
	}
	if rs, oldest, newest := Readings(nil, now); rs != nil || !oldest.IsZero() || !newest.IsZero() {
		t.Errorf("nothing held gave %v, %v, %v", rs, oldest, newest)
	}
}

// TestTooFewReadingsLeaveTheBackground is R-9.2: with fewer than three
// readings the backgrounds are handed back unchanged, nothing used; with
// three they are moved.
func TestTooFewReadingsLeaveTheBackground(t *testing.T) {
	b := background.Fields{FoF2: make([]float32, grid.Cells), M3000: make([]float32, grid.Cells)}
	for i := range b.FoF2 {
		b.FoF2[i], b.M3000[i] = 8, 3
	}
	hs := []giro.Held{held(40, -100, 0, 9), held(37, -95, 0, 9), held(43, -110, 0, 9)}
	rs, _, _ := Readings(hs[:2], now)
	if res := Field(b, rs); res.Used != 0 || &res.FoF2[0] != &b.FoF2[0] {
		t.Errorf("two readings used %d, the background copied or moved", res.Used)
	}
	rs, _, _ = Readings(hs, now)
	res := Field(b, rs)
	if v, _ := grid.Sample(res.FoF2, 40, -100); res.Used != 3 || v < 8.9 || v > 9.1 {
		t.Errorf("three readings used %d, foF2 at a station %.2f", res.Used, v)
	}
}
