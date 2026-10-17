package limits

import (
	"math"
	"testing"
)

// close6 is whether two values agree to a millionth, absolutely or in
// proportion.
func close6(a, b float64) bool { return math.Abs(a-b) <= 1e-6*math.Max(1, math.Abs(b)) }

// TestTheGeometryFollowsP533 is G6.2b against the equations as read from
// P.533-14 (02-analysis/evidence/p533-field-strength.md), test values worked
// apart from the code: the elevation angle (13), the angle of incidence
// (12), the slant range (19), and where a mode's hops cross 90 km.
func TestTheGeometryFollowsP533(t *testing.T) {
	for _, c := range []struct{ d, hr, want float64 }{{1000, 300, 28.117628}, {3000, 110, -2.605661}, {200, 300, 70.711293}} {
		if got := ElevationAngle(c.d, c.hr); math.Abs(got-c.want) > 1e-5 {
			t.Errorf("elevation over %v km from %v km: %.6f°; want %.6f", c.d, c.hr, got, c.want)
		}
	}
	if got := IncidenceAngle(30, 110); math.Abs(got-58.356255) > 1e-5 {
		t.Errorf("incidence at 110 km for 30°: %.6f; want 58.356255", got)
	}
	delta := ElevationAngle(1500, 300)
	if got := SlantRange(2, 1500, delta); math.Abs(got-3294.3766) > 1e-3 {
		t.Errorf("slant range of two 1500 km hops: %.4f km; want 3294.3766", got)
	}
	fr := PenetrationFractions(3000, 2, delta)
	if len(fr) != 4 || !(fr[0] > 0 && fr[0] < 0.25) || math.Abs(fr[0]+fr[3]-1) > 1e-12 || math.Abs(fr[1]+fr[2]-1) > 1e-12 || !(fr[1] < 0.5) {
		t.Errorf("two hops cross 90 km at %v; four points, two a hop, mirrored about the middle", fr)
	}
}

// TestTheMirrorHeightFollowsP533 is §5.1's F2 mirror-reflection height,
// eqs. (14) to (16), each case at test values worked apart from the code;
// held to 800 km.
func TestTheMirrorHeightFollowsP533(t *testing.T) {
	for _, c := range []struct{ foF2, foE, m3000, r12, f, d, want float64 }{
		{9, 2.5, 3.1, 50, 10, 2000, 191.114391},   // (14): x past 3.33, f above foF2
		{9, 2.5, 3.1, 50, 9, 500, 250.630205},     // (14) at f = foF2
		{12, 3.2, 2.8, 120, 14, 3000, 213.304354}, // (14)
		{9, 2.5, 3.1, 50, 4, 1000, 175.187903},    // (15): f below foF2
		{7, 3.0, 3.0, 50, 4, 1000, 331.276322},    // (16): x at or under 3.33
	} {
		if got := MirrorHeight(c.foF2, c.foE, c.m3000, c.r12, c.f, c.d); math.Abs(got-c.want) > 1e-5 {
			t.Errorf("%+v: %.6f km; want %.6f", c, got, c.want)
		}
	}
	if got := MirrorHeight(7, 3.0, 1.2, 50, 4, 3000); got != 800 {
		t.Errorf("a low M(3000)F2 gives %v km; held to 800", got)
	}
}

// TestTheLossesFollowP533 is §4 and §5.2.2: the screening frequency (11),
// the above-the-MUF loss (24) to (26) with its caps, the ground loss (27),
// a mode's field (17), (18) and the modes summed (28), (43), (44).
func TestTheLossesFollowP533(t *testing.T) {
	if got := ScreeningFrequency(3, 20); math.Abs(got-8.224293) > 1e-5 {
		t.Errorf("fs for foE 3 at 20°: %.6f; want 8.224293", got)
	}
	for _, c := range []struct {
		e         bool
		f, fb, lm float64
	}{{true, 12, 10, 5.2}, {false, 14.4, 10, 23.879698}, {true, 30, 10, 81}, {false, 300, 10, 62}, {true, 9, 10, 0}, {false, 10, 10, 0}} {
		if got := AboveMUFLoss(c.e, c.f, c.fb); math.Abs(got-c.lm) > 1e-5 {
			t.Errorf("Lm (E %v) at %v over %v: %v; want %v", c.e, c.f, c.fb, got, c.lm)
		}
	}
	ew := ModeField(10, 2000, 5, 0, 2, 0)
	want := 136.6 - 10 + 0 + 20 - (32.45 + 20 + 20*math.Log10(2000) + 5 + 0 + 2 + 0 + 8.72)
	if !close6(ew, want) {
		t.Errorf("a two-hop mode's field: %v dB(1 µV/m); eqs. (17), (18) give %v", ew, want)
	}
	pr := PowerFromField(10, SumFields([]float64{ew, ew}))
	if !close6(pr, ew+10*math.Log10(2)-20-107.2) {
		t.Errorf("two equal modes received at %v dBW; their power summed, %v", pr, ew+10*math.Log10(2)-20-107.2)
	}
	if !math.IsInf(PowerFromField(10, SumFields(nil)), -1) {
		t.Error("no mode was received as power")
	}
}

