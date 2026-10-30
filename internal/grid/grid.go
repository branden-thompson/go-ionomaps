// Package grid is the library's one grid (R-1.1, watchpost D-138): 2°
// cells over the whole globe, rows from the north, each west to east, a
// cell's value for its centre. Every package that lays values on it reads
// its shape and its centres here.
package grid

import "math"

// The grid's shape.
const (
	Step  = 2
	Cols  = 360 / Step
	Rows  = 180 / Step
	Cells = Cols * Rows
)

// Centre is the latitude and longitude of cell i's centre: rows from 89°N,
// columns from 179°W; no place (NaN) for a cell off the grid.
//
// Source: this library.
func Centre(i int) (lat, lon float64) {
	if i < 0 {
		return math.NaN(), math.NaN()
	}
	if i >= Cells {
		return math.NaN(), math.NaN()
	}
	return 89 - Step*float64(i/Cols), -179 + Step*float64(i%Cols)
}

// CellOf is the cell a place falls in, longitudes wrapped and the poles in
// the outermost rows; false for a place that is no number or a latitude off
// the globe.
//
// Source: this library.
func CellOf(lat, lon float64) (int, bool) {
	nowhere := math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0)
	if nowhere {
		return 0, false
	}
	if lat < -90 || lat > 90 {
		return 0, false
	}
	lon = math.Mod(math.Mod(lon+180, 360)+360, 360)
	col := min(int(lon/Step), Cols-1)
	row := min(max(int((90-lat)/Step), 0), Rows-1)
	return row*Cols + col, true
}

// Sample is a field's value at a place, bilinear between the four cell
// centres around it, longitudes wrapped across the antimeridian and
// latitudes held to the outermost rows; false for a field not on the grid,
// or a place that is no number.
//
// Source: this library.
func Sample(values []float32, lat, lon float64) (float64, bool) {
	if len(values) != Cells {
		return 0, false
	}
	nowhere := math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0)
	if nowhere {
		return 0, false
	}
	row := math.Max(0, math.Min(float64(Rows-1), (89-lat)/Step)) // rows from the 89° centre
	col := math.Mod((lon+179)/Step+float64(Cols)*2, float64(Cols))
	r0 := min(int(row), Rows-2)
	c0 := int(col)
	fr, fc := row-float64(r0), col-float64(c0)
	c1 := (c0 + 1) % Cols
	at := func(r, c int) float64 { return float64(values[r*Cols+c]) }
	return (1-fr)*((1-fc)*at(r0, c0)+fc*at(r0, c1)) + fr*((1-fc)*at(r0+1, c0)+fc*at(r0+1, c1)), true
}
