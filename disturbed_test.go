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
// open under both limits whose lower edge is below D-RAP's 1 dB frequency is
// disturbed, named as D-RAP's, at that frequency; a band above its upper
// limit, or closed by the lower (D-70), stays so; a path takes D-RAP's
// highest at its control points; with no D-RAP, nothing is disturbed.
func TestADisturbanceMarksTheBandsItCovers(t *testing.T) {
	seen := 0
	check := func(name string, bands []BandStatus, drap float32, upper float64) {
		t.Helper()
		for _, b := range bands {
			switch {
			case b.Status == Absorbed || b.Status == Weak:
				if b.Limit != LimitAbsorption && b.Limit != LimitCircuit {
					t.Errorf("%s %v: closed by the lower limit, named %v", name, b.Band, b.Limit)
				}
			case b.Band.LowMHz() >= upper:
				if b.Status != AboveUpper {
					t.Errorf("%s %v: %v; above the upper limit %.2f", name, b.Band, b.Status, upper)
				}
			case b.Band.LowMHz() < float64(drap):
				seen++
				if b.Status != Disturbed || b.Limit != LimitDRAP || b.LimitMHz != drap {
					t.Errorf("%s %v: %v by %v at %v; want disturbed by D-RAP at %v", name, b.Band, b.Status, b.Limit, b.LimitMHz, drap)
				}
			default:
				if b.Status != Open {
					t.Errorf("%s %v: %v; want open", name, b.Band, b.Status)
				}
			}
		}
	}
	s := disturbed(8, 8)
	area, err := s.Bands([]LatLon{{40, -100}}, 400, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	check("near-vertical, D-RAP 8", area[0].Bands, 8, 7) // foF2 7 is the area's upper limit
	wide := disturbed(15, 15)
	path, _ := wide.Path(LatLon{40, -100}, []LatLon{{40, -64.8}}, noonUTC)
	muf, _, _ := wide.pathMUF(hourReader(wide.Hours[0]), LatLon{40, -100}, LatLon{40, -64.8}, newSunTrack(noonUTC))
	check("a path, D-RAP 15", path[0].Bands, 15, muf)
	if seen == 0 {
		t.Fatal("no band was open under D-RAP: the test measures nothing")
	}
	far := disturbed(0, 15)
	paths, _ := far.Path(LatLon{40, -100}, []LatLon{{40, -64.8}, {40, 0}}, noonUTC)
	check("a path wholly where D-RAP is 0", paths[0].Bands, 0, muf)
	if st := statusOf(paths[1].Bands, Band20m); st != Disturbed {
		t.Errorf("a path whose far control point is under D-RAP's 15 MHz: 20 m is %v; want disturbed", st)
	}
	back, _ := far.Path(LatLon{40, 0}, []LatLon{{40, -100}}, noonUTC) // the disturbed control point first
	if st := statusOf(back[0].Bands, Band20m); st != Disturbed {
		t.Errorf("the same path reversed: 20 m is %v; want disturbed", st)
	}
	edge := disturb([]BandStatus{{Band: Band60m, Status: Open}, {Band: Band40m, Status: Open}, {Band: Band80m, Status: Absorbed}}, 7.0, true)
	if edge[0].Status != Disturbed || edge[1].Status != Open || edge[2].Status != Absorbed {
		t.Errorf("D-RAP at 7.0 MHz: 60 m %v, 40 m (from 7.0) %v, 80 m absorbed %v; want disturbed, open, absorbed", edge[0].Status, edge[1].Status, edge[2].Status)
	}
	plain := flatSnapshot(noonUTC, 7, 21)
	none, _ := plain.Bands([]LatLon{{40, -100}}, 400, noonUTC)
	for _, b := range none[0].Bands {
		if b.Status == Disturbed {
			t.Errorf("no D-RAP: %v is disturbed", b.Band)
		}
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
	for _, st := range []Status{Open, AboveUpper, Disturbed, Absorbed, Weak, StatusNoData} {
		if !seen[st] {
			t.Errorf("no answer was %v; the test needs every status", st)
		}
	}
}
