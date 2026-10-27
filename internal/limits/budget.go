package limits

import "math"

// The reference circuit's budget by ITU-R P.533-14 §4 to §7, each equation
// as read from the recommendation (02-analysis/evidence/p533-field-strength.md;
// watchpost D-73, D-91, D-186 to D-189).

// The reference circuit: 100 W, 0 dBi at both ends, +13 dB in 2.5 kHz
// (watchpost D-73, D-91, D-188).
const (
	TxPowerDBkW = -10.0 // 100 W in dB(1 kW)
	AntennaDBi  = 0.0
	RequiredSNR = 13.0
	BandwidthHz = 2500.0
)

// The heights P.533 measures at, km: the E layer's mirror and the angle of
// incidence (110), where a ray crosses the absorbing layer (90), and the
// limit of an F2 mirror (800); and the loss not otherwise included, dB.
const (
	ELayerKm      = 110.0
	PenetrationKm = 90.0
	maxMirrorKm   = 800.0
	lzDB          = 8.72
)

// ElevationAngle is a hop's elevation angle, degrees (eq. 13): a hop of d
// km from a mirror hr km high; negative past the horizon. No number for no
// hop, a hop past half the globe, or a height below the ground.
//
// Source: ITU-R P.533-14 §5.1 eq. (13).
func ElevationAngle(d, hr float64) float64 {
	if !(d > 0) || !(d < math.Pi*earthKm) {
		return math.NaN()
	}
	if !(hr >= 0) {
		return math.NaN()
	}
	psi := d / (2 * earthKm)
	return math.Atan(1/math.Tan(psi)-earthKm/(earthKm+hr)/math.Sin(psi)) * 180 / math.Pi
}

// IncidenceAngle is the angle of incidence, degrees, at a height h km of a
// ray at an elevation angle (eq. 12). No number for an elevation past ±90°
// or a height below the ground.
//
// Source: ITU-R P.533-14 §4 eq. (12).
func IncidenceAngle(delta, h float64) float64 {
	if !(math.Abs(delta) <= 90) || !(h >= 0) {
		return math.NaN()
	}
	return math.Asin(earthKm*math.Cos(delta*math.Pi/180)/(earthKm+h)) * 180 / math.Pi
}

// SlantRange is the virtual slant range, km, of n hops of d km at an
// elevation angle (eq. 19). No number for no hop or a hop of no length.
//
// Source: ITU-R P.533-14 §5.2.2 eq. (19).
func SlantRange(n int, d, delta float64) float64 {
	if n < 1 || !(d > 0) {
		return math.NaN()
	}
	psi := d / (2 * earthKm)
	return float64(n) * math.Abs(2*earthKm*math.Sin(psi)/math.Cos(delta*math.Pi/180+psi))
}

// PenetrationFractions are where a mode's n hops cross 90 km, as parts of a
// path of D km: two a hop, nearest each end of it (§5.2.2); none for no hop.
// Each is held to the path, from 0 to 1: a path under about a kilometre,
// its elevation rounding short of the vertical, would put a crossing far
// past its ends.
//
// Source: ITU-R P.533-14 §5.2.2 (reflection at the mirror, penetration at
// 90 km); the crossing's place from the hop's geometry.
func PenetrationFractions(D float64, n int, delta float64) []float64 {
	if n < 1 || !(D > 0) {
		return nil
	}
	i90 := IncidenceAngle(delta, PenetrationKm) * math.Pi / 180
	in := earthKm * (math.Pi/2 - delta*math.Pi/180 - i90) // the ground distance from a hop's end to its crossing
	dh := D / float64(n)
	out := make([]float64, 0, 2*n)
	for i := range n {
		up, down := (float64(i)*dh+in)/D, (float64(i+1)*dh-in)/D
		out = append(out, math.Min(1, math.Max(0, up)), math.Min(1, math.Max(0, down))) // held to the path
	}
	return out
}

