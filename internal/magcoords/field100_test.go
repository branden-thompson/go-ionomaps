package magcoords

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// TestTheFieldAt100kmIsTheTables is G6.2's inputs (P.533-14 §5.2.2): at a
// cell centre on a year's first moment the gyrofrequency and dip at 100 km
// are the table's; at Alpena they are IGRF-14's (ppigrf: dip 70.44°, 51,350
// nT) to the grid's interpolation; everywhere the gyrofrequency is 0.4 to
// 1.9 MHz and the dip within ±90°; a table of another kind is refused, and
// a place off the globe has none.
func TestTheFieldAt100kmIsTheTables(t *testing.T) {
	f := readYearTable(field100Table, field100Magic, 2)
	if len(f.parts) != 2 || len(f.parts[0]) == 0 {
		t.Fatal("the embedded 100 km field table is refused")
	}
	jan := time.Date(f.first+1, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, i := range []int{0, 4545, grid.Cells - 1} {
		lat, lon := grid.Centre(i)
		fh, dip := Field100(lat, lon, jan)
		if math.Abs(fh-float64(f.parts[0][1][i])) > 1e-6 || math.Abs(dip-float64(f.parts[1][1][i])) > 1e-4 {
			t.Errorf("cell %d: fH %.6f, dip %.4f; the table's %v, %v", i, fh, dip, f.parts[0][1][i], f.parts[1][1][i])
		}
	}
	fh, dip := Field100(45.1, -83.6, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if math.Abs(dip-70.4382) > 0.3 || math.Abs(fh-27.99249e-6*51350.26) > 0.01 {
		t.Errorf("Alpena: fH %.4f MHz, dip %.3f°; IGRF-14 gives 1.4374 and 70.438", fh, dip)
	}
	for lat := -88.0; lat <= 88; lat += 22 {
		for lon := -180.0; lon < 180; lon += 45 {
			if fh, dip := Field100(lat, lon, jan.AddDate(0, 6, 0)); !(fh > 0.4 && fh < 1.9) || !(math.Abs(dip) <= 90) {
				t.Fatalf("at %v°, %v°: fH %v MHz, dip %v°", lat, lon, fh, dip)
			}
		}
	}
	skipped := append([]byte(nil), field100Table...)
	skipped[len(field100Magic)+12+4]++ // the second year, one on: the years are not consecutive
	if len(readYearTable(skipped, field100Magic, 2).parts) != 0 {
		t.Error("a table whose years skip one was read")
	}
	if len(readYearTable(gyroTable, field100Magic, 2).parts) != 0 || len(readYearTable(field100Table[:len(field100Table)-4], field100Magic, 2).parts) != 0 {
		t.Error("a table of another kind, or not whole, was read")
	}
	if fh, dip := Field100(91, 0, jan); !math.IsNaN(fh) || !math.IsNaN(dip) {
		t.Error("a place off the globe has a field")
	}
	if v := (yearTable{}).value(0, 10, 10, jan); !math.IsNaN(v) {
		t.Errorf("a table not read gave %v", v)
	}
}
