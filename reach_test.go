package ionomaps

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
)

// kansas is the origin the reach tests ask from.
var kansas = LatLon{Lat: 37.7, Lon: -97.3}

// TestAFrequencysReachIsAField is R-2.9 (watchpost D-76, D-205): a
// frequency's reach from an origin is a global field a go-tuiMaps host
// draws, 1 where it reaches and 0 where it does not, naming the limit that
// bounds it; with foF2 7 and MUF(3000) 21 everywhere, 14 MHz skips the
// origin's area - over foF2 there - and 25 MHz, over every path's MUF,
// reaches nowhere, bounded by the MUF.
func TestAFrequencysReachIsAField(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	r, err := s.Reach(kansas, 14)
	if err != nil {
		t.Fatal(err)
	}
	f := r.Field
	if f.Cols != grid.Cols || f.Rows != grid.Rows || len(f.Values) != grid.Cells || f.West != -180 || f.North != 90 {
		t.Fatalf("the reach is not the library's global grid: %d by %d, %d values", f.Cols, f.Rows, len(f.Values))
	}
	if r.Limit == LimitNone || r.MHz != 14 || !r.Reaches {
		t.Errorf("the reach names %v at %v MHz, reaching %v; want a limit at 14 MHz, reaching", r.Limit, r.MHz, r.Reaches)
	}
	near := 0
	for i, v := range f.Values {
		if v != 0 && v != 1 {
			t.Fatalf("cell %d is %v; a reach is 1 or 0", i, v)
		}
		lat, lon := grid.Centre(i)
		if km := limits.Distance(kansas.Lat, kansas.Lon, lat, lon); km < 300 && v == 1 {
			near++
		}
	}
	if near > 0 {
		t.Errorf("14 MHz reaches %d cells within 300 km; foF2 is 7", near)
	}
	none, err := s.Reach(kansas, 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range none.Field.Values {
		if v == 1 {
			t.Fatal("25 MHz reaches a cell; the highest MUF is about 21 MHz")
		}
	}
	if none.Reaches || none.Limit != LimitMUF {
		t.Errorf("25 MHz: reaches %v, bounded by %v; want nowhere, by the MUF", none.Reaches, none.Limit)
	}
}

// TestTheSkipZoneIsReturned is R-2.9: the skip zone's outer edge is how far
// the nearest reached cell is; it grows with the frequency, and a frequency
// under foF2 has none.
func TestTheSkipZoneIsReturned(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	skip := map[float64]float32{}
	for _, mhz := range []float64{5, 10, 14} {
		r, err := s.Reach(kansas, mhz)
		if err != nil {
			t.Fatal(err)
		}
		skip[mhz] = r.SkipKm
	}
	if skip[5] != 0 {
		t.Errorf("5 MHz, under foF2 7, has a skip zone %v km out", skip[5])
	}
	if !(skip[10] > 0 && skip[10] < skip[14] && skip[14] > 500 && skip[14] < 2500) {
		t.Errorf("skip zones: 10 MHz %v km, 14 MHz %v km; each grows with the frequency, 14 MHz between 500 and 2500 km", skip[10], skip[14])
	}
}

// TestAReachRefusesBadInput is R-3.5 for Reach: a frequency outside 1.8 to
// 30 MHz, an origin off the globe and a snapshot with no field are refused.
func TestAReachRefusesBadInput(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	for _, c := range []struct {
		name   string
		origin LatLon
		mhz    float64
		want   error
	}{
		{"1.7 MHz", kansas, 1.7, errBadFrequency},
		{"30.1 MHz", kansas, 30.1, errBadFrequency},
		{"NaN MHz", kansas, math.NaN(), errBadFrequency},
		{"an origin off the globe", LatLon{Lat: 91, Lon: 0}, 14, errBadPlace},
	} {
		if _, err := s.Reach(c.origin, c.mhz); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := (Snapshot{Computed: noonUTC}).Reach(kansas, 14); !errors.Is(err, errNoField) {
		t.Errorf("a snapshot with no field: %v", err)
	}
}

// TestTheReachJudgesEachCellAsTheChartJudgesABand is R-2.9 and watchpost
// D-205: a frequency reaches a cell only under the path's MUF and over its
// lower limit at the reference circuit - the same judgment Path gives a
// band at that frequency. At local midnight 40 m's top, 7.3 MHz, is checked
// cell by cell against Path's 40 m status, far and near, closed and open.
func TestTheReachJudgesEachCellAsTheChartJudgesABand(t *testing.T) {
	s := flatSnapshot(noonUTC.Add(12*time.Hour), 7, 21)
	origin := LatLon{Lat: 45, Lon: 0} // local midnight
	r, err := s.Reach(origin, Band40m.HighMHz())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[Status]int{}
	for i := 0; i < grid.Cells; i += 97 {
		lat, lon := grid.Centre(i)
		cell := LatLon{Lat: lat, Lon: lon}
		if limits.Distance(origin.Lat, origin.Lon, lat, lon) < 2*nearKm || r.Field.NoData.Has(i) {
			continue
		}
		p, err := s.Path(origin, []LatLon{cell})
		if err != nil {
			t.Fatal(err)
		}
		st := statusOf(p[0].Bands, Band40m)
		seen[st]++
		if reached := r.Field.Values[i] == 1; reached != (st == Open) {
			t.Errorf("cell %d (%.0f, %.0f): reached %v; Path says 40 m is %v", i, lat, lon, reached, st)
		}
	}
	if seen[Open] == 0 || seen[Absorbed]+seen[Weak] == 0 {
		t.Errorf("the cells checked were %v; the check needs both open and closed cells", seen)
	}
}

// TestTheReachSaysWhichLimitStopsIt is watchpost D-205: the reach names the
// limit that bounds it - the MUF where the lower limit closes no cell under
// it, else the lower limit that closes the most - so a host says which; at
// local noon 160 m reaches nowhere for the lower limit, not the MUF, and at
// local midnight it reaches.
func TestTheReachSaysWhichLimitStopsIt(t *testing.T) {
	origin := LatLon{Lat: 45, Lon: 0}
	day, err := flatSnapshot(noonUTC, 7, 21).Reach(origin, 1.9)
	if err != nil {
		t.Fatal(err)
	}
	if day.Reaches || day.SkipKm != 0 || (day.Limit != LimitAbsorption && day.Limit != LimitCircuit) {
		t.Errorf("1.9 MHz at local noon: reaches %v (skip %v km), bounded by %v; want nowhere, by the lower limit", day.Reaches, day.SkipKm, day.Limit)
	}
	night, err := flatSnapshot(noonUTC.Add(12*time.Hour), 7, 21).Reach(origin, 1.9)
	if err != nil {
		t.Fatal(err)
	}
	if !night.Reaches {
		t.Errorf("1.9 MHz at local midnight reaches nowhere (bounded by %v)", night.Limit)
	}
	high, err := flatSnapshot(noonUTC.Add(12*time.Hour), 7, 21).Reach(origin, 25)
	if err != nil {
		t.Fatal(err)
	}
	if high.Reaches || high.Limit != LimitMUF {
		t.Errorf("25 MHz: reaches %v, bounded by %v; want nowhere, by the MUF", high.Reaches, high.Limit)
	}
}

// TestTheBoundIsTheLimitThatClosesTheMore is watchpost D-205: the MUF where
// the lower limit closed no cell under it; else absorption or the circuit,
// whichever closed the more, a tie to absorption.
func TestTheBoundIsTheLimitThatClosesTheMore(t *testing.T) {
	for _, c := range []struct {
		absorbed, weak int
		want           Limit
	}{
		{0, 0, LimitMUF},
		{3, 1, LimitAbsorption},
		{1, 3, LimitCircuit},
		{2, 2, LimitAbsorption},
		{0, 1, LimitCircuit},
	} {
		if got := reachBound(c.absorbed, c.weak); got != c.want {
			t.Errorf("%d absorbed, %d weak: bounded by %v; want %v", c.absorbed, c.weak, got, c.want)
		}
	}
}
