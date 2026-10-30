package limits

import (
	"math"
	"testing"
)

// TestFoEFollowsP1239 is G6.1's foE (P.533-14 §3.2, from P.1239-4 §4,
// watchpost D-58): the values worked by hand from the recommendation's
// equations as transcribed (02-analysis/evidence/p533-equations.md), by day,
// with the sun low, in twilight and at night, held to 1e-6 MHz.
func TestFoEFollowsP1239(t *testing.T) {
	for _, c := range []struct {
		lat, decl, chi, flux, since float64
		want                        float64
	}{
		{40, 0, 30, 150, 0, 3.722558},    // by day, mid-latitude
		{0, 20, 10, 70, 0, 3.448668},     // by day, the equator: p = 1.31
		{-35, -10, 80, 120, 0, 2.214021}, // the sun low, past 73°
		{50, 20, 100, 150, 3, 0.859573},  // night, three hours after sunset
		{60, -23, 95, 100, 6, 1.177123},  // night, the zenith term the greater
	} {
		if got := FoE(c.lat, c.decl, c.chi, c.flux, c.since); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("foE at %v°, declination %v°, zenith %v°, flux %v, %vh after sunset: %.6f; worked %.6f", c.lat, c.decl, c.chi, c.flux, c.since, got, c.want)
		}
	}
	min := math.Pow(0.004*math.Pow(1+0.021*150, 2), 0.25)
	if got := FoE(50, 20, 175, 150, 12); math.Abs(got-min) > 1e-9 {
		t.Errorf("deep night gives %.6f; the floor is %.6f (equation 18)", got, min)
	}
	for _, bad := range [][4]float64{{math.NaN(), 0, 30, 150}, {91, 0, 30, 150}, {40, 0, math.NaN(), 150}, {40, 0, 30, 0}} {
		if got := FoE(bad[0], bad[1], bad[2], bad[3], 0); !math.IsNaN(got) {
			t.Errorf("foE for %v is %v; want no number", bad, got)
		}
	}
}
