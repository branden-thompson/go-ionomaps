package limits

import (
	"math"
	"time"
)

// Paths past 7000 km by ITU-R P.533-14 §5.3 and §5.4 (watchpost D-191),
// each equation read from the recommendation
// (02-analysis/evidence/p533-field-strength.md); Tables 3 and 5 as the
// recommendation prints them and ITUHFProp holds them (D-189, D-191).

// The long-path method's constants: its mirror height and least elevation
// for fM's hops, km and degrees; the longest hop for fM and for fL, km; the
// focusing gain's limit and Ly, dB; the night decay's rate an hour.
const (
	LongMirrorKm = 300.0
	longMinElev  = 3.0
	fmHopKm      = 4000.0
	FLHopKm      = 3000.0
	maxGapDB     = 15.0
	lyDB         = -0.14
	nightDecay   = 0.23
	maxLongHops  = 12
	blendFromKm  = 7000.0
	blendSpanKm  = 2000.0
)

// fdCoefficients are eq. (30)'s distance reduction factor's, from the sixth
// power down.
var fdCoefficients = [...]float64{-2.40074637494790e-24, 25.8520201885984e-21, -92.4986988833091e-18, 102.342990689362e-15,
	22.0776941764705e-12, 87.4376851991085e-9, 29.1996868566837e-6}

// kTable is P.533-14 Table 3: W, X and Y for east-west and north-south
// paths, as the recommendation prints it and ITUHFProp holds it.
var kTable = [2][3]float64{{0.1, 1.2, 0.6}, {0.2, 0.2, 0.4}}

// winterAnomaly is P.533-14 Table 5: Aw at 60° geographic latitude by
// month, January first, northern then southern hemisphere, as the
// recommendation prints it and ITUHFProp holds it.
var winterAnomaly = [2][12]float64{
	{0.30, 0.15, 0.03, 0, 0, 0, 0, 0, 0.01, 0.03, 0.15, 0.30},
	{0, 0, 0, 0.03, 0.15, 0.30, 0.30, 0.15, 0.03, 0, 0, 0},
}

// LongHops are fM's hops over a path of D km (§5.3.1): the fewest of at
// most 4000 km, one more while the elevation from 300 km is under 3°; their
// number, length and elevation. No hops for no length.
//
// Source: ITU-R P.533-14 §5.3.1, eq. (13).
func LongHops(D float64) (n int, d, delta float64) {
	if !(D > 0) {
		return 0, math.NaN(), math.NaN()
	}
	n = int(math.Ceil(D / fmHopKm))
	for ; n < maxLongHops; n++ {
		d = D / float64(n)
		if delta = ElevationAngle(d, LongMirrorKm); delta >= longMinElev {
			break
		}
	}
	return n, D / float64(n), ElevationAngle(D/float64(n), LongMirrorKm)
}

// LongBasicMUF is a control point's basic MUF for a hop of dM km (eqs. 29,
// 30): its zero-distance MUF raised toward its 4000 km MUF by the distance
// reduction factor. No number for no foF2 or no hop.
//
// Source: ITU-R P.533-14 §5.3.1 eqs. (29), (30).
func LongBasicMUF(foF2, m3000, fH, dM float64) float64 {
	if !(foF2 > 0) || !(dM > 0) {
		return math.NaN()
	}
	fd := 0.0
	for _, c := range fdCoefficients {
		fd = (fd + c) * dM
	}
	f4, fz := 1.1*foF2*m3000, foF2+fH/2
	return fz + (f4-fz)*fd
}

// KFactor is eq. (32)'s K from a control point's basic MUF now, at local
// noon and its least in the day, W, X and Y read between Table 3's
// east-west and north-south values by the path's bearing at its centre,
// degrees. No number for a basic MUF not above zero.
//
// Source: ITU-R P.533-14 §5.3.1 eq. (32), Table 3.
func KFactor(fBM, fBMNoon, fBMMin, bearing float64) float64 {
	if !(fBM > 0) || !(fBMNoon > 0) || !(fBMMin > 0) {
		return math.NaN()
	}
	ns := math.Abs(math.Mod(math.Mod(bearing, 180)+180, 180)-90) / 90 // 0 east-west, 1 north-south
	coef := func(i int) float64 { return kTable[0][i] + ns*(kTable[1][i]-kTable[0][i]) }
	return 1.2 + coef(0)*fBM/fBMNoon + coef(1)*(math.Cbrt(fBMNoon/fBM)-1) + coef(2)*math.Pow(fBMMin/fBMNoon, 2)
}

