// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import "math"

// "After": one honest optimization pass.
//   - cell sin/cos from the grid's precomputed tables (Grid.Precompute);
//   - cos(zenith) as a dot product with the sun vector (the control point's
//     unit vector is already in hand), instead of 3 trig calls;
//   - no allocation in the inner loop: control points in a fixed array, the
//     result field reused by the caller, sample points in a fixed array;
//   - band-independent work (interpolation, zenith, foE, MUF, hop geometry)
//     hoisted out of the band loop; per-band pow(f+fH) precomputed once.
// The per-path physics calls are otherwise unchanged.

var bandFF [10]float64

func init() {
	for i, f := range Bands {
		bandFF[i] = freqFactor(f)
	}
}

func controlPoints(p1, p2 [3]float64, delta, d float64, out *[2]ctrl) int {
	sd := math.Sin(delta)
	if d <= 4000 {
		a := math.Sin(delta/2) / sd
		out[0] = slerp(p1, p2, a, a)
		return 1
	}
	t := 2000 / d
	a := math.Sin((1-t)*delta) / sd
	b := math.Sin(t*delta) / sd
	out[0] = slerp(p1, p2, a, b)
	out[1] = slerp(p1, p2, b, a)
	return 2
}

// pathFast evaluates one path at frequency f (ff = freqFactor(f)), no early exit.
func pathFast(s *Snapshot, f, ff, d float64, cps *[2]ctrl, n int) bool {
	hops, secI := hopGeometry(d)
	if n == 1 {
		c := &cps[0]
		fo, m := s.Interp(c.latD, c.lonD)
		cz := s.zenithVec(c.p)
		fe := foE(cz)
		muf := basicMUF(fo, m, fe, d)
		loss := absorption(hops, secI, solarFactor(cz), ff, f, fe)
		return f <= muf && loss <= lossMax
	}
	muf, loss := math.Inf(1), 0.0
	for i := range 2 {
		c := &cps[i]
		fo, m := s.Interp(c.latD, c.lonD)
		cz := s.zenithVec(c.p)
		fe := foE(cz)
		muf = math.Min(muf, basicMUF(fo, m, fe, 4000))
		loss += absorption(hops, secI, solarFactor(cz), ff, f, fe) / 2
	}
	return f <= muf && loss <= lossMax
}

// ReachFast fills out (its Reached slice is reused when large enough).
func ReachFast(s *Snapshot, oLatD, oLonD, f float64, out *ReachResult) {
	g := s.G
	n := g.Cols * g.Rows
	if cap(out.Reached) < n {
		out.Reached = make([]bool, n)
	}
	out.Reached = out.Reached[:n]
	out.Cols, out.Rows = g.Cols, g.Rows
	out.SkipKm, out.MaxKm = math.Inf(1), 0
	p1 := unitVec(oLatD, oLonD)
	ff := freqFactor(f)
	var cps [2]ctrl
	for r := range g.Rows {
		sl, cl := g.sinLat[r], g.cosLat[r]
		row := out.Reached[r*g.Cols : (r+1)*g.Cols]
		for c := range g.Cols {
			p2 := [3]float64{cl * g.cosLon[c], cl * g.sinLon[c], sl}
			delta := math.Acos(clamp(dot(p1, p2), -1, 1))
			d := delta * earthR
			row[c] = false
			if d < 1 || math.Sin(delta) < 1e-6 {
				continue
			}
			k := controlPoints(p1, p2, delta, d, &cps)
			if pathFast(s, f, ff, d, &cps, k) {
				row[c] = true
				if d < out.SkipKm {
					out.SkipKm = d
				}
				if d > out.MaxKm {
					out.MaxKm = d
				}
			}
		}
	}
}

type sample struct {
	latD, lonD float64
	p          [3]float64
}

