package limits

import (
	"math"
	"testing"
)

// TestTheLimitsRefuseWhatTheirMethodsCannotTake is R-3.3 for the limits:
// each gives no number, never a made-up one, for input outside its method -
// hours since sunset outside a day, no gyrofrequency, a latitude off the
// globe, a part of a path outside it, places opposite each other, a path
// longer than half the globe - and a
// number just inside each edge.
func TestTheLimitsRefuseWhatTheirMethodsCannotTake(t *testing.T) {
	for _, c := range []struct {
		name string
		got  float64
		want bool // whether a number is wanted
	}{
		{"foE, 25 hours since sunset", FoE(40, 0, 120, 150, 25), false},
		{"foE, -1 hours since sunset", FoE(40, 0, 120, 150, -1), false},
		{"foE, 24 hours since sunset", FoE(40, 0, 120, 150, 24), true},
		{"foE, at sunset", FoE(40, 0, 120, 150, 0), true},
		{"the MUF, a negative gyrofrequency", BasicMUF(1000, 7, 3, 3, -0.1), false},
		{"the MUF, no gyrofrequency", BasicMUF(1000, 7, 3, 3, math.NaN()), false},
		{"the MUF, a gyrofrequency of 0", BasicMUF(1000, 7, 3, 3, 0), true},
		{"a distance from 91°N", Distance(91, 0, 0, 0), false},
		{"a distance to 91°S", Distance(0, 0, -91, 0), false},
		{"a distance pole to pole", Distance(90, 0, -90, 0), true},
		{"a place past the end of a path", lat(Along(0, 0, 10, 10, 1.01)), false},
		{"a place before the start of a path", lat(Along(0, 0, 10, 10, -0.01)), false},
		{"the end of a path", lat(Along(0, 0, 10, 10, 1)), true},
		{"a place between antipodes", lat(Along(0, 0, 0, 180, 0.5)), false},
		{"a place on a path nearly round the world", lat(Along(0, 0, 0, 179, 0.5)), true},
		{"a path longer than half the globe", PathMUF(math.Pi*6371+1, mid, mid, mid), false},
		{"a path half the globe long", PathMUF(math.Pi*6371, mid, mid, mid), true},
	} {
		if number := !math.IsNaN(c.got); number != c.want {
			t.Errorf("%s: %v; a number wanted: %v", c.name, c.got, c.want)
		}
	}
}

// mid is a daytime mid-latitude control point.
var mid = Point{FoF2: 7, M3000: 3, FoE: 3, FH: 1}

// lat is the latitude of a place Along gives.
func lat(la, _ float64) float64 { return la }
