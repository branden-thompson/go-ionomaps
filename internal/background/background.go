// Package background builds the hour's background fields on the library's
// 2° grid (plan task G4.1, watchpost D-101): foF2 from GloTEC, regridded,
// else the climatology, named; M(3000)F2 from the climatology always. The
// climatology is read at each cell's quasi-dipole latitude and magnetic local
// time.
package background

import (
	"math"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/climatology"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// Source is what a background was made from.
type Source uint8

// The sources.
const (
	GloTEC Source = iota + 1
	Climatology
)

// Fields are one hour's backgrounds: foF2 and M(3000)F2 in every cell, what
// each was made from, how many cells GloTEC could not give and the
// climatology filled, and whether the coordinates were extrapolated.
type Fields struct {
	FoF2, M3000         []float32
	FoF2From, M3000From Source
	Filled              int
	CoordsExtrapolated  bool
}

// Build is the hour's backgrounds at a moment for an F10.7 mean: foF2 from
// g, regridded, when one is given, else the climatology (D-101, FR-5.2).
//
// Source: watchpost D-101; the climatology (PyIRI's method); NOAA SWPC
// GloTEC.
func Build(at time.Time, f107 float64, g *glotec.Grid) Fields {
	f := Fields{FoF2: make([]float32, grid.Cells), M3000: make([]float32, grid.Cells), FoF2From: Climatology, M3000From: Climatology}
	hour := climatology.NewHour(climatology.Refits(), at, f107)
	sub := magcoords.SubsolarQD(at)
	coords := magcoords.Cells(at)
	f.CoordsExtrapolated = coords.Extrapolated()
	usable := g != nil && len(g.FoF2) == g.Cols*g.Rows && len(g.NoData) == len(g.FoF2)
	fromGloTEC := 0
	for i := range f.FoF2 {
		qdlat, qdlon := coords.At(i)
		fo, m3 := hour.At(qdlat, magcoords.MLTFrom(qdlon, sub))
		f.M3000[i] = float32(m3)
		f.FoF2[i] = float32(fo)
		if !usable {
			continue
		}
		lat, lon := grid.Centre(i)
		if v, ok := regrid(g, lat, lon); ok {
			f.FoF2[i] = v
			fromGloTEC++
			continue
		}
		f.Filled++
	}
	if fromGloTEC > 0 {
		f.FoF2From = GloTEC
	}
	return f
}

// regrid is GloTEC's foF2 at a place: bilinear between the four cell centres
// around it, longitudes wrapped and latitudes held to the outermost rows,
// leaving out cells it refused and weighing the rest again; false when all
// four were refused.
func regrid(g *glotec.Grid, lat, lon float64) (float32, bool) {
	if g.Cols < 2 || g.Rows < 2 {
		return 0, false // no cells around anything to interpolate between
	}
	cw, rh := (g.East-g.West)/float64(g.Cols), (g.North-g.South)/float64(g.Rows)
	col := math.Mod((lon-g.West)/cw-0.5+float64(g.Cols)*2, float64(g.Cols))
	row := math.Max(0, math.Min(float64(g.Rows-1), (g.North-lat)/rh-0.5))
	c0, r0 := int(col), int(math.Min(row, float64(g.Rows-2)))
	fc, fr := col-float64(c0), row-float64(r0)
	c1 := (c0 + 1) % g.Cols
	sum, weight := 0.0, 0.0
	for _, k := range [4]struct {
		r, c int
		w    float64
	}{{r0, c0, (1 - fr) * (1 - fc)}, {r0, c1, (1 - fr) * fc}, {r0 + 1, c0, fr * (1 - fc)}, {r0 + 1, c1, fr * fc}} {
		i := k.r*g.Cols + k.c
		if g.NoData[i] || k.w == 0 {
			continue
		}
		sum += k.w * float64(g.FoF2[i])
		weight += k.w
	}
	if weight <= 0 {
		return 0, false
	}
	return float32(sum / weight), true
}
