package ionomaps

import (
	"math"
	"testing"
	"time"
)

// flatSnapshot is a snapshot of one hour whose fields are the same in every
// cell: foF2 fo and MUF(3000) muf, so M(3000)F2 is their ratio.
func flatSnapshot(at time.Time, fo, muf float32) Snapshot {
	field := func(v float32) Field {
		f := newField()
		for i := range f.Values {
			f.Values[i] = v
		}
		return f
	}
	return Snapshot{Computed: at, Hours: []Hour{{At: at.Truncate(time.Hour), FoF2: field(fo), MUF3000: field(muf)}}, Inputs: Inputs{F107Mean: 150}}
}

var noonUTC = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

// TestReadingsForManyPointsInOneCall is R-2.1 and R-2.3: foF2 and MUF(3000)
// at many places in one call, one reading each, in order.
func TestReadingsForManyPointsInOneCall(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	got, err := s.At([]LatLon{{40, -100}, {-30, 150}, {0, 0}}, noonUTC.Add(20*time.Minute))
	if err != nil || len(got) != 3 {
		t.Fatalf("%d readings, %v", len(got), err)
	}
	for _, r := range got {
		if r.NoData || r.FoF2 != 7 || r.MUF3000 != 21 {
			t.Errorf("a reading is %+v; want 7 and 21 MHz", r)
		}
	}
}

// TestEachBandsStatusForAPath is R-2.2 (D-175: upper limits): a path's
// bands are open below its P.533 MUF and above it past, each naming the MUF
// as its limit; at 3000 km the MUF is the MUF(3000) the snapshot draws, to
// within the gyrofrequency's share.
func TestEachBandsStatusForAPath(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	from := LatLon{Lat: 40, Lon: -100}
	to := LatLon{Lat: 40, Lon: -64.8} // about 3000 km east along the parallel's great circle
	got, err := s.Path(from, []LatLon{to}, noonUTC)
	if err != nil || len(got) != 1 || len(got[0]) != len(AllBands()) {
		t.Fatalf("%v, %v", got, err)
	}
	muf := float64(got[0][0].LimitMHz)
	if muf < 20.5 || muf > 22.5 {
		t.Errorf("the path MUF at about 3000 km is %.2f MHz; the MUF(3000) is 21", muf)
	}
	for _, b := range got[0] {
		want := AboveUpper
		if b.Band.LowMHz() < muf {
			want = Open
		}
		if b.Status != want || b.Limit != LimitMUF {
			t.Errorf("%v: %v by %v at %.2f MHz; want %v by the MUF", b.Band, b.Status, b.Limit, b.LimitMHz, want)
		}
	}
	near, _ := s.Path(from, []LatLon{{Lat: 40, Lon: -94}}, noonUTC) // about 500 km: its own hop, far shorter
	if float64(near[0][0].LimitMHz) > 0.6*muf {
		t.Errorf("a 500 km path's MUF %.2f is not well below the 3000 km path's %.2f", near[0][0].LimitMHz, muf)
	}
}

// TestShortPathsUseFoF2 is R-2.2 and R-2.8: the near-vertical area around a
// place takes its upper limit from foF2 there.
func TestShortPathsUseFoF2(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	got, err := s.Bands([]LatLon{{37.7, -97.3}}, 400, noonUTC)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v, %v", got, err)
	}
	for _, b := range got[0] {
		want := AboveUpper
		if b.Band.LowMHz() < 7 {
			want = Open
		}
		if b.Status != want || b.Limit != LimitFoF2 || b.LimitMHz != 7 {
			t.Errorf("%v near-vertical: %+v; want %v by foF2 at 7 MHz", b.Band, b, want)
		}
	}
}

