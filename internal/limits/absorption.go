package limits

import (
	"math"
	"time"
)

// The absorption of ITU-R P.533-14 §5.2.2 eq. (20) (watchpost D-186): its
// three factors - ATnoon (Fig. 1), the penetration factor phi (Fig. 2) and
// the diurnal exponent p (Fig. 3) - are ported from ITU-R Study Group 3's
// ITUHFProp (P533/Src/P533/MedianSkywaveFieldStrengthShort.c:
// AbsorptionFactor, AbsorptionLayerPenetrationFactor,
// DiurnalAbsorptionExponent; themselves from NTIA's REC533 CONTAT, PHIFUN
// and CONTP), which "may be used by implementers in their implementation of
// the Recommendation ... free from any copyright assertions". Held to
// ITUHFProp's own functions (testdata/itu-absorption-golden.txt).

// atNoonTable is ATnoon, the absorption factor at local noon for R12 = 0, a curve a
// season at 29 latitudes 0° to 70° by 2.5°: P.533-14 Fig. 1, as ITUHFProp
// holds it (AbsorptionFactor; REC533 CONTAT).
var atNoonTable = [9][29]float64{
	{323.9, 297.5, 274.5, 256.4, 244.2, 235.0, 229.5, 226.1, 226.8, 229.0, 232.5, 237.0, 243.4, 249.9, 258.1, 267.5, 277.5, 283.3, 283.2, 273.1, 257.0, 232.1, 201.4, 171.5, 146.0, 123.0, 103.1, 83.0, 66.6},
	{312.1, 285.1, 263.1, 251.8, 249.5, 250.9, 254.5, 260.3, 266.7, 272.3, 277.8, 280.3, 283.9, 284.5, 284.4, 283.0, 278.6, 273.0, 265.7, 256.3, 244.8, 232.0, 218.1, 204.5, 189.9, 172.3, 155.3, 135.5, 116.2},
	{347.7, 321.9, 302.5, 293.8, 291.4, 289.3, 292.1, 296.6, 304.3, 313.0, 321.7, 333.8, 342.6, 349.6, 355.2, 355.6, 352.2, 341.7, 327.3, 308.4, 286.0, 265.0, 244.1, 223.8, 202.8, 181.8, 160.8, 141.6, 123.4},
	{338.0, 313.2, 297.0, 290.2, 292.1, 299.4, 308.0, 320.4, 331.6, 340.7, 347.8, 353.8, 357.0, 360.0, 359.8, 358.3, 355.8, 350.8, 344.5, 332.7, 316.4, 292.5, 266.1, 236.4, 214.0, 193.8, 177.5, 165.0, 155.9},
	{328.1, 303.8, 287.7, 282.5, 284.4, 289.4, 294.8, 303.6, 312.9, 322.7, 332.3, 343.8, 350.6, 358.7, 364.3, 365.8, 362.4, 356.0, 346.7, 333.0, 318.8, 299.7, 282.1, 260.5, 240.5, 220.6, 203.9, 186.3, 173.0},
	{305.1, 288.5, 275.2, 273.7, 278.6, 288.9, 302.5, 319.3, 333.6, 346.3, 356.3, 364.7, 371.7, 373.6, 374.2, 373.1, 370.5, 365.1, 358.5, 347.7, 335.0, 320.3, 299.1, 276.6, 253.2, 230.7, 214.0, 196.6, 185.3},
	{345.4, 319.4, 298.7, 290.1, 290.0, 291.8, 296.3, 302.9, 312.1, 320.1, 327.8, 334.1, 340.2, 343.3, 345.7, 346.5, 345.3, 341.1, 334.5, 321.7, 304.2, 286.8, 265.9, 244.8, 224.1, 204.5, 183.6, 164.1, 145.2},
	{341.9, 314.8, 295.3, 277.9, 265.0, 258.2, 254.4, 255.8, 257.3, 262.9, 268.5, 279.0, 287.5, 295.2, 299.6, 300.2, 298.9, 291.5, 279.0, 262.6, 245.7, 227.0, 203.6, 182.3, 163.2, 147.1, 133.9, 119.9, 110.8},
	{318.8, 293.3, 268.3, 251.7, 240.4, 233.1, 229.4, 228.8, 230.5, 235.5, 239.7, 242.6, 245.4, 247.5, 248.9, 249.9, 248.5, 244.4, 237.3, 225.6, 213.5, 195.2, 172.7, 151.3, 131.1, 113.1, 100.1, 89.0, 80.0},
}

// atNoonCurve is the ATnoon curve a month reads, January first: winter,
// February, March, April, May and August, June and July, September,
// October, November (AbsorptionFactor).
var atNoonCurve = [12]int{0, 1, 2, 3, 4, 5, 5, 4, 6, 7, 8, 0}

