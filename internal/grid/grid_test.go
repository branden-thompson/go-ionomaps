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

// TestASampleIsBilinearBetweenCentres is the grid's sampler (G5): between
// cell centres a value is bilinear, across the antimeridian too, and held to
// the outermost rows past them.
func TestASampleIsBilinearBetweenCentres(t *testing.T) {
	v := make([]float32, Cells)
	for i := range v {
		lat, lon := Centre(i)
		v[i] = float32(lat + 0.5*lon)
	}
	for _, p := range [][2]float64{{10, 20}, {11, 21.3}, {-45.5, -100.7}, {0, 0}} {
		got, ok := Sample(v, p[0], p[1])
		if want := p[0] + 0.5*p[1]; !ok || math.Abs(got-want) > 1e-4 {
			t.Errorf("at %v: %v (%v); a linear field gives %v", p, got, ok, want)
		}
	}
	w := make([]float32, Cells)
	for i := range w {
		_, lon := Centre(i)
		w[i] = float32(math.Cos(lon * math.Pi / 180))
	}
	east, _ := Sample(w, 0, 179.9)
	west, _ := Sample(w, 0, -179.9)
	if math.Abs(east-west) > 1e-3 {
		t.Errorf("across the antimeridian: %v east and %v west", east, west)
	}
	if pole, ok := Sample(v, 90, 0); !ok || math.Abs(pole-89) > 1e-4 {
		t.Errorf("at the pole: %v; held to the 89° row", pole)
	}
	if _, ok := Sample(v[:10], 0, 0); ok {
		t.Error("a field of the wrong size was sampled")
	}
}

// TestAPlaceFallsInTheCellAroundIt: every cell's centre falls in that cell,
// a place on a cell's edge in the cell to its south and east, longitudes
// wrap, the poles fall in the outermost rows, and a place that is no number,
// or a latitude off the globe, falls in none.
func TestAPlaceFallsInTheCellAroundIt(t *testing.T) {
	for i := 0; i < Cells; i++ {
		lat, lon := Centre(i)
		if got, ok := CellOf(lat, lon); !ok || got != i {
			t.Fatalf("cell %d's centre falls in %d", i, got)
		}
	}
	for _, c := range []struct {
		lat, lon float64
		want     int
	}{{90, -180, 0}, {88, -178, Cols + 1}, {-90, 180, Cells - Cols}, {0, 181, (Rows / 2) * Cols}, {89, -541, Cols - 1}} {
		if got, ok := CellOf(c.lat, c.lon); !ok || got != c.want {
			t.Errorf("(%v, %v) falls in %d; want %d", c.lat, c.lon, got, c.want)
		}
	}
	for _, p := range [][2]float64{{math.NaN(), 0}, {0, math.Inf(1)}, {90.5, 0}, {-91, 10}} {
		if _, ok := CellOf(p[0], p[1]); ok {
			t.Errorf("%v falls in a cell", p)
		}
	}
}
