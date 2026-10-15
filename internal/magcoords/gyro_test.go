package magcoords

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// TestTheGyrofrequencyIsTheTables is G6.1's fH: at a cell centre on a
// year's first moment the gyrofrequency is the table's for that cell; it is
// between 0.4 and 1.8 MHz everywhere; a table of another shape is refused,
// and a place off the globe has none.
func TestTheGyrofrequencyIsTheTables(t *testing.T) {
	t0 := readYearTable(gyroTable, gyroMagic, 1)
	g := struct {
		first int
		cells [][]float32
	}{t0.first, nil}
	if len(t0.parts) == 1 {
		g.cells = t0.parts[0]
	}
	if len(g.cells) == 0 {
		t.Fatal("the embedded gyrofrequency table is refused")
	}
	jan := time.Date(g.first+1, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, i := range []int{0, 4545, grid.Cells - 1} {
		lat, lon := grid.Centre(i)
		if got, want := Gyro(lat, lon, jan), float64(g.cells[1][i]); math.Abs(got-want) > 1e-6 {
			t.Errorf("cell %d: fH %.6f; the table's %.6f", i, got, want)
		}
	}
	mid := time.Date(g.first+1, 7, 2, 12, 0, 0, 0, time.UTC) // about halfway between two years' tables
	i, w, _ := span(years{first: g.first, lat: g.cells}, mid)
	lat, lon := grid.Centre(4545)
	if got, want := Gyro(lat, lon, mid), float64(g.cells[i][4545])+w*(float64(g.cells[i+1][4545])-float64(g.cells[i][4545])); math.Abs(got-want) > 1e-6 || w < 0.4 || w > 0.6 {
		t.Errorf("mid-year fH %.6f; between the years' tables %.6f (part %.2f)", got, want, w)
	}
	for lat := -88.0; lat <= 88; lat += 22 {
		for lon := -180.0; lon < 180; lon += 45 {
			if f := Gyro(lat, lon, jan.AddDate(0, 6, 0)); !(f > 0.4 && f < 1.8) {
				t.Fatalf("fH at %v°, %v° is %v MHz", lat, lon, f)
			}
		}
	}
	bad := append([]byte(nil), gyroTable...)
	bad[0] = 'X'
	if len(readYearTable(bad, gyroMagic, 1).parts) != 0 || len(readYearTable(gyroTable[:len(gyroTable)-4], gyroMagic, 1).parts) != 0 {
		t.Error("a table with its magic or its length wrong was read")
	}
	if !math.IsNaN(Gyro(91, 0, jan)) || !math.IsNaN(Gyro(math.NaN(), 0, jan)) {
		t.Error("a place off the globe has a gyrofrequency")
	}
}
