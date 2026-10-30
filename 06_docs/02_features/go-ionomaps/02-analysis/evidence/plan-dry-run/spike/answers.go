// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import "math"

// ReachResult is the cols x rows reached field plus skip and maximum distance.
type ReachResult struct {
	Cols, Rows    int
	Reached       []bool
	SkipKm, MaxKm float64
}

// BandResult: per band, the share of area sample points usable now, and a
// bitmask of the hours today (bit h) the band is usable somewhere in the area.
type BandResult struct {
	OpenFrac [10]float64
	Hours    [10]uint32
}

// PathResult: per target and band, usable now and the hours today.
type PathResult struct {
	Now   [7][10]bool
	Hours [7][10]uint32
}

// CentreResult is a point reading plus Bands at that single point.
type CentreResult struct {
	FoF2, M3000, FoE, MUF3000, CosChi float64
	Bands                             BandResult
}

// Targets: one point per continent (lat, lon degrees).
var Targets = [7][2]float64{
	{51.5, -0.1},   // Europe
	{40.7, -74.0},  // North America
	{-23.5, -46.6}, // South America
	{35.7, 139.7},  // Asia
	{-33.9, 151.2}, // Oceania
	{-26.2, 28.0},  // Africa
	{-77.8, 166.7}, // Antarctica
}

// refDist: reference path lengths (km) a band's area status is judged over.
var refDist = [4]float64{500, 1500, 2500, 3500}

type ctrl struct {
	latD, lonD float64
	p          [3]float64
}

// slerp returns the great-circle point a*p1 + b*p2 (a, b already divided by
// sin(delta)): 2 atan2 + 1 sqrt.
func slerp(p1, p2 [3]float64, a, b float64) ctrl {
	x := a*p1[0] + b*p2[0]
	y := a*p1[1] + b*p2[1]
	z := a*p1[2] + b*p2[2]
	return ctrl{
		latD: math.Atan2(z, math.Sqrt(x*x+y*y)) / deg,
		lonD: math.Atan2(y, x) / deg,
		p:    [3]float64{x, y, z},
	}
}

// dest: point at bearing brg (rad) and distance km from (latD, lonD).
func dest(latD, lonD, brg, km float64) (float64, float64) {
	lat1, lon1 := latD*deg, lonD*deg
	dl := km / earthR
	sl1, cl1 := math.Sincos(lat1)
	sd, cd := math.Sincos(dl)
	sb, cb := math.Sincos(brg)
	lat2 := math.Asin(sl1*cd + cl1*sd*cb)
	lon2 := lon1 + math.Atan2(sb*sd*cl1, cd-sl1*math.Sin(lat2))
	return lat2 / deg, math.Remainder(lon2, 2*math.Pi) / deg
}

var ringCounts = [3]int{8, 16, 25}

const maxSamples = 50
