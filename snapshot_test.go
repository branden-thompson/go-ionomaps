package ionomaps

// snapshot_test.go — plan task G1.1 (R-1.1, R-1.2): a field is a 2° global
// grid a go-tuiMaps host hands in directly, and every field says when it is
// valid, when it was computed and what it was made from.

import (
	"math"
	"testing"
	"time"
)

// globalField is an empty field over the global grid: every value zero and
// every cell without data.
func globalField() Field {
	f := newField()
	for i := range f.Values {
		f.NoData.set(i)
	}
	return f
}

// TestTheGridIsTwoDegrees is R-1.1: 180 cells by 90, each 2° on a side, the
// first cell's centre at 179°W, 89°N; no grid-step option (D-138).
func TestTheGridIsTwoDegrees(t *testing.T) {
	f := globalField()
	if f.Cols != 180 || f.Rows != 90 {
		t.Fatalf("the grid is %d by %d cells; 2° over the globe is 180 by 90", f.Cols, f.Rows)
	}
	if w, h := (f.East-f.West)/float64(f.Cols), (f.North-f.South)/float64(f.Rows); w != 2 || h != 2 {
		t.Errorf("a cell is %v° by %v°; want 2° by 2°", w, h)
	}
	for _, c := range []struct {
		at   LatLon
		cell int
	}{{LatLon{89, -179}, 0}, {LatLon{89, -177}, 1}, {LatLon{87, -179}, f.Cols}, {LatLon{-89, 179}, f.Cols*f.Rows - 1}} {
		if i, ok := cellIndex(f, c.at); !ok || i != c.cell {
			t.Errorf("%+v falls in cell %d; want %d (rows from the north, each west to east)", c.at, i, c.cell)
		}
	}
}

// TestAGlobalFieldIsAValidTuimapsGrid is R-1.1: outer edges -180..180 and
// -90..90, no antimeridian crossing, one value a cell, every value finite
// (R-3.3), as go-tuiMaps' hand-in requires.
func TestAGlobalFieldIsAValidTuimapsGrid(t *testing.T) {
	f := globalField()
	if f.West != -180 || f.East != 180 || f.South != -90 || f.North != 90 {
		t.Errorf("the edges are %v..%v, %v..%v; want -180..180, -90..90", f.West, f.East, f.South, f.North)
	}
	if !(f.West < f.East && f.South < f.North) {
		t.Error("west is not west of east, or south not south of north")
	}
	if len(f.Values) != f.Cols*f.Rows {
		t.Fatalf("%d values for %d cells", len(f.Values), f.Cols*f.Rows)
	}
	for i, v := range f.Values {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("value %d is %v: no NaN or Inf leaves the library", i, v)
		}
	}
	if f.NoData.Len() != len(f.Values) {
		t.Errorf("NoData covers %d cells of %d", f.NoData.Len(), len(f.Values))
	}
}

// TestAFieldSaysWhenAndFromWhat is R-1.2: an hour says when it is valid, the
// snapshot when it was computed, and each field its background by name; a
// cell with no known value says so in NoData rather than in its value.
func TestAFieldSaysWhenAndFromWhat(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	s := Snapshot{Computed: at.Add(3 * time.Minute), Now: Hour{At: at, FoF2: globalField(), MUF3000: globalField()},
		Background: Backgrounds{FoF2: GloTEC, M3000: Climatology}}
	if s.Now.At != at || s.Computed.Before(s.Now.At) {
		t.Errorf("valid %v, computed %v", s.Now.At, s.Computed)
	}
	if s.Background.FoF2.String() != "GloTEC" || s.Background.M3000.String() != "climatology" || NoBackground.String() != "none" {
		t.Errorf("backgrounds named %q, %q and %q", s.Background.FoF2, s.Background.M3000, NoBackground)
	}
	f := s.Now.FoF2
	if !f.NoData.Has(0) {
		t.Error("an empty field's cells do not say they have no value")
	}
	f.NoData = newBitset(len(f.Values))
	f.NoData.set(5)
	if !f.NoData.Has(5) || f.NoData.Has(4) || f.NoData.Has(-1) || f.NoData.Has(len(f.Values)) {
		t.Error("NoData does not say exactly which cells have no value")
	}
}
