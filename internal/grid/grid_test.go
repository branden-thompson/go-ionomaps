package grid

import (
	"math"
	"testing"
)

// TestTheGridIsTwoDegreeCellCentres: 180 by 90 cells, the first centred at
// 89°N 179°W, the last at 89°S 179°E.
func TestTheGridIsTwoDegreeCellCentres(t *testing.T) {
	if Cols != 180 || Rows != 90 || Cells != 16200 {
		t.Fatalf("the grid is %d by %d", Cols, Rows)
	}
	for _, c := range []struct {
		i        int
		lat, lon float64
	}{{0, 89, -179}, {Cols - 1, 89, 179}, {Cols, 87, -179}, {Cells - 1, -89, 179}} {
		if lat, lon := Centre(c.i); lat != c.lat || lon != c.lon {
			t.Errorf("cell %d: %v, %v; want %v, %v", c.i, lat, lon, c.lat, c.lon)
		}
	}
	for _, off := range []int{-1, Cells} {
		if lat, lon := Centre(off); !math.IsNaN(lat) || !math.IsNaN(lon) {
			t.Errorf("cell %d, off the grid, is centred at %v, %v", off, lat, lon)
		}
	}
}