// pBreak is the modified dip, degrees, at which the diurnal exponent
// changes from one fit to the other, a month, January first (P.533-14
// Fig. 3, as ITUHFProp holds it: DiurnalAbsorptionExponent; REC533 CONTP).
var pBreak = [12]float64{30.0, 30.0, 30.0, 27.5, 32.5, 35.0, 37.5, 35.0, 32.5, 30.0, 30.0, 30.0}

// pFits are the diurnal exponent's polynomials in the normalised modified
// dip: a month, January first; below and above the break; seven
// coefficients from the constant term.
var pFits = [12][2][7]float64{
	{
		{1.510, -0.353, -0.090, 0.191, 0.133, -0.067, -0.053},
		{1.400, -0.365, -1.212, -0.049, 1.187, 0.119, -0.400},
	},
	{
		{1.490, -0.348, -0.055, 0.164, 0.160, -0.041, -0.080},
		{1.450, -0.119, -0.913, -0.640, 0.347, 0.458, 0.107},
	},
	{
		{1.520, -0.410, -0.138, 0.308, 0.267, -0.113, -0.133},
		{1.500, -0.492, -0.958, 0.216, 0.267, -0.029, 0.187},
	},
	{
		{1.580, -0.129, -0.228, -0.192, 0.200, 0.116, -0.027},
		{1.530, -0.468, -1.312, 0.096, 0.973, 0.057, -0.187},
	},
	{
		{1.590, 0.002, -0.102, -0.579, -0.467, 0.522, 0.613},
		{1.490, -0.937, -1.622, 1.365, 1.720, -0.873, -0.453},
	},
	{
		{1.600, -0.060, -0.175, -0.037, 0.147, -0.008, -0.027},
		{1.460, -0.881, -1.595, 0.901, 2.133, -0.395, -0.933},
	},
	{
		{1.60, -0.030, -0.135, -0.137, 0.053, 0.072, 0.027},
		{1.43, -0.902, -1.667, 0.905, 2.480, -0.383, -1.173},
	},
	{
		{1.59, -0.032, -0.083, -0.119, 0.000, 0.031, 0.053},
		{1.46, -0.831, -1.653, 0.708, 2.320, -0.257, -1.067},
	},
	{
		{1.59, -0.060, -0.180, -0.181, 0.267, 0.081, -0.107},
		{1.51, -0.809, -1.740, 0.750, 2.240, -0.301, -0.960},
	},
	{
		{1.57, -0.189, -0.207, -0.005, 0.293, 0.004, -0.107},
		{1.52, -0.433, -1.015, -0.017, 0.440, 0.115, 0.080},
	},
	{
		{1.55, -0.292, -0.275, 0.093, 0.427, -0.026, -0.187},
		{1.44, -0.279, -0.770, -0.266, 0.053, 0.245, 0.267},
	},
	{
		{1.51, -0.347, -0.082, 0.160, 0.093, -0.048, -0.027},
		{1.40, -0.355, -1.212, -0.102, 1.187, 0.172, -0.400},
	},
}

// The limits of eq. (20)'s inputs: the solar zenith angle is held to 102°
// (§5.2.2), F(chi) to 0.02, the modified dip to 70° (Fig. 3's range), the
// latitude ATnoon reads to below 70° (Fig. 1's).
const (
	maxChi      = 102.0
	minFChi     = 0.02
	maxModDip   = 70.0
	maxATLat    = 69.99
	atLatStep   = 2.5
	chiFactor   = 0.881
	phiScale    = 0.34
	r12Absorbed = 0.0067
)

// atNoon is ATnoon, the absorption factor at local noon for R12 = 0, at a
// latitude (degrees) and month: linear between Fig. 1's latitudes, the
// month's curve, either hemisphere alike; no number off the globe or with
// no month.
func atNoon(lat float64, month time.Month) float64 {
	onGlobe := lat >= -90 && lat <= 90
	if !onGlobe {
		return math.NaN()
	}
	if month < time.January || month > time.December {
		return math.NaN()
	}
	curve := atNoonTable[atNoonCurve[month-1]]
	x := math.Min(math.Abs(lat), maxATLat) / atLatStep
	j := int(x)
	frac := x - float64(j)
	return curve[j+1]*frac + curve[j]*(1-frac)
}

