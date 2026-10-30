// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package tmbench

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	tuimaps "github.com/branden-thompson/go-tuimaps"
)

var valid = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// foF2 is a synthetic whole-globe field on a 2-degree grid, 180 x 91 values,
// smooth from 2 to 14 (MHz-like): an equatorial-anomaly-ish shape plus a
// longitudinal day/night swing.
func foF2(lines bool) tuimaps.Overlay {
	const cols, rows = 180, 91
	g := tuimaps.Grid{West: -180, South: -90, East: 180, North: 90, Cols: cols, Rows: rows, Lines: lines}
	g.Values = make([]float64, 0, cols*rows)
	for r := range rows {
		lat := 90 - float64(r)*2
		for c := range cols {
			lon := -180 + float64(c)*2
			day := 0.5 + 0.5*math.Cos((lon-30)*math.Pi/180)
			crest := math.Exp(-math.Pow((math.Abs(lat)-15)/12, 2))
			polar := math.Cos(lat * math.Pi / 180)
			v := 2 + 12*day*(0.45*polar+0.55*crest)
			g.Values = append(g.Values, v)
		}
	}
	// The temperature preset's ramp, with foF2-like breaks every 1 MHz:
	// 11 breaks, 12 classes, so 11 contour levels.
	g.Type = tuimaps.Type{Preset: "temperature", Unit: "C", Breaks: []float64{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}}
	return tuimaps.Overlay{ID: "fof2", Valid: valid, Keeps: time.Hour, Grid: &g}
}

func globe(tb testing.TB, cols, rows int, lines bool) *tuimaps.Map {
	tb.Helper()
	m, err := tuimaps.New(tuimaps.WithSize(cols, rows)) // no Embed, no Source: no basemap tiles at all
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { m.Close() })
	m.ColourDepth(tuimaps.Truecolor)
	if err := m.FitWorld(); err != nil {
		tb.Fatal(err)
	}
	if _, err := m.Set(foF2(lines)); err != nil {
		tb.Fatal(err)
	}
	if _, err := m.Settle(context.Background()); err != nil {
		tb.Fatal(err)
	}
	if _, err := m.Render(tuimaps.Size{Cols: cols, Rows: rows}, valid); err != nil {
		tb.Fatal(err)
	}
	return m
}

var sizes = [][2]int{{200, 56}, {400, 110}}

func BenchmarkGlobe(b *testing.B) {
	for _, s := range sizes {
		for _, lines := range []bool{false, true} {
			name := "fill"
			if lines {
				name = "fill+contours"
			}
			tag := fmt.Sprintf("%dx%d/%s", s[0], s[1], name)
			b.Run("pan/"+tag, func(b *testing.B) {
				m := globe(b, s[0], s[1], lines)
				size := tuimaps.Size{Cols: s[0], Rows: s[1]}
				b.ReportAllocs()
				for i := 0; b.Loop(); i++ {
					step := 1
					if i%2 == 1 {
						step = -1
					}
					if err := m.PanCells(step, 0); err != nil {
						b.Fatal(err)
					}
					if _, err := m.Render(size, valid); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("same/"+tag, func(b *testing.B) {
				m := globe(b, s[0], s[1], lines)
				size := tuimaps.Size{Cols: s[0], Rows: s[1]}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := m.Render(size, valid); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// TestLook checks the frames really carry the field and contour labels.
func TestLook(t *testing.T) {
	for _, lines := range []bool{false, true} {
		m := globe(t, 200, 56, lines)
		if err := m.PanCells(1, 0); err != nil {
			t.Fatal(err)
		}
		f, err := m.Render(tuimaps.Size{Cols: 200, Rows: 56}, valid)
		if err != nil {
			t.Fatal(err)
		}
		all := strings.Join(f.Lines, "\n")
		digits := 0
		for _, r := range stripEsc(all) {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		t.Logf("lines=%v rows=%d bytes=%d bg-escapes=%d digits=%d warnings=%v", lines, len(f.Lines), len(all), strings.Count(all, "48;2;"), digits, m.Warnings())
		if testing.Verbose() && lines {
			for _, l := range f.Lines[:20] {
				t.Log(stripEsc(l))
			}
		}
	}
}

func stripEsc(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}
