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
