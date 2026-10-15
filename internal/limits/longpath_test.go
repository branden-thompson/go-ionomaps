package limits

import (
	"math"
	"testing"
	"time"
)

// TestTheLongPathFollowsP533 is §5.3 and §5.4 (watchpost D-191) against the
// equations as read from P.533-14, test values worked apart from the code:
// fM's hops (13), the basic MUF (29, 30), K (32) with Table 3 by bearing,
// the LUF (33), the field (39 to 41) and the blend (42).
func TestTheLongPathFollowsP533(t *testing.T) {
	for _, c := range []struct {
		D           float64
		n           int
		d, delta, p float64
	}{{7500, 3, 2500, 7.534635, 7870.833542}, {10000, 4, 2500, 7.534635, 10494.444722}, {15000, 5, 3000, 4.261476, 15604.761574}, {12500, 4, 3125, 3.546225, 12982.647557}} {
		n, d, delta := LongHops(c.D)
		if n != c.n || math.Abs(d-c.d) > 1e-6 || math.Abs(delta-c.delta) > 1e-5 || math.Abs(SlantRange(n, d, delta)-c.p) > 1e-4 {
			t.Errorf("%v km: %d hops of %v at %v°; want %d of %v at %v", c.D, n, d, delta, c.n, c.d, c.delta)
		}
	}
	if got := LongBasicMUF(9, 3.2, 1.2, 3500); math.Abs(got-31.069099332) > 1e-6 {
		t.Errorf("fBM %v; want 31.069099332", got)
	}
	for _, c := range []struct{ bearing, want float64 }{{90, 1.508523616}, {270, 1.508523616}, {0, 1.479198381}, {180, 1.479198381}, {45, 1.493860998}} {
		if got := KFactor(20, 24, 12, c.bearing); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("K at bearing %v: %v; want %v", c.bearing, got, c.want)
		}
	}
	if got := LongLUF(100, 3.4, IncidenceAngle(ElevationAngle(2500, 300), 90), 10494.444722, 1.1, 0.15); math.Abs(got-11.670047926) > 1e-5 {
		t.Errorf("fL %v; want 11.670047926", got)
	}
	for _, c := range []struct{ f, want float64 }{{14, -9.347589492}, {6, -37.902073007}, {25, -37.902073007}} {
		if got := LongField(c.f, 25, 6, 1.1, 10494.444722, 10000); math.Abs(got-c.want) > 1e-5 {
			t.Errorf("El at %v MHz: %v; want %v", c.f, got, c.want)
		}
	}
	if got := LongField(14, 25, 6, 1.1, 20000, 19800); math.Abs(got-0.991827274) > 1e-6 { // near the antipode the focusing gain, 19.6 dB, is held to 15
		t.Errorf("El at 19800 km: %v; want 0.991827274", got)
	}
	for _, c := range []struct{ D, want float64 }{{7000, 20}, {8000, 15.287189538}, {9000, 10}} {
		if got := BlendField(c.D, 20, 10); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("Eti at %v km: %v; want %v", c.D, got, c.want)
		}
	}
}

// TestTheWinterAnomalyIsTable5 is §5.3.2's Aw: none to 30° and at 90°,
// Table 5's as the recommendation prints it at 60° (northern September
// 0.01, where ITUHFProp holds 0.00), linear between.
func TestTheWinterAnomalyIsTable5(t *testing.T) {
	for _, c := range []struct {
		lat  float64
		m    time.Month
		want float64
	}{{60, time.January, 0.30}, {-60, time.June, 0.30}, {60, time.September, 0.01}, {45, time.January, 0.15}, {75, time.January, 0.15}, {20, time.January, 0}, {90, time.January, 0}, {-60, time.January, 0}} {
		if got := WinterAnomaly(c.lat, c.m); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("Aw at %v° in %v: %v; want %v", c.lat, c.m, got, c.want)
		}
	}
}

// TestTheNightLUFDecays is eqs. (36) to (38): every hour at least the
// night LUF; where the LUF falls through twice it, that hour and the next
// three decay by e^-0.23 from the hour before, kept where larger.
func TestTheNightLUFDecays(t *testing.T) {
	if got := NightLUF(12000); got != 2 {
		t.Errorf("the night LUF over 12000 km is %v; want 2", got)
	}
	var fl [24]float64
	for h := range fl {
		fl[h] = 1 // under the night LUF
	}
	fl[10], fl[11] = 9, 3 // falls through 4 at hour 11
	out := LUFDecay(fl, 2)
	k := math.Exp(-0.23)
	dt := (4.0 - 3.0) / (9.0 - 3.0)
	v := k * 9 * (dt*(1-k) + k) // eq. (37) at hour 11, from hour 10's 9
	if out[0] != 2 || out[10] != 9 || math.Abs(out[11]-v) > 1e-12 || math.Abs(out[14]-v*k*k*k) > 1e-12 || out[15] != 2 {
		t.Errorf("decayed %v; want 2 to the night LUF, 9 kept, %v at hour 11 then times e^-0.23 for three hours, then 2", out, v)
	}
	fl[11] = 8 // still over 4 at hour 11: the fall through it, and the decay, start at hour 12
	w := k * 8 * ((4.0-2.0)/(8.0-2.0)*(1-k) + k)
	if got := LUFDecay(fl, 2); got[11] != 8 || math.Abs(got[12]-w) > 1e-12 {
		t.Errorf("the fall at hour 12: %v; want 8 kept at 11, %v at 12", got, w)
	}
}

// TestTheLongPathRefusesBadInput is R-3.3.
func TestTheLongPathRefusesBadInput(t *testing.T) {
	n, d, _ := LongHops(0)
	for name, v := range map[string]float64{
		"no path": d, "fBM of no foF2": LongBasicMUF(0, 3, 1, 3000), "fBM of no hop": LongBasicMUF(9, 3, 1, 0),
		"K of no fBM": KFactor(0, 20, 10, 90), "K of no noon": KFactor(20, 0, 10, 90),
		"Aw off the globe": WinterAnomaly(91, time.May), "Aw in no month": WinterAnomaly(60, 0),
		"fL past the horizon": LongLUF(100, 3, 90, 10000, 1, 0), "fL of no slant": LongLUF(100, 3, 60, 0, 1, 0),
		"night LUF of no path": NightLUF(0), "El of no slant": LongField(14, 25, 6, 1, 0, 10000), "El of no LUF": LongField(14, 25, 0, 1, 10000, 10000),
	} {
		if !math.IsNaN(v) {
			t.Errorf("%s: %v", name, v)
		}
	}
	if n != 0 {
		t.Errorf("no path took %d hops", n)
	}
}
