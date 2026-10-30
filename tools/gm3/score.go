package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/hybrid"
)

// pairWithin is how far a sounding may lie from a grid's time and still be
// paired with it (week.py).
const pairWithin = 10 * time.Minute

// pairing is how the other stations' readings are taken at a grid's time.
type pairing int

const (
	// asShipped takes each other station's newest reading at or before the
	// grid's time, at most an hour old, as the library assimilates what its
	// GIRO burst holds (internal/hybrid; watchpost D-237): the product's
	// best case, its burst just before the grid.
	asShipped pairing = iota
	// asWeekPy takes each other station's sounding nearest the grid's time
	// within ten minutes either side, as week.py did: the replica check.
	asWeekPy
)

// station is one GIRO station's reply: where it is and its readings.
type station struct {
	code     string
	lat, lon float64
	readings []giro.Reading
}

// moment is one grid's time and its two backgrounds: the hybrid's (foF2
// from GloTEC, M(3000)F2 the climatology's) and the climatology's alone; and
// with -grid-age, the hybrid's built from an older grid at the same time,
// as a product still holding that grid would show it (watchpost D-265).
type moment struct {
	at           time.Time
	hybrid, clim background.Fields
	stale        *background.Fields
}

// pair is one station held out at one moment: what it measured, what the
// hybrid predicted from the other stations, and what the climatology said.
type pair struct {
	at            time.Time
	code          string
	lat, lon      float64
	nearKm        float64 // to the nearest other station whose reading was assimilated
	obsFo, obsMUF float64
	hyFo, hyM     float64
	climFo, climM float64
	oldFo, oldM   float64 // from the older grid, with -grid-age (D-265)
}

var errNoPairs = errors.New("gm3: no station could be held out at any grid time: nothing to score")

// sounding is a station's reading nearest a moment, within pairWithin, that
// carries M(3000)F2: what a held-out station is scored against, and the
// other stations' readings under asWeekPy; false when it has none.
func sounding(s station, at time.Time) (giro.Reading, bool) {
	var best giro.Reading
	found := false
	for _, r := range s.readings {
		gap := r.At.Sub(at).Abs()
		usable := gap <= pairWithin && r.HasM300 // the parser gives MUF(3000) with M(3000)F2, foF2 within its bounds
		if !usable {
			continue
		}
		if !found || gap < best.At.Sub(at).Abs() {
			best, found = r, true
		}
	}
	return best, found
}

// newest is a station's newest reading at or before a moment: what the
// library holds for it after a burst then; false when it has none.
func newest(s station, at time.Time) (giro.Reading, bool) {
	var best giro.Reading
	found := false
	for _, r := range s.readings {
		if !r.At.After(at) && (!found || r.At.After(best.At)) {
			best, found = r, true
		}
	}
	return best, found
}

// held are the other stations' readings at the moment as the pairing takes
// them, and the clock the assimilation judges their age by.
func held(m moment, stations []station, out int, p pairing) ([]giro.Held, time.Time) {
	var hs []giro.Held
	for j, s := range stations {
		if j == out {
			continue
		}
		pick := newest
		if p == asWeekPy {
			pick = sounding
		}
		if r, ok := pick(s, m.at); ok {
			hs = append(hs, giro.Held{Reading: r, Station: giro.Station{Code: s.code, Lat: s.lat, Lon: s.lon}})
		}
	}
	if p == asWeekPy {
		return hs, m.at.Add(pairWithin) // every sounding within ten minutes either side is fresh
	}
	return hs, m.at
}

