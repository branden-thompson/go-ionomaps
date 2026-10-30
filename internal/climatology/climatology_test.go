package climatology

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// goldenTolerance is how far the port may stand from PyIRI's own values, in
// MHz for foF2 and in M(3000)F2's own unit: float64 sums in another order.
// Measured at 3.7e-14 on the golden's 512 values.
const goldenTolerance = 1e-12

// TestTheFallbackMatchesPyIRI is plan task G3.1: the port's monthly foF2 and
// M(3000)F2 at fixed quasi-dipole latitudes, magnetic local times and hours,
// for solar levels 0 and 100, agree with PyIRI 0.1.7's own (testdata, from
// tools/export).
func TestTheFallbackMatchesPyIRI(t *testing.T) {
	f, err := os.Open("testdata/pyiri-golden.txt")
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
		var v [8]float64
		for i, field := range strings.Fields(line) {
			if i < 8 {
				v[i], err = strconv.ParseFloat(field, 64)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		month, ut, qdlat, mlt := int(v[0]), v[1], v[2], v[3]
		for i, got := range []float64{
			Monthly(refits.FoF2(month, Min), ut, qdlat, mlt), Monthly(refits.FoF2(month, Max), ut, qdlat, mlt),
			Monthly(refits.M3000(month, Min), ut, qdlat, mlt), Monthly(refits.M3000(month, Max), ut, qdlat, mlt),
		} {
			d := math.Abs(got - v[4+i])
			worst = max(worst, d)
			if d > goldenTolerance {
				t.Errorf("month %d, %v UT, QD %v, MLT %v: value %d is %.12f; PyIRI gives %.12f", month, ut, qdlat, mlt, i, got, v[4+i])
			}
		}
		rows++
	}
	if rows < 100 {
		t.Fatalf("%d golden rows read", rows)
	}
	t.Logf("%d golden rows, worst difference %.3g", rows, worst)
}

// constant is supplied tables whose every month and level holds one value in
// the first coefficient alone: the mean term, the same everywhere.
type constant float64

func (c constant) coefficients() Coefficients {
	out := make(Coefficients, Terms*Harmonics)
	out[0] = float64(c)
	return out
}
func (c constant) FoF2(int, Level) Coefficients  { return c.coefficients() }
func (c constant) M3000(int, Level) Coefficients { return c.coefficients() }

// TestTheClimatologyRunsOnAnySuppliedTables is plan task G3.4 (R-4.4,
// watchpost D-43): the evaluation reads its tables through the seam alone,
// so other tables give their own values: a constant table gives that
// constant at every place and hour.
func TestTheClimatologyRunsOnAnySuppliedTables(t *testing.T) {
	var tab Tables = constant(7.25)
	for _, p := range [][3]float64{{0, 0, 0}, {13.5, -60, 7}, {23, 85, 18.5}} {
		if got := Monthly(tab.FoF2(6, Max), p[0], p[1], p[2]); math.Abs(got-7.25) > 1e-12 {
			t.Errorf("at %v a constant table of 7.25 gives %v", p, got)
		}
	}
	if got := Monthly(nil, 0, 0, 0); got != 0 {
		t.Errorf("no coefficients give %v; want 0", got)
	}
	for _, p := range [][3]float64{{math.NaN(), 0, 0}, {0, math.NaN(), 0}, {0, 0, math.Inf(1)}, {0, 91, 0}} {
		if got := Monthly(tab.FoF2(6, Max), p[0], p[1], p[2]); got != 0 {
			t.Errorf("at %v, no finite place or time, it gives %v; want 0", p, got)
		}
	}
}
