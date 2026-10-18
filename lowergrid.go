package ionomaps

import (
	"math"

	"github.com/branden-thompson/go-ionomaps/internal/limits"
)

// The polar grid a reach reads its lower limit from (watchpost D-227): a
// node every lowerAzStep degrees of bearing from the origin and every
// lowerKmStep km of distance from lowerKmFirst out, each node the reference
// circuit to that place; a cell between nodes takes the S/N margins
// interpolated from its four. The figures are the ones the accuracy test
// holds against each cell's own circuit (TestTheReachsLowerLimitIsHeldToTheCircuit).
const (
	lowerAzStep  = 10.0
	lowerKmStep  = 250.0
	lowerKmFirst = 125.0
	lowerAzNodes = 36
	lowerKmNodes = 80
)

// lowerNearDB is how near the +13 dB threshold an interpolated margin may
// come and still decide a cell; nearer, the cell takes its own circuit.
const lowerNearDB = 2.0

// lowerKmLast is the farthest node's distance; a cell past it, toward the
// antipode, takes its own circuit.
const lowerKmLast = lowerKmFirst + (lowerKmNodes-1)*lowerKmStep

// The states a node is in: not yet worked out, holding margins, or with no
// circuit to give them.
const (
	nodeUnknown uint8 = iota
	nodeHeld
	nodeNone
)

// lowerGrid is one reach's polar grid: its origin and frequency, what it
// reads them with, and each node's S/N margins over the reference circuit's
// +13 dB, with the absorption and without, worked out when a cell first
// needs the node.
type lowerGrid struct {
	s       Snapshot
	read    reader
	sun     *sunTrack
	day     *[24]typicalHour
	origin  LatLon
	mhz     float64
	state   [lowerAzNodes * lowerKmNodes]uint8
	margins [lowerAzNodes * lowerKmNodes][2]float64
}

// newLowerGrid is a reach's polar grid, no node yet worked out.
func (s Snapshot) newLowerGrid(read reader, origin LatLon, sun *sunTrack, day *[24]typicalHour, mhz float64) *lowerGrid {
	if !(mhz > 0) {
		return nil // no frequency: every cell takes its own circuit
	}
	return &lowerGrid{s: s, read: read, sun: sun, day: day, origin: origin, mhz: mhz}
}

// node is a node's margins, worked out the first time it is asked; false
// where no circuit, or no S/N, can be had to it.
func (g *lowerGrid) node(az, km int) (m [2]float64, ok bool) {
	if az < 0 || az >= lowerAzNodes || km < 0 || km >= lowerKmNodes {
		return m, false // no such node
	}
	i := az*lowerKmNodes + km
	if g.state[i] == nodeUnknown {
		g.state[i] = nodeNone
		to := destination(g.origin, float64(az)*lowerAzStep, lowerKmFirst+float64(km)*lowerKmStep)
		if c, okC := g.s.newCircuit(g.read, g.origin, to, g.sun, g.day); okC {
			with, okW := c.snr(g.mhz, true)
			bare, okB := c.snr(g.mhz, false)
			if okW && okB {
				g.state[i], g.margins[i] = nodeHeld, [2]float64{with - limits.RequiredSNR, bare - limits.RequiredSNR}
			}
		}
	}
	return g.margins[i], g.state[i] == nodeHeld
}

// status is the lower limit's status at a place from the grid, as lowerAt
// judges a circuit: Open where the margin with the absorption is at least
// zero, Absorbed where only the margin without it is, Weak where neither;
// false nearer than the first node, past the last, or where a node around
// it has no circuit - the cell then takes its own.
func (g *lowerGrid) status(to LatLon) (Status, bool) {
	if g == nil {
		return Open, false
	}
	km := limits.Distance(g.origin.Lat, g.origin.Lon, to.Lat, to.Lon)
	if !(km >= lowerKmFirst && km <= lowerKmLast) {
		return Open, false
	}
	az := bearingFrom(g.origin, to) / lowerAzStep
	a0 := int(az) % lowerAzNodes
	a1, fa := (a0+1)%lowerAzNodes, az-math.Floor(az)
	d := (km - lowerKmFirst) / lowerKmStep
	k0 := min(int(d), lowerKmNodes-2)
	fk := d - float64(k0)
	var corner [4][2]float64
	for j, n := range [4][2]int{{a0, k0}, {a1, k0}, {a0, k0 + 1}, {a1, k0 + 1}} {
		m, ok := g.node(n[0], n[1])
		if !ok {
			return Open, false
		}
		corner[j] = m
	}
	var m [2]float64
	for i := range m {
		near := corner[0][i]*(1-fa) + corner[1][i]*fa
		far := corner[2][i]*(1-fa) + corner[3][i]*fa
		m[i] = near*(1-fk) + far*fk
	}
	if math.Abs(m[0]) < lowerNearDB || (m[0] < 0 && math.Abs(m[1]) < lowerNearDB) {
		return Open, false // too near the threshold for the interpolation to decide
	}
	switch {
	case m[0] >= 0:
		return Open, true
	case m[1] >= 0:
		return Absorbed, true
	}
	return Weak, true
}
