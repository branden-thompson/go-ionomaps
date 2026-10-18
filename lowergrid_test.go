package ionomaps

import (
	"flag"
	"fmt"
	"testing"
	"time"
)

// measureLower runs the polar grid's full accuracy measurement and prints
// its figures, the ones the build log records (watchpost D-227).
var measureLower = flag.Bool("measure-lower", false, "measure the reach's polar grid against each cell's own circuit and print the figures")

// lowerCase is one reach the grid is measured on.
type lowerCase struct {
	at     time.Time
	origin LatLon
	mhz    float64
}

// lowerCases are the reaches measured: the quick set the test holds, or with
// -measure-lower every six hours from five origins on seven frequencies.
func lowerCases(full bool) []lowerCase {
	origins, freqs, hours := []LatLon{kansas, {Lat: 45, Lon: 0}}, []float64{3.6, 14.2}, []int{0, 12}
	if full {
		origins = []LatLon{kansas, {Lat: 45, Lon: 0}, {Lat: -33.9, Lon: 151.2}, {Lat: 61, Lon: -150}, {Lat: 1.3, Lon: 103.8}}
		freqs, hours = []float64{1.9, 3.6, 7.2, 10.1, 14.2, 18.1, 21.2}, []int{0, 6, 12, 18}
	}
	var out []lowerCase
	for _, h := range hours {
		for _, o := range origins {
			for _, f := range freqs {
				out = append(out, lowerCase{noonUTC.Add(time.Duration(h) * time.Hour), o, f})
			}
		}
	}
	return out
}

// TestTheReachsLowerLimitIsHeldToTheCircuit is watchpost D-227: the reach
// reads its lower limit from a polar grid around the origin, interpolated,
// and is held to each cell's own circuit - at most 1 % of the reached cells
// differing (the full measurement: 0.65 %), the same verdict on whether it
// reaches and on its limit - and is cheaper for it.
func TestTheReachsLowerLimitIsHeldToTheCircuit(t *testing.T) {
	var differ, reached int
	var polar, exact time.Duration
	for _, c := range lowerCases(*measureLower) {
		s := flatSnapshot(c.at, 7, 21)
		s.typical = &typicalDays{}
		start := time.Now()
		p, errP := s.reach(c.origin, c.mhz, true)
		polar += time.Since(start)
		start = time.Now()
		e, errE := s.reach(c.origin, c.mhz, false)
		exact += time.Since(start)
		if errP != nil || errE != nil {
			t.Fatal(errP, errE)
		}
		for i, v := range e.Field.Values {
			if v != p.Field.Values[i] {
				differ++
			}
			if v == 1 {
				reached++
			}
		}
		if p.Reaches != e.Reaches || p.Limit != e.Limit {
			t.Errorf("%v from %v at %.1f MHz: reaches %v, bounded by %v; its own circuits say %v, %v", c.at.Format("15:04"), c.origin, c.mhz, p.Reaches, p.Limit, e.Reaches, e.Limit)
		}
	}
	if reached == 0 || 100*differ > reached {
		t.Errorf("%d cells differ of %d reached; at most 1 %%", differ, reached)
	}
	if *measureLower {
		fmt.Printf("D-227 polar grid: %d of %d reached cells differ (%.2f %%); polar %v, own circuits %v (%.1fx)\n",
			differ, reached, 100*float64(differ)/float64(reached), polar, exact, float64(exact)/float64(polar))
	}
}
