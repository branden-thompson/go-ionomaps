package climatology

import (
	"math"
	"testing"
	"time"
)

// TestAnHourMatchesThePointValues: the hour's collapsed coefficients give,
// at every place, what Values gives there - the month blend, the solar
// interpolation and the Fourier sum in UT are linear, so folding them first
// changes nothing but the cost.
func TestAnHourMatchesThePointValues(t *testing.T) {
	refits := Refits()
	for _, at := range []time.Time{time.Date(2026, 10, 9, 13, 30, 0, 0, time.UTC), time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)} {
		for _, f107 := range []float64{70, 150, 260} {
			h := NewHour(refits, at, f107)
			for _, p := range [][2]float64{{-70, 1}, {-12, 7.5}, {0, 12}, {33, 18.25}, {66, 23}, {88, 3}} {
				fo, m3 := Values(refits, at, f107, p[0], p[1])
				gf, gm := h.At(p[0], p[1])
				if math.Abs(fo-gf) > 1e-9 || math.Abs(m3-gm) > 1e-9 {
					t.Errorf("%v, F10.7 %v, QD %v, MLT %v: the hour gives %v, %v; Values %v, %v", at, f107, p[0], p[1], gf, gm, fo, m3)
				}
			}
		}
	}
}

// BenchmarkAnHourAt is a grid's cost: one place of the 16,200 a field holds.
func BenchmarkAnHourAt(b *testing.B) {
	h := NewHour(Refits(), time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC), 150)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		h.At(float64(i%180)-89.5, float64(i%24))
	}
}
