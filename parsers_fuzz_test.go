package ionomaps

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/drap"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/scales"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
)

// FuzzEveryInputParser is R-3.1 and R-3.3: whatever a source sends, no
// parser panics, and none hands on a value that is no number, nor a scale
// outside its bounds (R-2.10).
func FuzzEveryInputParser(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("#\n2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //\n"))
	f.Add([]byte(`{"time_tag":"2026-10-05T12:05:00Z","features":[]}`))
	f.Add([]byte("2026 10 05  150  140  1  2  0  0  0  0  0  0  0\n"))
	f.Add([]byte("# Product Valid At : 2026-10-05 22:18 UTC\n 89 |  0.0\n"))
	f.Add([]byte(`{"0":{"DateStamp":"2026-10-05","TimeStamp":"12:00:00","R":{"Scale":"1"},"S":{"Scale":null},"G":{"Scale":"0"}},"1":{"DateStamp":"2026-10-05","R":{"MinorProb":"20"},"G":{"Scale":"1"}}}`))
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, body []byte) {
		finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
		if r, err := giro.ParseReply(body, "XX00X", now.Add(-24*time.Hour), now, now); err == nil {
			for _, x := range r.Readings {
				if !finite(float64(x.FoF2)) || !finite(float64(x.MUF3000)) || !finite(float64(x.M3000)) {
					t.Fatalf("GIRO: a reading that is no number: %+v", x)
				}
			}
		}
		if g, err := glotec.Decode(body, now); err == nil {
			for _, v := range g.FoF2 {
				if !finite(float64(v)) {
					t.Fatal("GloTEC: a cell that is no number")
				}
			}
		}
		if days, err := solar.Parse(body); err == nil {
			for _, d := range days {
				if !finite(d.F107) {
					t.Fatalf("the solar indices: a day that is no number: %+v", d)
				}
			}
		}
		if g, err := drap.ParseTable(body, now); err == nil {
			for _, v := range g.MHz {
				if !finite(float64(v)) {
					t.Fatal("D-RAP: a cell that is no number")
				}
			}
		}
		if l, err := scales.Read(body, now); err == nil {
			inBounds := func(v, top int) bool { return v == scales.None || (v >= 0 && v <= top) }
			if !inBounds(l.R, 5) || !inBounds(l.S, 5) || !inBounds(l.G, 5) {
				t.Fatalf("the scales: a level outside 0 to 5: %+v", l)
			}
			for _, d := range l.Days {
				if !inBounds(d.RMinorPct, 100) || !inBounds(d.RMajorPct, 100) || !inBounds(d.SPct, 100) || !inBounds(d.G, 5) {
					t.Fatalf("the scales: an outlook value outside its bounds: %+v", d)
				}
			}
		}
		glotec.Newest(body, now)
	})
}