// WinterAnomaly is Aw at a geographic latitude in a month (Table 5): none
// to 30° and at 90°, Table 5's at 60°, linear between. No number for a
// latitude off the globe or no month.
//
// Source: ITU-R P.533-14 §5.3.2, Table 5.
func WinterAnomaly(lat float64, month time.Month) float64 {
	if !(math.Abs(lat) <= 90) || month < time.January || month > time.December {
		return math.NaN()
	}
	hemi := 0
	if lat < 0 {
		hemi = 1
	}
	peak, a := winterAnomaly[hemi][month-1], math.Abs(lat)
	switch {
	case a <= 30:
		return 0
	case a <= 60:
		return peak * (a - 30) / 30
	}
	return peak * (90 - a) / 30
}

// LongLUF is eq. (33)'s LUF, MHz: from the sunspot number, the sum of
// cos^0.5 of the sun's zenith angle at the path's penetration points (none
// where the sun is down), the angle of incidence at 90 km, degrees, the
// slant range, km, the mean gyrofrequency and the winter anomaly. No number
// for an angle of incidence outside 0° to under 90°, or a slant range not
// between zero and 9.5e6 km.
//
// Source: ITU-R P.533-14 §5.3.2 eq. (33).
func LongLUF(r12, sumCosChi, i90, slant, fH, aw float64) float64 {
	if !(i90 >= 0 && i90 < 90) || !(slant > 0) || !(slant < 9.5e6) {
		return math.NaN()
	}
	cosI := math.Cos(i90 * math.Pi / 180)
	return (5.3*math.Sqrt((1+0.009*r12)*sumCosChi/(cosI*math.Log(9.5e6/slant))) - fH) * (aw + 1)
}

// NightLUF is eq. (36)'s night LUF, MHz, for a path of D km.
//
// Source: ITU-R P.533-14 §5.3.2 eq. (36).
func NightLUF(D float64) float64 {
	if !(D > 0) {
		return math.NaN()
	}
	return math.Sqrt(D / 3000)
}

// LUFDecay is the day's LUF by UTC hour after the night LUF and the decay
// from day to night (eqs. 36 to 38): each hour at least the night LUF; where
// the LUF falls through twice the night LUF, that hour and the next three
// recomputed as a decay, kept where larger.
//
// Source: ITU-R P.533-14 §5.3.2 eqs. (36) to (38).
func LUFDecay(fl [24]float64, fLN float64) [24]float64 {
	for h := range fl {
		fl[h] = math.Max(fl[h], fLN)
	}
	out := fl
	k := math.Exp(-nightDecay)
	for h := range fl {
		prev := fl[(h+23)%24]
		falls := prev > 2*fLN && fl[h] < 2*fLN
		if !falls {
			continue
		}
		dt := (2*fLN - fl[h]) / (prev - fl[h])
		v := k * prev * (dt*(1-k) + k)
		for n := range 4 {
			i := (h + n) % 24
			out[i] = math.Max(out[i], v)
			v *= k
		}
	}
	return out
}

// LongField is a path's median field strength past 7000 km, dB(1 µV/m)
// (eqs. 39 to 41): the free-space field of 3 MW e.i.r.p. over the slant
// range, shaped between the LUF and the operational MUF, for the reference
// circuit's power and gain, with the focusing gain (at most 15 dB) and Ly.
// No number for a slant range not above zero, or no MUF or LUF.
//
// Source: ITU-R P.533-14 §5.3.3 eqs. (39) to (41).
func LongField(f, fM, fL, fH, slant, D float64) float64 {
	if !(slant > 0) || !(fM > 0) || !(fL > 0) || !(D > 0) {
		return math.NaN()
	}
	e0 := 139.6 - 20*math.Log10(slant)
	m, l, w := (fM+fH)*(fM+fH), (fL+fH)*(fL+fH), (f+fH)*(f+fH)
	gap := math.Min(10*math.Log10(D/(earthKm*math.Abs(math.Sin(D/earthKm)))), maxGapDB)
	return e0*(1-m/(m+l)*(l/w+w/m)) - 30 + TxPowerDBkW + AntennaDBi + gap - lyDB
}

// BlendField is eq. (42): from 7000 to 9000 km the field interpolated
// between §5.2's Es and §5.3's El, dB(1 µV/m).
//
// Source: ITU-R P.533-14 §5.4 eq. (42).
func BlendField(D, es, el float64) float64 {
	xs, xl := math.Pow(10, 0.01*es), math.Pow(10, 0.01*el)
	return 100 * math.Log10(xs+(D-blendFromKm)/blendSpanKm*(xl-xs))
}
