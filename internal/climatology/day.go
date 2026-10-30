package climatology

import (
	"math"
	"time"
)

// Values is foF2 and M(3000)F2 at a moment, a quasi-dipole latitude in degrees
// and a magnetic local time in hours, for a solar flux F10.7: the two
// mid-month tables either side of the date blended by the day, then each
// parameter interpolated linearly in its solar index - IG12 for foF2, R12 for
// M(3000)F2 - converted from F10.7, past index 100 as well (R-7.3, A-29).
// Zeros for a flux or place that is no finite number.
//
// Source: PyIRI 0.1.7 (MIT), sh_library.IRI_density_1day,
// main_library.day_of_the_month_corr, solar_interpolate, F107_2_R12 and
// R12_2_IG12 (the post-2015 sunspot series); Forsythe et al. 2024.
func Values(t Tables, at time.Time, f107, qdlat, mlt float64) (foF2, m3000 float64) {
	if t == nil {
		return 0, 0
	}
	finite := !math.IsNaN(f107) && !math.IsInf(f107, 0)
	if !finite {
		return 0, 0
	}
	at = at.UTC()
	ut := float64(at.Hour()) + float64(at.Minute())/60 + float64(at.Second())/3600
	before, after, w1, w2 := monthBlend(at)
	blend := func(c func(int, Level) Coefficients, level Level) float64 {
		return Monthly(c(int(before.Month()), level), ut, qdlat, mlt)*w1 + Monthly(c(int(after.Month()), level), ut, qdlat, mlt)*w2
	}
	r12 := r12From(f107)
	foF2 = interpolate(blend(t.FoF2, Min), blend(t.FoF2, Max), ig12From(r12))
	m3000 = interpolate(blend(t.M3000, Min), blend(t.M3000, Max), r12)
	return foF2, m3000
}

// monthBlend is the two months' middles either side of a date, and the
// weight of each: the 15th of the month and the 15th 30 days before or after,
// weighted by whole days, as PyIRI's day_of_the_month_corr does.
func monthBlend(at time.Time) (before, after time.Time, w1, w2 float64) {
	event := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	mid := time.Date(at.Year(), at.Month(), 15, 0, 0, 0, 0, time.UTC)
	if at.Day() >= 15 {
		next := mid.AddDate(0, 0, 30)
		before, after = mid, time.Date(next.Year(), next.Month(), 15, 0, 0, 0, 0, time.UTC)
	} else {
		prev := mid.AddDate(0, 0, -30)
		before, after = time.Date(prev.Year(), prev.Month(), 15, 0, 0, 0, 0, time.UTC), mid
	}
	span := days(after.Sub(before))
	return before, after, days(after.Sub(event)) / span, days(event.Sub(before)) / span
}

// days is a duration in whole days.
func days(d time.Duration) float64 { return math.Floor(d.Hours() / 24) }

// r12From is the sunspot number R12 for a flux F10.7: the root of
// F10.7 = 63.75 + 0.728 R12 + 8.9e-4 R12².
func r12From(f107 float64) float64 {
	const a, b = 8.9e-4, 0.728
	disc := b*b - 4*a*(63.75-f107)
	if disc < 0 {
		return -b / (2 * a) // below any flux the relation reaches: its least R12
	}
	return (-b + math.Sqrt(disc)) / (2 * a)
}

// ig12From is the ionospheric index IG12 for a sunspot number R12, on the
// post-2015 sunspot series.
func ig12From(r12 float64) float64 { return -11.5634 + 1.5332*r12 - 0.0031*r12*r12 }

// interpolate is a parameter at a solar index from its values at 0 and 100,
// linear, past either end as well.
func interpolate(min, max, index float64) float64 {
	return min*(100-index)/100 + max*index/100
}
