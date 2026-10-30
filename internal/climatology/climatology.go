// Package climatology is the monthly-mean foF2 and M(3000)F2, a Go port of
// PyIRI's spherical-harmonic method (R-9.1): a Fourier series in universal
// time over real spherical harmonics in quasi-dipole latitude and magnetic
// local time, for each month, at two solar levels. Its coefficients are read
// through one seam, Tables, so they can be replaced (R-4.4, watchpost D-43);
// the shipped ones are NRL's refits (Refits).
//
// PyIRI is MIT licensed, Copyright (c) 2023 victoriyaforsythe; the method is
// Forsythe et al. (2024), doi:10.1029/2023SW003739.
package climatology

import (
	_ "embed"
	"encoding/binary"
	"math"
	"sync"
)

// The tables' shape: 11 real Fourier terms in universal time and 900 real
// spherical harmonics, degree and order up to 29.
const (
	Terms     = 11
	Harmonics = 900
	lmax      = 29
	months    = 12
	levels    = 2
)

// Level is a solar level the tables hold: index 0 (Min) and 100 (Max), in
// IG12 for foF2 and R12 for M(3000)F2.
type Level uint8

// The solar levels.
const (
	Min Level = iota
	Max
)

// Coefficients are one month's at one solar level: Terms rows of Harmonics,
// Fourier term first.
type Coefficients []float64

// Tables are the seam the climatology reads its coefficients through
// (R-4.4): for a month, 1 to 12, and a solar level.
type Tables interface {
	FoF2(month int, solar Level) Coefficients
	M3000(month int, solar Level) Coefficients
}

// Monthly is a parameter's monthly mean at an hour of universal time, a
// quasi-dipole latitude in degrees and a magnetic local time in hours, from
// one month's coefficients at one solar level; 0 for coefficients of another
// shape, or a place or time that is no finite number.
//
// Source: PyIRI 0.1.7 (MIT), sh_library.IRI_sh_params, real_FS_func and
// real_SH_func; Forsythe et al. 2024.
func Monthly(c Coefficients, ut, qdlat, mlt float64) float64 {
	if len(c) != Terms*Harmonics {
		return 0
	}
	finite := !math.IsNaN(ut) && !math.IsInf(ut, 0) && !math.IsNaN(mlt) && !math.IsInf(mlt, 0)
	if !finite {
		return 0 // no NaN or Inf leaves the library (R-3.3)
	}
	if !(qdlat >= -90 && qdlat <= 90) {
		return 0 // off the sphere, or no number
	}
	fs := fourier(ut)
	var sh [Harmonics]float64
	harmonics(&sh, (90-qdlat)*math.Pi/180, mlt*15*math.Pi/180)
	total := 0.0
	for j, f := range fs {
		row := c[j*Harmonics : (j+1)*Harmonics]
		s := 0.0
		for k, y := range sh {
			s += row[k] * y
		}
		total += f * s
	}
	return total
}

// fourier is the real Fourier basis at an hour: 1, then the cosine and sine
// of each of five harmonics of the day.
func fourier(ut float64) [Terms]float64 {
	var f [Terms]float64
	f[0] = 1
	for k := 1; k <= (Terms-1)/2; k++ {
		phase := 2 * math.Pi * float64(k) * ut / 24
		f[2*k-1], f[2*k] = math.Cos(phase), math.Sin(phase)
	}
	return f
}

// harmonics writes the real spherical-harmonic basis at a colatitude and
// longitude in radians into out, 4π-normalised with the Condon-Shortley
// phase, the mode of degree l and order m at l(l+1)+m and its sine twin at
// l(l+1)-m. Every element is written. The arrays are filled in place, never
// returned, so a place costs no 14 KB copy.
func harmonics(out *[Harmonics]float64, theta, phi float64) {
	z := math.Cos(theta)
	var p [lmax + 1][lmax + 1]float64
	legendre(&p, z)
	// cos(mφ) and sin(mφ) for every order, stepped by the angle-sum rule.
	var cm, sm [lmax + 1]float64
	c1, s1 := math.Cos(phi), math.Sin(phi)
	cm[0] = 1
	for m := 1; m <= lmax; m++ {
		cm[m], sm[m] = cm[m-1]*c1-sm[m-1]*s1, sm[m-1]*c1+cm[m-1]*s1
	}
	for l := 0; l <= lmax; l++ {
		base := l * (l + 1)
		out[base] = p[l][0] * norms[l][0]
		for m := 1; m <= l; m++ {
			pn := p[l][m] * norms[l][m]
			out[base+m] = pn * cm[m]
			out[base-m] = pn * sm[m]
		}
	}
}

