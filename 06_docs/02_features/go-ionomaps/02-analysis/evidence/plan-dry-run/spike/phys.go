// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import "math"

// PLACEHOLDER PHYSICS OF EQUIVALENT COST.
//
// The functions in this file are NOT the published ITU-R P.533 / P.1239
// equations and must not be used for any answer. They exist so the benchmark
// pays the same kind and count of transcendental calls per path as a
// P.533-style computation would:
//
//   foE                one pow (E-layer critical frequency from the zenith angle)
//   basicMUF           one sin, several divisions, two 6th-order polynomials
//   hopGeometry        cos, sin, atan, asin, cos, ceil (elevation and sec i)
//   solarFactor        acos, cos, pow (absorption zenith dependence)
//   freqFactor         pow (absorption frequency dependence)
//   phiN               exp (absorption dependence on f/foE)
//
// Coefficients are chosen only so outputs stay in plausible ranges.

const (
	earthR  = 6371.0 // km
	hopH    = 300.0  // km, reflection height (placeholder)
	absH    = 110.0  // km, absorbing layer height (placeholder)
	fH      = 1.2    // MHz, gyrofrequency (placeholder)
	r12     = 100.0  // sunspot number (placeholder)
	lossMax = 35.0   // dB, absorption limit for "usable" (placeholder)
)

// Bands are the 10 amateur HF bands, MHz.
var Bands = [10]float64{1.8, 3.5, 5.3, 7, 10.1, 14, 18.1, 21, 24.9, 28}

func foE(cosChi float64) float64 {
	c := cosChi
	if c < 0.02 {
		c = 0.02
	}
	return 0.9 * math.Pow((180+1.44*r12)*c, 0.25)
}

func cdPoly(z float64) float64 {
	return 0.74 + z*(-0.591+z*(-0.424+z*(-0.090+z*(0.088+z*(0.181+z*0.096)))))
}

// basicMUF: placeholder of equivalent cost to a P.533-style F2 basic MUF for
// distance d km from foF2, M(3000)F2 and foE at a control point.
func basicMUF(fo, m, fe, d float64) float64 {
	x := fo / fe
	if x < 2 {
		x = 2
	}
	b := m - 0.124 + (m*m-4)*(0.0215+0.005*math.Sin(7.854/x-1.9635))
	x2 := x * x
	x4 := x2 * x2
	dmax := 4780 + (12610+2140/x2-49720/x4+688900/(x4*x2))*(1/b-0.303)
	dmax = clamp(dmax, 1000, 4000)
	if d > dmax {
		d = dmax
	}
	cd := cdPoly(1 - 2*d/dmax)
	c3 := cdPoly(1 - 2*3000/dmax)
	return (1+cd/c3*(b-1))*fo + fH/2*(1-d/dmax)
}

// hopGeometry: hop count and sec(i) at the absorbing layer for a d km path.
func hopGeometry(d float64) (hops, secI float64) {
	hops = math.Ceil(d / 4000)
	if hops < 1 {
		hops = 1
	}
	theta := d / hops / (2 * earthR)
	elev := math.Atan((math.Cos(theta) - earthR/(earthR+hopH)) / math.Sin(theta))
	i := math.Asin(earthR * math.Cos(elev) / (earthR + absH))
	return hops, 1 / math.Cos(i)
}

func solarFactor(cosChi float64) float64 {
	chi := math.Acos(clamp(cosChi, -1, 1))
	c := math.Cos(0.881 * chi)
	if c < 0.02 {
		c = 0.02
	}
	return math.Pow(c, 1.3)
}

func freqFactor(f float64) float64 { return math.Pow(f+fH, 1.98) + 10.2 }

func phiN(f, fe float64) float64 { return 1 + 0.5*math.Exp(-f/fe) }

// absorption: placeholder loss in dB from precomputed factors.
func absorption(hops, secI, sf, ff, f, fe float64) float64 {
	return hops * 677.2 * secI * (1 + 0.0067*r12) * sf * phiN(f, fe) / ff
}

// absorptionLoss is the unfactored form used by the naive path.
func absorptionLoss(f, fe, cosChi, d float64) float64 {
	hops, secI := hopGeometry(d)
	return absorption(hops, secI, solarFactor(cosChi), freqFactor(f), f, fe)
}
