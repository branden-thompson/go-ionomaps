package limits

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTheAbsorptionFactorsAreITUHFProps is D-186 (watchpost): eq. (20)'s
// three factors - ATnoon by latitude and month, the penetration factor phi
// by fv/foE, the diurnal exponent p by latitude, dip and month - agree with
// ITUHFProp's own functions at every golden value its harness wrote
// (testdata/itu-absorption-golden.txt, outside-the-tree harness g62/gen.c).
func TestTheAbsorptionFactorsAreITUHFProps(t *testing.T) {
	f, err := os.Open("testdata/itu-absorption-golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := map[string]int{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		x := strings.Fields(s.Text())
		if len(x) == 0 || x[0][0] == '#' {
			continue
		}
		v := make([]float64, len(x)-1)
		for i := range v {
			v[i], _ = strconv.ParseFloat(x[i+1], 64)
		}
		var got, want float64
		switch x[0] {
		case "AT":
			got, want = atNoon(v[1], time.Month(v[0]+1)), v[2]
		case "PHI":
			got, want = penetration(v[0]), v[1]
		case "P":
			got, want = diurnalExponent(v[1], v[2], time.Month(v[0]+1)), v[3]
		}
		n[x[0]]++
		if math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
			t.Errorf("%s: %.10g; ITUHFProp %.10g", s.Text(), got, want)
		}
	}
	if n["AT"] != 528 || n["PHI"] < 170 || n["P"] != 2772 {
		t.Errorf("golden values read: %v", n)
	}
}

// TestDaytimeAbsorptionClosesTheLowBands is R-2.4's shape (G6.2): one
// penetration point at mid-latitude in October, its term ATnoon times phi
// times F(chi)/F(chi at noon); the loss Li of eq. (20) falls with frequency
// as (f + fL)^-2, rises with the sun's height and with R12, and at night
// (chi past 102°, held there) is a twentieth or less of noon's.
func TestDaytimeAbsorptionClosesTheLowBands(t *testing.T) {
	noon := Penetration{Lat: 40, Dip: 66, Chi: 32, ChiNoon: 32, FoE: 3.2}
	term := AbsorptionTerm(noon, time.October, 3.6*math.Cos(20*math.Pi/180))
	want := atNoon(40, time.October) * penetration(3.6*math.Cos(20*math.Pi/180)/3.2)
	if math.Abs(term-want) > 1e-9 {
		t.Errorf("at noon the term is %v; ATnoon times phi is %v", term, want)
	}
	night := noon
	night.Chi, night.FoE = 130, 0.5
	pe := diurnalExponent(40, 66, time.October)
	atNight := atNoon(40, time.October) * penetration(3.4/0.5) * 0.02 / math.Pow(math.Cos(0.881*32*math.Pi/180), pe) // F at 102°, under 0.02, held there
	if got := AbsorptionTerm(night, time.October, 3.4); math.Abs(got-atNight) > 1e-9 || got/term > 0.05 {
		t.Errorf("at night the term is %v (%v of noon's); want %v", got, got/term, atNight)
	}
	cosI := math.Cos(20 * math.Pi / 180)
	li80 := AbsorptionLoss(1, 50, 3.6, 1.2, cosI, term)
	li20 := AbsorptionLoss(1, 50, 14.0, 1.2, cosI, term)
	if want := 1 * (1 + 0.0067*50) * term / (4.8 * 4.8 * cosI); math.Abs(li80-want) > 1e-9 {
		t.Errorf("Li at 3.6 MHz %v; eq. (20) gives %v", li80, want)
	}
	if math.Abs(li80/li20-(15.2*15.2)/(4.8*4.8)) > 1e-9 || AbsorptionLoss(2, 50, 3.6, 1.2, cosI, term) != 2*li80 {
		t.Errorf("Li does not scale as (f+fL)^-2 and with the hops: %v, %v", li80, li20)
	}
	if AbsorptionLoss(1, 150, 3.6, 1.2, cosI, term) <= li80 {
		t.Error("Li does not rise with R12")
	}
}

// TestAbsorptionRefusesBadInput is R-3.3: a factor, term or loss with no
// meaning - foE not above zero, a point off the globe or a dip past 90°, no
// month, no hop, a ray at or past the horizon, a frequency not above zero,
// a sunspot number below zero, inputs that are no number - is no number,
// never a value.
func TestAbsorptionRefusesBadInput(t *testing.T) {
	ok := Penetration{Lat: 40, Dip: 66, Chi: 32, ChiNoon: 32, FoE: 3.2}
	bad := []Penetration{{Lat: 40, Dip: 66, Chi: 32, ChiNoon: 32}, {Lat: 91, Dip: 66, FoE: 3}, {Lat: math.NaN(), FoE: 3}, {Lat: 40, Dip: 91, FoE: 3}}
	for _, p := range bad {
		if v := AbsorptionTerm(p, time.May, 3); !math.IsNaN(v) {
			t.Errorf("%+v gave a term of %v", p, v)
		}
	}
	if v := AbsorptionTerm(ok, 13, 3); !math.IsNaN(v) {
		t.Errorf("month 13 gave %v", v)
	}
	for name, v := range map[string]float64{
		"ATnoon off the globe": atNoon(95, time.May), "ATnoon at no latitude": atNoon(math.NaN(), time.May), "ATnoon in month 0": atNoon(40, 0), "ATnoon in month 13": atNoon(40, 13),
		"phi of no ratio": penetration(math.NaN()),
		"p off the globe": diurnalExponent(95, 10, time.May), "p at a dip past 90": diurnalExponent(40, 95, time.May), "p in month 0": diurnalExponent(40, 10, 0),
		"Li at R12 -1": AbsorptionLoss(1, -1, 3.6, 1.2, 0.9, 100), "Li at no R12": AbsorptionLoss(1, math.NaN(), 3.6, 1.2, 0.9, 100), "Li at no fL": AbsorptionLoss(1, 50, 3.6, math.NaN(), 0.9, 100), "Li at fL -1": AbsorptionLoss(1, 50, 3.6, -1, 0.9, 100),
	} {
		if !math.IsNaN(v) {
			t.Errorf("%s: %v", name, v)
		}
	}
	for _, c := range [][6]float64{{0, 50, 3.6, 1.2, 0.9, 100}, {1, 50, 3.6, 1.2, 0, 100}, {1, 50, 0, 1.2, 0.9, 100}, {1, 50, 3.6, 1.2, 0.9, math.NaN()}, {1, 50, 3.6, 1.2, 0.9, math.Inf(1)}} {
		if v := AbsorptionLoss(int(c[0]), c[1], c[2], c[3], c[4], c[5]); !math.IsNaN(v) {
			t.Errorf("%v gave a loss of %v", c, v)
		}
	}
}
