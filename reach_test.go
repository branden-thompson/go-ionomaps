package ionomaps

import (
	"errors"
	"math"
	"testing"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
)

// kansas is the origin the reach tests ask from.
var kansas = LatLon{Lat: 37.7, Lon: -97.3}

// TestAFrequencysReachIsAField is R-2.9 (watchpost D-76, D-175: upper
// limits): a frequency's reach from an origin is a global field a go-tuiMaps
// host draws, 1 where the frequency is under the path's MUF from the origin,
// 0 where it is over it; with foF2 7 and MUF(3000) 21 everywhere, 14 MHz
// skips the origin's area and reaches 3000 km away, and 25 MHz reaches
// nowhere.
func TestAFrequencysReachIsAField(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	r, err := s.Reach(kansas, 14, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	f := r.Field
	if f.Cols != grid.Cols || f.Rows != grid.Rows || len(f.Values) != grid.Cells || f.West != -180 || f.North != 90 {
		t.Fatalf("the reach is not the library's global grid: %d by %d, %d values", f.Cols, f.Rows, len(f.Values))
	}
	if r.Limit != LimitMUF || r.MHz != 14 || !r.Reaches {
		t.Errorf("the reach names %v at %v MHz, reaching %v; want the path MUF at 14 MHz, reaching", r.Limit, r.MHz, r.Reaches)
	}
	near, far := 0, 0
	for i, v := range f.Values {
		if v != 0 && v != 1 {
			t.Fatalf("cell %d is %v; a reach is 1 or 0", i, v)
		}
		lat, lon := grid.Centre(i)
		switch km := limits.Distance(kansas.Lat, kansas.Lon, lat, lon); {
		case km < 300 && v == 1:
			near++
		case km > 2800 && km < 3200 && v == 0 && !f.NoData.Has(i):
			far++
		}
	}
	if near > 0 || far > 0 {
		t.Errorf("14 MHz reaches %d cells within 300 km (foF2 is 7) and misses %d about 3000 km out (MUF(3000) is 21)", near, far)
	}
	none, err := s.Reach(kansas, 25, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range none.Field.Values {
		if v == 1 {
			t.Fatal("25 MHz reaches a cell; the highest MUF is about 21 MHz")
		}
	}
	if none.Reaches {
		t.Error("25 MHz says it reaches somewhere")
	}
}

// TestTheSkipZoneIsReturned is R-2.9: the skip zone's outer edge is how far
// the nearest reached cell is; it grows with the frequency, and a frequency
// under foF2 has none.
func TestTheSkipZoneIsReturned(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	skip := map[float64]float32{}
	for _, mhz := range []float64{5, 10, 14} {
		r, err := s.Reach(kansas, mhz, noonUTC)
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
// 30 MHz, an origin off the globe and an hour the snapshot does not hold are
// refused.
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
		if _, err := s.Reach(c.origin, c.mhz, noonUTC); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := s.Reach(kansas, 14, noonUTC.Add(5*3600e9)); !errors.Is(err, errNoHour) {
		t.Errorf("an hour the snapshot does not hold: %v", err)
	}
}
