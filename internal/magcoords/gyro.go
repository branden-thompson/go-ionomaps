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

//go:embed field100.bin
var field100Table []byte

// field100Magic opens the 100 km field table, as tools/tables writes it.
const field100Magic = "IONOF100"

// yearTable is a build-time table of whole years read: the first year, and
// each part's cells a year (parts[part][year][cell]).
type yearTable struct {
	first int
	parts [][][]float32
}

var (
	gyroLoaded, field100Loaded yearTable
	gyroOnce, field100Once     sync.Once
)

// readYearTable is a table of whole years read: the magic, the number of
// years, the grid's columns and rows, the years, consecutive, then each
// year's parts in turn, each a value a cell; empty where it is not whole,
// not this kind or not the library's grid.
func readYearTable(b []byte, magic string, parts int) yearTable {
	head := len(magic) + 12
	if len(b) < head || string(b[:len(magic)]) != magic {
		return yearTable{}
	}
	u32 := func(at int) int { return int(binary.LittleEndian.Uint32(b[at:])) }
	n := u32(len(magic))
	whole := n > 0 && n <= 100 && u32(len(magic)+4) == cols && u32(len(magic)+8) == rows && len(b) == head+4*n+4*n*parts*cells
	if !whole {
		return yearTable{}
	}
	y := yearTable{first: u32(head), parts: make([][][]float32, parts)}
	for k := range n {
		if u32(head+4*k) != y.first+k {
			return yearTable{}
		}
	}
	at := head + 4*n
	for range n {
		for p := range parts {
			year := make([]float32, cells)
			for i := range year {
				year[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at+4*i:]))
			}
			at += 4 * cells
			y.parts[p] = append(y.parts[p], year)
		}
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
	gyroOnce.Do(func() { gyroLoaded = readYearTable(gyroTable, gyroMagic, 1) })
	return gyroLoaded.value(0, lat, lon, at)
}

// Field100 is the electron gyrofrequency, MHz, and the field's dip,
// degrees, positive downward, at 100 km, at a geographic place and moment,
// as Gyro interpolates; no number for a place off the globe or with no
// table.
//
// Source: IGRF-14 (IAGA), the field at 100 km against the local geodetic
// vertical, generated at build time by tools/tables; ITU-R P.533-14 §5.2.2
// reads both at 100 km (fL = fH sin I; p by the modified dip).
func Field100(lat, lon float64, at time.Time) (fH, dip float64) {
	field100Once.Do(func() { field100Loaded = readYearTable(field100Table, field100Magic, 2) })
	return field100Loaded.value(0, lat, lon, at), field100Loaded.value(1, lat, lon, at)
}

// value is one part of a table at a place and moment: between the cell
// centres around it and the years either side, the nearest year's outside
// them; no number for a place off the globe or a table not read.
func (g yearTable) value(part int, lat, lon float64, at time.Time) float64 {
	read := part < len(g.parts) && len(g.parts[part]) > 0
	if !read || lat != lat || lon != lon {
		return math.NaN()
	}
	if lat < -90 || lat > 90 {
		return math.NaN()
	}
	cells := g.parts[part]
	i, w, _ := span(years{first: g.first, lat: cells}, at)
	a := cellValue(cells[i], lat, lon)
	if w == 0 || i+1 >= len(cells) {
		return a
	}
	return a + w*(cellValue(cells[i+1], lat, lon)-a)
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
