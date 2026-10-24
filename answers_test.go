package ionomaps

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
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
	return Snapshot{Computed: at, Now: Hour{At: at, FoF2: field(fo), MUF3000: field(muf)}, Inputs: Inputs{F107Mean: 150}}
}

var noonUTC = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

// TestReadingsForManyPointsInOneCall is R-2.1 and R-2.3: foF2 and MUF(3000)
// at many places in one call, one reading each, in order.
func TestReadingsForManyPointsInOneCall(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	got, err := s.Readings([]LatLon{{40, -100}, {-30, 150}, {0, 0}})
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
	got, err := s.Path(from, []LatLon{to})
	if err != nil || len(got) != 1 || len(got[0].Bands) != len(AllBands()) {
		t.Fatalf("%v, %v", got, err)
	}
	muf := mufOf(got[0].Bands)
	if muf < 20.5 || muf > 22.5 {
		t.Errorf("the path MUF at about 3000 km is %.2f MHz; the MUF(3000) is 21", muf)
	}
	for _, b := range got[0].Bands {
		switch {
		case b.Band.LowMHz() >= muf:
			if b.Status != AboveUpper || b.Limit != LimitMUF {
				t.Errorf("%v: %v by %v; above the MUF, named the MUF", b.Band, b.Status, b.Limit)
			}
		case b.Status == Open:
			if b.Limit != LimitMUF {
				t.Errorf("%v: open, named %v; want the MUF", b.Band, b.Limit)
			}
		case b.Status == Absorbed && b.Limit == LimitAbsorption, b.Status == Weak && b.Limit == LimitCircuit:
		default:
			t.Errorf("%v under the MUF: %v by %v; open, or closed by the lower limit and named so", b.Band, b.Status, b.Limit)
		}
	}
	near, _ := s.Path(from, []LatLon{{Lat: 40, Lon: -94}}) // about 500 km: its own hop, far shorter
	if m := mufOf(near[0].Bands); m > 0.6*muf {
		t.Errorf("a 500 km path's MUF %.2f is not well below the 3000 km path's %.2f", m, muf)
	}
}

// mufOf is the upper limit an answer's bands name.
func mufOf(bands []BandStatus) float64 {
	for _, b := range bands {
		if b.Limit == LimitMUF || b.Limit == LimitFoF2 {
			return float64(b.LimitMHz)
		}
	}
	return 0
}

// TestShortPathsUseFoF2 is R-2.2 and R-2.8: the near-vertical area around a
// place takes its upper limit from foF2 there.
func TestShortPathsUseFoF2(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	got, err := s.Bands([]LatLon{{37.7, -97.3}}, 400)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v, %v", got, err)
	}
	for _, b := range got[0].Bands {
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
		{"65 places", func() error { _, err := s.Readings(many); return err }()},
		{"off the globe", func() error { _, err := s.Readings([]LatLon{{91, 0}}); return err }()},
		{"no number", func() error { _, err := s.Path(LatLon{math.NaN(), 0}, []LatLon{{0, 0}}); return err }()},
		{"no radius", func() error { _, err := s.Bands([]LatLon{{0, 0}}, 0); return err }()},
		{"a radius past 2000 km", func() error { _, err := s.Bands([]LatLon{{0, 0}}, 2500); return err }()},
		{"no field", func() error { _, err := (Snapshot{Computed: noonUTC}).Readings([]LatLon{{0, 0}}); return err }()},
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
	for i := range s.Now.FoF2.Values {
		s.Now.FoF2.NoData.set(i)
	}
	got, err := s.Bands([]LatLon{{0, 0}}, 400)
	if err != nil || got[0].Bands[0].Status != StatusNoData {
		t.Errorf("a place with no foF2: %+v, %v", got, err)
	}
}

