package ionomaps_test

import (
	"context"
	"math"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// The typical offset's seed, measured over PLAN's week (watchpost D-105):
// mean -0.5 MHz, one standard deviation 0.225 (the 2-SD band 0.45), counted
// as the week's 56 updates.
const (
	seedMHz    = -0.5
	seedSpread = 0.225
	seedWeight = 56
)

// near is whether a and b agree to the float32 a snapshot carries.
func near(a float32, b float64) bool { return math.Abs(float64(a)-b) < 1e-5 }

// TestTheLiveOffsetIsCarried is R-9.6 (D-105 a): an update with five or more
// readings assimilated over GloTEC carries the live offset, the mean of
// station foF2 minus GloTEC's at the stations; with none, fewer than five,
// or the climatology the background, none is measured. A live offset more
// than two spreads from the typical is said apart (D-105 d).
func TestTheLiveOffsetIsCarried(t *testing.T) {
	l, n, c := giroLibrary(t)
	if first := update(t, l); first.Offset.Measured || first.Offset.Apart() {
		t.Errorf("no readings held, and a live offset is said measured: %+v", first.Offset)
	}
	s := update(t, l) // every station 7.5 MHz over GloTEC's 7
	if !s.Offset.Measured || !near(s.Offset.LiveMHz, 0.5) {
		t.Errorf("40 stations at 7.5 MHz over GloTEC's 7: live offset %+v; want measured, +0.5", s.Offset)
	}
	if !s.Offset.Apart() {
		t.Errorf("a live offset of +0.5 against a typical near -0.5 (spread near 0.23) is not said apart: %+v", s.Offset)
	}
	for _, o := range []struct {
		live  float32
		apart bool
	}{{0.4, true}, {-0.1, false}, {-0.9, false}, {-1, true}} { // the band is 2 x 0.225 either way of -0.5
		if got := (ionomaps.Offset{LiveMHz: o.live, TypicalMHz: -0.5, SpreadMHz: 0.225, Measured: true}).Apart(); got != o.apart {
			t.Errorf("live %v against typical -0.5, spread 0.225: apart %v; want %v", o.live, got, o.apart)
		}
	}
	c.Add(70 * time.Minute) // the grid over an hour old: the climatology is the background
	update(t, l)
	if over := update(t, l); over.Background.FoF2 != ionomaps.Climatology || over.Offset.Measured || over.Offset.TypicalMHz != s.Offset.TypicalMHz {
		t.Errorf("over the climatology a live offset against GloTEC is said measured, or learned from: %v, %+v; was %+v", over.Background.FoF2, over.Offset, s.Offset)
	}
	for _, speakers := range []int{4, 5} {
		l, n, _ = giroLibrary(t)
		n.giro.speakers = speakers
		update(t, l)
		s := update(t, l)
		if want := speakers >= 5; s.Offset.Measured != want || s.Inputs.Stations != speakers {
			t.Errorf("%d readings assimilated (%d): measured %v; want %v", speakers, s.Inputs.Stations, s.Offset.Measured, want)
		}
		if !s.Offset.Measured && (s.Offset.LiveMHz != 0 || !near(s.Offset.TypicalMHz, seedMHz)) {
			t.Errorf("no live offset measured, and one given or learned from: %+v", s.Offset)
		}
	}
}

// TestTheTypicalOffsetLearnsFromUpdates is R-9.6 (D-105 b): the typical
// offset starts at its seed and learns once from each burst's readings,
// never again from the same readings between bursts; a new library starts
// from the seed again (nothing persisted, D-78).
func TestTheTypicalOffsetLearnsFromUpdates(t *testing.T) {
	l, n, c := giroLibrary(t)
	first := update(t, l)
	if !near(first.Offset.TypicalMHz, seedMHz) || !near(first.Offset.SpreadMHz, seedSpread) || first.Offset.Basis == "" {
		t.Errorf("before any reading the typical offset is %+v; want the seed, -0.5 and 0.225, with its basis", first.Offset)
	}
	mean, sum := seedMHz, seedSpread*seedSpread*seedWeight
	learn := func(n int, x float64) {
		d := x - mean
		mean += d / float64(n)
		sum += d * (x - mean)
	}
	learn(seedWeight+1, 0.5)
	s := update(t, l)
	if !near(s.Offset.TypicalMHz, mean) || !near(s.Offset.SpreadMHz, math.Sqrt(sum/(seedWeight+1))) {
		t.Errorf("after one burst at +0.5 the typical offset is %+v; want %.5f, spread %.5f", s.Offset, mean, math.Sqrt(sum/(seedWeight+1)))
	}
	for range 3 {
		c.Add(10 * time.Minute)
		n.list(t, c.Now().Add(-30*time.Minute), 7)
		if again := update(t, l); again.Offset.TypicalMHz != s.Offset.TypicalMHz || !again.Offset.Measured {
			t.Errorf("the same readings over a new grid learned again: %+v, was %+v", again.Offset, s.Offset)
		}
	}
	c.Add(31 * time.Minute)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	update(t, l)
	learn(seedWeight+2, 0.5)
	if next := update(t, l); !near(next.Offset.TypicalMHz, mean) {
		t.Errorf("the next burst's readings: typical %v; want %.5f", next.Offset.TypicalMHz, mean)
	}
	fresh, _, _ := giroLibrary(t)
	if s := update(t, fresh); !near(s.Offset.TypicalMHz, seedMHz) {
		t.Errorf("a new library's typical offset is %v; nothing is persisted, so the seed", s.Offset.TypicalMHz)
	}
}

// TestTheNoReadingsCorrectionIsNamed is R-9.6 (D-105 c): asked for, and with
// no readings assimilated, foF2 is GloTEC's moved by the typical offset,
// MUF(3000) with it, and the snapshot says so; with readings assimilated, or
// over the climatology, it is not applied, and not said.
func TestTheNoReadingsCorrectionIsNamed(t *testing.T) {
	correct := ionomaps.UpdateOptions{CorrectNoReadings: true}
	l, n, _ := giroLibrary(t)
	plain, _, _ := giroLibrary(t)
	s, err := l.Update(context.Background(), correct)
	if err != nil {
		t.Fatal(err)
	}
	as := update(t, plain)
	if !s.Offset.Corrected || as.Offset.Corrected {
		t.Fatalf("corrected %v asked, %v not asked; want true, false", s.Offset.Corrected, as.Offset.Corrected)
	}
	for _, i := range []int{0, 4000, 16199} {
		if !near(s.Hours[0].FoF2.Values[i], 7+seedMHz) || !near(as.Hours[0].FoF2.Values[i], 7) {
			t.Fatalf("cell %d: foF2 %v corrected, %v as is; want 6.5 and 7", i, s.Hours[0].FoF2.Values[i], as.Hours[0].FoF2.Values[i])
		}
		ratio := float64(s.Hours[0].MUF3000.Values[i] / as.Hours[0].MUF3000.Values[i])
		if math.Abs(ratio-6.5/7) > 1e-5 {
			t.Fatalf("cell %d: MUF(3000) moved by %v; foF2 by 6.5/7", i, ratio)
		}
	}
	n.giro.speakers = 2 // two readings held: too few to assimilate, so still none used
	if s, _ := l.Update(context.Background(), correct); !s.Offset.Corrected || s.Inputs.Stations != 0 {
		t.Errorf("two readings, none assimilated: corrected %v, stations %d", s.Offset.Corrected, s.Inputs.Stations)
	}
	l, _, _ = giroLibrary(t)
	l.Update(context.Background(), correct)
	if s, _ := l.Update(context.Background(), correct); s.Offset.Corrected || s.Inputs.Stations != 40 {
		t.Errorf("readings assimilated, and the correction applied: %+v, %d stations", s.Offset, s.Inputs.Stations)
	}
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l) // the typical offset learns +0.5 once
	c.Add(61 * time.Minute)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	n.giro.speak = false // the readings held are now over an hour old, and no new ones come
	l.Update(context.Background(), correct)
	learned := seedMHz + (0.5-seedMHz)/(seedWeight+1)
	if s, _ := l.Update(context.Background(), correct); !s.Offset.Corrected || !near(s.Hours[0].FoF2.Values[0], 7+learned) {
		t.Errorf("after learning: corrected %v, foF2 %v; want GloTEC's 7 moved by the typical offset learned, %.5f", s.Offset.Corrected, s.Hours[0].FoF2.Values[0], learned)
	}
	l, _, _ = library(t)
	if s, err := l.Update(context.Background(), correct); err != nil || s.Background.FoF2 != ionomaps.Climatology || s.Offset.Corrected {
		t.Errorf("over the climatology the correction is said applied: %v, %v, %+v", err, s.Background.FoF2, s.Offset)
	}
}

