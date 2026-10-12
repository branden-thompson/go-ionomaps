package magcoords

import (
	_ "embed"
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// gyroMagic opens the gyrofrequency table, as tools/tables writes it.
const gyroMagic = "IONOFH01"

//go:embed gyro.bin
var gyroTable []byte

// gyroYears is the gyrofrequency table read: the first year, and each
// year's cells.
type gyroYears struct {
	first int
	cells [][]float32
}

var (
	gyroLoaded gyroYears
	gyroOnce   sync.Once
)

// readGyro is a gyrofrequency table read: the magic, the number of years,
// the grid's columns and rows, the years, consecutive, then each year's
// cells; empty where it is not whole or not the library's grid.
func readGyro(b []byte) gyroYears {
	head := len(gyroMagic) + 12
	if len(b) < head || string(b[:len(gyroMagic)]) != gyroMagic {
		return gyroYears{}
	}
	u32 := func(at int) int { return int(binary.LittleEndian.Uint32(b[at:])) }
	n := u32(len(gyroMagic))
	whole := n > 0 && n <= 100 && u32(len(gyroMagic)+4) == cols && u32(len(gyroMagic)+8) == rows && len(b) == head+4*n+4*n*cells
	if !whole {
		return gyroYears{}
	}
	y := gyroYears{first: u32(head)}
	for k := range n {
		if u32(head+4*k) != y.first+k {
			return gyroYears{}
		}
	}
	at := head + 4*n
	for range n {
		year := make([]float32, cells)
		for i := range year {
			year[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at+4*i:]))
		}
		at += 4 * cells
		y.cells = append(y.cells, year)
	}
	return y
}

// Gyro is the electron gyrofrequency at 300 km, MHz, at a geographic place
// and moment: between the cell centres around it and the years either side,
// the nearest year's outside them; no number for a place off the globe or
// with no table.
//
// Source: IGRF-14 (IAGA), the field's strength at 300 km, generated at
// build time by tools/tables; ITU-R P.533-14 §3.5 takes it at 300 km.
func Gyro(lat, lon float64, at time.Time) float64 {
	gyroOnce.Do(func() { gyroLoaded = readGyro(gyroTable) })
	g := gyroLoaded
	if len(g.cells) == 0 || lat != lat || lon != lon {
		return math.NaN()
	}
	if lat < -90 || lat > 90 {
		return math.NaN()
	}
	i, w, _ := span(years{first: g.first, lat: g.cells}, at)
	a := cellValue(g.cells[i], lat, lon)
	if w == 0 || i+1 >= len(g.cells) {
		return a
	}
	return a + w*(cellValue(g.cells[i+1], lat, lon)-a)
}

// cellValue is the bilinear interpolation of one year's cells at a place,
// longitudes wrapped, latitudes held to the outermost rows.
func cellValue(v []float32, glat, glon float64) float64 {
	col := math.Mod(glon+179+720, 360) / 2
	row := math.Max(0, math.Min(rows-1, (89-glat)/2))
	c0, r0 := int(col), int(math.Min(row, rows-2))
	fc, fr := col-float64(c0), row-float64(r0)
	c1 := (c0 + 1) % cols
	c0 %= cols
	at := func(r, c int) float64 { return float64(v[r*cols+c]) }
	return (1-fr)*((1-fc)*at(r0, c0)+fc*at(r0, c1)) + fr*((1-fc)*at(r0+1, c0)+fc*at(r0+1, c1))
}
