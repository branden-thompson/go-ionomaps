package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
	"github.com/branden-thompson/go-ionomaps/ionomapstest"
)

// spread is one residual's distribution over the readings: how many, the
// 99.9th percentile of its size and its largest, MHz or (M(3000)F2) a
// plain number.
type spread struct {
	n             int
	p999, largest float64
}

// spreadOf is the sizes' distribution, nearest rank; zero for none.
func spreadOf(xs []float64) spread {
	if len(xs) == 0 {
		return spread{}
	}
	sort.Float64s(xs)
	rank := int(math.Ceil(0.999*float64(len(xs)))) - 1
	return spread{n: len(xs), p999: xs[max(rank, 0)], largest: xs[len(xs)-1]}
}

// residuals is -residuals: every recorded reading within the hour before
// each grid, its residual against the backgrounds the update assimilates
// it onto - foF2 over GloTEC and over the climatology, M(3000)F2 over the
// climatology - read at its station, and foF2's fold against each
// background, the larger of reading over background and background over
// reading: what the plausibility bounds are set from, so that no reading of
// the recorded days is refused (watchpost D-248, D-279).
func residuals(dir, solarFile string, now time.Time) (string, error) {
	rec, err := ionomapstest.Open(dir, solarFile)
	if err != nil {
		return "", err
	}
	days, err := solar.Parse(rec.Solar(now))
	if err != nil {
		return "", err
	}
	grids, err := readGrids(rec, now)
	if err != nil {
		return "", err
	}
	stations, _ := readStations(rec, grids[0].Valid.Add(-2*time.Hour), grids[len(grids)-1].Valid.Add(time.Hour), now)
	var foG, foC, mC, foldG, foldC []float64
	for i := range grids {
		f107, n, ok := solar.Mean(days, grids[i].Valid)
		if !ok || n < minF107Days {
			continue
		}
		m := momentAt(&grids[i], f107)
		for _, s := range stations {
			r, held := newest(s, m.at)
			fresh := held && m.at.Sub(r.At) <= time.Hour
			if !fresh {
				continue
			}
			g, okG := grid.Sample(m.hybrid.FoF2, s.lat, s.lon)
			c, okC := grid.Sample(m.clim.FoF2, s.lat, s.lon)
			cm, okM := grid.Sample(m.clim.M3000, s.lat, s.lon)
			x := float64(r.FoF2)
			if okG {
				foG = append(foG, math.Abs(x-g))
			}
			if okG && x > 0 && g > 0 {
				foldG = append(foldG, math.Max(x/g, g/x))
			}
			if okC {
				foC = append(foC, math.Abs(x-c))
			}
			if okC && x > 0 && c > 0 {
				foldC = append(foldC, math.Max(x/c, c/x))
			}
			if okM && r.HasM300 {
				mC = append(mC, math.Abs(float64(r.M3000)-cm))
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Residuals at the stations: %d grids, %d stations\n\nTool: gm3 at %s.\n\n| Residual | Readings | 99.9th percentile | Largest |\n|---|---|---|---|\n", len(grids), len(stations), revision())
	for _, row := range []struct {
		name string
		s    spread
	}{{"foF2 over GloTEC, MHz", spreadOf(foG)}, {"foF2 over the climatology, MHz", spreadOf(foC)}, {"M(3000)F2 over the climatology", spreadOf(mC)},
		{"foF2's fold against GloTEC", spreadOf(foldG)}, {"foF2's fold against the climatology", spreadOf(foldC)}} {
		fmt.Fprintf(&b, "| %s | %d | %.2f | %.2f |\n", row.name, row.s.n, row.s.p999, row.s.largest)
	}
	return b.String(), nil
}
