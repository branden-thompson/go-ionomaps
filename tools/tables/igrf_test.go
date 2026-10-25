package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// igrfTolerance is how far the evaluation may stand from ppigrf's, in nT:
// ppigrf interpolates the coefficients in calendar time between epochs,
// IAGA's definition (followed here) in decimal years. Measured at 0.081 nT
// at most, between epochs; nothing else differs.
const igrfTolerance = 0.1

// TestIGRFMatchesItsPublishedValues is plan task G3.5: IGRF-14 evaluated from
// IAGA's own coefficient file (igrf/igrf14coeffs.txt) agrees with ppigrf's
// (MIT) field at eight places, from the surface to 7,500 km, on dates in the
// definitive models, the current one and its secular variation.
func TestIGRFMatchesItsPublishedValues(t *testing.T) {
	model, err := loadIGRF("igrf/igrf14coeffs.txt")
	if err != nil {
		t.Fatal(err)
	}
	if model.degree != 13 || model.epochs[0] != 1900 || model.epochs[len(model.epochs)-1] != 2025 {
		t.Fatalf("model of degree %d, epochs %v to %v", model.degree, model.epochs[0], model.epochs[len(model.epochs)-1])
	}
	f, err := os.Open("igrf/golden-igrf.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read only
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
		year := decimalYear(time.Date(int(v[3]), time.Month(v[4]), int(v[5]), 0, 0, 0, 0, time.UTC))
		br, bt, bp := model.at(year).field(v[0], v[1]*math.Pi/180, v[2]*math.Pi/180)
		for i, got := range []float64{br, bt, bp} {
			d := math.Abs(got - v[6+i])
			worst = max(worst, d)
			if d > igrfTolerance {
				t.Errorf("r %v, colat %v, lon %v, %v: component %d is %.3f nT; ppigrf gives %.3f", v[0], v[1], v[2], year, i, got, v[6+i])
			}
		}
		rows++
	}
	if rows < 30 {
		t.Fatalf("%d golden rows", rows)
	}
	t.Logf("%d golden rows, worst difference %.3g nT", rows, worst)
}

// TestTheMagneticModelIsNotNearItsEnd is watchpost D-107: IGRF-14's secular
// variation runs to 2030; this fails a year before, so the tables are
// regenerated from IGRF-15 before the library's coordinates are extrapolated.
func TestTheMagneticModelIsNotNearItsEnd(t *testing.T) {
	model, err := loadIGRF("igrf/igrf14coeffs.txt")
	if err != nil {
		t.Fatal(err)
	}
	if end := model.epochs[len(model.epochs)-1] + 5; decimalYear(time.Now()) > end-1 {
		t.Errorf("IGRF's last model runs to %v and it is within a year of it: regenerate the magnetic tables from the next IGRF (D-107)", end)
	}
}

// TestAnInputPastItsBoundIsRefused is RC-12 (watchpost D-268): the tables'
// readers stop at a line bound and refuse what runs past it - an IGRF table
// longer than IAGA's and an export longer than its lines can be - so neither
// loop is bounded by its input alone; at the bound itself nothing is refused
// for its length.
func TestAnInputPastItsBoundIsRefused(t *testing.T) {
	var m igrf
	if _, err := m.readRows(bufio.NewScanner(strings.NewReader(strings.Repeat("#\n", maxIGRFLines+1)))); err == nil {
		t.Error("an IGRF table a line past the bound was read")
	}
	var at igrf
	if _, err := at.readRows(bufio.NewScanner(strings.NewReader(strings.Repeat("#\n", maxIGRFLines)))); err != nil {
		t.Errorf("an IGRF table at the bound was refused for its length: %v", err)
	}
	// A shape the lines inside the bound fill exactly, then one line past it,
	// so only the bound can refuse it.
	long := fmt.Sprintf("# shape: %d 1 1 1\n", maxExportLines-1) + strings.Repeat("1\n", maxExportLines-1) + "# past the bound\n"
	if _, err := parseExport(strings.NewReader(long)); err == nil {
		t.Error("an export a line past the bound was read")
	}
}
