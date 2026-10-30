package magcoords

import (
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// TestACellsCoordinatesAreQDsExactly: the coordinates read by cell are QD's
// at the cell's centre, bit for bit, in the table's years, between them, on
// a year's first moment and outside the years either side.
func TestACellsCoordinatesAreQDsExactly(t *testing.T) {
	for _, at := range []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC),
		time.Date(2024, 3, 9, 6, 0, 0, 0, time.UTC),
		time.Date(2031, 11, 20, 18, 0, 0, 0, time.UTC),
		time.Date(1990, 5, 5, 0, 0, 0, 0, time.UTC),
	} {
		m := Cells(at)
		for i := range grid.Cells {
			lat, lon := grid.Centre(i)
			wantLat, wantLon, wantOut := QD(lat, lon, at)
			gotLat, gotLon := m.At(i)
			if gotLat != wantLat || gotLon != wantLon || m.Extrapolated() != wantOut {
				t.Fatalf("%v cell %d: %v, %v, %v; QD gives %v, %v, %v", at, i, gotLat, gotLon, m.Extrapolated(), wantLat, wantLon, wantOut)
			}
		}
	}
}

func BenchmarkCells(b *testing.B) {
	at := time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC)
	for b.Loop() {
		m := Cells(at)
		for i := range grid.Cells {
			_, _ = m.At(i)
		}
	}
}

// TestATableOfAnotherShapeIsRefused: the embedded table is read; one with
// another magic, another grid, years out of order or a length that is not
// whole reads as no table at all.
func TestATableOfAnotherShapeIsRefused(t *testing.T) {
	if y := read(table); len(y.lat) == 0 {
		t.Fatal("the embedded table is refused")
	}
	head := len(magic) + 12
	for name, plant := range map[string]func(b []byte) []byte{
		"magic":  func(b []byte) []byte { b[0] = 'X'; return b },
		"cols":   func(b []byte) []byte { b[len(magic)+4]++; return b },
		"rows":   func(b []byte) []byte { b[len(magic)+8]++; return b },
		"years":  func(b []byte) []byte { b[head+4]++; return b }, // the second year not the first's next
		"length": func(b []byte) []byte { return b[:len(b)-1] },
	} {
		if y := read(plant(append([]byte(nil), table...))); len(y.lat) != 0 {
			t.Errorf("a table with its %s planted wrong was read", name)
		}
	}
}