// heldOut holds out in turn each station with a sounding at the moment:
// the others' readings, as the pairing takes them, assimilated by the
// library's own recipe (internal/hybrid), the result read at the held-out
// station's place beside the climatology's there.
//
// Source: go-ionomaps 02-analysis/evidence/week.py (gp_loo, the hybrid);
// watchpost D-101, D-108, D-237.
func heldOut(m moment, stations []station, p pairing) ([]pair, error) {
	var out []pair
	for i, s := range stations {
		truth, scored := sounding(s, m.at)
		if !scored {
			continue
		}
		rs, _, _ := hybrid.Readings(held(m, stations, i, p))
		res := hybrid.Field(m.hybrid, rs)
		if res.Used == 0 {
			continue // fewer than three others: the product shows its background unmoved, nothing assimilated to score
		}
		near := math.Inf(1)
		for _, r := range rs {
			near = math.Min(near, kmBetween(s.lat, s.lon, r.Lat, r.Lon))
		}
		pr := pair{at: m.at, code: s.code, lat: s.lat, lon: s.lon, nearKm: near,
			obsFo: float64(truth.FoF2), obsMUF: float64(truth.MUF3000)}
		var ok [4]bool
		pr.hyFo, ok[0] = grid.Sample(res.FoF2, pr.lat, pr.lon)
		pr.hyM, ok[1] = grid.Sample(res.M3000, pr.lat, pr.lon)
		pr.climFo, ok[2] = grid.Sample(m.clim.FoF2, pr.lat, pr.lon)
		pr.climM, ok[3] = grid.Sample(m.clim.M3000, pr.lat, pr.lon)
		if ok != [4]bool{true, true, true, true} {
			return nil, fmt.Errorf("%s at %s: a field could not be read at the station", s.code, m.at.Format(time.RFC3339))
		}
		// A NaN in one prediction would make every RMS it enters NaN, and a
		// NaN compares false: the verdicts would read as missed for no reason
		// the report could give.
		if m.stale != nil {
			old := hybrid.Field(*m.stale, rs)
			pr.oldFo, ok[0] = grid.Sample(old.FoF2, pr.lat, pr.lon)
			pr.oldM, ok[1] = grid.Sample(old.M3000, pr.lat, pr.lon)
			if !ok[0] || !ok[1] {
				return nil, fmt.Errorf("%s at %s: the older grid's field could not be read at the station", s.code, m.at.Format(time.RFC3339))
			}
		}
		sum := pr.hyFo + pr.hyM + pr.climFo + pr.climM + pr.oldFo + pr.oldM
		if math.IsNaN(sum) || math.IsInf(sum, 0) {
			return nil, fmt.Errorf("%s at %s: a prediction is not a number", s.code, m.at.Format(time.RFC3339))
		}
		out = append(out, pr)
	}
	return out, nil
}

// kmBetween is the great-circle distance between two places, km, as week.py
// and the assimilation measure it.
func kmBetween(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	c := math.Sin(lat1*r)*math.Sin(lat2*r) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Cos((lon1-lon2)*r)
	return 6371.0 * math.Acos(math.Max(-1, math.Min(1, c)))
}

// pacific are the Pacific stations D-116 and D-129 report apart (week.py).
var pacific = map[string]bool{"EA653": true, "LL721": true, "WA619": true, "GU513": true}

// mainlandUS is week.py's box for the contiguous United States.
func mainlandUS(p pair) bool {
	return p.lat >= 24 && p.lat <= 50 && p.lon >= -125 && p.lon <= -66
}

// group is a named selection of pairs.
type group struct {
	name string
	in   func(pair) bool
}

// regions are the groups every table is split into (week.py).
var regions = []group{
	{"all", func(pair) bool { return true }},
	{"mainland US", mainlandUS},
	{"Pacific", func(p pair) bool { return pacific[p.code] }},
	{"elsewhere", func(p pair) bool { return !mainlandUS(p) && !pacific[p.code] }},
}

// bands are the distances to the nearest other reporting station each
// figure is also given by (D-116, D-129).
var bands = []group{
	{"0-500 km", func(p pair) bool { return p.nearKm < 500 }},
	{"500-1000 km", func(p pair) bool { return p.nearKm >= 500 && p.nearKm < 1000 }},
	{"1000-2000 km", func(p pair) bool { return p.nearKm >= 1000 && p.nearKm < 2000 }},
	{"2000 km and over", func(p pair) bool { return p.nearKm >= 2000 }},
}

// errs is one method's error over a set of pairs: foF2 and MUF(3000), RMS
// and bias, MHz.
type errs struct {
	n                          int
	foRMS, foBias, mRMS, mBias float64
}

// errorsOf is the hybrid's error over the pairs, or the climatology's.
func errorsOf(ps []pair, hybrid bool) errs {
	if hybrid {
		return errorsBy(ps, func(p pair) (float64, float64) { return p.hyFo, p.hyM })
	}
	return errorsBy(ps, func(p pair) (float64, float64) { return p.climFo, p.climM })
}

