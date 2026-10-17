// Package drap reads NOAA SWPC's D-region absorption table (plan task
// G2.3, R-2.5): the highest frequency absorbed by 1 dB, MHz, at points 2° of
// latitude by 4° of longitude, laid onto the library's 2° grid. Every value
// is checked against D-RAP's bounds and a changed layout is an error, never
// a silently wrong field (R-3.1, R-3.3). No error quotes the input (R-3.4).
package drap

import (
	"bytes"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// The table's layout, as measured in PLAN's dry run: 90 rows from 89°N to
// 89°S by 2°, the library's own, and 90 points from 178°W to 178°E by 4°.
const (
	points   = 90
	firstLon = -178
	lonStep  = 4
)

// maxMHz bounds a point (R-3.3; A-9): D-RAP's own scale ends at 35 MHz.
const maxMHz = 40.0

// maxBytes bounds a file: about six times the measured 42 KB.
const maxBytes = 256 << 10

// clockSkew is how far a table's time may lie past the host's clock.
const clockSkew = 5 * time.Minute

// validPrefix is the comment line that gives the table's time.
const validPrefix = "# Product Valid At :"

// Grid is D-RAP's table on the library's grid: MHz a cell, rows from the
// north, each west to east, and the time it is valid for.
type Grid struct {
	Valid time.Time
	MHz   []float32
}

// The errors: each says what was wrong, never what the file held.
var (
	errTooLarge = errors.New("drap: the table is larger than any D-RAP table")
	errLayout   = errors.New("drap: the table is not D-RAP's 2 by 4 degree layout")
	errNoTime   = errors.New("drap: the table has no valid time")
	errFuture   = errors.New("drap: the table's time is in the future")
	errRange    = errors.New("drap: a frequency is outside D-RAP's bounds or no number")
)

// ParseTable reads a D-RAP table: its valid time, no later than the host's
// clock allows, its longitudes D-RAP's, its 90 rows in order with 90 points
// each, every point a frequency from 0 to 40 MHz; then the points laid on
// the library's grid.
//
// Source: NOAA SWPC, text/drap_global_frequencies.txt (public domain); its
// layout measured in PLAN's dry run (watchpost dry-run.md).
func ParseTable(body []byte, now time.Time) (Grid, error) {
	if len(body) > maxBytes {
		return Grid{}, errTooLarge
	}
	var g Grid
	rows := make([][points]float32, 0, points)
	lonsSeen := false
	for line := range bytes.Lines(body) {
		text := strings.TrimSpace(string(line))
		switch {
		case strings.HasPrefix(text, validPrefix):
			t, err := time.Parse("2006-01-02 15:04 UTC", strings.TrimSpace(text[len(validPrefix):]))
			if err != nil {
				return Grid{}, errNoTime
			}
			g.Valid = t
		case text == "" || text[0] == '#' || strings.Trim(text, "-") == "":
		case strings.Contains(text, "|"):
			if len(rows) == grid.Rows {
				return Grid{}, errLayout // past the last row: the count is capped (R-3.3)
			}
			r, err := row(text, 89-2*len(rows))
			if err != nil {
				return Grid{}, err
			}
			rows = append(rows, r)
		case !lonsSeen && len(rows) == 0:
			if !lonsAre(text) {
				return Grid{}, errLayout
			}
			lonsSeen = true
		default:
			return Grid{}, errLayout
		}
	}
	return checked(g, rows, lonsSeen, now)
}

// checked is the table read, refused when its time or layout is wrong,
// else laid on the library's grid.
func checked(g Grid, rows [][points]float32, lonsSeen bool, now time.Time) (Grid, error) {
	if g.Valid.IsZero() {
		return Grid{}, errNoTime
	}
	if g.Valid.After(now.Add(clockSkew)) {
		return Grid{}, errFuture
	}
	if !lonsSeen {
		return Grid{}, errLayout
	}
	g.MHz = onGrid(rows)
	if g.MHz == nil {
		return Grid{}, errLayout // not D-RAP's 90 rows
	}
	return g, nil
}

// lonsAre is whether a header line is D-RAP's longitudes, 178°W to 178°E by 4°.
func lonsAre(text string) bool {
	f := strings.Fields(text)
	if len(f) != points {
		return false
	}
	for i, s := range f {
		if s != strconv.Itoa(firstLon+lonStep*i) {
			return false
		}
	}
	return true
}

// row is one row of points, its latitude the one expected next, each point
// a frequency within D-RAP's bounds.
func row(text string, lat int) ([points]float32, error) {
	var out [points]float32
	head, rest, _ := strings.Cut(text, "|")
	if strings.TrimSpace(head) != strconv.Itoa(lat) {
		return out, errLayout
	}
	f := strings.Fields(rest)
	if len(f) != points {
		return out, errLayout
	}
	for i, s := range f {
		v, err := strconv.ParseFloat(s, 64)
		inBounds := err == nil && v >= 0 && v <= maxMHz
		if !inBounds {
			return out, errRange
		}
		out[i] = float32(v)
	}
	return out, nil
}

// onGrid lays D-RAP's rows on the library's grid: its rows are the grid's,
// and each cell's value is linear in longitude between the two points either
// side of its centre, across the antimeridian too (A-9); nothing for rows
// that are not the grid's.
func onGrid(rows [][points]float32) []float32 {
	if len(rows) != grid.Rows {
		return nil
	}
	out := make([]float32, grid.Cells)
	for i := range out {
		lat, lon := grid.Centre(i)
		r := rows[int(89-lat)/2]
		x := math.Mod((lon-firstLon)/lonStep+points, points) // points from 178°W, wrapped
		k := int(x)
		frac := x - float64(k)
		out[i] = float32((1-frac)*float64(r[k]) + frac*float64(r[(k+1)%points]))
	}
	return out
}