// TestTheAnswersRefuseBadInput is R-3.5: more than 64 places, a place off
// the globe or no number, a radius not above 0 or above 2000 km, or a
// moment outside the snapshot's hours, are refused, never answered.
func TestTheAnswersRefuseBadInput(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	many := make([]LatLon, 65)
	for _, c := range []struct {
		name string
		err  error
	}{
		{"65 places", func() error { _, err := s.At(many, noonUTC); return err }()},
		{"off the globe", func() error { _, err := s.At([]LatLon{{91, 0}}, noonUTC); return err }()},
		{"no number", func() error { _, err := s.Path(LatLon{math.NaN(), 0}, []LatLon{{0, 0}}, noonUTC); return err }()},
		{"no radius", func() error { _, err := s.Bands([]LatLon{{0, 0}}, 0, noonUTC); return err }()},
		{"a radius past 2000 km", func() error { _, err := s.Bands([]LatLon{{0, 0}}, 2500, noonUTC); return err }()},
		{"an hour not held", func() error { _, err := s.At([]LatLon{{0, 0}}, noonUTC.Add(3*time.Hour)); return err }()},
	} {
		if c.err == nil {
			t.Errorf("%s was answered", c.name)
		}
	}
}

// TestAPlaceWithNoDataHasNoStatus is R-2.6: where the field has no value
// the bands say no data, never a guess.
func TestAPlaceWithNoDataHasNoStatus(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	for i := range s.Hours[0].FoF2.Values {
		s.Hours[0].FoF2.NoData.Set(i)
	}
	got, err := s.Bands([]LatLon{{0, 0}}, 400, noonUTC)
	if err != nil || got[0][0].Status != StatusNoData {
		t.Errorf("a place with no foF2: %+v, %v", got, err)
	}
}

// TestTheDaysOpenHours is R-2.8 (watchpost D-135): each band carries the UTC
// day's hours it is typically open, from the climatology at the snapshot's
// F10.7 mean: spans within the day, in order, neither overlapping nor
// touching; a lower band open at least as long as a higher one; a path, its
// MUF above foF2, open longer than the near-vertical area.
func TestTheDaysOpenHours(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	day := noonUTC.Truncate(24 * time.Hour)
	near, err := s.Bands([]LatLon{{37.7, -97.3}}, 400, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.Path(LatLon{37.7, -97.3}, []LatLon{{40.7, -74}}, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	total := map[string]time.Duration{}
	for name, bands := range map[string][]BandStatus{"near-vertical": near[0], "a path": path[0]} {
		last := 25 * time.Hour
		for _, b := range bands {
			open := time.Duration(0)
			end := day.Add(-time.Nanosecond)
			for _, sp := range b.OpenHours {
				if !sp.From.After(end) || !sp.To.After(sp.From) || sp.To.After(day.Add(24*time.Hour)) {
					t.Errorf("%s %v: a span %v to %v is out of the day, out of order or touching the one before", name, b.Band, sp.From, sp.To)
				}
				open += sp.To.Sub(sp.From)
				end = sp.To
			}
			total[name] += open
			if open > last {
				t.Errorf("%s: %v is open %v, longer than the band below it (%v)", name, b.Band, open, last)
			}
			last = open
		}
		if len(bands[0].OpenHours) == 0 {
			t.Errorf("%s: 160 m is never open in the day", name)
		}
		if name == "near-vertical" && !partly(bands) {
			t.Errorf("near-vertical, every band is open all day or not at all; the day's foF2 rises and falls")
		}
	}
	if total["a path"] <= total["near-vertical"] {
		t.Errorf("the path's bands are open %v in all, the near-vertical area's %v; a path's MUF is above foF2", total["a path"], total["near-vertical"])
	}
}

// partly reports whether some band is open for part of the day only.
func partly(bands []BandStatus) bool {
	for _, b := range bands {
		open := time.Duration(0)
		for _, sp := range b.OpenHours {
			open += sp.To.Sub(sp.From)
		}
		if open > 0 && open < 24*time.Hour {
			return true
		}
	}
	return false
}
