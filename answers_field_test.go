package ionomaps

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// slopedSnapshot is a snapshot whose foF2 rises with latitude, 5 MHz at the
// equator and a thirtieth of a megahertz a degree, and whose MUF(3000) is
// three times it: a field any reading of it can be checked against.
func slopedSnapshot(at time.Time) Snapshot {
	fo, muf := newField(), newField()
	for i := range fo.Values {
		lat, _ := grid.Centre(i)
		fo.Values[i] = float32(5 + lat/30)
		muf.Values[i] = 3 * fo.Values[i]
	}
	return Snapshot{Computed: at, Now: Hour{At: at, FoF2: fo, MUF3000: muf}, Inputs: Inputs{F107Mean: 150}}
}

// TestAReadingAtAPointComesFromTheField is R-2.1: a reading at any place is
// the field's value in the 2° cell the place falls in - worked out here from
// the cell's centre, not by the library's own lookup - and a cell with no
// data reads as none.
func TestAReadingAtAPointComesFromTheField(t *testing.T) {
	s := slopedSnapshot(noonUTC)
	places := []LatLon{{40, -100}, {41.3, -99.1}, {-33.9, 151.2}, {0, 0}}
	got, err := s.Readings(places)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range got {
		centre := 89 - 2*math.Floor((90-places[i].Lat)/2) // the latitude of the cell's centre, rows of 2° from 90°N
		want := 5 + centre/30
		if r.NoData || math.Abs(float64(r.FoF2)-want) > 1e-4 || math.Abs(float64(r.MUF3000)-3*want) > 3e-4 {
			t.Errorf("at %+v: %+v; the field there is foF2 %.4f", places[i], r, want)
		}
	}
	i, _ := cellIndex(s.Now.FoF2, LatLon{40, -100})
	s.Now.FoF2.NoData.set(i)
	if got, _ := s.Readings([]LatLon{{40, -100}}); !got[0].NoData {
		t.Errorf("a cell with no data reads %+v", got[0])
	}
}

// TestBestBandsForAnArea is R-2.8: for the near-vertical area around a
// place each band has its status - under the area's foF2 open or closed by
// the lower limit, above it above the upper limit with foF2 its limit - and
// the hours of the day it is typically open.
func TestBestBandsForAnArea(t *testing.T) {
	night := time.Date(2026, 10, 11, 7, 0, 0, 0, time.UTC) // local midnight at 100°W: no daytime absorption
	s := flatSnapshot(night, 7, 21)
	got, err := s.Bands([]LatLon{{40, -100}}, 400)
	if err != nil || len(got) != 1 || len(got[0].Bands) != len(AllBands()) {
		t.Fatalf("%v, %v", got, err)
	}
	open := 0
	for _, b := range got[0].Bands {
		switch {
		case b.Band.LowMHz() >= 7:
			if b.Status != AboveUpper || b.Limit != LimitFoF2 || b.LimitMHz != 7 {
				t.Errorf("%v over foF2 7: %v by %v at %.1f; above the upper limit, foF2's", b.Band, b.Status, b.Limit, b.LimitMHz)
			}
		case b.Status == Open:
			open++
		case b.Status != Weak && b.Status != Absorbed:
			t.Errorf("%v under foF2 7: %v; open, or closed by the lower limit", b.Band, b.Status)
		}
		if b.OpenHours == nil && b.Status == Open {
			t.Errorf("%v is open now and carries no typical open hours", b.Band)
		}
	}
	if open == 0 {
		t.Error("no band under foF2 7 MHz is open at local midnight")
	}
}

// FuzzAnswersNeverReturnNaN is R-3.3 and R-3.5: whatever place, radius,
// frequency and snapshot moment a host asks, no answer carries NaN or Inf - it
// answers, or refuses.
func FuzzAnswersNeverReturnNaN(f *testing.F) {
	f.Add(40.0, -100.0, 400.0, 14.2, int16(0))
	f.Add(91.0, 181.0, 0.0, 0.5, int16(-30))
	f.Add(math.NaN(), math.Inf(1), 2001.0, 31.0, int16(500))
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	f.Fuzz(func(t *testing.T, lat, lon, radius, mhz float64, minutes int16) {
		s, p := slopedSnapshot(noonUTC.Add(time.Duration(minutes)*time.Minute)), LatLon{Lat: lat, Lon: lon}
		if rs, err := s.Readings([]LatLon{p}); err == nil {
			for _, r := range rs {
				if !finite(float64(r.FoF2)) || !finite(float64(r.MUF3000)) || !finite(float64(r.Nearest.Km)) {
					t.Fatalf("At(%+v): %+v", p, r)
				}
			}
		}
		check := func(call string, as []Answer, err error) {
			if err != nil {
				return
			}
			for _, a := range as {
				for _, b := range a.Bands {
					if !finite(float64(b.LimitMHz)) {
						t.Fatalf("%s(%+v): %+v", call, p, b)
					}
				}
			}
		}
		as, err := s.Bands([]LatLon{p}, radius)
		check("Bands", as, err)
		as, err = s.Path(LatLon{Lat: 40, Lon: -100}, []LatLon{p})
		check("Path", as, err)
	})
}

// FuzzAReachNeverReturnsNaN is R-3.3 and R-3.5 for Reach, a fuzz target of
// its own because a reach costs the whole globe's circuits: whatever origin
// and frequency a host asks, the reach is finite in every cell - or refused.
func FuzzAReachNeverReturnsNaN(f *testing.F) {
	f.Add(40.0, -100.0, 14.2)
	f.Add(91.0, 181.0, 0.5)
	f.Add(math.NaN(), math.Inf(1), 31.0)
	s := slopedSnapshot(noonUTC)
	s.typical = &typicalDays{} // as a library's snapshots keep the typical day
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	f.Fuzz(func(t *testing.T, lat, lon, mhz float64) {
		p := LatLon{Lat: lat, Lon: lon}
		if r, err := s.Reach(p, mhz); err == nil {
			if !finite(float64(r.SkipKm)) || !finite(float64(r.MHz)) {
				t.Fatalf("Reach(%+v, %v): %+v", p, mhz, r)
			}
			for _, v := range r.Field.Values {
				if !finite(float64(v)) {
					t.Fatalf("Reach(%+v, %v): a cell that is no number", p, mhz)
				}
			}
		}
	})
}

// TestAPlaceThatIsNoNumberFallsInNoCell is L-F12: a place that is no number
// - a control point worked out from one - falls in no cell, so it reads no
// value and no station, never an arbitrary cell's.
func TestAPlaceThatIsNoNumberFallsInNoCell(t *testing.T) {
	s := withStations(slopedSnapshot(noonUTC))
	for _, p := range []LatLon{{math.NaN(), 0}, {0, math.NaN()}, {math.Inf(1), 0}, {0, math.Inf(-1)}} {
		if i, ok := cellIndex(s.Now.FoF2, p); ok {
			t.Errorf("%+v falls in cell %d", p, i)
		}
		if _, ok := cellAt(s.Now.FoF2, p); ok {
			t.Errorf("%+v reads a value", p)
		}
		if n := s.nearestAt(s.Now.FoF2, p); n.Known {
			t.Errorf("%+v has a nearest station %+v", p, n)
		}
	}
}
