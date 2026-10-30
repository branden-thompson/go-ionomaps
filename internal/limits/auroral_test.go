package limits

import (
	"math"
	"testing"
	"time"
)

// TestTheGeomagneticLatitudeIsP533sDipole is §5.2.2's Lh: latitude about the
// dipole with its pole at 78.5° N, 68.2° W, at values worked apart from the
// code.
func TestTheGeomagneticLatitudeIsP533sDipole(t *testing.T) {
	for _, c := range []struct{ lat, lon, want float64 }{{78.5, -68.2, 90}, {0, 111.8, -11.5}, {45, -83.6, 55.978143}, {64.8, -147.7, 64.439708}, {69.6, 18.9, 67.217532}} {
		if got := GeomagneticLatitude(c.lat, c.lon); math.Abs(got-c.want) > 1e-5 {
			t.Errorf("%v, %v: %.6f; want %.6f", c.lat, c.lon, got, c.want)
		}
	}
}

// TestTheAuroralLossIsTable2 is D-189: Lh from Table 2 at cells read from the
// recommendation's own table - by range, season in the point's hemisphere,
// geomagnetic band and mid-path local time - and nil below 42.5°.
func TestTheAuroralLossIsTable2(t *testing.T) {
	for _, c := range []struct {
		km, lat, gn float64
		m           time.Month
		hours, want float64
	}{
		{1000, 60, 65, time.January, 5, 16.0},   // to 2500 km, northern winter, 62.5 to 67.5, 04 to 07
		{1000, 60, 80, time.January, 2, 2.0},    // 77.5 and above, 01 to 04
		{1000, 60, 45, time.January, 23.5, 0.3}, // 42.5 to 47.5, 22 to 01
		{1000, 60, 45, time.January, 0.5, 0.3},  // 22 to 01, past midnight
		{1000, 60, 70, time.April, 8, 21.4},     // equinox, 67.5 to 72.5, 07 to 10
		{1000, 60, 50, time.July, 20, 3.1},      // northern summer, 47.5 to 52.5, 19 to 22
		{1000, -60, -50, time.January, 20, 3.1}, // southern summer is December to February
		{1000, -60, -65, time.July, 5, 16.0},    // southern winter is June to August
		{3000, 60, 65, time.January, 5, 7.0},    // past 2500 km
		{3000, 60, 75, time.October, 11, 5.7},   // equinox, 72.5 to 77.5, 10 to 13
		{3000, 60, 42.5, time.July, 1, 0.7},     // 42.5 itself is in the last band
		{1000, 60, 42.4, time.January, 5, 0},    // under 42.5: nil
	} {
		if got := AuroralLoss(c.km, c.lat, c.gn, c.m, c.hours); got != c.want {
			t.Errorf("%+v: Lh %v; Table 2 gives %v", c, got, c.want)
		}
	}
	for name, v := range map[string]float64{"no latitude": AuroralLoss(1000, math.NaN(), 60, time.May, 3), "no geomagnetic latitude": AuroralLoss(1000, 60, math.NaN(), time.May, 3),
		"no time": AuroralLoss(1000, 60, 60, time.May, math.NaN()), "no month": AuroralLoss(1000, 60, 60, 0, 3), "off the globe": GeomagneticLatitude(91, 0)} {
		if !math.IsNaN(v) {
			t.Errorf("%s: %v", name, v)
		}
	}
}
