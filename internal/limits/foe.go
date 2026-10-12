// Package limits is the bands' limits for a path (go-ionomaps G6): the
// basic MUF of ITU-R P.533-14 and the E layer's critical frequency it takes
// from P.1239-4; daytime and disturbance absorption follow.
package limits

import "math"

// FoE is the E layer's monthly median critical frequency, MHz, at a
// geographic latitude, for the sun's declination and zenith angle there
// (degrees), the 10.7 cm flux, and at night the hours since sunset; no
// number for a place off the globe, a zenith that is no number, no flux, or
// hours since sunset outside a day.
//
// Source: ITU-R P.1239-4 §4, equations (12) to (18), as P.533-14 §3.2
// takes it (02-analysis/evidence/p533-equations.md).
func FoE(lat, decl, chi, flux, sinceSunset float64) float64 {
	if lat != lat || chi != chi || decl != decl {
		return math.NaN()
	}
	if lat < -90 || lat > 90 {
		return math.NaN()
	}
	if !(flux > 0) {
		return math.NaN()
	}
	if !(sinceSunset >= 0 && sinceSunset <= 24) {
		return math.NaN()
	}
	r := math.Pi / 180
	n := math.Max(-80, math.Min(80, lat-decl)) // (14): N held to 80°
	m, x, y := -1.93+1.92*math.Cos(lat*r), 23.0, 116.0
	if math.Abs(lat) >= 32 {
		m, x, y = 0.11-0.49*math.Cos(lat*r), 92, 35 // (15b), (16c)
	}
	p := 1.20
	if math.Abs(lat) <= 12 {
		p = 1.31
	}
	a := 1 + 0.0094*(flux-66)                  // (13)
	b := math.Pow(math.Cos(n*r), m)            // (14)
	c := x + y*math.Cos(lat*r)                 // (16a)
	floor := 0.004 * math.Pow(1+0.021*flux, 2) // (18)
	switch {
	case chi <= 73:
		return math.Pow(a*b*c*math.Pow(math.Cos(chi*r), p), 0.25) // (17a)
	case chi < 90:
		dchi := 6.27e-13 * math.Pow(chi-50, 8) // (17c)
		return math.Pow(a*b*c*math.Pow(math.Cos((chi-dchi)*r), p), 0.25)
	}
	d := math.Max(math.Pow(0.072, p)*math.Exp(-1.4*sinceSunset), math.Pow(0.072, p)*math.Exp(25.2-0.28*chi)) // (17d), (17e)
	return math.Pow(math.Max(a*b*c*d, floor), 0.25)
}