// norms is each degree and order's 4π normalisation: √(2l+1) for order 0,
// √(2(2l+1)(l-m)!/(l+m)!) for the rest. Fixed, so made once.
var norms = func() (n [lmax + 1][lmax + 1]float64) {
	for l := 0; l <= lmax; l++ {
		n[l][0] = math.Sqrt(float64(2*l + 1))
		for m := 1; m <= l; m++ {
			n[l][m] = math.Sqrt(2 * float64(2*l+1) * ratio(l, m))
		}
	}
	return n
}()

// ratio is (l-m)!/(l+m)!, as a product, which never overflows.
func ratio(l, m int) float64 {
	r := 1.0
	for i := l - m + 1; i <= l+m; i++ {
		r /= float64(i)
	}
	return r
}

// legendre writes the associated Legendre functions P_l^m(z) into p, with
// the Condon-Shortley phase, for every degree and order up to lmax (m <= l),
// by the standard recurrences.
func legendre(p *[lmax + 1][lmax + 1]float64, z float64) {
	s := math.Sqrt(math.Max(0, 1-z*z))
	p[0][0] = 1
	for m := 1; m <= lmax; m++ {
		p[m][m] = -float64(2*m-1) * s * p[m-1][m-1]
	}
	for m := 0; m < lmax; m++ {
		p[m+1][m] = z * float64(2*m+1) * p[m][m]
	}
	for m := 0; m <= lmax; m++ {
		for l := m + 2; l <= lmax; l++ {
			p[l][m] = (float64(2*l-1)*z*p[l-1][m] - float64(l+m-1)*p[l-2][m]) / float64(l-m)
		}
	}
}

// The refits, converted by tools/tables from the one-time export (watchpost
// D-114): NRL's spherical-harmonic refits of the CCIR foF2 map and of
// M(3000)F2, from PyIRI 0.1.7.
var (
	//go:embed tables/fof2_ccir.bin
	fof2Table []byte
	//go:embed tables/m3000f2.bin
	m3000Table []byte
)

// The compact form opens with its magic, the format's name and version, and
// its shape as four little-endian uint32s: levels, months, terms, harmonics.
const (
	tableMagic  = "IONOTAB1"
	tableHeader = len(tableMagic) + 16
)

// refits reads the embedded tables.
type refits struct{}

// Refits are the shipped tables: NRL's refits of the CCIR foF2 map (watchpost
// A-28) and of M(3000)F2.
//
// Source: PyIRI 0.1.7 (MIT), coefficients/SH/foF2_CCIR.nc and M3000F2.nc,
// NRL's refits (Forsythe et al. 2024), exported once (watchpost D-114).
func Refits() Tables { return refits{} }

// FoF2 is foF2's coefficients for a month and solar level, decoded once and
// shared: read them, never write them.
//
// Source: as Refits.
func (refits) FoF2(month int, solar Level) Coefficients {
	return decoded(&fof2Decoded, fof2Table, month, solar)
}

// M3000 is M(3000)F2's coefficients for a month and solar level, decoded
// once and shared: read them, never write them.
//
// Source: as Refits.
func (refits) M3000(month int, solar Level) Coefficients {
	return decoded(&m3000Decoded, m3000Table, month, solar)
}

// onceTable is a table's coefficients, decoded the first time each month
// and level is asked: an hour folds eight of them, and the same eight again
// the next hour.
type onceTable [levels][months]struct {
	once sync.Once
	c    Coefficients
}

var fof2Decoded, m3000Decoded onceTable

// decoded is a month and level's coefficients from a table, decoded once;
// none for a month or level the table does not hold.
func decoded(cache *onceTable, table []byte, month int, solar Level) Coefficients {
	if month < 1 || month > months || solar > Max {
		return nil
	}
	e := &cache[solar][month-1]
	e.once.Do(func() { e.c = slice(table, month, solar) })
	return e.c
}

// shaped is whether a compact table is the one this package reads: its
// magic, its shape and its length.
func shaped(table []byte) bool {
	if len(table) != tableHeader+8*levels*months*Terms*Harmonics || string(table[:len(tableMagic)]) != tableMagic {
		return false
	}
	for i, want := range [4]int{levels, months, Terms, Harmonics} {
		if int(binary.LittleEndian.Uint32(table[len(tableMagic)+4*i:])) != want {
			return false
		}
	}
	return true
}

// slice decodes one month and level from a compact table; none for a month
// or level the table does not hold, or a table of another shape.
func slice(table []byte, month int, solar Level) Coefficients {
	if month < 1 || month > months || solar > Max {
		return nil
	}
	if !shaped(table) {
		return nil
	}
	n := Terms * Harmonics
	start := tableHeader + 8*((int(solar)*months+month-1)*n)
	out := make(Coefficients, n)
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(table[start+8*i:]))
	}
	return out
}
