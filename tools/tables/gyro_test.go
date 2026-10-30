package main

import (
	"math"
	"testing"
	"time"
)

// TestTheGyrofrequencyTableRoundTrips is G6.1's fH (P.533-14 §3.5, the
// electron gyrofrequency at 300 km): each cell is the field's strength there
// times the electron's gyro constant, from the evaluator held to IGRF-14
// (TestIGRFMatchesItsPublishedValues); every cell between 0.4 and 1.8 MHz,
// as the Earth's field at 300 km allows; written and read back whole.
func TestTheGyrofrequencyTableRoundTrips(t *testing.T) {
	model, err := loadIGRF("igrf/igrf14coeffs.txt")
	if err != nil {
		t.Fatal(err)
	}
	years := []int{2026, 2027}
	tab := generateGyro(model, years)
	if len(tab) != 2 || len(tab[0]) != coordCells {
		t.Fatalf("%d years of %d cells", len(tab), len(tab[0]))
	}
	md := model.at(decimalYear(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	for _, i := range []int{0, 4500, 8189, coordCells - 1} {
		glat, glon := 89-2*float64(i/coordCols), -179+2*float64(i%coordCols)
		want := gyroPerNanotesla * norm(md.fieldXYZ(geocentric(glat, glon, 300)))
		if got := float64(tab[0][i]); math.Abs(got-want) > 1e-6 {
			t.Errorf("cell %d (%v°, %v°): fH %.6f MHz; the field gives %.6f", i, glat, glon, got, want)
		}
	}
	for _, v := range tab[1] {
		if !(v > 0.4 && v < 1.8) {
			t.Fatalf("a cell's fH is %v MHz", v)
		}
	}
	back, gotYears, err := readGyro(writeGyro(years, tab))
	if err != nil || len(gotYears) != 2 || gotYears[1] != 2027 || back[1][4500] != tab[1][4500] {
		t.Errorf("read back %v years %v: %v", len(back), gotYears, err)
	}
	if _, _, err := readGyro(writeGyro(years, tab)[:100]); err == nil {
		t.Error("a short table was read")
	}
}
