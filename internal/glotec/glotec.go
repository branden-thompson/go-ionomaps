// Package glotec reads NOAA SWPC's GloTEC grid (plan task G2.2): typed
// GeoJSON, not generic maps (R-8.2), every value checked against physical
// bounds (R-3.3), and a changed format an error, never a silently wrong
// field (R-3.1). No error quotes the input (R-3.4).
package glotec

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

// The grid's layout: 72 by 72 points at the centres of 5° by 2.5° cells.
const (
	gridCols  = 72
	gridRows  = 72
	cellLon   = 5.0
	cellLat   = 2.5
	gridCells = gridCols * gridRows
)

// The physical bounds a cell is held to (R-3.3; A-1): foF2 in MHz, hmF2 in
// km, and the quality flag, the count of F-region observations behind the
// cell, 0 to 5.
const (
	minFoF2, maxFoF2 = 0.5, 25.0
	minHmF2, maxHmF2 = 150.0, 800.0
	maxQuality       = 5
)

// maxBytes bounds a grid's size: NOAA's reply bound, about three times the
// measured 2.5 MB.
const maxBytes = terms.NOAAReplyBytes

// clockSkew is how far a grid's time may lie past the host's clock.
const clockSkew = 5 * time.Minute

// Grid is one GloTEC grid as cells: rows from the north, each west to east.
type Grid struct {
	Valid                    time.Time
	West, South, East, North float64
	Cols, Rows               int
	FoF2                     []float32 // MHz
	HmF2                     []float32 // km
	Quality                  []uint8   // F-region observations behind the cell, 0 to 5
	NoData                   Cells     // cells whose values were refused
	Rejected                 int       // how many
}

// Cells is a set of cells by index: true where a cell's values were refused.
type Cells []bool

// The errors: each says what was wrong, never what the input held.
var (
	errTooLarge = errors.New("glotec: the grid is larger than any GloTEC grid")
	errNotJSON  = errors.New("glotec: the grid is not the GloTEC GeoJSON format")
	errNoTime   = errors.New("glotec: the grid has no valid time")
	errFuture   = errors.New("glotec: the grid's time is in the future")
	errLayout   = errors.New("glotec: the grid's points are not GloTEC's 72 by 72 layout")
)

type document struct {
	TimeTag  string    `json:"time_tag"`
	Features []feature `json:"features"`
}

type feature struct {
	Geometry struct {
		Coordinates [2]float64 `json:"coordinates"` // an array: no allocation a point; a short array reads as 0, 0, no cell centre
	} `json:"geometry"`
	Properties struct {
		NmF2    float64 `json:"NmF2"`
		HmF2    float64 `json:"hmF2"`
		Quality int     `json:"quality_flag"`
	} `json:"properties"`
}

// FoF2 is the plasma frequency of an electron density: foF2 in MHz from NmF2
// in electrons per cubic metre, 8.98e-6 times its square root.
//
// Source: standard ionospheric physics (the plasma frequency); ITU-R P.1239.
func FoF2(nmF2 float64) float64 {
	finite := !math.IsInf(nmF2, 0)
	if !(nmF2 > 0) || !finite {
		return 0
	}
	return 8.98e-6 * math.Sqrt(nmF2)
}

// Decode reads one grid, asked at now. The layout, the time and the size are
// checked whole; a cell past its bounds is refused, counted and marked.
//
// Source: NOAA SWPC GloTEC's GeoJSON product, as served.
func Decode(data []byte, now time.Time) (Grid, error) {
	if len(data) > maxBytes {
		return Grid{}, errTooLarge
	}
	var doc document
	err := json.Unmarshal(data, &doc)
	if err != nil {
		return Grid{}, errNotJSON
	}
	valid, err := time.Parse(time.RFC3339, doc.TimeTag)
	timed := err == nil && !valid.IsZero()
	if !timed {
		return Grid{}, errNoTime
	}
	future := valid.After(now.Add(clockSkew))
	if future {
		return Grid{}, errFuture
	}
	if len(doc.Features) != gridCells {
		return Grid{}, errLayout
	}
	g := Grid{Valid: valid.UTC(), West: -180, South: -90, East: 180, North: 90, Cols: gridCols, Rows: gridRows,
		FoF2: make([]float32, gridCells), HmF2: make([]float32, gridCells), Quality: make([]uint8, gridCells), NoData: make(Cells, gridCells)}
	seen := make([]bool, gridCells)
	for _, f := range doc.Features { // exactly gridCells of them
		i, ok := cellOf(f.Geometry.Coordinates)
		if !ok || seen[i] {
			return Grid{}, errLayout
		}
		seen[i] = true
		if !g.place(i, f) {
			g.NoData[i] = true
			g.Rejected++
		}
	}
	return g, nil
}

// place puts one point's values in its cell; false, and nothing put, when
// one of them is past its bounds.
func (g *Grid) place(i int, f feature) bool {
	fo, hm, q := FoF2(f.Properties.NmF2), f.Properties.HmF2, f.Properties.Quality
	inBounds := fo >= minFoF2 && fo <= maxFoF2 && hm >= minHmF2 && hm <= maxHmF2 && q >= 0 && q <= maxQuality
	if !inBounds {
		return false
	}
	g.FoF2[i], g.HmF2[i], g.Quality[i] = float32(fo), float32(hm), uint8(q)
	return true
}

// cellOf is the cell a point's coordinates name, rows from the north; false
// for a point that is not one of the grid's centres.
func cellOf(c [2]float64) (int, bool) {
	col, row := (c[0]+180)/cellLon-0.5, (90-c[1])/cellLat-0.5
	ci, ri := math.Round(col), math.Round(row)
	onCentre := math.Abs(col-ci) <= 1e-9 && math.Abs(row-ri) <= 1e-9
	if !onCentre {
		return 0, false
	}
	if ci < 0 || ci >= gridCols || ri < 0 || ri >= gridRows {
		return 0, false
	}
	return int(ri)*gridCols + int(ci), true
}
