// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import (
	"math"
	"sync"
	"testing"
)

const solarW = 0.4

var (
	ccOnce sync.Once
	ccFix  *ClimCoeffs
)

func climCoeffs() *ClimCoeffs {
	ccOnce.Do(func() { ccFix = NewClimCoeffs(7) })
	return ccFix
}

func TestClimAgreement(t *testing.T) {
	cc := climCoeffs()
	g := fixtures()["2deg"].g
	fo, m3 := ClimHourNaive(g, cc, 13, solarW)
	for _, mode := range []int{LegNone, Leg64, Leg32} {
		d := NewClimDay(g, cc, mode)
		var wk ClimWork
		fo2, m32 := make([]float64, len(fo)), make([]float64, len(fo))
		d.Hour(cc, 13, solarW, &wk, fo2, m32)
		maxd, lo, hi := 0.0, math.Inf(1), math.Inf(-1)
		for i := range fo {
			maxd = math.Max(maxd, math.Max(math.Abs(fo[i]-fo2[i]), math.Abs(m3[i]-m32[i])))
			lo, hi = math.Min(lo, fo[i]), math.Max(hi, fo[i])
		}
		t.Logf("mode %d: cache %d bytes, max |naive-fast| %.2e, foF2 range %.2f..%.2f", mode, d.CacheBytes(), maxd, lo, hi)
		tol := 1e-9
		if mode == Leg32 {
			tol = 1e-4
		}
		if maxd > tol {
			t.Fatalf("mode %d differs by %g", mode, maxd)
		}
	}
}

func BenchmarkClimHour(b *testing.B) {
	cc := climCoeffs()
	for _, name := range []string{"2deg", "1deg"} {
		g := fixtures()[name].g
		b.Run(name+"/naive", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ClimHourNaive(g, cc, 13, solarW)
			}
		})
		for _, v := range []struct {
			n    string
			mode int
		}{{"fast-qdonly", LegNone}, {"fast-leg64", Leg64}, {"fast-leg32", Leg32}} {
			d := NewClimDay(g, cc, v.mode)
			n := g.Cols * g.Rows
			fo, m3 := make([]float64, n), make([]float64, n)
			b.Run(name+"/"+v.n, func(b *testing.B) {
				b.ReportAllocs()
				var wk ClimWork
				for b.Loop() {
					d.Hour(cc, 13, solarW, &wk, fo, m3)
				}
			})
		}
	}
}

func BenchmarkClimDayCache(b *testing.B) {
	cc := climCoeffs()
	for _, name := range []string{"2deg", "1deg"} {
		g := fixtures()[name].g
		for _, v := range []struct {
			n    string
			mode int
		}{{"qdonly", LegNone}, {"leg64", Leg64}, {"leg32", Leg32}} {
			b.Run(name+"/"+v.n, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					NewClimDay(g, cc, v.mode)
				}
			})
		}
	}
}

// First open: now plus 24 hours ahead, both fields, outputs kept.
func BenchmarkClimDay25(b *testing.B) {
	cc := climCoeffs()
	for _, name := range []string{"2deg", "1deg"} {
		g := fixtures()[name].g
		b.Run(name+"/naive", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for h := range 25 {
					ClimHourNaive(g, cc, float64(h), solarW)
				}
			}
		})
		for _, v := range []struct {
			n    string
			mode int
		}{{"fast-qdonly", LegNone}, {"fast-leg64", Leg64}, {"fast-leg32", Leg32}} {
			b.Run(name+"/"+v.n, func(b *testing.B) {
				b.ReportAllocs()
				n := g.Cols * g.Rows
				for b.Loop() {
					d := NewClimDay(g, cc, v.mode)
					var wk ClimWork
					for h := range 25 {
						fo, m3 := make([]float64, n), make([]float64, n)
						d.Hour(cc, float64(h), solarW, &wk, fo, m3)
					}
				}
			})
		}
	}
}
