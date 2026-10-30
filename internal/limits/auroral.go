package limits

import (
	"math"
	"time"
)

// The auroral and other signal losses of ITU-R P.533-14 §5.2.2, Lh, from
// its Table 2 (watchpost D-189): ported from ITU-R Study Group 3's ITUHFProp
// (MedianSkywaveFieldStrengthShort.c, FindLh), checked value for value
// against the recommendation's own table.

// lhTable is P.533-14 Table 2, Lh in dB, as ITUHFProp holds it (FindLh) and the
// recommendation prints it, value for value: by transmission range (to 2500 km,
// past it), season (winter, equinox, summer), geomagnetic latitude band (77.5°
// and above, then down by 5° to 42.5° to 47.5°) and mid-path local time (01 to
// 04 hours, then every three, to 22 to 01).
var lhTable = [2][3][8][8]float64{
	{
		{
			{2, 6.6, 6.2, 1.5, 0.5, 1.4, 1.5, 1},
			{3.4, 8.3, 8.6, 0.9, 0.5, 2.5, 3, 3},
			{6.2, 15.6, 12.8, 2.3, 1.5, 4.6, 7, 5},
			{7, 16, 14, 3.6, 2, 6.8, 9.8, 6.6},
			{2, 4.5, 6.6, 1.4, 0.8, 2.7, 3, 2},
			{1.3, 1, 3.2, 0.3, 0.4, 1.8, 2.3, 0.9},
			{0.9, 0.6, 2.2, 0.2, 0.2, 1.2, 1.5, 0.6},
			{0.4, 0.3, 1.1, 0.1, 0.1, 0.6, 0.7, 0.3},
		},
		{
			{1.4, 2.5, 7.4, 3.8, 1, 2.4, 2.4, 3.3},
			{3.3, 11, 11.6, 5.1, 2.6, 4, 6, 7},
			{6.5, 12, 21.4, 8.5, 4.8, 6, 10, 13.7},
			{6.7, 11.2, 17, 9, 7.2, 9, 10.9, 15},
			{2.4, 4.4, 7.5, 5, 2.6, 4.8, 5.5, 6.1},
			{1.7, 2, 5, 3, 2.2, 4, 3, 4},
			{1.1, 1.3, 3.3, 2, 1.4, 2.6, 2, 2.6},
			{0.5, 0.6, 1.6, 1, 0.7, 1.3, 1, 1.3},
		},
		{
			{2.2, 2.7, 1.2, 2.3, 2.2, 3.8, 4.2, 3.8},
			{2.4, 3, 2.8, 3, 2.7, 4.2, 4.8, 4.5},
			{4.9, 4.2, 6.2, 4.5, 3.8, 5.4, 7.7, 7.2},
			{6.5, 4.8, 9, 6, 4.8, 9.1, 9.5, 8.9},
			{3.2, 2.7, 4, 3, 3, 6.5, 6.7, 5},
			{2.5, 1.8, 2.4, 2.3, 2.6, 5, 4.6, 4},
			{1.6, 1.2, 1.6, 1.5, 1.7, 3.3, 3.1, 2.6},
			{0.8, 0.6, 0.8, 0.7, 0.8, 1.6, 1.5, 1.3},
		},
	},
	{
		{
			{1.5, 2.7, 2.5, 0.8, 0, 0.9, 0.8, 1.6},
			{2.5, 4.5, 4.3, 0.8, 0.3, 1.6, 2, 4.8},
			{5.5, 5, 7, 1.9, 0.5, 3, 4.5, 9.6},
			{5.3, 7, 5.9, 2, 0.7, 4, 4.5, 10},
			{1.6, 2.4, 2.7, 0.6, 0.4, 1.7, 1.8, 3.5},
			{0.9, 1, 1.3, 0.1, 0.1, 1, 1.5, 1.4},
			{0.6, 0.6, 0.8, 0.1, 0.1, 0.6, 1, 0.5},
			{0.3, 0.3, 0.4, 0, 0, 0.3, 0.5, 0.4},
		},
		{
			{1, 1.2, 2.7, 3, 0.6, 2, 2.3, 1.6},
			{1.8, 2.9, 4.1, 5.7, 1.5, 3.2, 5.6, 3.6},
			{3.7, 5.6, 7.7, 8.1, 3.5, 5, 9.5, 7.3},
			{3.9, 5.2, 7.6, 9, 5, 7.5, 10, 7.9},
			{1.4, 2, 3.2, 3.8, 1.8, 4, 5.4, 3.4},
			{0.9, 0.9, 1.8, 2, 1.3, 3.1, 2.7, 2},
			{0.6, 0.6, 1.2, 1.3, 0.8, 2, 1.8, 1.3},
			{0.3, 0.3, 0.6, 0.6, 0.4, 1, 0.9, 0.6},
		},
		{
			{1.9, 3.8, 2.2, 1.1, 2.1, 1.2, 2.3, 2.4},
			{1.9, 4.6, 2.9, 1.3, 2.2, 1.3, 2.8, 2.7},
			{4.4, 6.3, 5.9, 1.9, 3.3, 1.7, 4.4, 4.5},
			{5.5, 8.5, 7.6, 2.6, 4.2, 3.2, 5.5, 5.7},
			{2.8, 3.8, 3.7, 1.4, 2.7, 1.6, 4.5, 3.2},
			{2.2, 2.4, 2.2, 1, 2.2, 1.2, 4.4, 2.5},
			{1.4, 1.6, 1.4, 0.6, 1.4, 0.8, 2.9, 1.6},
			{0.7, 0.8, 0.7, 0.3, 0.7, 0.4, 1.4, 0.8},
		},
	},
}

