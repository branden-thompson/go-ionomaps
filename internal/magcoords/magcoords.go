// Package magcoords is the library's own quasi-dipole coordinates and
// magnetic local time (watchpost D-107): a table generated at build time by
// tools/tables, tracing IGRF-14's field lines to their apexes (Richmond 1995),
// for each 2° cell centre on 1 January of each year it holds. A place and
// moment between are interpolated; outside the years, the nearest year's
// are used and said to be extrapolated (R-9.7).
package magcoords

import (
	_ "embed"
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// The table's grid is the library's own (R-1.1).
const (
	cols  = grid.Cols
	rows  = grid.Rows
	cells = grid.Cells
	magic = "IONOQD01"
)

//go:embed coords.bin
var table []byte

// years is the table read: each year's quasi-dipole latitudes and
// longitudes, by cell.
type years struct {
	first int
	lat   [][]float32
	lon   [][]float32
}

var (
	loaded   years
	loadOnce sync.Once
)

// load reads the embedded table once.
func load() years {
	loadOnce.Do(func() { loaded = read(table) })
	return loaded
}

// read is a table read: the magic, the number of years, the grid's columns
// and rows, the years, consecutive, then each year's latitudes and
// longitudes by cell; an empty table where it is not whole or not the
// library's grid.
func read(b []byte) years {
	head := len(magic) + 12
	if len(b) < head || string(b[:len(magic)]) != magic {
		return years{}
	}
	u32 := func(at int) int { return int(binary.LittleEndian.Uint32(b[at:])) }
	n := u32(len(magic))
	whole := n > 0 && n <= 100 && u32(len(magic)+4) == cols && u32(len(magic)+8) == rows && len(b) == head+4*n+8*n*cells
	if !whole {
		return years{}
	}
	y := years{first: u32(head)}
	for k := range n {
		if u32(head+4*k) != y.first+k {
			return years{}
		}
	}
	at := head + 4*n
	for range n {
		lat, lon := make([]float32, cells), make([]float32, cells)
		for i := range cells {
			lat[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at+4*i:]))
			lon[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[at+4*(cells+i):]))
		}
		at += 8 * cells
		y.lat, y.lon = append(y.lat, lat), append(y.lon, lon)
	}
	return y
}

// QD is a geographic place's quasi-dipole latitude and longitude in degrees
// at a moment, height 0: interpolated between the cell centres around it and
// between the years either side; extrapolated when the moment is outside the
// table's years, the nearest year's then used.
//
// Source: Richmond (1995), J. Geomag. Geoelectr. 47, 191-212; IGRF-14 (IAGA),
// traced at build time by tools/tables.
func QD(lat, lon float64, at time.Time) (qdlat, qdlon float64, extrapolated bool) {
	y := load()
	if len(y.lat) == 0 {
		return 0, 0, true // no table: nothing known
	}
	place := !math.IsNaN(lat) && !math.IsNaN(lon) && !math.IsInf(lat, 0) && !math.IsInf(lon, 0)
	if !place {
		return 0, 0, true
	}
	i, w, extrapolated := span(y, at)
	la0, lo0 := cellValues(y.lat[i], y.lon[i], lat, lon)
	if w == 0 || i+1 >= len(y.lat) {
		return la0, lo0, extrapolated
	}
	la1, lo1 := cellValues(y.lat[i+1], y.lon[i+1], lat, lon)
	return la0 + w*(la1-la0), blendLongitude(lo0, lo1, w), extrapolated
}

// span is where a moment falls in the table's years: the year at or before
// it, the part of the way to the next, and whether it is outside them, held
// to the first or last year.
func span(y years, at time.Time) (i int, w float64, extrapolated bool) {
	t := yearFraction(at) - float64(y.first)
	last := float64(len(y.lat) - 1)
	extrapolated = t < 0 || t > last
	t = math.Max(0, math.Min(last, t))
	i = int(math.Min(t, last-1))
	if last == 0 {
		i = 0
	}
	return i, t - float64(i), extrapolated
}

// node is one year's coordinates at a cell's centre.
type node struct{ lat, lon float64 }

// centres are each year's coordinates at every cell's centre, as QD reads
// them, each year worked out the first time a moment needs it.
var (
	centres []struct {
		once  sync.Once
		nodes []node
	}
	centresOnce sync.Once
)

// yearCentres is year k's coordinates at every cell's centre.
func yearCentres(y years, k int) []node {
	centresOnce.Do(func() {
		centres = make([]struct {
			once  sync.Once
			nodes []node
		}, len(y.lat))
	})
	c := &centres[k]
	c.once.Do(func() {
		c.nodes = make([]node, cells)
		for i := range c.nodes {
			lat, lon := grid.Centre(i)
			c.nodes[i].lat, c.nodes[i].lon = cellValues(y.lat[k], y.lon[k], lat, lon)
		}
	})
	return c.nodes
}

// Moment is the table read for one moment over the library's grid: every
// cell's coordinates at its centre, as QD gives them, the table read once
// for them all.
type Moment struct {
	from, to     []node
	w            float64
	extrapolated bool
}

// Cells is the table read for a moment over the library's grid.
//
// Source: as QD.
func Cells(at time.Time) Moment {
	y := load()
	if len(y.lat) == 0 {
		return Moment{extrapolated: true} // no table: nothing known
	}
	i, w, extrapolated := span(y, at)
	m := Moment{from: yearCentres(y, i), w: w, extrapolated: extrapolated}
	if w != 0 && i+1 < len(y.lat) {
		m.to = yearCentres(y, i+1)
	}
	return m
}

