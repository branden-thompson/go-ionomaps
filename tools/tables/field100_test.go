package main

import (
	"math"
	"testing"
	"time"
)

// field100Golden is IGRF-14 at 100 km over the geodetic place, from
// ppigrf 2.1.0 (igrf, geodetic; 2026-01-01): the dip, positive downward,
// degrees, and the field's strength, nT.
var field100Golden = []struct{ lat, lon, dip, nT float64 }{
	{45.1, -83.6, 70.4382, 51350.26},
	{40.0, -105.0, 66.0694, 48731.37},
	{-35.3, 149.0, -65.9686, 55245.29},
	{0.0, -75.0, 19.3251, 26705.54},
	{70.0, 20.0, 78.6229, 51778.79},
	{-60.0, 140.0, -86.3003, 63488.53},
	{12.0, 77.0, 12.3327, 39303.36},
	{89.0, -179.0, 88.8238, 54701.76},
}

// TestTheDipAt100kmIsIGRFs is G6.2's inputs (P.533-14 §5.2.2: fL = fH sin I
// at 100 km; p by the modified dip at 100 km): the dip against the local
// geodetic vertical and the field's strength at 100 km agree with ppigrf's
// IGRF-14 to 0.01° and 1 nT.
func TestTheDipAt100kmIsIGRFs(t *testing.T) {
	model, err := loadIGRF("igrf/igrf14coeffs.txt")
	if err != nil {
		t.Fatal(err)
	}
	md := model.at(decimalYear(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	for _, g := range field100Golden {
		dip, nT := dipAndStrength(md, g.lat, g.lon, 100)
		if math.Abs(dip-g.dip) > 0.01 || math.Abs(nT-g.nT) > 1 {
			t.Errorf("%v, %v: dip %.4f°, %.2f nT; ppigrf %.4f°, %.2f nT", g.lat, g.lon, dip, nT, g.dip, g.nT)
		}
	}
}

// TestTheField100TableRoundTrips is the table the library embeds: each
// year's gyrofrequency and dip at 100 km at each cell centre, the
// gyrofrequency between 0.4 and 1.9 MHz and the dip within ±90°, written and
// read back whole; a short table refused.
func TestTheField100TableRoundTrips(t *testing.T) {
	model, err := loadIGRF("igrf/igrf14coeffs.txt")
	if err != nil {
		t.Fatal(err)
	}
	years := []int{2026, 2027}
	fh, dip := generateField100(model, years)
	if len(fh) != 2 || len(dip[1]) != coordCells {
		t.Fatalf("%d years, %d cells", len(fh), len(dip[1]))
	}
	md := model.at(decimalYear(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	for _, i := range []int{0, 4500, 8189, coordCells - 1} {
		glat, glon := 89-2*float64(i/coordCols), -179+2*float64(i%coordCols)
		d, nT := dipAndStrength(md, glat, glon, 100)
		if math.Abs(float64(dip[0][i])-d) > 1e-4 || math.Abs(float64(fh[0][i])-gyroPerNanotesla*nT) > 1e-6 {
			t.Errorf("cell %d: dip %v, fH %v; the field gives %v, %v", i, dip[0][i], fh[0][i], d, gyroPerNanotesla*nT)
		}
	}
	for i := range fh[1] {
		if !(fh[1][i] > 0.4 && fh[1][i] < 1.9) || !(dip[1][i] >= -90 && dip[1][i] <= 90) {
			t.Fatalf("cell %d: fH %v MHz, dip %v°", i, fh[1][i], dip[1][i])
		}
	}
	b := writeField100(years, fh, dip)
	backFH, backDip, gotYears, err := readField100(b)
	if err != nil || len(gotYears) != 2 || gotYears[1] != 2027 || backFH[1][4500] != fh[1][4500] || backDip[0][8189] != dip[0][8189] {
		t.Errorf("read back: %v years, %v", gotYears, err)
	}
	if _, _, _, err := readField100(b[:100]); err == nil {
		t.Error("a short table was read")
	}
}
