package background

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/climatology"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

var noon = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

// expected is the climatology at one of our cells, as the background must
// give it: the cell centre's quasi-dipole coordinates and MLT, the hour
// folded.
func expected(row, col int) (fo, m3 float64) {
	lat, lon := 89-2*float64(row), -179+2*float64(col)
	qdlat, qdlon, _ := magcoords.QD(lat, lon, noon)
	h := climatology.NewHour(climatology.Refits(), noon, 150)
	return h.At(qdlat, magcoords.MLTFrom(qdlon, magcoords.SubsolarQD(noon)))
}

// flatGloTEC is a GloTEC grid whose foF2 rises with latitude, 5 MHz at the
// equator and 1 MHz every 30°, so bilinear regridding is exact within its
// rows.
func flatGloTEC() *glotec.Grid {
	g := &glotec.Grid{Valid: noon, West: -180, South: -90, East: 180, North: 90, Cols: 72, Rows: 72,
		FoF2: make([]float32, 72*72), HmF2: make([]float32, 72*72), Quality: make([]uint8, 72*72), NoData: make(glotec.Cells, 72*72)}
	for r := range 72 {
		for c := range 72 {
			g.FoF2[r*72+c] = float32(5 + (88.75-2.5*float64(r))/30)
		}
	}
	return g
}

// TestTheFallbackIsUsedAndNamed is G4.1 (watchpost D-101, FR-5.2): with no
// GloTEC, foF2's background is the climatology at every cell, and named so;
// with GloTEC it is GloTEC's, regridded, and named so.
func TestTheFallbackIsUsedAndNamed(t *testing.T) {
	b := Build(noon, 150, nil)
	if b.FoF2From != Climatology || b.M3000From != Climatology {
		t.Fatalf("without GloTEC the backgrounds are %v and %v", b.FoF2From, b.M3000From)
	}
	for _, rc := range [][2]int{{0, 0}, {20, 33}, {44, 90}, {70, 150}, {89, 179}} {
		fo, _ := expected(rc[0], rc[1])
		if got := b.FoF2[rc[0]*grid.Cols+rc[1]]; math.Abs(float64(got)-fo) > 1e-4 {
			t.Errorf("cell %v: foF2 %v; the climatology gives %v", rc, got, fo)
		}
	}
	g := Build(noon, 150, flatGloTEC())
	if g.FoF2From != GloTEC || g.Filled != 0 {
		t.Fatalf("with GloTEC the background is %v, %d cells filled", g.FoF2From, g.Filled)
	}
	for _, row := range []int{1, 22, 45, 67, 88} {
		lat := 89 - 2*float64(row)
		if got, want := float64(g.FoF2[row*grid.Cols+50]), 5+lat/30; math.Abs(got-want) > 1e-4 {
			t.Errorf("row %d (%v°): foF2 %v; GloTEC regridded gives %v", row, lat, got, want)
		}
	}
}

// TestEveryCellIsTheClimatologyAtItsCentre: without GloTEC, every cell is
// the climatology read at its centre's quasi-dipole coordinates and MLT,
// exactly as one place is read.
func TestEveryCellIsTheClimatologyAtItsCentre(t *testing.T) {
	b := Build(noon, 150, nil)
	h := climatology.NewHour(climatology.Refits(), noon, 150)
	for i := range grid.Cells {
		lat, lon := 89-2*float64(i/180), -179+2*float64(i%180)
		qdlat, qdlon, _ := magcoords.QD(lat, lon, noon)
		fo, m3 := h.At(qdlat, magcoords.MLTFrom(qdlon, magcoords.SubsolarQD(noon)))
		if b.FoF2[i] != float32(fo) || b.M3000[i] != float32(m3) {
			t.Fatalf("cell %d: %v, %v; the climatology at its centre gives %v, %v", i, b.FoF2[i], b.M3000[i], float32(fo), float32(m3))
		}
	}
}

// TestExtrapolatedCoordinatesAreSaidOfTheHour is R-9.7: an hour outside
// the coordinate table's years says its coordinates were extrapolated; one
// inside them does not.
func TestExtrapolatedCoordinatesAreSaidOfTheHour(t *testing.T) {
	if Build(noon, 150, nil).CoordsExtrapolated {
		t.Error("an hour inside the table's years says its coordinates were extrapolated")
	}
	if !Build(time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC), 150, nil).CoordsExtrapolated {
		t.Error("an hour past the table's years does not say its coordinates were extrapolated")
	}
}

// TestM3000BackgroundIsTheClimatology is R-9.3 (watchpost D-101): M(3000)F2
// is the climatology's at every cell, with GloTEC or without.
func TestM3000BackgroundIsTheClimatology(t *testing.T) {
	b := Build(noon, 150, flatGloTEC())
	for _, rc := range [][2]int{{3, 7}, {30, 100}, {45, 0}, {60, 60}, {86, 170}} {
		_, m3 := expected(rc[0], rc[1])
		if got := b.M3000[rc[0]*grid.Cols+rc[1]]; math.Abs(float64(got)-m3) > 1e-4 {
			t.Errorf("cell %v: M(3000)F2 %v; the climatology gives %v", rc, got, m3)
		}
	}
}

