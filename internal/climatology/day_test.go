package climatology

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTheClimatologyTakesF107 is R-7.3 and R-9.1: the day's foF2 and
// M(3000)F2 - the two mid-month tables either side of the date blended by
// the day, each parameter interpolated in its solar index from F10.7 - agree
// with PyIRI's own (testdata, from tools/export), at fluxes of 65, 120 and
// 250 and on dates across a month's middle and a year's end.
func TestTheClimatologyTakesF107(t *testing.T) {
	rows, worst := dayGolden(t, func(float64) bool { return true })
	if rows < 100 {
		t.Fatalf("%d golden rows read", rows)
	}
	t.Logf("%d golden rows, worst difference %.3g", rows, worst)
}

// TestHighFluxFollowsPyIRI is R-7.3 (A-29): at a flux of 250, past the
// tables' solar index of 100, the climatology extrapolates as PyIRI does,
// linearly in the index, with no saturation of its own.
func TestHighFluxFollowsPyIRI(t *testing.T) {
	if rows, _ := dayGolden(t, func(f107 float64) bool { return f107 > 200 }); rows < 30 {
		t.Fatalf("%d high-flux rows", rows)
	}
}

// dayGolden checks the golden's rows whose flux keep accepts: their count,
// and the worst difference.
func dayGolden(t *testing.T, keep func(f107 float64) bool) (int, float64) {
	t.Helper()
	f, err := os.Open("testdata/pyiri-golden-day.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read only
	refits := Refits()
	rows, worst := 0, 0.0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		var v [9]float64
		for i, field := range strings.Fields(line) {
			if i < 9 {
				if v[i], err = strconv.ParseFloat(field, 64); err != nil {
					t.Fatal(err)
				}
			}
		}
		if !keep(v[3]) {
			continue
		}
		at := time.Date(int(v[0]), time.Month(v[1]), int(v[2]), 0, 0, 0, 0, time.UTC).Add(time.Duration(v[4] * float64(time.Hour)))
		fo, m3 := Values(refits, at, v[3], v[5], v[6])
		for i, d := range []float64{math.Abs(fo - v[7]), math.Abs(m3 - v[8])} {
			worst = max(worst, d)
			if d > goldenTolerance {
				t.Errorf("%v, F10.7 %v, QD %v, MLT %v: value %d differs from PyIRI's by %.3g", at, v[3], v[5], v[6], i, d)
			}
		}
		rows++
	}
	return rows, worst
}