// TestTogglingTheCorrectionMakesNoRequest is R-9.6 and A-30 (4): the
// correction is the update's own setting, so turning it on or off within
// the ten minutes makes no request, and the answer is the last snapshot
// with the correction as asked, said too soon.
func TestTogglingTheCorrectionMakesNoRequest(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	first, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	n.take()
	giroAsks := len(n.giro.asks)
	c.Add(time.Minute)
	for _, on := range []bool{true, false, true} {
		s, err := l.Update(context.Background(), ionomaps.UpdateOptions{CorrectNoReadings: on})
		if err != nil {
			t.Fatal(err)
		}
		if a := n.take(); len(a) != 0 || len(n.giro.asks) != giroAsks {
			t.Errorf("turning the correction to %v asked %v and %d of GIRO", on, a, len(n.giro.asks)-giroAsks)
		}
		want := 7.0
		if on {
			want += seedMHz
		}
		if s.Offset.Corrected != on || !near(s.Hours[0].FoF2.Values[0], want) || s.Early != ionomaps.TooSoon || !s.Computed.Equal(first.Computed) {
			t.Errorf("turned to %v: corrected %v, foF2 %v, %v, computed %v; want %v, %v, too soon, the last snapshot's time", on, s.Offset.Corrected, s.Hours[0].FoF2.Values[0], s.Early, s.Computed, on, want)
		}
	}
	on, off := ionomaps.UpdateOptions{CorrectNoReadings: true}, ionomaps.UpdateOptions{}
	for _, asked := range [][]ionomaps.UpdateOptions{{off}, {on, off}} { // NOAA's alone, GIRO not due; then NOAA's and the burst's (D-63)
		c.Add(10 * time.Minute)
		if len(asked) == 2 {
			c.Add(time.Hour)
		}
		n.list(t, c.Now().Add(-30*time.Minute), 7)
		for _, o := range asked {
			l.Update(context.Background(), o)
		}
		c.Add(time.Minute)
		if s, _ := l.Update(context.Background(), on); !s.Offset.Corrected {
			t.Errorf("updates asked %v, then one within ten minutes asking the correction: not corrected", asked)
		}
	}
}

// TestTheNoiseIsTheUpdatesOption is D-187: the noise environment the update
// is asked with is the snapshot's; a change within the ten minutes rebuilds
// the last snapshot from what is held, asking nothing.
func TestTheNoiseIsTheUpdatesOption(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{Noise: ionomaps.NoiseCity})
	if err != nil || s.Noise != ionomaps.NoiseCity {
		t.Fatalf("asked for city noise, the snapshot's is %v (%v)", s.Noise, err)
	}
	n.take()
	c.Add(time.Minute)
	s, _ = l.Update(context.Background(), ionomaps.UpdateOptions{Noise: ionomaps.NoiseQuietRural})
	if a := n.take(); len(a) != 0 || s.Noise != ionomaps.NoiseQuietRural || s.Early != ionomaps.TooSoon {
		t.Errorf("the noise changed within ten minutes: asked %v, noise %v, %v", a, s.Noise, s.Early)
	}
}
