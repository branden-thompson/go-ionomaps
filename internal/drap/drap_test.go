package drap

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

var now = time.Date(2026, 10, 6, 22, 21, 0, 0, time.UTC)

// table is a D-RAP file in the layout NOAA's server writes, built here
// (R-4.3): valid at the time given, each point's frequency from cell.
func table(valid string, cell func(lat, lon int) string) []byte {
	var b strings.Builder
	b.WriteString("# DRAP Tabular Values\n# Product: D-Region Absorption\n# Product Valid At : " + valid + "\n#\n")
	b.WriteString("#  X-RAY Message : Normal X-ray Background\n#\n# Frequency (MHz) as a function of Latitude and Longitude\n#\n    ")
	for lon := -178; lon <= 178; lon += 4 {
		fmt.Fprintf(&b, " %4d", lon)
	}
	b.WriteString("\n" + strings.Repeat("-", 456) + "\n")
	for lat := 89; lat >= -89; lat -= 2 {
		fmt.Fprintf(&b, "%3d |", lat)
		for lon := -178; lon <= 178; lon += 4 {
			b.WriteString(" " + cell(lat, lon))
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// byLon is a table whose frequency rises with longitude: lon/20 + 10 MHz,
// so between two points it is linear, and across the antimeridian it jumps.
func byLon(lat, lon int) string { return fmt.Sprintf("%4.1f", float64(lon)/20+10) }

// TestDRAPGridIsTwoByFour is G2.3 (from the measured file's layout, PLAN's
// dry run): D-RAP's 2° by 4° points come onto the library's 2° cells, rows
// as they are, each column linear between the two points either side of
// it, across the antimeridian too; the valid time is the file's own.
func TestDRAPGridIsTwoByFour(t *testing.T) {
	g, err := ParseTable(table("2026-10-06 22:18 UTC", byLon), now)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Valid.Equal(time.Date(2026, 10, 6, 22, 18, 0, 0, time.UTC)) || len(g.MHz) != grid.Cells {
		t.Fatalf("valid %v, %d cells", g.Valid, len(g.MHz))
	}
	for _, c := range []struct {
		lat, lon float64
		want     float64
	}{
		{89, -177, -177.0/20 + 10},                           // a quarter of the way from -178 to -174
		{-89, 1, 1.0/20 + 10},                                // between -2 and 2
		{45, 177, 0.25*(174.0/20+10) + 0.75*(178.0/20+10)},   // three quarters of the way from 174 to 178
		{45, -179, 0.25*(178.0/20+10) + 0.75*(-178.0/20+10)}, // across the antimeridian: 3° from 178, 1° from -178
	} {
		row, col := int((89-c.lat)/2), int((c.lon+179)/2)
		if got := float64(g.MHz[row*grid.Cols+col]); math.Abs(got-c.want) > 1e-4 {
			t.Errorf("%v, %v: %.4f MHz; want %.4f", c.lat, c.lon, got, c.want)
		}
	}
	lat := func(r int) string { return fmt.Sprintf("%4.1f", float64(r+89)/10) } // each row its own value
	g, _ = ParseTable(table("2026-10-06 22:18 UTC", func(l, _ int) string { return lat(l) }), now)
	if g.MHz[0] != 17.8 || g.MHz[grid.Cells-1] != 0 {
		t.Errorf("the first row %v, the last %v; want 89°N's 17.8 and 89°S's 0", g.MHz[0], g.MHz[grid.Cells-1])
	}
}

// TestAChangedDRAPIsRefused is R-3.1 and R-3.3: a file that is not D-RAP's
// layout, holds a frequency past its bounds or no number, has no valid time
// or one in the future, or is larger than any D-RAP file, is refused, and
// the error never quotes it (R-3.4).
func TestAChangedDRAPIsRefused(t *testing.T) {
	good := table("2026-10-06 22:18 UTC", byLon)
	for name, body := range map[string][]byte{
		"no valid time":         bytes.Replace(good, []byte("Product Valid At"), []byte("Product Made At"), 1),
		"a valid time unread":   table("2026-13-06 22:18 UTC", byLon),
		"a valid time ahead":    table("2026-10-06 22:30 UTC", byLon),
		"a frequency past 40":   table("2026-10-06 22:18 UTC", func(lat, lon int) string { return map[bool]string{true: "41.0", false: " 1.0"}[lat == 1 && lon == 2] }),
		"a frequency below 0":   table("2026-10-06 22:18 UTC", func(lat, lon int) string { return map[bool]string{true: "-0.1", false: " 1.0"}[lat == 1 && lon == 2] }),
		"a frequency no number": table("2026-10-06 22:18 UTC", func(lat, lon int) string { return map[bool]string{true: " NaN", false: " 1.0"}[lat == 1 && lon == 2] }),
		"a row short":           bytes.Replace(good, []byte(" 18.9\n"), []byte("\n"), 1),
		"a row missing":         bytes.Replace(good, good[bytes.Index(good, []byte("\n  1 |"))+1:bytes.Index(good, []byte("\n -1 |"))+1], nil, 1),
		"rows out of order":     bytes.Replace(good, []byte("\n  1 |"), []byte("\n  3 |"), 1),
		"longitudes changed":    bytes.Replace(good, []byte(" -178 "), []byte(" -180 "), 1),
		"cut short":             good[:bytes.Index(good, []byte("\n-89 |"))+1],
		"no longitudes":         bytes.Replace(good, good[bytes.Index(good, []byte("\n     -178"))+1:bytes.Index(good, []byte("\n---"))+1], nil, 1),
		"a longitude missing":   bytes.Replace(good, []byte("  178\n"), []byte("\n"), 1),
		"a frequency not read":  table("2026-10-06 22:18 UTC", func(lat, lon int) string { return map[bool]string{true: " x.x", false: " 1.0"}[lat == 1 && lon == 2] }),
		"too large":             append(bytes.Clone(good), bytes.Repeat([]byte("#\n"), maxBytes)...),
		"empty":                 nil,
	} {
		_, err := ParseTable(body, now)
		if err == nil {
			t.Errorf("%s: read", name)
			continue
		}
		if strings.Contains(err.Error(), "41.0") || strings.Contains(err.Error(), "NaN") || strings.Contains(err.Error(), "2026") {
			t.Errorf("%s: the error quotes the file: %v", name, err)
		}
	}
}

// FuzzDRAPParser is R-3.1 and R-3.3: whatever arrives, the parser never
// panics, and a table it reads is the library's grid of frequencies from
// 0 to 40 MHz.
func FuzzDRAPParser(f *testing.F) {
	f.Add(table("2026-10-06 22:18 UTC", byLon))
	f.Add([]byte("# Product Valid At : 2026-10-06 22:18 UTC\n 89 | 1.0\n"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, body []byte) {
		g, err := ParseTable(body, now)
		if err != nil {
			return
		}
		if len(g.MHz) != grid.Cells {
			t.Fatalf("%d cells", len(g.MHz))
		}
		for _, v := range g.MHz {
			if !(v >= 0 && v <= 40) {
				t.Fatalf("a cell of %v MHz", v)
			}
		}
	})
}