// penetration is phi, the absorption layer penetration factor, at the
// ratio of the equivalent vertical frequency to foE (Fig. 2): two
// polynomials, the second held to 0.53, a linear tail to 10, flat past it,
// scaled by 0.34; no number for a ratio that is none.
func penetration(t float64) float64 {
	if t != t {
		return math.NaN()
	}
	var phi float64
	switch {
	case t < 0:
		phi = 0
	case t <= 1:
		x := (t - 0.475) / 0.475
		phi = (((((-0.093*x+0.04)*x+0.127)*x-0.027)*x+0.044)*x+0.159)*x + 0.225 // peaks at 0.504 on 0 to 1: under the second's cap
	case t <= 2.2:
		x := (t - 1.65) / 0.55
		phi = math.Min((((((0.043*x-0.07)*x-0.027)*x+0.034)*x+0.054)*x-0.049)*x+0.375, 0.53)
	case t <= 10:
		phi = 0.34 + (10-t)*0.02/7.8
	default:
		phi = 0.34
	}
	return phi / phiScale
}

// diurnalExponent is p, the diurnal absorption exponent, at a latitude and
// a dip at 100 km (degrees) and a month (Fig. 3): a polynomial in the
// modified dip, atan(I / sqrt(cos lat)), held to 70°, one fit below the
// month's break and one above, the months moved by six in the south; no
// number off the globe, for a dip past ±90°, or with no month.
func diurnalExponent(lat, dip float64, month time.Month) float64 {
	onGlobe := lat >= -90 && lat <= 90 && dip >= -90 && dip <= 90
	if !onGlobe {
		return math.NaN()
	}
	if month < time.January || month > time.December {
		return math.NaN()
	}
	r := math.Pi / 180
	modDip := math.Min(math.Abs(math.Atan2(dip*r, math.Sqrt(math.Cos(lat*r))))/r, maxModDip)
	m := int(month) - 1
	if lat < 0 {
		m = (m + 6) % 12
	}
	brk, fit := pBreak[m], 0
	x := -1 + 2*modDip/brk
	if modDip > brk {
		fit, x = 1, -1+2*(modDip-brk)/(maxModDip-brk)
	}
	p, pow := 0.0, 1.0
	for _, a := range pFits[m][fit] {
		p += a * pow
		pow *= x
	}
	return p
}

// Penetration is what eq. (20) reads where a ray crosses 90 km: the place's
// latitude and the dip at 100 km, degrees; the sun's zenith angle there now
// and at local noon, degrees; and foE, MHz.
type Penetration struct {
	Lat, Dip     float64
	Chi, ChiNoon float64
	FoE          float64
}

// AbsorptionTerm is one penetration point's part of eq. (20):
// ATnoon times phi(fv/foE) times F(chi) / F(chi at noon), with F(chi) =
// cos^p(0.881 chi), at least 0.02, chi held to 102°; no number for a point
// off the globe, a dip past ±90°, foE not above zero, or no month.
//
// Source: ITU-R P.533-14 §5.2.2 eqs. (20), (21); the factors as ITUHFProp
// holds them (D-186).
func AbsorptionTerm(p Penetration, month time.Month, fv float64) float64 {
	onGlobe := p.Lat >= -90 && p.Lat <= 90 && p.Dip >= -90 && p.Dip <= 90
	if !onGlobe || !(p.FoE > 0) {
		return math.NaN()
	}
	if month < time.January || month > time.December {
		return math.NaN()
	}
	pe := diurnalExponent(p.Lat, p.Dip, month)
	f := func(chi float64) float64 {
		return math.Max(math.Pow(math.Cos(chiFactor*math.Min(chi, maxChi)*math.Pi/180), pe), minFChi)
	}
	return atNoon(p.Lat, month) * penetration(fv/p.FoE) * f(p.Chi) / f(p.ChiNoon)
}

// AbsorptionLoss is Li, dB, for a mode of some hops (eq. 20): the hops
// times (1 + 0.0067 R12) times the mean of its penetration points' terms,
// over (f + fL)^2 and the cosine of the angle of incidence at 110 km; no
// number for no hop, a ray at or past the horizon, a frequency not above
// zero, a sunspot number below zero, a gyrofrequency that is no number, or
// a term that is none.
//
// Source: ITU-R P.533-14 §5.2.2 eq. (20); ITUHFProp's form of it (D-186).
func AbsorptionLoss(hops int, r12, f, fL, cosI110, meanTerm float64) float64 {
	if hops < 1 || !(cosI110 > 0) || !(f > 0) {
		return math.NaN()
	}
	if math.IsNaN(meanTerm) || math.IsInf(meanTerm, 0) {
		return math.NaN()
	}
	if !(r12 >= 0) {
		return math.NaN()
	}
	if !(fL >= 0) {
		return math.NaN()
	}
	return float64(hops) * (1 + r12Absorbed*r12) * meanTerm / ((f + fL) * (f + fL) * cosI110)
}
