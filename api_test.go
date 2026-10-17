package ionomaps

import (
	"slices"
	"testing"
	"time"
)

// TestASnapshotIsOneHour is C-11 (watchpost D-216): a snapshot holds the
// hour it answers for, from the moment it was computed; a copy of it keeps
// whether that hour is typical.
func TestASnapshotIsOneHour(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	if !s.Now.At.Equal(s.Computed) || s.Now.Typical {
		t.Errorf("the hour now is at %v, typical %v; computed %v", s.Now.At, s.Now.Typical, s.Computed)
	}
	a, err := s.Ahead(2)
	if err != nil {
		t.Fatal(err)
	}
	if c := a.clone(); !c.Now.Typical {
		t.Error("a copy of an hour ahead is no longer typical")
	}
}

// TestAnHourAheadCarriesNoLiveDisturbance is L-F2 (watchpost D-216): an hour
// ahead is a snapshot of its own - the climatology's, typical, at that hour -
// that carries none of now's measurements: no D-RAP disturbance, no live
// offset or correction; so no band of an hour ahead is said disturbed, while
// now's are.
func TestAnHourAheadCarriesNoLiveDisturbance(t *testing.T) {
	s := disturbed(25, 25)
	s.Offset = Offset{LiveMHz: 1.2, TypicalMHz: 0.4, SpreadMHz: 0.3, Measured: true, Corrected: true, Basis: "seed"}
	s.Background = Backgrounds{FoF2: GloTEC, M3000: Climatology}
	now, err := s.Bands([]LatLon{{40, -100}}, 500)
	if err != nil || !slices.ContainsFunc(now[0].Bands, func(b BandStatus) bool { return b.Status == Disturbed }) {
		t.Fatalf("now's bands %+v (%v); D-RAP at 25 MHz disturbs the open ones", now[0].Bands, err)
	}
	for _, k := range []int{1, 6, 12, 18, 24} {
		a, err := s.Ahead(k)
		if err != nil {
			t.Fatal(err)
		}
		if !a.Now.Typical || !a.Now.At.Equal(s.Now.At.Add(time.Duration(k)*time.Hour)) || !a.Computed.Equal(s.Computed) {
			t.Errorf("+%d h: typical %v, at %v, computed %v", k, a.Now.Typical, a.Now.At, a.Computed)
		}
		if len(a.Disturbance.Values) != 0 || a.Inputs.DRAPValid != (time.Time{}) {
			t.Errorf("+%d h carries now's D-RAP", k)
		}
		if a.Offset.Measured || a.Offset.Corrected || a.Offset.LiveMHz != 0 || a.Offset.TypicalMHz != 0.4 {
			t.Errorf("+%d h: offset %+v; the typical offset only", k, a.Offset)
		}
		if a.Background != (Backgrounds{FoF2: Climatology, M3000: Climatology}) {
			t.Errorf("+%d h: backgrounds %+v; the climatology's", k, a.Background)
		}
		got, err := a.Bands([]LatLon{{40, -100}}, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range got[0].Bands {
			if b.Status == Disturbed || b.Limit == LimitDRAP {
				t.Errorf("+%d h: %v said disturbed", k, b.Band)
			}
		}
	}
}

// TestEveryBandStatusAndLimitIsNamed is L-F8 (watchpost D-216): a host keys
// and logs by name - every band, status and limit has its own, and a value
// that is none says so.
func TestEveryBandStatusAndLimitIsNamed(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range AllBands() {
		if n := b.String(); n == "" || n == "no band" || seen[n] {
			t.Errorf("band %d is named %q", b, n)
		}
		seen[b.String()] = true
	}
	if Band20m.String() != "20m" || Band160m.String() != "160m" || Band(0).String() != "no band" || Band(11).String() != "no band" {
		t.Errorf("names %q %q %q %q", Band20m, Band160m, Band(0), Band(11))
	}
	for _, s := range []Status{StatusNoData, Open, AboveUpper, Disturbed, Absorbed, Weak} {
		if n := s.String(); n == "" || n == "no status" || seen[n] {
			t.Errorf("status %d is named %q", s, n)
		}
		seen[s.String()] = true
	}
	for _, l := range []Limit{LimitNone, LimitMUF, LimitFoF2, LimitDRAP, LimitAbsorption, LimitCircuit} {
		if n := l.String(); n == "" || n == "no limit name" || seen[n] {
			t.Errorf("limit %d is named %q", l, n)
		}
		seen[l.String()] = true
	}
	if Status(99).String() != "no status" || Limit(99).String() != "no limit name" || Open.String() != "open" || LimitMUF.String() != "MUF" {
		t.Errorf("names %q %q %q %q", Status(99), Limit(99), Open, LimitMUF)
	}
}

// TestWithNoDataMarksACopy is C-20 and L-F11 (watchpost D-216): a snapshot's
// fields cannot be written through, and a host's test still marks cells with
// no data - on a copy, the original untouched; a cell outside the field is
// passed over.
func TestWithNoDataMarksACopy(t *testing.T) {
	f := flatSnapshot(noonUTC, 7, 21).Now.FoF2
	g := f.WithNoData(3, 16199, -1, f.Cols*f.Rows)
	if !g.NoData.Has(3) || !g.NoData.Has(16199) || g.NoData.Has(4) {
		t.Errorf("the copy's no-data cells: 3 %v, 16199 %v, 4 %v", g.NoData.Has(3), g.NoData.Has(16199), g.NoData.Has(4))
	}
	if f.NoData.Has(3) || f.NoData.Has(16199) {
		t.Error("marking a copy wrote into the original")
	}
	s := flatSnapshot(noonUTC, 7, 21)
	s.Now.FoF2 = s.Now.FoF2.WithNoData(cellOf(s.Now.FoF2, LatLon{40, -100}))
	r, err := s.Readings([]LatLon{{40, -100}, {-30, 150}})
	if err != nil || !r[0].NoData || r[1].NoData {
		t.Errorf("readings %+v (%v); the first place's cell has no data", r, err)
	}
}

// cellOf is the cell a place falls in.
func cellOf(f Field, p LatLon) int {
	i, _ := cellIndex(f, p)
	return i
}
