package limits

import (
	"math"
	"testing"
)

// TestPathMUFFollowsP533 is G6.1 (R-2.7, D-58): the F2 basic MUF for a hop,
// held to values worked by hand from P.533-14's equations (3) to (6) as
// transcribed, and to what the method carries - at 3000 km the MUF(3000)
// there, B·foF2 and the gyrofrequency's share; rising with distance to
// dmax, which is held to 4000 km; past it, dmax's.
func TestPathMUFFollowsP533(t *testing.T) {
	for _, c := range []struct {
		d, foF2, m3000, foE, fH float64
		want                    float64
	}{
		{3000, 7, 3.0, 3.2, 1.2, 21.209225},
		{1000, 7, 3.0, 3.2, 1.2, 12.522551},
		{500, 5, 2.8, 2.0, 1.4, 6.632196},
		{2500, 10, 3.3, 3.5, 1.0, 31.696941},
		{3800, 9, 3.1, 3.0, 0.9, 29.133137},
		{5000, 9, 3.1, 3.0, 0.9, 29.192488}, // past dmax: dmax's
	} {
		if got := BasicMUF(c.d, c.foF2, c.m3000, c.foE, c.fH); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("MUF for %v km (foF2 %v, M %v, foE %v, fH %v): %.6f; worked %.6f", c.d, c.foF2, c.m3000, c.foE, c.fH, got, c.want)
		}
	}
	foF2, m, foE, fH := 7.0, 3.0, 3.2, 1.2
	b := m - 0.124 + (m*m-4)*(0.0215+0.005*math.Sin(7.854/math.Max(foF2/foE, 2)-1.9635))
	if got, want := BasicMUF(3000, foF2, m, foE, fH), b*foF2+fH/2*(1-3000.0/4000); math.Abs(got-want) > 1e-9 {
		t.Errorf("at 3000 km the MUF is %.6f; B·foF2 and the gyrofrequency's share give %.6f", got, want)
	}
	last := 0.0
	for d := 200.0; d <= 4000; d += 200 {
		got := BasicMUF(d, foF2, m, foE, fH)
		if got <= last {
			t.Errorf("the MUF falls from %.4f to %.4f at %v km: it rises to dmax", last, got, d)
		}
		last = got
	}
	if !math.IsNaN(BasicMUF(-1, 7, 3, 3, 1)) || !math.IsNaN(BasicMUF(1000, 0, 3, 3, 1)) || !math.IsNaN(BasicMUF(1000, 7, 1.5, 3, 1)) {
		t.Error("a hop of no length, no foF2 or an M(3000)F2 below 2 gave a MUF")
	}
}
