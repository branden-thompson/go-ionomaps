// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import (
	"errors"
	"math"
)

// Gaussian-process assimilation spike (G-G1). Kernel var*exp(-d/L) in
// great-circle distance, noise*var on the diagonal, one 39x39 Cholesky solve
// per field, then prediction at every grid cell added to the background.

const (
	gpL     = 4000.0 // km
	gpNoise = 1.0
	varFo   = 1.0  // MHz^2 (placeholder)
	varM    = 0.04 // (placeholder)
)

type Station struct {
	LatD, LonD  float64
	ResFo, ResM float64 // observed minus background
}

// Stations: 39 positions, dense in Europe and North America, sparse south and
// Pacific. Residuals are deterministic synthetic values.
var Stations = func() []Station {
	pos := [][2]float64{
		// Europe (14)
		{51.7, -1.3}, {50.1, 4.6}, {52.0, 14.0}, {54.6, 13.4}, {49.6, 3.0}, {41.8, 12.5}, {40.4, -3.7},
		{38.0, 23.5}, {50.0, 14.6}, {60.0, 25.0}, {59.4, 17.9}, {69.6, 19.2}, {55.7, 37.6}, {44.4, 26.1},
		// North America (10)
		{40.0, -105.3}, {42.6, -71.5}, {37.9, -75.5}, {30.4, -86.7}, {34.6, -120.6}, {64.9, -147.8},
		{52.4, -106.6}, {45.4, -75.7}, {32.4, -106.3}, {19.5, -155.6},
		// Asia (5)
		{35.7, 139.5}, {31.2, 130.6}, {40.0, 116.3}, {25.0, 121.2}, {13.0, 77.6},
		// South America, Africa, Oceania, Antarctica, Pacific (10)
		{-12.0, -76.9}, {-23.2, -45.9}, {-34.6, -58.4}, {-25.7, 28.1}, {-34.4, 18.5}, {-35.3, 149.0},
		{-42.9, 147.3}, {-77.8, 166.7}, {-17.7, -149.4}, {13.6, 144.9},
	}
	st := make([]Station, len(pos))
	for i, p := range pos {
		st[i] = Station{LatD: p[0], LonD: p[1],
			ResFo: 1.5 * math.Sin(float64(i)*1.7), ResM: 0.2 * math.Cos(float64(i)*2.3)}
	}
	return st
}()

