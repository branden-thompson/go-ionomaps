// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
// Package spike is a throwaway performance spike: it measures the per-keypress
// cost of the reference chart's answers over synthetic fields. Not production
// code, not accurate science.
package spike

import "math"

const deg = math.Pi / 180

// Grid is a regular lat/lon grid. Rows run -90..+90 inclusive, columns
// -180..180-step.
type Grid struct {
	Cols, Rows int
	Step       float64
	latD, lonD []float64

	// Per-row and per-column sin/cos tables, filled by Precompute and used by
	// the optimized path. A cell's unit vector is
	// (cosLat[r]*cosLon[c], cosLat[r]*sinLon[c], sinLat[r]).
	sinLat, cosLat, sinLon, cosLon []float64
}

func NewGrid(step float64) *Grid {
	cols := int(math.Round(360 / step))
	rows := int(math.Round(180/step)) + 1
	g := &Grid{Cols: cols, Rows: rows, Step: step}
	g.latD = make([]float64, rows)
	g.lonD = make([]float64, cols)
	for r := range rows {
		g.latD[r] = -90 + float64(r)*step
	}
	for c := range cols {
		g.lonD[c] = -180 + float64(c)*step
	}
	return g
}

// Precompute fills the sin/cos tables. Needed once per grid (at worst once per
// snapshot); its cost is measured separately.
func (g *Grid) Precompute() {
	g.sinLat = make([]float64, g.Rows)
	g.cosLat = make([]float64, g.Rows)
	g.sinLon = make([]float64, g.Cols)
	g.cosLon = make([]float64, g.Cols)
	for r, v := range g.latD {
		g.sinLat[r], g.cosLat[r] = math.Sincos(v * deg)
	}
	for c, v := range g.lonD {
		g.sinLon[c], g.cosLon[c] = math.Sincos(v * deg)
	}
}

// Snapshot is one hour's pair of global fields plus the sun position.
type Snapshot struct {
	G           *Grid
	Hour        int
	FoF2, M3000 []float64
	sinDecl     float64
	cosDecl     float64
	ssLon       float64    // sub-solar longitude, radians
	sun         [3]float64 // sub-solar unit vector
}

// Synth builds a smooth synthetic snapshot: foF2 in [2,14] MHz following the
// sub-solar point, M(3000)F2 in [2.5,3.8].
func Synth(g *Grid, hour int) *Snapshot {
	s := &Snapshot{G: g, Hour: hour}
	decl := -5 * deg
	s.sinDecl, s.cosDecl = math.Sincos(decl)
	s.ssLon = math.Remainder((12-float64(hour))*15*deg, 2*math.Pi)
	s.sun = [3]float64{s.cosDecl * math.Cos(s.ssLon), s.cosDecl * math.Sin(s.ssLon), s.sinDecl}
	n := g.Cols * g.Rows
	s.FoF2 = make([]float64, n)
	s.M3000 = make([]float64, n)
	for r := range g.Rows {
		lat := g.latD[r] * deg
		for c := range g.Cols {
			lon := g.lonD[c] * deg
			cz := s.sinDecl*math.Sin(lat) + s.cosDecl*math.Cos(lat)*math.Cos(lon-s.ssLon)
			fo := 7 + 5*cz + 1.5*math.Cos(2*lat)*math.Sin(lon+0.2*float64(hour))
			m := 3.15 + 0.45*math.Sin(lat)*math.Cos(lon+0.3*float64(hour)) + 0.15*cz
			s.FoF2[r*g.Cols+c] = clamp(fo, 2, 14)
			s.M3000[r*g.Cols+c] = clamp(m, 2.5, 3.8)
		}
	}
	return s
}

// Interp is bilinear interpolation of both fields at (latD, lonD) degrees.
func (s *Snapshot) Interp(latD, lonD float64) (fo, m float64) {
	g := s.G
	fc := float64(g.Cols)
	fx := (lonD + 180) / g.Step
	if fx < 0 {
		fx += fc
	} else if fx >= fc {
		fx -= fc
	}
	fy := (latD + 90) / g.Step
	if fy < 0 {
		fy = 0
	} else if fy > float64(g.Rows-1) {
		fy = float64(g.Rows - 1)
	}
	x0 := int(fx)
	if x0 >= g.Cols {
		x0 = g.Cols - 1
	}
	y0 := int(fy)
	if y0 >= g.Rows-1 {
		y0 = g.Rows - 2
	}
	tx, ty := fx-float64(x0), fy-float64(y0)
	x1 := x0 + 1
	if x1 == g.Cols {
		x1 = 0
	}
	i00, i01 := y0*g.Cols+x0, y0*g.Cols+x1
	i10, i11 := i00+g.Cols, i01+g.Cols
	w00, w01 := (1-tx)*(1-ty), tx*(1-ty)
	w10, w11 := (1-tx)*ty, tx*ty
	fo = w00*s.FoF2[i00] + w01*s.FoF2[i01] + w10*s.FoF2[i10] + w11*s.FoF2[i11]
	m = w00*s.M3000[i00] + w01*s.M3000[i01] + w10*s.M3000[i10] + w11*s.M3000[i11]
	return fo, m
}

// zenithNaive: cos(solar zenith) from lat/lon degrees, three trig calls.
func (s *Snapshot) zenithNaive(latD, lonD float64) float64 {
	lat, lon := latD*deg, lonD*deg
	return s.sinDecl*math.Sin(lat) + s.cosDecl*math.Cos(lat)*math.Cos(lon-s.ssLon)
}

// zenithVec: cos(solar zenith) from a unit vector, no trig.
func (s *Snapshot) zenithVec(p [3]float64) float64 {
	return p[0]*s.sun[0] + p[1]*s.sun[1] + p[2]*s.sun[2]
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func unitVec(latD, lonD float64) [3]float64 {
	sl, cl := math.Sincos(latD * deg)
	so, co := math.Sincos(lonD * deg)
	return [3]float64{cl * co, cl * so, sl}
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
