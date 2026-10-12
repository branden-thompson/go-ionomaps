package limits

import "math"

// BasicMUF is the F2 layer's basic MUF, MHz, for a hop of d km at a control
// point, from foF2, M(3000)F2 and foE there (MHz) and the electron
// gyrofrequency at 300 km (MHz); a hop past dmax takes dmax's. No number
// for a hop of no length, no foF2 or foE, an M(3000)F2 at 2 or below,
// where the method's B has no meaning, or no gyrofrequency.
//
// Source: ITU-R P.533-14 §3.5.1, equations (3) to (6), dmax held to 4000 km
// for the basic MUF (02-analysis/evidence/p533-equations.md).
func BasicMUF(d, foF2, m3000, foE, fH float64) float64 {
	if !(d > 0) || !(foF2 > 0) || !(foE > 0) {
		return math.NaN()
	}
	if !(m3000 > 2) {
		return math.NaN()
	}
	if !(fH >= 0) {
		return math.NaN()
	}
	b, dmax := hopShape(foF2, m3000, foE)
	d = math.Min(d, dmax)
	cd := func(hop float64) float64 { // (4)
		z := 1 - 2*hop/dmax
		return 0.74 - 0.591*z - 0.424*z*z - 0.090*math.Pow(z, 3) + 0.088*math.Pow(z, 4) + 0.181*math.Pow(z, 5) + 0.096*math.Pow(z, 6)
	}
	return (1+cd(d)/cd(3000)*(b-1))*foF2 + fH/2*(1-d/dmax) // (3)
}

// Dmax is the longest F2 hop, km, at a control point, held to 4000 km for
// the basic MUF: the hop the lowest-order mode is counted by. No number
// where BasicMUF has none.
//
// Source: ITU-R P.533-14 §3.5.1, equations (5) and (6).
func Dmax(foF2, m3000, foE float64) float64 {
	if !(foF2 > 0) || !(foE > 0) {
		return math.NaN()
	}
	if !(m3000 > 2) {
		return math.NaN()
	}
	_, dmax := hopShape(foF2, m3000, foE)
	return dmax
}

// hopShape is equation (6)'s B and equation (5)'s dmax, held to 4000 km.
func hopShape(foF2, m3000, foE float64) (b, dmax float64) {
	x := math.Max(foF2/foE, 2)
	b = m3000 - 0.124 + (m3000*m3000-4)*(0.0215+0.005*math.Sin(7.854/x-1.9635))                           // (6)
	dmax = math.Min(4000, 4780+(12610+2140/(x*x)-49720/math.Pow(x, 4)+688900/math.Pow(x, 6))*(1/b-0.303)) // (5)
	return b, dmax
}
