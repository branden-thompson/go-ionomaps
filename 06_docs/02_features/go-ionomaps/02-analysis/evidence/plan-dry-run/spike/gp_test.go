// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import (
	"math"
	"os"
	"testing"
)

// glotecPath is one GloTEC grid, given by GLOTEC_FILE (kept outside the tree); unset, the tests skip.
var glotecPath = os.Getenv("GLOTEC_FILE")

func TestGPAgreement(t *testing.T) {
	for _, name := range []string{"2deg", "1deg"} {
		bg := fixtures()[name].snaps[now]
		a, err := AssimilateNaive(bg, Stations)
		if err != nil {
			t.Fatal(err)
		}
		var w GPWork
		var b Snapshot
		if err := AssimilateFast(bg, Stations, &w, &b); err != nil {
			t.Fatal(err)
		}
		maxd, maxr := 0.0, 0.0
		for i := range a.FoF2 {
			maxd = math.Max(maxd, math.Abs(a.FoF2[i]-b.FoF2[i])+math.Abs(a.M3000[i]-b.M3000[i]))
			maxr = math.Max(maxr, math.Abs(a.FoF2[i]-bg.FoF2[i]))
		}
		t.Logf("%s max |naive-fast| %.2e, max foF2 residual added %.2f MHz", name, maxd, maxr)
		if maxd > 1e-6 {
			t.Fatalf("%s: naive and fast differ by %g", name, maxd)
		}
	}
}

func TestGloTECDecode(t *testing.T) {
	data, err := os.ReadFile(glotecPath)
	if err != nil {
		t.Skip(err)
	}
	var g GloTEC
	if err := DecodeTyped(data, &g); err != nil {
		t.Fatal(err)
	}
	m, err := DecodeGeneric(data)
	if err != nil {
		t.Fatal(err)
	}
	f := g.Features[len(g.Features)-1]
	t.Logf("typed: %d features, time %s, last %v NmF2 %.3g hmF2 %.1f q %d; generic: %d features",
		len(g.Features), g.TimeTag, f.Geometry.Coordinates, f.Properties.NmF2, f.Properties.HmF2, f.Properties.QualityFlag,
		len(m["features"].([]any)))
}

func BenchmarkAssimilate(b *testing.B) {
	for _, name := range []string{"2deg", "1deg"} {
		bg := fixtures()[name].snaps[now]
		b.Run(name+"/naive", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := AssimilateNaive(bg, Stations); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(name+"/fast", func(b *testing.B) {
			b.ReportAllocs()
			var w GPWork
			var out Snapshot
			for b.Loop() {
				if err := AssimilateFast(bg, Stations, &w, &out); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var sinkG GloTEC
var sinkM map[string]any

func BenchmarkGloTECDecode(b *testing.B) {
	data, err := os.ReadFile(glotecPath)
	if err != nil {
		b.Skip(err)
	}
	b.Run("typed", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		for b.Loop() {
			sinkG = GloTEC{}
			if err := DecodeTyped(data, &sinkG); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("generic", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(data)))
		for b.Loop() {
			m, err := DecodeGeneric(data)
			if err != nil {
				b.Fatal(err)
			}
			sinkM = m
		}
	})
}
