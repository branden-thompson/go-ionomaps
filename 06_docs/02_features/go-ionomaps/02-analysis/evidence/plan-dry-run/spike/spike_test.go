// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import (
	"fmt"
	"sync"
	"testing"
)

const (
	oLat, oLon = 39.7, -105.0 // origin
	freq       = 14.0
	now        = 18
)

type fixture struct {
	g     *Grid
	snaps []*Snapshot
}

var (
	fixOnce sync.Once
	fix     map[string]*fixture
)

func fixtures() map[string]*fixture {
	fixOnce.Do(func() {
		fix = map[string]*fixture{}
		for name, step := range map[string]float64{"2deg": 2, "1deg": 1} {
			g := NewGrid(step)
			g.Precompute()
			f := &fixture{g: g}
			for h := range 24 {
				f.snaps = append(f.snaps, Synth(g, h))
			}
			fix[name] = f
		}
	})
	return fix
}

func TestAgreement(t *testing.T) {
	for _, name := range []string{"2deg", "1deg"} {
		f := fixtures()[name]
		for _, fr := range []float64{3.5, 7, 14, 21, 28} {
			a := ReachNaive(f.snaps[now], oLat, oLon, fr)
			var b ReachResult
			ReachFast(f.snaps[now], oLat, oLon, fr, &b)
			diff, on := 0, 0
			for i := range a.Reached {
				if a.Reached[i] != b.Reached[i] {
					diff++
				}
				if a.Reached[i] {
					on++
				}
			}
			t.Logf("%s %.1f MHz: cells %d reached %d skip %.0f max %.0f diff %d", name, fr, len(a.Reached), on, a.SkipKm, a.MaxKm, diff)
			if float64(diff) > 0.001*float64(len(a.Reached)) {
				t.Fatalf("%s %.1f: %d cells differ", name, fr, diff)
			}
		}
		bn := BandsNaive(f.snaps, now, oLat, oLon, 400, true)
		bf := BandsFast(f.snaps, now, oLat, oLon, 400, true)
		if bn != bf {
			t.Errorf("%s bands differ:\n%v\n%v", name, bn, bf)
		}
		pn := PathNaive(f.snaps, now, oLat, oLon, true)
		pf := PathFast(f.snaps, now, oLat, oLon, true)
		if pn != pf {
			t.Errorf("%s path differ", name)
		}
		cn, cf := CentreNaive(f.snaps, now, oLat, oLon), CentreFast(f.snaps, now, oLat, oLon)
		if cn.Bands != cf.Bands {
			t.Errorf("%s centre differ", name)
		}
		t.Logf("%s bands open-now %v hours %x", name, bf.OpenFrac, bf.Hours)
	}
}

func BenchmarkPrecompute(b *testing.B) {
	for _, name := range []string{"2deg", "1deg"} {
		g := fixtures()[name].g
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				g.Precompute()
			}
		})
	}
}

func BenchmarkReach(b *testing.B) {
	for _, name := range []string{"2deg", "1deg"} {
		s := fixtures()[name].snaps[now]
		b.Run(name+"/naive", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = ReachNaive(s, oLat, oLon, freq)
			}
		})
		b.Run(name+"/fast", func(b *testing.B) {
			b.ReportAllocs()
			var out ReachResult
			for b.Loop() {
				ReachFast(s, oLat, oLon, freq, &out)
			}
		})
	}
}

func benchPair(b *testing.B, naive, fast func()) {
	b.Run("naive", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			naive()
		}
	})
	b.Run("fast", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			fast()
		}
	})
}

var sinkB BandResult
var sinkP PathResult
var sinkC CentreResult

func BenchmarkBandsAreaNow(b *testing.B) {
	sn := fixtures()["2deg"].snaps
	benchPair(b,
		func() { sinkB = BandsNaive(sn, now, oLat, oLon, 400, false) },
		func() { sinkB = BandsFast(sn, now, oLat, oLon, 400, false) })
}

func BenchmarkBandsAreaDay(b *testing.B) {
	sn := fixtures()["2deg"].snaps
	benchPair(b,
		func() { sinkB = BandsNaive(sn, now, oLat, oLon, 400, true) },
		func() { sinkB = BandsFast(sn, now, oLat, oLon, 400, true) })
}

func BenchmarkPathDay(b *testing.B) {
	sn := fixtures()["2deg"].snaps
	benchPair(b,
		func() { sinkP = PathNaive(sn, now, oLat, oLon, true) },
		func() { sinkP = PathFast(sn, now, oLat, oLon, true) })
}

func BenchmarkCentre(b *testing.B) {
	sn := fixtures()["2deg"].snaps
	benchPair(b,
		func() { sinkC = CentreNaive(sn, now, oLat, oLon) },
		func() { sinkC = CentreFast(sn, now, oLat, oLon) })
}

// Keypress (frequency change): Reach + Centre.
func BenchmarkKeypress(b *testing.B) {
	for _, name := range []string{"2deg", "1deg"} {
		sn := fixtures()[name].snaps
		var out ReachResult
		b.Run(name, func(b *testing.B) {
			benchPair(b,
				func() {
					_ = ReachNaive(sn[now], oLat, oLon, freq)
					sinkC = CentreNaive(sn, now, oLat, oLon)
				},
				func() {
					ReachFast(sn[now], oLat, oLon, freq, &out)
					sinkC = CentreFast(sn, now, oLat, oLon)
				})
		})
	}
}

// Hour step: Reach + BandsArea now + Path now + Centre.
func BenchmarkHourStep(b *testing.B) {
	for _, name := range []string{"2deg", "1deg"} {
		sn := fixtures()[name].snaps
		var out ReachResult
		b.Run(name, func(b *testing.B) {
			benchPair(b,
				func() {
					_ = ReachNaive(sn[now], oLat, oLon, freq)
					sinkB = BandsNaive(sn, now, oLat, oLon, 400, false)
					sinkP = PathNaive(sn, now, oLat, oLon, false)
					sinkC = CentreNaive(sn, now, oLat, oLon)
				},
				func() {
					ReachFast(sn[now], oLat, oLon, freq, &out)
					sinkB = BandsFast(sn, now, oLat, oLon, 400, false)
					sinkP = PathFast(sn, now, oLat, oLon, false)
					sinkC = CentreFast(sn, now, oLat, oLon)
				})
		})
	}
}

var _ = fmt.Sprint