// TestTheNoiseIsP372s is D-187: P.372-18's man-made noise by environment at
// 3 MHz, galactic noise added in power where it is heard; the S/N of eq.
// (45) in the reference circuit's 2.5 kHz.
func TestTheNoiseIsP372s(t *testing.T) {
	for env, want := range map[Environment]float64{City: 63.5837, Residential: 59.2837, Rural: 53.9837, QuietRural: 39.9543} {
		if got := NoiseFactor(env, 3, false); math.Abs(got-want) > 1e-4 {
			t.Errorf("%v at 3 MHz: Fa %.4f; want %.4f", env, got, want)
		}
	}
	quiet, heard := NoiseFactor(QuietRural, 14, false), NoiseFactor(QuietRural, 14, true)
	gal := 52 - 23*math.Log10(14)
	if want := 10 * math.Log10(math.Pow(10, quiet/10)+math.Pow(10, gal/10)); !close6(heard, want) {
		t.Errorf("quiet rural at 14 MHz with the galaxy heard: %v; the two in power, %v", heard, want)
	}
	if got := SNR(-120, 50); !close6(got, -120-50-10*math.Log10(2500)+204) {
		t.Errorf("S/N %v", got)
	}
}

// TestR12FromTheFlux is ITU-R P.371-9 eq. (4) inverted: the sunspot number
// whose flux is the one given, none below 63.7.
func TestR12FromTheFlux(t *testing.T) {
	if got := R12FromFlux(150); math.Abs(got-105.052189) > 1e-5 {
		t.Errorf("R12 at a flux of 150: %v; want 105.052189", got)
	}
	if got := R12FromFlux(60); got != 0 {
		t.Errorf("R12 at a flux of 60: %v; held to 0", got)
	}
}

// TestTheBudgetRefusesBadInput is R-3.3: geometry, heights, losses and noise
// with no meaning are no number.
func TestTheBudgetRefusesBadInput(t *testing.T) {
	for name, v := range map[string]float64{
		"no hop": ElevationAngle(0, 300), "no height": ElevationAngle(1000, -1), "a hop past the globe": ElevationAngle(30000, 300),
		"incidence past 90": IncidenceAngle(91, 110), "incidence below ground": IncidenceAngle(30, -1),
		"no hops": SlantRange(0, 1000, 20), "a hop of no length": SlantRange(1, 0, 20),
		"no foF2": MirrorHeight(0, 2.5, 3, 50, 4, 1000), "no foE": MirrorHeight(9, 0, 3, 50, 4, 1000), "no frequency": MirrorHeight(9, 2.5, 3, 50, 0, 1000), "an M(3000)F2 of no use": MirrorHeight(9, 2.5, math.NaN(), 50, 4, 1000),
		"an M(3000)F2 past the method": MirrorHeight(9, 2.5, 9, 50, 4, 1000), "power at no frequency": PowerFromField(0, SumFields([]float64{10})),
		"fs of no foE": ScreeningFrequency(0, 20), "Lm of no basic MUF": AboveMUFLoss(false, 10, 0),
		"a field of no frequency": ModeField(0, 2000, 5, 0, 1, 0), "a field of no hops": ModeField(10, 2000, 5, 0, 0, 0), "a field with no loss": ModeField(10, 2000, math.NaN(), 0, 1, 0),
		"noise of no frequency": NoiseFactor(Residential, 0, false), "noise of no environment": NoiseFactor(9, 3, false),
		"R12 of no flux": R12FromFlux(math.NaN()),
	} {
		if !math.IsNaN(v) {
			t.Errorf("%s: %v", name, v)
		}
	}
	if fr := PenetrationFractions(3000, 0, 20); fr != nil {
		t.Errorf("no hops crossed 90 km at %v", fr)
	}
}