// errorsBy is one prediction's error over the pairs, the prediction taken by
// pick; MUF(3000) as foF2 times M(3000)F2, against the station's MUF(3000)
// (week.py's errors).
func errorsBy(ps []pair, pick func(pair) (fo, m float64)) errs {
	var e errs
	var fo2, m2 float64
	for _, p := range ps {
		fo, m := pick(p)
		dfo, dm := fo-p.obsFo, fo*m-p.obsMUF
		e.foBias, e.mBias = e.foBias+dfo, e.mBias+dm
		fo2, m2 = fo2+dfo*dfo, m2+dm*dm
		e.n++
	}
	if e.n == 0 {
		return errs{foRMS: math.NaN(), foBias: math.NaN(), mRMS: math.NaN(), mBias: math.NaN()}
	}
	n := float64(e.n)
	e.foRMS, e.mRMS, e.foBias, e.mBias = math.Sqrt(fo2/n), math.Sqrt(m2/n), e.foBias/n, e.mBias/n
	return e
}

// selected is the pairs a group takes.
func selected(ps []pair, g group) []pair {
	var out []pair
	for _, p := range ps {
		if g.in(p) {
			out = append(out, p)
		}
	}
	return out
}

// verdict is one target or floor judged.
type verdict struct {
	name, measured, target string
	met                    bool
}

// G-M3's targets (D-108) and D-75's floor.
const (
	usBelow       = 0.15 // the hybrid's held-out RMS at least 15 % below the climatology's in the mainland US
	allBelow      = 0.10 // and at least 10 % below overall
	nearKm        = 500.0
	nearFoMHz     = 0.5 // within 500 km of a reporting station
	nearMUFMHz    = 1.6
	minPairsJudge = 1 // a group with no pair judges nothing: it fails
)

// judge is G-M3 (D-108) and D-75's floor (1) over the pairs.
func judge(ps []pair) []verdict {
	us, all := selected(ps, regions[1]), ps
	near := selected(ps, group{"near", func(p pair) bool { return p.nearKm < nearKm }})
	hyUS, clUS, hyAll, clAll, hyNear := errorsOf(us, true), errorsOf(us, false), errorsOf(all, true), errorsOf(all, false), errorsOf(near, true)
	below := func(name string, hy, cl float64, n int, by float64) verdict {
		cut := (1 - by) * cl
		return verdict{name, fmt.Sprintf("%.2f against %.2f (%+.0f %%)", hy, cl, 100*(hy-cl)/cl),
			fmt.Sprintf("at most %.2f (%.0f %% below)", cut, 100*by), n >= minPairsJudge && hy <= cut}
	}
	beats := func(name string, hy, cl float64, n int) verdict {
		return verdict{name, fmt.Sprintf("%.2f against %.2f", hy, cl), "below the climatology's", n >= minPairsJudge && hy < cl}
	}
	at := func(name string, v, limit float64, n int) verdict {
		return verdict{name, fmt.Sprintf("%.2f (n=%d)", v, n), fmt.Sprintf("at most %.1f", limit), n >= minPairsJudge && v <= limit}
	}
	return []verdict{
		below("G-M3: foF2 RMS, mainland US", hyUS.foRMS, clUS.foRMS, hyUS.n, usBelow),
		below("G-M3: MUF(3000) RMS, mainland US", hyUS.mRMS, clUS.mRMS, hyUS.n, usBelow),
		below("G-M3: foF2 RMS, all stations", hyAll.foRMS, clAll.foRMS, hyAll.n, allBelow),
		below("G-M3: MUF(3000) RMS, all stations", hyAll.mRMS, clAll.mRMS, hyAll.n, allBelow),
		at("G-M3: foF2 RMS within 500 km of a reporting station", hyNear.foRMS, nearFoMHz, hyNear.n),
		at("G-M3: MUF(3000) RMS within 500 km of a reporting station", hyNear.mRMS, nearMUFMHz, hyNear.n),
		beats("D-75 floor (1): foF2, mainland US", hyUS.foRMS, clUS.foRMS, hyUS.n),
		beats("D-75 floor (1): MUF(3000), mainland US", hyUS.mRMS, clUS.mRMS, hyUS.n),
	}
}

// covered is what the pairs cover: the first and last grid times scored
// and the stations held out, sorted.
func covered(ps []pair) (first, last time.Time, codes []string) {
	seen := map[string]bool{}
	for i, p := range ps {
		if i == 0 || p.at.Before(first) {
			first = p.at
		}
		if p.at.After(last) {
			last = p.at
		}
		if !seen[p.code] {
			seen[p.code] = true
			codes = append(codes, p.code)
		}
	}
	sort.Strings(codes)
	return first, last, codes
}