// MirrorHeight is an F2 mode's mirror-reflection height, km, for the
// elevation angle (§5.1, eqs. 14 to 16): at a control point's foF2, foE and
// M(3000)F2, the sunspot number, the wave frequency and the hop; held to
// 800 km. No number for no foF2, foE or frequency, or an M(3000)F2 that is
// no number or so high the height H falls to -140 km.
//
// Source: ITU-R P.533-14 §5.1 eqs. (14) to (16) and the height H with ΔM.
func MirrorHeight(foF2, foE, m3000, r12, f, d float64) float64 {
	if !(foF2 > 0) || !(foE > 0) || !(f > 0) {
		return math.NaN()
	}
	if m3000 != m3000 {
		return math.NaN()
	}
	x := foF2 / foE
	y := math.Max(x, 1.8)
	h := 1490/(m3000+0.18/(y-1.4)+0.096*(r12-25)/150) - 316
	if !(h > -140) {
		return math.NaN() // the heights' terms divide by H + 140: an M(3000)F2 past any the method knows
	}
	xr := f / foF2
	var hr float64
	switch {
	case x > 3.33 && xr >= 1:
		hr = mirrorAbove(h, xr, d)
	case x > 3.33:
		hr = mirrorBelow(h, xr, d)
	default:
		j := -0.7126*y*y*y + 5.863*y*y - 16.13*y + 16.07
		u := 8e-5*(h-80)*(1+11*math.Pow(y, -2.2)) + 1.2e-3*h*math.Pow(y, -3.6)
		hr = 115 + h*j + u*d // (16)
	}
	return math.Min(hr, maxMirrorKm)
}

// mirrorAbove is eq. (14)'s height, the wave at or above foF2.
func mirrorAbove(h, xr, d float64) float64 {
	e1 := -0.09707*xr*xr*xr + 0.6870*xr*xr - 0.7506*xr + 0.6
	f1 := 1.21 + 0.2*xr
	if xr <= 1.71 {
		f1 = -1.862*math.Pow(xr, 4) + 12.95*xr*xr*xr - 32.03*xr*xr + 33.50*xr - 10.91
	}
	g := 19.25
	if xr <= 3.7 {
		g = -2.102*math.Pow(xr, 4) + 19.50*xr*xr*xr - 63.15*xr*xr + 90.47*xr - 44.73
	}
	a1 := 140 + (h-47)*e1
	b1 := 150 + (h-17)*f1 - a1
	a := (d - (160 + (h+43)*g)) / (h + 140)
	if b1 >= 0 && a >= 0 {
		return a1 + b1*math.Pow(2.4, -a)
	}
	return a1 + b1
}

// mirrorBelow is eq. (15)'s height, the wave below foF2.
func mirrorBelow(h, xr, d float64) float64 {
	z := math.Max(xr, 0.1)
	e2 := 0.1906*z*z + 0.00583*z + 0.1936
	f2 := 0.645*z*z + 0.883*z + 0.162
	a2 := 151 + (h-47)*e2
	b2 := 141 + (h-24)*f2 - a2
	df := math.Min(0.115*d/(z*(h+140)), 0.65)
	b := -7.535*math.Pow(df, 4) + 15.75*df*df*df - 8.834*df*df - 0.378*df + 1
	if b2 >= 0 {
		return a2 + b2*b
	}
	return a2 + b2
}

// ScreeningFrequency is the E layer's maximum screening frequency, MHz, for
// an F2 mode at an elevation angle (eqs. 11, 12): 1.05 foE sec i at 110 km.
// No number for no foE.
//
// Source: ITU-R P.533-14 §4 eqs. (11), (12).
func ScreeningFrequency(foE, deltaF float64) float64 {
	if !(foE > 0) {
		return math.NaN()
	}
	return 1.05 * foE / math.Cos(IncidenceAngle(deltaF, ELayerKm)*math.Pi/180)
}

// AboveMUFLoss is Lm, dB (eqs. 24 to 26): none at or below the mode's basic
// MUF fb; above it, 130 ((f/fb) - 1)^2 for an E mode, at most 81 dB, and
// 36 ((f/fb) - 1)^0.5 for an F2 mode, at most 62 dB. No number for no basic
// MUF.
//
// Source: ITU-R P.533-14 §5.2.2 eqs. (24) to (26).
func AboveMUFLoss(eMode bool, f, fb float64) float64 {
	if !(fb > 0) {
		return math.NaN()
	}
	over := f/fb - 1
	switch {
	case !(over > 0):
		return 0
	case eMode:
		return math.Min(130*over*over, 81)
	}
	return math.Min(36*math.Sqrt(over), 62)
}

