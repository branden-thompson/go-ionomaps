package climatology

import (
	"math"
	"time"
)

// Hour is the climatology for one hour, F10.7 and its date folded in: the
// month blend, the solar interpolation and the Fourier sum in UT are linear
// in the coefficients, so they are applied once, leaving one coefficient a
// harmonic for each parameter. A place then costs one spherical-harmonic sum
// (a field's 16,200 cells, not 16,200 times eight table evaluations).
type Hour struct {
	fo, m3 [Harmonics]float64
	ok     bool
}

// NewHour folds the tables for a moment and an F10.7: what Values computes,
// before the place is known.
//
// Source: as Values; the folding is this library's.
func NewHour(t Tables, at time.Time, f107 float64) Hour {
	var h Hour
	finite := !math.IsNaN(f107) && !math.IsInf(f107, 0)
	if t == nil || !finite {
		return h
	}
	at = at.UTC()
	ut := float64(at.Hour()) + float64(at.Minute())/60 + float64(at.Second())/3600
	before, after, w1, w2 := monthBlend(at)
	fs := fourier(ut)
	r12 := r12From(f107)
	h.ok = fold(&h.fo, t.FoF2, int(before.Month()), int(after.Month()), w1, w2, fs, ig12From(r12)) &&
		fold(&h.m3, t.M3000, int(before.Month()), int(after.Month()), w1, w2, fs, r12)
	return h
}

// fold writes one parameter's hour coefficients: each table's, weighted by
// month, solar level and Fourier term, summed; false for tables of another
// shape.
func fold(out *[Harmonics]float64, c func(int, Level) Coefficients, m1, m2 int, w1, w2 float64, fs [Terms]float64, index float64) bool {
	for _, part := range [4]struct {
		month int
		level Level
		w     float64
	}{{m1, Min, w1 * (100 - index) / 100}, {m2, Min, w2 * (100 - index) / 100}, {m1, Max, w1 * index / 100}, {m2, Max, w2 * index / 100}} {
		coeffs := c(part.month, part.level)
		if len(coeffs) != Terms*Harmonics {
			return false
		}
		for j, f := range fs {
			wf := part.w * f
			row := coeffs[j*Harmonics : (j+1)*Harmonics]
			for k := range out {
				out[k] += wf * row[k]
			}
		}
	}
	return true
}

// At is foF2 and M(3000)F2 at a quasi-dipole latitude and magnetic local
// time, for the hour folded; zeros for a place that is no finite number or
// off the sphere, or an hour whose tables were not whole.
//
// Source: as Values.
func (h *Hour) At(qdlat, mlt float64) (foF2, m3000 float64) {
	if !h.ok {
		return 0, 0
	}
	finite := !math.IsNaN(mlt) && !math.IsInf(mlt, 0)
	if !finite || !(qdlat >= -90 && qdlat <= 90) {
		return 0, 0
	}
	var sh [Harmonics]float64
	harmonics(&sh, (90-qdlat)*math.Pi/180, mlt*15*math.Pi/180)
	for k, y := range sh {
		foF2 += h.fo[k] * y
		m3000 += h.m3[k] * y
	}
	return foF2, m3000
}
