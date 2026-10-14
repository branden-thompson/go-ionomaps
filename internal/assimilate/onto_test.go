package assimilate

import (
	"math"
	"testing"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// flat is a background of one value in every cell.
func flat(v float32) []float32 {
	out := make([]float32, grid.Cells)
	for i := range out {
		out[i] = v
	}
	return out
}

// readings are five stations, foF2 a megahertz either way of a background
// of 7, M(3000)F2 at 3.3 over a background of 3, one unscored (D-61).
var readings = []Reading{
	{Lat: 45.1, Lon: -83.6, FoF2: 8, M3000: 3.3, HasM: true},
	{Lat: 37.1, Lon: -6.7, FoF2: 6, M3000: 3.3, HasM: true},
	{Lat: 50.1, Lon: 4.6, FoF2: 8, M3000: 3.3, HasM: true},
	{Lat: -35.3, Lon: 149.0, FoF2: 6, M3000: 3.3, HasM: true, Unscored: true},
	{Lat: 13.6, Lon: 144.9, FoF2: 7.5, HasM: false},
}

// cellOf is the cell a place falls in.
func cellOf(lat, lon float64) int {
	row := min(max(int((90-lat)/grid.Step), 0), grid.Rows-1)
	col := min(max(int((lon+180)/grid.Step), 0), grid.Cols-1)
	return row*grid.Cols + col
}

// TestTheReadingsAreAssimilatedOntoTheBackground is R-9.2 (the hybrid,
// D-101): foF2 near a station moves toward its reading, M(3000)F2 toward
// the readings that carry one; every value is a number.
func TestTheReadingsAreAssimilatedOntoTheBackground(t *testing.T) {
	r, err := Onto(flat(7), flat(3), readings)
	if err != nil {
		t.Fatal(err)
	}
	if r.Used != 5 || r.UsedM != 4 {
		t.Errorf("%d readings used for foF2 and %d for M(3000)F2; want 5 and 4", r.Used, r.UsedM)
	}
	high, low := r.FoF2[cellOf(45.1, -83.6)], r.FoF2[cellOf(37.1, -6.7)]
	if !(high > 7.3) || !(low < 6.9) {
		t.Errorf("foF2 is %v where a station read 8 and %v where one read 6, over a background of 7", high, low)
	}
	if m := r.M3000[cellOf(50.1, 4.6)]; math.Abs(float64(m)-3.3) > 0.05 {
		t.Errorf("M(3000)F2 near a station that read 3.3 is %v", m)
	}
	for i := range r.FoF2 {
		if !(r.FoF2[i] > 0) || math.IsInf(float64(r.FoF2[i]), 0) || !(r.M3000[i] > 0) {
			t.Fatalf("cell %d: foF2 %v, M(3000)F2 %v", i, r.FoF2[i], r.M3000[i])
		}
	}
}

// TestAFieldSaysWhereItIsMeasured is R-3.2 and R-2.11 (D-116, D-61): every
// cell carries the distance to its nearest station used, and whether that
// station is unscored.
func TestAFieldSaysWhereItIsMeasured(t *testing.T) {
	r, _ := Onto(flat(7), flat(3), readings)
	if km := r.NearestKm[cellOf(45.1, -83.6)]; km > 160 {
		t.Errorf("the cell holding a station is %v km from its nearest station", km)
	}
	if km := r.NearestKm[cellOf(-80, -60)]; km < 2000 {
		t.Errorf("a cell in Antarctica is %v km from its nearest station", km)
	}
	if !r.NearestUnscored[cellOf(-35.3, 149.0)] || r.NearestUnscored[cellOf(45.1, -83.6)] {
		t.Error("the unscored station's cell is not marked, or a scored one's is")
	}
}

// TestFewerThanThreeReadingsLeaveTheBackground is week.py's floor: with
// fewer than three readings nothing is assimilated and the background is
// unchanged; M(3000)F2 alone is left when fewer than three carry it.
func TestFewerThanThreeReadingsLeaveTheBackground(t *testing.T) {
	r, err := Onto(flat(7), flat(3), readings[:2])
	if err != nil || r.Used != 0 || r.FoF2[0] != 7 || r.M3000[0] != 3 || len(r.NearestKm) != 0 {
		t.Errorf("two readings: %d used, foF2 %v, M %v, %d distances, %v", r.Used, r.FoF2[0], r.M3000[0], len(r.NearestKm), err)
	}
	onlyFo := []Reading{readings[4], {Lat: 0, Lon: 0, FoF2: 7}, {Lat: 10, Lon: 10, FoF2: 7}}
	r, _ = Onto(flat(7), flat(3), onlyFo)
	if r.Used != 3 || r.UsedM != 0 || r.M3000[cellOf(0, 0)] != 3 {
		t.Errorf("readings without M(3000)F2: %d, %d used; M %v", r.Used, r.UsedM, r.M3000[cellOf(0, 0)])
	}
	if _, err := Onto(flat(7)[:5], flat(3), readings); err == nil {
		t.Error("a background off the grid was taken")
	}
	if _, err := Onto(flat(7), flat(3), make([]Reading, maxReadings+1)); err == nil {
		t.Error("more readings than any update holds were taken")
	}
}
