package main

import (
	"bufio"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The tolerances our quasi-dipole coordinates are held to against PyIRI's
// Apex.nc (watchpost D-107), set from the measured agreement on the sample,
// by Apex.nc's own latitude: worst 0.36° to 80°, 0.50° from 80° to 85° and
// 1.96° beyond; longitude along the parallel, 0.42° anywhere. Apex.nc is a
// degree-20 spherical-harmonic fit of ApexPy's coordinates, which flattens
// toward the quasi-dipole poles, where our traced values run on toward 90°.
const (
	qdLatTolerance      = 0.4 // degrees, where Apex.nc's latitude is within ±80°
	qdLatHighTolerance  = 0.6 // degrees, from 80° to 85°
	qdLatPoleTolerance  = 2.5 // degrees, beyond
	qdLonTolerance      = 0.5 // degrees along the parallel: the longitude difference times cos(latitude)
	qdLatMedianExpected = 0.1 // degrees
)

// TestOurCoordinatesAgreeWithApex is plan task G3.5 (watchpost D-107): our
// quasi-dipole latitude and longitude, traced from IGRF-14 at height 0,
// agree with PyIRI's Apex.nc at the 1,800 points of the committed sample.
func TestOurCoordinatesAgreeWithApex(t *testing.T) {
	m, err := loadIGRF("igrf/igrf14coeffs.txt")
	if err != nil {
		t.Fatal(err)
	}
	md := m.at(decimalYear(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)))
	rows := readSample(t)
	var lats []float64
	for _, r := range rows {
		qd, ql, ok := md.quasiDipole(r.glat, r.glon, 0)
		if !ok {
			t.Fatalf("%v, %v: no apex", r.glat, r.glon)
		}
		dlat := math.Abs(qd - r.qdlat)
		dlon := math.Abs(math.Mod(ql-r.qdlon+540, 360)-180) * math.Cos(r.qdlat*math.Pi/180)
		lats = append(lats, dlat)
		limit := qdLatTolerance
		switch a := math.Abs(r.qdlat); {
		case a > 85:
			limit = qdLatPoleTolerance
		case a > 80:
			limit = qdLatHighTolerance
		}
		if dlat > limit || dlon > qdLonTolerance {
			t.Errorf("%v, %v: ours %.3f, %.3f; Apex.nc %.3f, %.3f", r.glat, r.glon, qd, ql, r.qdlat, r.qdlon)
		}
	}
	sort.Float64s(lats)
	if median := lats[len(lats)/2]; median > qdLatMedianExpected {
		t.Errorf("the median latitude difference is %.3f°", median)
	}
	t.Logf("%d points: latitude difference median %.3f°, 99th percentile %.3f°, worst %.3f°", len(lats), lats[len(lats)/2], lats[len(lats)*99/100], lats[len(lats)-1])
}

// sampleRow is one point of the Apex.nc sample.
type sampleRow struct{ glat, glon, qdlat, qdlon, mlt0, mlt12 float64 }

func readSample(t *testing.T) []sampleRow {
	t.Helper()
	f, err := os.Open("apex/apex-sample.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read only
	var out []sampleRow
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "#") {
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) != 6 {
			t.Fatalf("sample line %q", sc.Text())
		}
		var v [6]float64
		for i, s := range fields {
			if v[i], err = strconv.ParseFloat(s, 64); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, sampleRow{v[0], v[1], v[2], v[3], v[4], v[5]})
	}
	if len(out) < 1000 {
		t.Fatalf("%d sample points", len(out))
	}
	return out
}

// TestTheCoordinateTableRoundTrips: a table of years written in the compact
// form reads back cell for cell.
func TestTheCoordinateTableRoundTrips(t *testing.T) {
	years := []int{2025, 2026}
	tab := make([][2][]float32, len(years))
	for y := range years {
		for k := range 2 {
			tab[y][k] = make([]float32, coordCells)
			for i := range tab[y][k] {
				tab[y][k][i] = float32(y*1000+k*100) + float32(i)/1000
			}
		}
	}
	back, backYears, err := readCoords(writeCoords(years, tab))
	if err != nil || len(backYears) != 2 || backYears[1] != 2026 {
		t.Fatalf("%v %v", backYears, err)
	}
	for y := range years {
		for k := range 2 {
			for i := range coordCells {
				if back[y][k][i] != tab[y][k][i] {
					t.Fatalf("year %d, %d, cell %d: %v back, %v written", years[y], k, i, back[y][k][i], tab[y][k][i])
				}
			}
		}
	}
}
