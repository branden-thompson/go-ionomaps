package magcoords

import (
	"bufio"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/climatology"
)

// sample is the committed Apex.nc sample (watchpost D-107): each point's
// geographic place, PyIRI's quasi-dipole latitude and longitude, and its MLT
// at 00:00 and 12:00 UT on 2026-06-01; and PyIRI's subsolar points.
type sample struct {
	points [][6]float64
	sun    map[int][2]float64 // UT hour -> lon, lat
}

func readSample(t *testing.T) sample {
	t.Helper()
	f, err := os.Open("testdata/apex-sample.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read only
	s := sample{sun: map[int][2]float64{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		var h int
		var lon, lat float64
		if n, _ := fmtSscanf(line, &h, &lon, &lat); n == 3 {
			s.sun[h] = [2]float64{lon, lat}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		var v [6]float64
		for i, field := range strings.Fields(line) {
			if i < 6 {
				if v[i], err = strconv.ParseFloat(field, 64); err != nil {
					t.Fatal(err)
				}
			}
		}
		s.points = append(s.points, v)
	}
	if len(s.points) < 1000 || len(s.sun) != 2 {
		t.Fatalf("%d points and %d subsolar points read", len(s.points), len(s.sun))
	}
	return s
}

// fmtSscanf reads a subsolar header line, "# subsolar at HH:00 UT: lon X lat Y".
func fmtSscanf(line string, h *int, lon, lat *float64) (int, error) {
	if !strings.HasPrefix(line, "# subsolar at ") {
		return 0, nil
	}
	f := strings.Fields(strings.NewReplacer(":00", "", ":", "").Replace(line))
	// # subsolar at HH UT lon X lat Y
	if len(f) < 9 {
		return 0, nil
	}
	var err error
	if *h, err = strconv.Atoi(f[3]); err != nil {
		return 0, err
	}
	if *lon, err = strconv.ParseFloat(f[6], 64); err != nil {
		return 0, err
	}
	if *lat, err = strconv.ParseFloat(f[8], 64); err != nil {
		return 0, err
	}
	return 3, nil
}

var june1 = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// TestTheSubsolarPointFollowsPyIRI: the subsolar point, from PyIRI's own
// formula, at 00:00 and 12:00 UT on 2026-06-01.
func TestTheSubsolarPointFollowsPyIRI(t *testing.T) {
	s := readSample(t)
	for h, want := range s.sun {
		lon, lat := Subsolar(june1.Add(time.Duration(h) * time.Hour))
		if math.Abs(lon-want[0]) > 1e-6 || math.Abs(lat-want[1]) > 1e-6 {
			t.Errorf("%02d:00 UT: %v, %v; PyIRI gives %v, %v", h, lon, lat, want[0], want[1])
		}
	}
}

// TestOurCoordinatesAgreeWithApex is watchpost D-107 and D-123 for the
// library's table: our coordinates, looked up at the sample's places on
// 2026-06-01, stand within the tolerances tools/tables holds the trace to;
// and the difference they make to MUF(3000) - foF2 times M(3000)F2 from the
// climatology at 12:00 UT, F10.7 150, ours against PyIRI's coordinates and
// MLT - is reported in MHz and held to its measured bound.
func TestOurCoordinatesAgreeWithApex(t *testing.T) {
	s := readSample(t)
	at := june1.Add(12 * time.Hour)
	var dmuf []float64
	for _, p := range s.points {
		qd, ql, extrapolated := QD(p[0], p[1], at)
		if extrapolated {
			t.Fatal("2026 is within the tables, yet extrapolated")
		}
		limit := 0.4
		switch a := math.Abs(p[2]); {
		case a > 85:
			limit = 2.5
		case a > 80:
			limit = 0.6
		}
		if d := math.Abs(qd - p[2]); d > limit {
			t.Errorf("%v, %v: QD latitude %.3f; Apex.nc %.3f", p[0], p[1], qd, p[2])
		}
		if d := math.Abs(math.Mod(ql-p[3]+540, 360)-180) * math.Cos(p[2]*math.Pi/180); d > 0.5 {
			t.Errorf("%v, %v: QD longitude %.3f; Apex.nc %.3f", p[0], p[1], ql, p[3])
		}
		ours := muf(qd, MLTFrom(ql, SubsolarQD(at)), at)
		theirs := muf(p[2], p[5], at)
		dmuf = append(dmuf, math.Abs(ours-theirs))
	}
	sort.Float64s(dmuf)
	n := len(dmuf)
	t.Logf("MUF(3000) from our coordinates against PyIRI's, 12:00 UT, F10.7 150: median %.3f, 99th percentile %.3f, worst %.3f MHz", dmuf[n/2], dmuf[n*99/100], dmuf[n-1])
	if dmuf[n-1] > mufAgreement {
		t.Errorf("the worst MUF(3000) difference is %.3f MHz, past %.2f", dmuf[n-1], mufAgreement)
	}
}

// mufAgreement is the most MUF(3000) may differ, in MHz, between our
// coordinates and PyIRI's, set from the measured worst: median 0.017, 99th
// percentile 0.172, worst 0.305 MHz on the sample (D-123's report).
const mufAgreement = 0.35

func muf(qdlat, mlt float64, at time.Time) float64 {
	fo, m3 := climatology.Values(climatology.Refits(), at, 150, qdlat, mlt)
	return fo * m3
}

// TestOurMLTAgreesWithPyIRI: magnetic local time from our coordinates
// agrees with PyIRI's at the sample's places, at 00:00 and 12:00 UT, along
// the parallel - the hours difference, as degrees, times cos(latitude) -
// within 0.6°, as the longitudes do (measured worst 0.551°).
func TestOurMLTAgreesWithPyIRI(t *testing.T) {
	s := readSample(t)
	for i, h := range []int{0, 12} {
		at := june1.Add(time.Duration(h) * time.Hour)
		for _, p := range s.points {
			_, ql, _ := QD(p[0], p[1], at)
			d := math.Abs(math.Mod(MLTFrom(ql, SubsolarQD(at))-p[4+i]+36, 24)-12) * 15 * math.Cos(p[2]*math.Pi/180)
			if d > 0.6 {
				t.Errorf("%v, %v at %02d UT: MLT %.3f h; PyIRI %.3f h", p[0], p[1], h, MLTFrom(ql, SubsolarQD(at)), p[4+i])
			}
		}
	}
}

// TestExtrapolatedCoordinatesAreSaid is R-9.7: past the tables' last year,
// or before their first, the coordinates are the nearest year's, and said to
// be extrapolated, never silently.
func TestExtrapolatedCoordinatesAreSaid(t *testing.T) {
	for _, c := range []struct {
		at   time.Time
		want bool
	}{{june1, false}, {time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), false}, {time.Date(2031, 3, 1, 0, 0, 0, 0, time.UTC), true}, {time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), true}} {
		if _, _, got := QD(40, -105, c.at); got != c.want {
			t.Errorf("%v: extrapolated %v; want %v", c.at, got, c.want)
		}
	}
	if q, _, _ := QD(40, -105, time.Date(2031, 3, 1, 0, 0, 0, 0, time.UTC)); math.Abs(q-48) > 1 {
		t.Errorf("past the tables the latitude is %v; the last year's is near 48", q)
	}
}