// TestARefusedGloTECCellIsFilled is R-3.3: GloTEC cells refused by their
// bounds are left out of the regridding; a cell with no valid GloTEC around
// it takes the climatology's value and is counted; nothing leaves that is
// no finite number.
func TestARefusedGloTECCellIsFilled(t *testing.T) {
	g := flatGloTEC()
	for r := 30; r < 40; r++ { // a band of refused cells, about 25° wide
		for c := 20; c < 30; c++ {
			g.NoData[r*72+c] = true
			g.FoF2[r*72+c] = 0
		}
	}
	b := Build(noon, 150, g)
	if b.Filled == 0 {
		t.Error("no cell was filled from the climatology inside a block of refused GloTEC cells")
	}
	for i, v := range b.FoF2 {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v <= 0 {
			t.Fatalf("cell %d: foF2 %v", i, v)
		}
	}
	all := flatGloTEC()
	for i := range all.NoData {
		all.NoData[i] = true
	}
	if b := Build(noon, 150, all); b.FoF2From != Climatology || b.Filled != grid.Cells {
		t.Errorf("a GloTEC grid with no valid cell: background %v, %d filled", b.FoF2From, b.Filled)
	}
}

// TestAGridTooSmallToRegridIsTheClimatology: a GloTEC grid of one row or
// one column has no cells to interpolate between; the background is the
// climatology's, every cell filled, and nothing panics.
func TestAGridTooSmallToRegridIsTheClimatology(t *testing.T) {
	for _, cr := range [][2]int{{72, 1}, {1, 72}} {
		g := &glotec.Grid{Valid: noon, West: -180, South: -90, East: 180, North: 90, Cols: cr[0], Rows: cr[1],
			FoF2: make([]float32, cr[0]*cr[1]), NoData: make(glotec.Cells, cr[0]*cr[1])}
		for i := range g.FoF2 {
			g.FoF2[i] = 7
		}
		if b := Build(noon, 150, g); b.FoF2From != Climatology || b.Filled != grid.Cells {
			t.Errorf("a %d by %d grid: background %v, %d filled", cr[0], cr[1], b.FoF2From, b.Filled)
		}
	}
}

// BenchmarkBuild is one hour's backgrounds over the whole grid, with GloTEC
// (G1's cost: an update builds the current hour and up to 24 typical ones).
func BenchmarkBuild(b *testing.B) {
	g := flatGloTEC()
	b.ReportAllocs()
	for b.Loop() {
		Build(noon, 150, g)
	}
}

// TestTheNorthWestPacificTakesTheClimatology is watchpost D-334: inside the
// box GloTEC is measured worse than the climatology, so foF2's background
// there is the climatology's; outside it GloTEC's stands; between, over the
// box's edge, the two are blended by distance, so the map has no seam.
func TestTheNorthWestPacificTakesTheClimatology(t *testing.T) {
	b := Build(noon, 150, flatGloTEC())
	cell := func(lat, lon float64) (got, clim, glotec float64) {
		row, col := int((89-lat)/2), int((lon+179)/2)
		fo, _ := expected(row, col)
		return float64(b.FoF2[row*grid.Cols+col]), fo, 5 + (89-2*float64(row))/30
	}
	if got, clim, _ := cell(15, 155); math.Abs(got-clim) > 1e-4 {
		t.Errorf("inside the box (15°N, 155°E): foF2 %v; the climatology gives %v", got, clim)
	}
	if got, _, gt := cell(15, -79); math.Abs(got-gt) > 1e-4 {
		t.Errorf("far from the box (15°N, 79°W): foF2 %v; GloTEC regridded gives %v", got, gt)
	}
	got, clim, gt := cell(33, 155) // 3° north of the box: a quarter climatology
	if want := 0.75*gt + 0.25*clim; math.Abs(got-want) > 1e-3 {
		t.Errorf("3° outside the box (33°N, 155°E): foF2 %v; a quarter of the climatology's %v into GloTEC's %v gives %v", got, clim, gt, want)
	}
}

// TestTheClimatologysShareIsOneInsideAndNoneBeyondTheEdge pins the weight
// the blend uses (D-334), across the antimeridian as well, and none for a
// place that is no number.
func TestTheClimatologysShareIsOneInsideAndNoneBeyondTheEdge(t *testing.T) {
	for _, c := range []struct {
		lat, lon, want float64
	}{{15, 155, 1}, {0, 130, 1}, {30, -180, 1}, {15, 128, 0.5}, {15, -178, 0.5}, {32, 155, 0.5}, {-2, 155, 0.5}, {15, 125, 0}, {15, -175, 0}, {35, 155, 0}, {15, -79, 0}, {-60, 155, 0}, {math.NaN(), 155, 0}, {15, math.NaN(), 0}} {
		if got := ClimatologyShare(c.lat, c.lon); !(math.Abs(got-c.want) <= 1e-9) { // NaN fails too
			t.Errorf("at %v°, %v°: the climatology's share %v; want %v", c.lat, c.lon, got, c.want)
		}
	}
}