func haversineKm(lat1D, lon1D, lat2D, lon2D float64) float64 {
	lat1, lat2 := lat1D*deg, lat2D*deg
	dlat, dlon := lat2-lat1, (lon2D-lon1D)*deg
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * earthR * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// cholSolve factors the n x n matrix k (row-major, overwritten with L) and
// solves k*x = y in place in y.
func cholSolve(k []float64, y []float64, n int) error {
	for j := 0; j < n; j++ {
		s := k[j*n+j]
		for p := 0; p < j; p++ {
			s -= k[j*n+p] * k[j*n+p]
		}
		if s <= 0 {
			return errors.New("not positive definite")
		}
		d := math.Sqrt(s)
		k[j*n+j] = d
		for i := j + 1; i < n; i++ {
			t := k[i*n+j]
			for p := 0; p < j; p++ {
				t -= k[i*n+p] * k[j*n+p]
			}
			k[i*n+j] = t / d
		}
	}
	for i := 0; i < n; i++ { // L z = y
		t := y[i]
		for p := 0; p < i; p++ {
			t -= k[i*n+p] * y[p]
		}
		y[i] = t / k[i*n+i]
	}
	for i := n - 1; i >= 0; i-- { // L^T x = z
		t := y[i]
		for p := i + 1; p < n; p++ {
			t -= k[p*n+i] * y[p]
		}
		y[i] = t / k[i*n+i]
	}
	return nil
}

// AssimilateNaive: first draft. Fresh matrices and output fields per call,
// haversine from degrees for every (cell, station) pair, one pass per field.
func AssimilateNaive(bg *Snapshot, st []Station) (*Snapshot, error) {
	g := bg.G
	n := len(st)
	out := &Snapshot{G: g, Hour: bg.Hour, sinDecl: bg.sinDecl, cosDecl: bg.cosDecl, ssLon: bg.ssLon, sun: bg.sun}
	for field := 0; field < 2; field++ {
		v, bgF := varFo, bg.FoF2
		if field == 1 {
			v, bgF = varM, bg.M3000
		}
		k := make([]float64, n*n)
		y := make([]float64, n)
		for i := range st {
			for j := range st {
				k[i*n+j] = v * math.Exp(-haversineKm(st[i].LatD, st[i].LonD, st[j].LatD, st[j].LonD)/gpL)
			}
			k[i*n+i] += gpNoise * v
			y[i] = st[i].ResFo
			if field == 1 {
				y[i] = st[i].ResM
			}
		}
		if err := cholSolve(k, y, n); err != nil {
			return nil, err
		}
		res := make([]float64, g.Cols*g.Rows)
		for r := range g.Rows {
			for c := range g.Cols {
				sum := 0.0
				for i := range st {
					d := haversineKm(g.latD[r], g.lonD[c], st[i].LatD, st[i].LonD)
					sum += v * math.Exp(-d/gpL) * y[i]
				}
				res[r*g.Cols+c] = bgF[r*g.Cols+c] + sum
			}
		}
		if field == 0 {
			out.FoF2 = res
		} else {
			out.M3000 = res
		}
	}
	return out, nil
}

const maxStations = 64

// GPWork is reusable workspace for AssimilateFast.
type GPWork struct {
	k       [maxStations * maxStations]float64
	p       [maxStations][3]float64
	aFo, aM [maxStations]float64
}

// AssimilateFast writes into out (FoF2/M3000 reused when large enough).
// Station unit vectors once, distance from a dot product, one exp per
// (cell, station) shared by both fields, no allocation in the loop.
func AssimilateFast(bg *Snapshot, st []Station, w *GPWork, out *Snapshot) error {
	g := bg.G
	n := len(st)
	for i := range st {
		w.p[i] = unitVec(st[i].LatD, st[i].LonD)
	}
	for field := 0; field < 2; field++ {
		v, a := varFo, w.aFo[:n]
		if field == 1 {
			v, a = varM, w.aM[:n]
		}
		k := w.k[:n*n]
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				d := math.Acos(clamp(dot(w.p[i], w.p[j]), -1, 1)) * earthR
				k[i*n+j] = v * math.Exp(-d/gpL)
			}
			k[i*n+i] += gpNoise * v
			a[i] = st[i].ResFo
			if field == 1 {
				a[i] = st[i].ResM
			}
		}
		if err := cholSolve(k, a, n); err != nil {
			return err
		}
	}
	// Fold the variance into the weights.
	var wFo, wM [maxStations]float64
	for i := 0; i < n; i++ {
		wFo[i], wM[i] = varFo*w.aFo[i], varM*w.aM[i]
	}
	cells := g.Cols * g.Rows
	if cap(out.FoF2) < cells {
		out.FoF2 = make([]float64, cells)
	}
	if cap(out.M3000) < cells {
		out.M3000 = make([]float64, cells)
	}
	out.FoF2, out.M3000 = out.FoF2[:cells], out.M3000[:cells]
	out.G, out.Hour, out.sinDecl, out.cosDecl, out.ssLon, out.sun = g, bg.Hour, bg.sinDecl, bg.cosDecl, bg.ssLon, bg.sun
	const invLR = earthR / gpL
	for r := range g.Rows {
		sl, cl := g.sinLat[r], g.cosLat[r]
		for c := range g.Cols {
			q := [3]float64{cl * g.cosLon[c], cl * g.sinLon[c], sl}
			sFo, sM := 0.0, 0.0
			for i := 0; i < n; i++ {
				e := math.Exp(-math.Acos(clamp(dot(q, w.p[i]), -1, 1)) * invLR)
				sFo += e * wFo[i]
				sM += e * wM[i]
			}
			idx := r*g.Cols + c
			out.FoF2[idx] = bg.FoF2[idx] + sFo
			out.M3000[idx] = bg.M3000[idx] + sM
		}
	}
	return nil
}