// ModeField is a mode's median field strength, dB(1 µV/m) (eqs. 17, 18,
// 27): the reference circuit's power and gain, over the basic loss of n hops
// with their slant range, absorption Li, above-the-MUF loss Lm and auroral
// loss Lh. No number for no frequency, no hop, or a loss that is no number.
//
// Source: ITU-R P.533-14 §5.2.2 eqs. (17), (18), (27); Lz 8.72 dB.
func ModeField(f, slant, li, lm float64, hops int, lh float64) float64 {
	if !(f > 0) || hops < 1 {
		return math.NaN()
	}
	loss := 32.45 + 20*math.Log10(f) + 20*math.Log10(slant) + li + lm + 2*float64(hops-1) + lh + lzDB
	if math.IsNaN(loss) || math.IsInf(loss, 0) {
		return math.NaN()
	}
	return 136.6 + TxPowerDBkW + AntennaDBi + 20*math.Log10(f) - loss
}

// SumFields is the modes' field strengths summed in power, dB(1 µV/m) (eq.
// 28); minus infinity with no mode.
//
// Source: ITU-R P.533-14 §5.2.2 eq. (28).
func SumFields(fields []float64) float64 {
	sum := 0.0
	for _, ew := range fields {
		sum += math.Pow(10, ew/10)
	}
	return 10 * math.Log10(sum)
}

// PowerFromField is the power available from the reference circuit's
// receiving antenna for a field strength, dBW (eq. 43). No number for no
// frequency.
//
// Source: ITU-R P.533-14 §6 eq. (43).
func PowerFromField(f, e float64) float64 {
	if !(f > 0) {
		return math.NaN()
	}
	return e + AntennaDBi - 20*math.Log10(f) - 107.2
}

// Environment is P.372's man-made noise category (watchpost D-187).
type Environment uint8

// The categories.
const (
	Residential Environment = iota
	City
	Rural
	QuietRural
)

// manMade is each category's c and d in Fam = c - d log f (P.372-18 Table 1).
var manMade = [...][2]float64{Residential: {72.5, 27.7}, City: {76.8, 27.7}, Rural: {67.2, 27.7}, QuietRural: {53.6, 28.6}}

// NoiseFactor is the external noise factor Fa, dB(kT0b), at a frequency in
// MHz: the category's man-made noise, and galactic noise added in power
// where it is heard (above foF2); atmospheric noise is not modelled (D-187).
// No number for no frequency or no category.
//
// Source: ITU-R P.372-18 Table 1 and §4.1 (galactic noise, 52 - 23 log f).
func NoiseFactor(env Environment, f float64, galactic bool) float64 {
	if !(f > 0) || int(env) >= len(manMade) {
		return math.NaN()
	}
	cd := manMade[env]
	fa := cd[0] - cd[1]*math.Log10(f)
	if galactic {
		fa = 10 * math.Log10(math.Pow(10, fa/10)+math.Pow(10, (52-23*math.Log10(f))/10))
	}
	return fa
}

// SNR is the median signal-to-noise ratio, dB, in the reference circuit's
// 2.5 kHz (eq. 45).
//
// Source: ITU-R P.533-14 §7 eq. (45).
func SNR(pr, fa float64) float64 { return pr - fa - 10*math.Log10(BandwidthHz) + 204 }

// R12FromFlux is the sunspot number whose 10.7 cm flux is the one given, by
// ITU-R P.371-9 eq. (4) inverted; none below 63.7, the flux of R12 = 0. No
// number for a flux that is none.
//
// Source: ITU-R P.371-9 (08/2023) eq. (4): Φ12 = 63.7 + 0.728 R12 + 8.9e-4 R12².
func R12FromFlux(phi float64) float64 {
	if phi != phi {
		return math.NaN()
	}
	if phi <= 63.7 {
		return 0
	}
	const a, b = 8.9e-4, 0.728
	return (-b + math.Sqrt(b*b+4*a*(phi-63.7))) / (2 * a)
}
