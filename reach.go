package ionomaps

import (
	"errors"
	"math"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
)

var errBadFrequency = errors.New("ionomaps: the frequency is not from 1.8 to 30 MHz")

// ReachField is a frequency's reach from an origin (R-2.9): a field over
// the library's global grid, 1 where the frequency is under the path's upper
// limit from the origin and over its lower limit at the reference circuit,
// 0 where it is not; how far out the skip zone ends; whether any cell is
// reached; the limit that bounds it - the MUF where the lower limit closes
// no cell under it, else LimitAbsorption or LimitCircuit, whichever closes
// the more.
type ReachField struct {
	Field   Field
	SkipKm  float32 // the nearest reached cell's distance; 0 when the origin's own cell is reached
	Reaches bool
	Limit   Limit
	MHz     float32
}

// Reach is where a frequency reaches from an origin in the snapshot's hour,
// as the band statuses judge it: under the path's basic MUF by ITU-R
// P.533-14 to every cell, and over the path's lower limit at the reference
// circuit - the circuit worked out on a polar grid around
// the origin and interpolated between its nodes, within the measured
// share of cells of each cell's own circuit; and the skip zone inside it
// (R-2.9).
//
// Source: ITU-R P.533-14 §3.5 and §5 to §7 (02-analysis/evidence/p533-equations.md,
// p533-field-strength.md).
func (s Snapshot) Reach(origin LatLon, mhz float64) (ReachField, error) {
	return s.reach(origin, mhz, true)
}

// reach is Reach, each cell's lower limit read from the polar grid when
// polar is true (watchpost D-227), else from the cell's own circuit - the
// measure the grid is held to.
func (s Snapshot) reach(origin LatLon, mhz float64, polar bool) (ReachField, error) {
	if !(mhz >= 1.8 && mhz <= 30) {
		return ReachField{}, errBadFrequency
	}
	h, err := s.hourFor([]LatLon{origin})
	if err != nil {
		return ReachField{}, err
	}
	out := ReachField{Field: newField(), MHz: float32(mhz)}
	read, sun, day := hourReader(h), newSunTrack(h.At), s.typicalDay(h.At)
	skip := math.Inf(1)
	var absorbed, weak int // the cells under the MUF the lower limit closed, by why
	var lower *lowerGrid
	if polar {
		lower = s.newLowerGrid(read, origin, sun, &day, mhz)
	}
	for i := range out.Field.Values {
		lat, lon := grid.Centre(i)
		cell := LatLon{Lat: lat, Lon: lon}
		muf, ok := s.reachLimit(read, origin, cell, sun)
		if !ok {
			out.Field.NoData.set(i)
			continue
		}
		if mhz >= muf {
			continue
		}
		st, gridded := lower.status(cell)
		if !gridded {
			st = s.reachLower(read, origin, cell, sun, &day, mhz)
		}
		if st == Absorbed {
			absorbed++
		}
		if st == Weak {
			weak++
		}
		if st != Open {
			continue
		}
		out.Field.Values[i] = 1
		skip = math.Min(skip, limits.Distance(origin.Lat, origin.Lon, lat, lon))
	}
	if out.Reaches = skip < math.Inf(1); out.Reaches {
		out.SkipKm = float32(skip)
	}
	out.Limit = reachBound(absorbed, weak)
	if v, ok := s.reachLimit(read, origin, origin, sun); ok && mhz < v && s.reachLower(read, origin, origin, sun, &day, mhz) == Open {
		out.SkipKm, out.Reaches = 0, true // the origin's own area is reached: no skip zone
	}
	return out, nil
}

// reachBound is the limit that bounds a reach, by the cells under the MUF
// the lower limit closed: the MUF where it closed none, else the lower
// limit that closed the more.
func reachBound(absorbed, weak int) Limit {
	switch {
	case absorbed == 0 && weak == 0:
		return LimitMUF
	case absorbed >= weak:
		return LimitAbsorption
	}
	return LimitCircuit
}

// reachLower is the lower limit's status at a frequency from an origin to a
// place: the reference circuit on the path, or across the near-vertical
// area for a place too near for one, as Bands judges it; Open where no
// circuit can be made, as the band statuses leave a band.
func (s Snapshot) reachLower(read reader, origin, to LatLon, sun *sunTrack, day *[24]typicalHour, mhz float64) Status {
	from := origin
	if limits.Distance(origin.Lat, origin.Lon, to.Lat, to.Lon) < nearKm {
		from, to = areaEnds(origin, nearKm)
	}
	c, ok := s.newCircuit(read, from, to, sun, day)
	if !ok {
		return Open
	}
	return c.lowerAt(mhz)
}

// reachLimit is the upper limit from an origin to a place: the path's basic
// MUF, or foF2 at the origin for a place too near for a path (R-2.2).
func (s Snapshot) reachLimit(read reader, origin, to LatLon, sun *sunTrack) (float64, bool) {
	if limits.Distance(origin.Lat, origin.Lon, to.Lat, to.Lon) < nearKm {
		fo, _, ok := read(origin)
		return fo, ok
	}
	muf, _, ok := s.pathMUF(read, origin, to, sun)
	return muf, ok
}

// nearKm is the distance within which the reach takes foF2 at the origin:
// half a grid cell at the equator.
const nearKm = 111
