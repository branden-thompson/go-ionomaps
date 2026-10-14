package ionomaps

import (
	"testing"
)

// disturbed is a snapshot of foF2 7 MHz and MUF(3000) 21 everywhere, with
// D-RAP's 1 dB frequency at mhz everywhere west of 35°W and east at east.
func disturbed(mhz, east float32) Snapshot {
	s := flatSnapshot(noonUTC, 7, 21)
	s.Disturbance = newField()
	for i := range s.Disturbance.Values {
		s.Disturbance.Values[i] = mhz
		if lon, _, _ := s.Disturbance.CellCentre(i%s.Disturbance.Cols, i/s.Disturbance.Cols); lon > -35 {
			s.Disturbance.Values[i] = east
		}
	}
	return s
}

// TestADisturbanceMarksTheBandsItCovers is R-2.5 (watchpost D-73): a band
// open under its upper limit whose lower edge is below D-RAP's 1 dB
// frequency is disturbed, named as D-RAP's, at that frequency; a band above
// its upper limit stays so; a path takes D-RAP's highest at its control
// points; with no D-RAP, nothing is disturbed.
func TestADisturbanceMarksTheBandsItCovers(t *testing.T) {
	s := disturbed(8, 8)
	area, err := s.Bands([]LatLon{{40, -100}}, 400, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range area[0].Bands {
		want := BandStatus{Band: b.Band, Status: AboveUpper, Limit: LimitFoF2, LimitMHz: 7} // foF2 7: 40 m's 7.0 is not under it
		if b.Band.LowMHz() < 7 {
			want = BandStatus{Band: b.Band, Status: Disturbed, Limit: LimitDRAP, LimitMHz: 8}
		}
		if b.Status != want.Status || b.Limit != want.Limit || b.LimitMHz != want.LimitMHz {
			t.Errorf("near-vertical %v: %v by %v at %v; want %v by %v at %v", b.Band, b.Status, b.Limit, b.LimitMHz, want.Status, want.Limit, want.LimitMHz)
		}
	}
	path, _ := s.Path(LatLon{40, -100}, []LatLon{{40, -64.8}}, noonUTC)
	muf, _, _ := s.pathMUF(hourReader(s.Hours[0]), LatLon{40, -100}, LatLon{40, -64.8}, newSunTrack(noonUTC))
	for _, b := range path[0].Bands {
		want := AboveUpper
		switch {
		case b.Band.LowMHz() < 8:
			want = Disturbed
		case b.Band.LowMHz() < muf:
			want = Open
		}
		if b.Status != want {
			t.Errorf("a path, D-RAP at 8 MHz: %v is %v; want %v", b.Band, b.Status, want)
		}
	}
	far := disturbed(0, 15)
	paths, _ := far.Path(LatLon{40, -100}, []LatLon{{40, -64.8}, {40, 0}}, noonUTC)
	if st := statusOf(paths[0].Bands, Band160m); st != Open {
		t.Errorf("a path wholly where D-RAP is 0: 160 m is %v", st)
	}
	if st := statusOf(paths[1].Bands, Band20m); st != Disturbed {
		t.Errorf("a path whose far control point is under D-RAP's 15 MHz: 20 m is %v; want disturbed", st)
	}
	back, _ := far.Path(LatLon{40, 0}, []LatLon{{40, -100}}, noonUTC) // the disturbed control point first
	if st := statusOf(back[0].Bands, Band20m); st != Disturbed {
		t.Errorf("the same path reversed: 20 m is %v; want disturbed", st)
	}
	edge := disturbed(7, 7)
	if p, _ := edge.Path(LatLon{40, -100}, []LatLon{{40, -64.8}}, noonUTC); statusOf(p[0].Bands, Band40m) != Open || statusOf(p[0].Bands, Band60m) != Disturbed {
		t.Errorf("D-RAP at 7.0 MHz: 40 m (from 7.0) %v, 60 m %v; want open and disturbed", statusOf(p[0].Bands, Band40m), statusOf(p[0].Bands, Band60m))
	}
	plain := flatSnapshot(noonUTC, 7, 21)
	none, _ := plain.Bands([]LatLon{{40, -100}}, 400, noonUTC)
	if st := statusOf(none[0].Bands, Band160m); st != Open {
		t.Errorf("no D-RAP: 160 m is %v; want open", st)
	}
}

// statusOf is one band's status in an answer.
func statusOf(bands []BandStatus, b Band) Status {
	for _, x := range bands {
		if x.Band == b {
			return x.Status
		}
	}
	return StatusNoData
}

// TestEveryStatusNamesItsLimit is R-2.6: every status but no data names the
// limit that decided it, and no data names none.
func TestEveryStatusNamesItsLimit(t *testing.T) {
	s := disturbed(8, 15)
	blank := flatSnapshot(noonUTC, 7, 21)
	for i := range blank.Hours[0].FoF2.Values {
		blank.Hours[0].FoF2.NoData.Set(i)
	}
	var all []BandStatus
	for _, snap := range []Snapshot{s, blank} {
		a, _ := snap.Bands([]LatLon{{40, -100}, {40, 0}}, 400, noonUTC)
		p, _ := snap.Path(LatLon{40, -100}, []LatLon{{40, -64.8}, {40, 0}}, noonUTC)
		for _, x := range append(a, p...) {
			all = append(all, x.Bands...)
		}
	}
	seen := map[Status]bool{}
	for _, b := range all {
		seen[b.Status] = true
		if named := b.Limit != LimitNone; named != (b.Status != StatusNoData) {
			t.Errorf("%v %v names limit %v", b.Band, b.Status, b.Limit)
		}
	}
	for _, st := range []Status{Open, AboveUpper, Disturbed, StatusNoData} {
		if !seen[st] {
			t.Errorf("no answer was %v; the test needs every status", st)
		}
	}
}