// The geomagnetic dipole P.533 reads Lh by: its north pole, degrees; and
// the latitude below which Lh is nil.
const (
	dipolePoleLat = 78.5
	dipolePoleLon = -68.2
	lhFromGn      = 42.5
	lhLongKm      = 2500.0
)

// GeomagneticLatitude is a place's latitude, degrees, about the Earth-centred
// dipole P.533 names for Lh (pole at 78.5° N, 68.2° W); no number off the
// globe.
//
// Source: ITU-R P.533-14 §5.2.2 (Lh).
func GeomagneticLatitude(lat, lon float64) float64 {
	if !(math.Abs(lat) <= 90) || lon != lon {
		return math.NaN()
	}
	r := math.Pi / 180
	s := math.Sin(lat*r)*math.Sin(dipolePoleLat*r) + math.Cos(lat*r)*math.Cos(dipolePoleLat*r)*math.Cos((lon-dipolePoleLon)*r)
	return math.Asin(math.Max(-1, math.Min(1, s))) / r
}

// AuroralLoss is Lh, dB, at one control point (Table 2): by the path's
// length (to 2500 km, or past it), the season in the point's hemisphere,
// its geomagnetic latitude and the mid-path local time, hours; nil below
// 42.5° geomagnetic. No number for a latitude or time that is no number, or
// no month.
//
// Source: ITU-R P.533-14 §5.2.2 and Table 2.
func AuroralLoss(pathKm, lat, gn float64, month time.Month, localHours float64) float64 {
	if lat != lat || gn != gn || localHours != localHours {
		return math.NaN()
	}
	if month < time.January || month > time.December {
		return math.NaN()
	}
	g := math.Abs(gn)
	if g < lhFromGn {
		return 0
	}
	band := min(7, max(0, int(math.Ceil((77.5-g)/5)))) // 77.5 and above is 0, each 5° below it one more
	long := 0
	if pathKm > lhLongKm {
		long = 1
	}
	t := int(math.Mod(math.Mod(localHours-1, 24)+24, 24) / 3) // 01 to 04 is 0 ... 22 to 01 is 7
	return lhTable[long][lhSeason(lat, month)][band][min(t, 7)]
}

// lhSeason is Table 2's season: winter December to February in the north,
// June to August in the south; summer the other way; equinox otherwise.
func lhSeason(lat float64, month time.Month) int {
	winter, summer := 0, 2
	if lat < 0 {
		winter, summer = 2, 0
	}
	switch month {
	case time.December, time.January, time.February:
		return winter
	case time.June, time.July, time.August:
		return summer
	}
	return 1
}