// TestTheDaysOpenHours is R-2.8 (watchpost D-135): each band carries the UTC
// day's hours it is typically open, from the climatology at the snapshot's
// F10.7 mean: spans within the day, in order, neither overlapping nor
// touching; and, under the upper limits alone, a lower band open at least
// as long as a higher one and a path, its MUF above foF2, open longer than
// the near-vertical area (the lower limit, which closes the low bands by
// day, is TestTheTypicalDayKeepsTheLowerLimit's).
func TestTheDaysOpenHours(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	day := noonUTC.Truncate(24 * time.Hour)
	near, err := s.Bands([]LatLon{{37.7, -97.3}}, 400)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.Path(LatLon{37.7, -97.3}, []LatLon{{40.7, -74}})
	if err != nil {
		t.Fatal(err)
	}
	for name, bands := range map[string][]BandStatus{"near-vertical": near[0].Bands, "a path": path[0].Bands} {
		for _, b := range bands {
			end := day.Add(-time.Nanosecond)
			for _, sp := range b.OpenHours {
				if !sp.From.After(end) || !sp.To.After(sp.From) || sp.To.After(day.Add(24*time.Hour)) {
					t.Errorf("%s %v: a span %v to %v is out of the day, out of order or touching the one before", name, b.Band, sp.From, sp.To)
				}
				end = sp.To
			}
		}
	}
	upperOnly := func(lim func(*typicalHour, *[24]typicalHour) hourLimits) func(*typicalHour, *[24]typicalHour) hourLimits {
		return func(t *typicalHour, day *[24]typicalHour) hourLimits {
			h := lim(t, day)
			h.absorbed = notAbsorbed
			return h
		}
	}
	typical := s.typicalDay(noonUTC)
	withHours := func(open [][]Span) []BandStatus { return withOpenHours(statuses(1, true, LimitMUF), open) }
	total := map[string]time.Duration{}
	for name, bands := range map[string][]BandStatus{
		"near-vertical": withHours(openHours(&typical, upperOnly(s.areaLimit(LatLon{37.7, -97.3}, 400)))),
		"a path":        withHours(openHours(&typical, upperOnly(s.pathLimit(LatLon{37.7, -97.3}, LatLon{40.7, -74})))),
	} {
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

// withStations is a snapshot whose nearest station is near everywhere west
// of 35°W and far, and unscored, east of it.
func withStations(s Snapshot) Snapshot {
	f := s.Now.FoF2
	s.NearestKm, s.NearestUnscored = make([]float32, len(f.Values)), newBitset(len(f.Values))
	for i := range s.NearestKm {
		s.NearestKm[i] = 100
		if _, lon := grid.Centre(i); lon > -35 {
			s.NearestKm[i] = 3000
			s.NearestUnscored.set(i)
		}
	}
	return s
}

// TestAnswersCarryTheNearestStationDistance is R-2.11 (D-65, watchpost
// D-185): every point answer says how far its nearest assimilated station
// is and whether that station is unscored - a path, the control point
// furthest from one, so a path whose far end lies far from every station
// says so though its middle is near one; with no station assimilated, none
// is known.
func TestAnswersCarryTheNearestStationDistance(t *testing.T) {
	s := withStations(flatSnapshot(noonUTC, 7, 21))
	near, far := LatLon{40, -100}, LatLon{40, 0}
	at, err := s.Readings([]LatLon{near, far})
	if err != nil {
		t.Fatal(err)
	}
	if at[0].Nearest != (Nearest{Km: 100, Known: true}) || at[1].Nearest != (Nearest{Km: 3000, Unscored: true, Known: true}) {
		t.Errorf("At: %+v and %+v; want 100 km scored, 3000 km unscored", at[0].Nearest, at[1].Nearest)
	}
	bands, err := s.Bands([]LatLon{near, far}, 400)
	if err != nil || bands[0].Nearest.Km != 100 || bands[1].Nearest != at[1].Nearest || len(bands[0].Bands) != len(AllBands()) {
		t.Errorf("Bands: %+v, %+v (%v); want each origin's own", bands[0].Nearest, bands[1].Nearest, err)
	}
	paths, err := s.Path(near, []LatLon{{40, -64.8}, far})
	if err != nil {
		t.Fatal(err)
	}
	if paths[0].Nearest.Km != 100 || paths[0].Nearest.Unscored {
		t.Errorf("a path wholly west of 35°W: %+v; want 100 km, scored", paths[0].Nearest)
	}
	if mid, _ := s.Readings([]LatLon{{56, -50}}); mid[0].Nearest.Km != 100 { // the long path's middle, near a station
		t.Fatalf("the long path's middle reads %+v; the test needs it near a station", mid[0].Nearest)
	}
	if paths[1].Nearest != (Nearest{Km: 3000, Unscored: true, Known: true}) {
		t.Errorf("a path whose far control point is 3000 km from any station: %+v; want that point's", paths[1].Nearest)
	}
	blank := withStations(flatSnapshot(noonUTC, 7, 21))
	for i := range blank.Now.FoF2.Values {
		blank.Now.FoF2.NoData.set(i)
	}
	if p, _ := blank.Path(near, []LatLon{far}); p[0].Nearest.Km != 100 || p[0].Bands[0].Status != StatusNoData {
		t.Errorf("a path with no value at its middle: %+v, %v; want its middle's nearest, 100 km, and no data", p[0].Nearest, p[0].Bands[0].Status)
	}
	on := withStations(flatSnapshot(noonUTC, 7, 21))
	clear(on.NearestKm) // a station at every cell's centre
	if p, _ := on.Path(near, []LatLon{far}); p[0].Nearest != (Nearest{Km: 0, Unscored: p[0].Nearest.Unscored, Known: true}) {
		t.Errorf("a path whose control points each sit on a station: %+v; want known, 0 km", p[0].Nearest)
	}
	plain := flatSnapshot(noonUTC, 7, 21)
	a, _ := plain.Readings([]LatLon{near})
	b, _ := plain.Bands([]LatLon{near}, 400)
	p, _ := plain.Path(near, []LatLon{far})
	if a[0].Nearest != (Nearest{}) || b[0].Nearest != (Nearest{}) || p[0].Nearest != (Nearest{}) {
		t.Errorf("no station assimilated, and one is known: %+v, %+v, %+v", a[0].Nearest, b[0].Nearest, p[0].Nearest)
	}
}

// TestAPathTakesSixtyFourDestinations is R-3.5 and RC-13: Path answers up to
// 64 destinations, its origin besides, as its error says; a 65th is refused.
func TestAPathTakesSixtyFourDestinations(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	to := make([]LatLon, maxPlaces+1)
	for i := range to {
		to[i] = LatLon{Lat: float64(i%60) - 30, Lon: float64(i)}
	}
	if _, err := s.Path(LatLon{Lat: 42, Lon: -71}, to[:maxPlaces]); err != nil {
		t.Errorf("64 destinations: %v", err)
	}
	if _, err := s.Path(LatLon{Lat: 42, Lon: -71}, to); err != errTooManyPlaces {
		t.Errorf("65 destinations: %v; want refused", err)
	}
}
