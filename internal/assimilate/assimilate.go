// Package assimilate is the station residuals' spread over the sphere (plan
// task G5.1, R-9.2): a Gaussian process with an exponential kernel in
// great-circle distance - valid on the sphere (Gneiting 2013) - fitted to
// the residuals about their mean, its scale their variance and its noise a
// part of it, as PLAN's week was scored (watchpost D-101; week.py's
// gp_loo). foF2's residuals are taken over GloTEC, M(3000)F2's over the
// climatology, each with its own kernel.
package assimilate

import (
	"errors"
	"math"
)

// Kernel is a fit's length and noise, the noise a part of the residuals'
// variance.
type Kernel struct{ LengthKm, Noise float64 }

// The kernels tuned on PLAN's week (A-29; week.py section "kernel tuned",
// unchanged with the unknown scores taken, D-61).
var (
	FoF2Kernel  = Kernel{LengthKm: 2000, Noise: 0.5} // foF2 over GloTEC
	M3000Kernel = Kernel{LengthKm: 2000, Noise: 0.2} // M(3000)F2 over the climatology
)

// minStations is the fewest residuals a fit is made from (week.py).
const minStations = 3

// minVariance holds the scale off zero when every residual is the same.
const minVariance = 1e-4

// earthKm is the mean Earth radius distances are measured on, km.
const earthKm = 6371.0

// Obs is one station's residual at its place, degrees.
type Obs struct{ Lat, Lon, Residual float64 }

// Fit is a fitted process: the stations, their weights, the mean residual
// and the scale.
type Fit struct {
	obs      []Obs
	alpha    []float64
	mean     float64
	variance float64
	kernel   Kernel
}

var (
	errFew      = errors.New("assimilate: fewer than three residuals")
	errNoNum    = errors.New("assimilate: a residual or a place that is no number")
	errSingular = errors.New("assimilate: the covariance does not solve")
	errKernel   = errors.New("assimilate: a kernel needs a length above 0 and noise of 0 or more")
)

// NewFit fits the residuals.
//
// Source: Rasmussen and Williams (2006), ch. 2; Gneiting (2013),
// doi:10.3150/12-BEJSP06; go-ionomaps 02-analysis/evidence/week.py gp_loo.
func NewFit(obs []Obs, k Kernel) (Fit, error) {
	n := len(obs)
	if n < minStations {
		return Fit{}, errFew
	}
	shapeless := !(k.LengthKm > 0) || !(k.Noise >= 0)
	if shapeless {
		return Fit{}, errKernel // a kernel of no length divides by zero
	}
	sum := 0.0
	for _, o := range obs {
		finite := !math.IsNaN(o.Residual) && !math.IsInf(o.Residual, 0) && !math.IsNaN(o.Lat) && !math.IsNaN(o.Lon)
		if !finite {
			return Fit{}, errNoNum
		}
		sum += o.Residual
	}
	f := Fit{obs: append([]Obs(nil), obs...), mean: sum / float64(n), kernel: k}
	y := make([]float64, n)
	sq := 0.0
	for i, o := range obs {
		y[i] = o.Residual - f.mean
		sq += y[i] * y[i]
	}
	f.variance = math.Max(sq/float64(n), minVariance)
	cov := make([]float64, n*n)
	for i := range n {
		for j := range n {
			cov[i*n+j] = f.variance * math.Exp(-distance(obs[i], obs[j].Lat, obs[j].Lon)/k.LengthKm)
		}
		cov[i*n+i] += k.Noise * f.variance
	}
	alpha, ok := solve(cov, y, n)
	if !ok {
		return Fit{}, errSingular
	}
	f.alpha = alpha
	return f, nil
}

// At is the fit at a place: the mean residual and the stations' weighted
// pull.
//
// Source: as NewFit.
func (f Fit) At(lat, lon float64) float64 {
	unfitted := len(f.alpha) != len(f.obs)
	if unfitted {
		return 0 // no fit: the background unchanged
	}
	nowhere := math.IsNaN(lat) || math.IsNaN(lon)
	if nowhere {
		return 0
	}
	v := f.mean
	for i, o := range f.obs {
		v += f.variance * math.Exp(-distance(o, lat, lon)/f.kernel.LengthKm) * f.alpha[i]
	}
	return v
}

// distance is the great-circle distance from a station to a place, km, as
// week.py measures it.
func distance(o Obs, lat, lon float64) float64 {
	r := math.Pi / 180
	c := math.Sin(o.Lat*r)*math.Sin(lat*r) + math.Cos(o.Lat*r)*math.Cos(lat*r)*math.Cos((o.Lon-lon)*r)
	return earthKm * math.Acos(math.Max(-1, math.Min(1, c)))
}

// solve is the solution of a symmetric positive-definite system by its
// Cholesky factor; false when it is not positive definite.
func solve(a, b []float64, n int) ([]float64, bool) {
	empty := n <= 0
	if empty {
		return nil, false
	}
	misshapen := len(a) != n*n || len(b) != n
	if misshapen {
		return nil, false
	}
	l := make([]float64, n*n)
	for i := range n {
		for j := 0; j <= i; j++ {
			s := a[i*n+j]
			for k := range j {
				s -= l[i*n+k] * l[j*n+k]
			}
			if i == j {
				if !(s > 0) {
					return nil, false
				}
				l[i*n+i] = math.Sqrt(s)
				continue
			}
			l[i*n+j] = s / l[j*n+j]
		}
	}
	x := make([]float64, n)
	for i := range n { // L z = b
		s := b[i]
		for k := range i {
			s -= l[i*n+k] * x[k]
		}
		x[i] = s / l[i*n+i]
	}
	for i := n - 1; i >= 0; i-- { // Lᵀ x = z
		s := x[i]
		for k := i + 1; k < n; k++ {
			s -= l[k*n+i] * x[k]
		}
		x[i] = s / l[i*n+i]
	}
	return x, true
}