func samplePoints(latD, lonD, radiusKm float64, out *[maxSamples]sample) int {
	out[0] = sample{latD, lonD, unitVec(latD, lonD)}
	if radiusKm <= 0 {
		return 1
	}
	k := 1
	for i, n := range ringCounts {
		rk := radiusKm * float64(i+1) / 3
		for j := range n {
			la, lo := dest(latD, lonD, 2*math.Pi*float64(j)/float64(n), rk)
			out[k] = sample{la, lo, unitVec(la, lo)}
			k++
		}
	}
	return k
}

const absK = 677.2 * (1 + 0.0067*r12)

func BandsFast(snaps []*Snapshot, now int, latD, lonD, radiusKm float64, allDay bool) BandResult {
	var pts [maxSamples]sample
	n := samplePoints(latD, lonD, radiusKm, &pts)
	var geo [len(refDist)]float64 // hops*secI per reference distance
	for k, d := range refDist {
		h, si := hopGeometry(d)
		geo[k] = h * si
	}
	var res BandResult
	for h := range 24 {
		if !allDay && h != now {
			continue
		}
		s := snaps[h]
		var open [10]int
		for i := range n {
			p := &pts[i]
			fo, m := s.Interp(p.latD, p.lonD)
			cz := s.zenithVec(p.p)
			fe := foE(cz)
			sf := solarFactor(cz)
			var muf [len(refDist)]float64
			for k, d := range refDist {
				muf[k] = basicMUF(fo, m, fe, d)
			}
			for bi, f := range Bands {
				base := absK * sf * phiN(f, fe) / bandFF[bi]
				ok := false
				for k := range refDist {
					if f <= muf[k] && geo[k]*base <= lossMax {
						ok = true
					}
				}
				if ok {
					open[bi]++
				}
			}
		}
		for bi := range Bands {
			if open[bi] > 0 {
				res.Hours[bi] |= 1 << h
			}
			if h == now {
				res.OpenFrac[bi] = float64(open[bi]) / float64(n)
			}
		}
	}
	return res
}

type pathGeo struct {
	d, geo float64 // distance km, hops*secI
	cps    [2]ctrl
	n      int
}

func PathFast(snaps []*Snapshot, now int, oLatD, oLonD float64, allDay bool) PathResult {
	var res PathResult
	var pg [len(Targets)]pathGeo
	p1 := unitVec(oLatD, oLonD)
	for ti, t := range Targets {
		p2 := unitVec(t[0], t[1])
		delta := math.Acos(clamp(dot(p1, p2), -1, 1))
		g := &pg[ti]
		g.d = delta * earthR
		g.n = controlPoints(p1, p2, delta, g.d, &g.cps)
		h, si := hopGeometry(g.d)
		g.geo = h * si
	}
	for h := range 24 {
		if !allDay && h != now {
			continue
		}
		s := snaps[h]
		for ti := range pg {
			g := &pg[ti]
			var fe, sf [2]float64
			muf := math.Inf(1)
			for i := range g.n {
				c := &g.cps[i]
				fo, m := s.Interp(c.latD, c.lonD)
				cz := s.zenithVec(c.p)
				fe[i] = foE(cz)
				sf[i] = solarFactor(cz)
				dm := g.d
				if g.n == 2 {
					dm = 4000
				}
				muf = math.Min(muf, basicMUF(fo, m, fe[i], dm))
			}
			for bi, f := range Bands {
				loss := 0.0
				for i := range g.n {
					loss += g.geo * absK * sf[i] * phiN(f, fe[i]) / bandFF[bi]
				}
				loss /= float64(g.n)
				if f <= muf && loss <= lossMax {
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

func CentreFast(snaps []*Snapshot, now int, latD, lonD float64) CentreResult {
	s := snaps[now]
	var c CentreResult
	c.FoF2, c.M3000 = s.Interp(latD, lonD)
	c.CosChi = s.zenithVec(unitVec(latD, lonD))
	c.FoE = foE(c.CosChi)
	c.MUF3000 = basicMUF(c.FoF2, c.M3000, c.FoE, 3000)
	c.Bands = BandsFast(snaps, now, latD, lonD, 0, true)
	return c
}