// At is cell i's quasi-dipole latitude and longitude in degrees, as QD
// gives them at the cell's centre.
//
// Source: as QD.
func (m Moment) At(i int) (qdlat, qdlon float64) {
	if m.from == nil {
		return 0, 0
	}
	a := m.from[i]
	if m.to == nil {
		return a.lat, a.lon
	}
	b := m.to[i]
	return a.lat + m.w*(b.lat-a.lat), blendLongitude(a.lon, b.lon, m.w)
}

// Extrapolated is whether the moment is outside the table's years.
//
// Source: as QD.
func (m Moment) Extrapolated() bool { return m.extrapolated }

// yearFraction is a moment as a year and the part of it past.
func yearFraction(at time.Time) float64 {
	at = at.UTC()
	start := time.Date(at.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	return float64(at.Year()) + at.Sub(start).Seconds()/end.Sub(start).Seconds()
}

// cellValues is the bilinear interpolation of one year's table at a place:
// between the four cell centres around it, longitudes wrapped, latitudes held
// to the outermost rows.
func cellValues(lat, lon []float32, glat, glon float64) (float64, float64) {
	col := math.Mod(glon+179+720, 360) / 2 // 0 at 179°W; one a 2° column
	row := math.Max(0, math.Min(rows-1, (89-glat)/2))
	c0, r0 := int(col), int(math.Min(row, rows-2))
	fc, fr := col-float64(c0), row-float64(r0)
	c1 := (c0 + 1) % cols
	c0 %= cols
	idx := func(r, c int) int { return r*cols + c }
	la := (1-fr)*((1-fc)*float64(lat[idx(r0, c0)])+fc*float64(lat[idx(r0, c1)])) +
		fr*((1-fc)*float64(lat[idx(r0+1, c0)])+fc*float64(lat[idx(r0+1, c1)]))
	// Longitudes are averaged as directions, so 359° and 1° give 0°, not 180°.
	var x, y float64
	for _, k := range [4]struct {
		i int
		w float64
	}{{idx(r0, c0), (1 - fr) * (1 - fc)}, {idx(r0, c1), (1 - fr) * fc}, {idx(r0+1, c0), fr * (1 - fc)}, {idx(r0+1, c1), fr * fc}} {
		a := float64(lon[k.i]) * math.Pi / 180
		x, y = x+k.w*math.Cos(a), y+k.w*math.Sin(a)
	}
	return la, wrap360(math.Atan2(y, x) * 180 / math.Pi)
}

// blendLongitude is the longitude a part w of the way from a to b, the short
// way round.
func blendLongitude(a, b, w float64) float64 {
	d := math.Mod(b-a+540, 360) - 180
	return wrap360(a + w*d)
}

// wrap360 is an angle in [0, 360).
func wrap360(a float64) float64 { return math.Mod(math.Mod(a, 360)+360, 360) }

// MLT is the magnetic local time, hours, of a quasi-dipole longitude at a
// moment: its distance east of the subsolar point's quasi-dipole longitude,
// turned to midnight, as PyIRI's Apex does (after Laundal and Richmond 2017).
//
// Source: PyIRI 0.1.7 (MIT), sh_library.Apex ('GEO_2_MLT'); Laundal and
// Richmond (2017), Space Sci. Rev. 206, 27-59.
func MLT(qdlon float64, at time.Time) float64 { return MLTFrom(qdlon, SubsolarQD(at)) }

// SubsolarQD is the subsolar point's quasi-dipole longitude at a moment: an
// hour's MLT for every place needs it once.
//
// Source: as MLT.
func SubsolarQD(at time.Time) float64 {
	slon, slat := Subsolar(at)
	_, sqdlon, _ := QD(slat, slon, at)
	return sqdlon
}

// MLTFrom is the magnetic local time of a quasi-dipole longitude, given the
// subsolar point's.
//
// Source: as MLT.
func MLTFrom(qdlon, subsolarQDLon float64) float64 {
	return math.Mod(math.Mod((qdlon-subsolarQDLon+180)/15, 24)+24, 24)
}

// Subsolar is the geographic longitude and latitude, degrees, of the point
// the sun stands over at a moment.
//
// Source: PyIRI 0.1.7 (MIT), main_library.subsolar_point and juldat.
func Subsolar(at time.Time) (lon, lat float64) {
	t := (julian(at.UTC()) - 2451545) / 36525
	lms := math.Mod(280.460+36000.771*t, 360)
	anom := math.Mod(357.527723+35999.05034*t, 360) * math.Pi / 180
	ecl := (lms + 1.914666471*math.Sin(anom) + 0.01999464*math.Sin(2*anom)) * math.Pi / 180
	eps := (23.439291 - 0.0130042*t) * math.Pi / 180
	dec := math.Asin(math.Sin(eps) * math.Sin(ecl))
	gmst := math.Mod(67310.54841+(876600*3600+8640184.812866)*t+0.93104*t*t-6.2e-6*t*t*t, 86400) / 240
	ra := math.Atan2(math.Cos(eps)*math.Sin(ecl)/math.Cos(dec), math.Cos(ecl)/math.Cos(dec)) * 180 / math.Pi
	lon = math.Mod(-(gmst-ra)+180+720, 360) - 180
	return lon, dec * 180 / math.Pi
}

// julian is a moment's Julian date, as PyIRI's juldat computes it.
func julian(at time.Time) float64 {
	y, m, d := float64(at.Year()), float64(at.Month()), float64(at.Day())
	hours := float64(at.Hour()) + float64(at.Minute())/60 + float64(at.Second())/3600
	sign := 1.0
	if 100*y+m-190002.5 < 0 {
		sign = -1
	}
	return 367*y - math.Trunc(7*(y+math.Trunc((m+9)/12))/4) + math.Trunc(275*m/9) + d + 1721013.5 + hours/24 - 0.5*sign + 0.5
}
