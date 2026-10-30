// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import "math"

// "Before": straightforward first-draft code. Per-cell trig recomputed on
// every call, zenith from degrees, a fresh result and control-point slice per
// call/path, nothing hoisted out of the band loop.

func controlPointsNaive(p1, p2 [3]float64, delta, d float64) []ctrl {
	sd := math.Sin(delta)
	if d <= 4000 {
		a := math.Sin(delta/2) / sd
		return []ctrl{slerp(p1, p2, a, a)}
	}
	t := 2000 / d
	a := math.Sin((1-t)*delta) / sd
	b := math.Sin(t*delta) / sd
	return []ctrl{slerp(p1, p2, a, b), slerp(p1, p2, b, a)}
}

// pathNaive evaluates the full path (no early exit, worst-case cost).
func pathNaive(s *Snapshot, f, d float64, cps []ctrl) bool {
	if len(cps) == 1 {
		c := cps[0]
		fo, m := s.Interp(c.latD, c.lonD)
		cz := s.zenithNaive(c.latD, c.lonD)
		fe := foE(cz)
		muf := basicMUF(fo, m, fe, d)
		loss := absorptionLoss(f, fe, cz, d)
		return f <= muf && loss <= lossMax
	}
	muf, loss := math.Inf(1), 0.0
	for _, c := range cps {
		fo, m := s.Interp(c.latD, c.lonD)
		cz := s.zenithNaive(c.latD, c.lonD)
		fe := foE(cz)
		muf = math.Min(muf, basicMUF(fo, m, fe, 4000))
		loss += absorptionLoss(f, fe, cz, d) / float64(len(cps))
	}
	return f <= muf && loss <= lossMax
}

func ReachNaive(s *Snapshot, oLatD, oLonD, f float64) ReachResult {
	g := s.G
	out := ReachResult{Cols: g.Cols, Rows: g.Rows, Reached: make([]bool, g.Cols*g.Rows), SkipKm: math.Inf(1)}
	p1 := unitVec(oLatD, oLonD)
	for r := range g.Rows {
		for c := range g.Cols {
			p2 := unitVec(g.latD[r], g.lonD[c])
			delta := math.Acos(clamp(dot(p1, p2), -1, 1))
			d := delta * earthR
			if d < 1 || math.Sin(delta) < 1e-6 {
				continue
			}
			cps := controlPointsNaive(p1, p2, delta, d)
			if pathNaive(s, f, d, cps) {
				out.Reached[r*g.Cols+c] = true
				out.SkipKm = math.Min(out.SkipKm, d)
				out.MaxKm = math.Max(out.MaxKm, d)
			}
		}
	}
	return out
}

func samplePointsNaive(latD, lonD, radiusKm float64) [][2]float64 {
	pts := [][2]float64{{latD, lonD}}
	if radiusKm <= 0 {
		return pts
	}
	for i, n := range ringCounts {
		rk := radiusKm * float64(i+1) / 3
		for j := range n {
			la, lo := dest(latD, lonD, 2*math.Pi*float64(j)/float64(n), rk)
			pts = append(pts, [2]float64{la, lo})
		}
	}
	return pts
}

func BandsNaive(snaps []*Snapshot, now int, latD, lonD, radiusKm float64, allDay bool) BandResult {
	pts := samplePointsNaive(latD, lonD, radiusKm)
	var res BandResult
	for h := range 24 {
		if !allDay && h != now {
			continue
		}
		s := snaps[h]
		for bi, f := range Bands {
			open := 0
			for _, p := range pts {
				ok := false
				for _, d := range refDist {
					fo, m := s.Interp(p[0], p[1])
					cz := s.zenithNaive(p[0], p[1])
					fe := foE(cz)
					muf := basicMUF(fo, m, fe, d)
					loss := absorptionLoss(f, fe, cz, d)
					if f <= muf && loss <= lossMax {
						ok = true
					}
				}
				if ok {
					open++
				}
			}
			if open > 0 {
				res.Hours[bi] |= 1 << h
			}
			if h == now {
				res.OpenFrac[bi] = float64(open) / float64(len(pts))
			}
		}
	}
	return res
}

func PathNaive(snaps []*Snapshot, now int, oLatD, oLonD float64, allDay bool) PathResult {
	var res PathResult
	for h := range 24 {
		if !allDay && h != now {
			continue
		}
		s := snaps[h]
		for ti, t := range Targets {
			for bi, f := range Bands {
				p1 := unitVec(oLatD, oLonD)
				p2 := unitVec(t[0], t[1])
				delta := math.Acos(clamp(dot(p1, p2), -1, 1))
				d := delta * earthR
				cps := controlPointsNaive(p1, p2, delta, d)
				if pathNaive(s, f, d, cps) {
					res.Hours[ti][bi] |= 1 << h
					if h == now {
						res.Now[ti][bi] = true
					}
				}
			}
		}
	}
	return res
}

func CentreNaive(snaps []*Snapshot, now int, latD, lonD float64) CentreResult {
	s := snaps[now]
	var c CentreResult
	c.FoF2, c.M3000 = s.Interp(latD, lonD)
	c.CosChi = s.zenithNaive(latD, lonD)
	c.FoE = foE(c.CosChi)
	c.MUF3000 = basicMUF(c.FoF2, c.M3000, c.FoE, 3000)
	c.Bands = BandsNaive(snaps, now, latD, lonD, 0, true)
	return c
}
