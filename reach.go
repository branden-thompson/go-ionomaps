package ionomaps

import (
	"errors"
	"math"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
)

var errBadFrequency = errors.New("ionomaps: the frequency is not from 1.8 to 30 MHz")

// ReachField is a frequency's reach from an origin (R-2.9): a field over
// the library's global grid, 1 where the frequency is under the path's upper
// limit from the origin and 0 where it is over it; how far out the skip zone
// ends; whether any cell is reached; the limit that bounds it.
type ReachField struct {
	Field   Field
	SkipKm  float32 // the nearest reached cell's distance; 0 when the origin's own cell is reached
	Reaches bool
	Limit   Limit
	MHz     float32
}

// Reach is where a frequency reaches from an origin in the hour that holds
// the moment, under the path's basic MUF by ITU-R P.533-14 to every cell -
// the upper limit only until the lower limit lands (watchpost D-175) - and
// the skip zone inside it (R-2.9).
//
// Source: ITU-R P.533-14 §3.5 (02-analysis/evidence/p533-equations.md);
// watchpost D-76.
func (s Snapshot) Reach(origin LatLon, mhz float64, at time.Time) (ReachField, error) {
	if !(mhz >= 1.8 && mhz <= 30) {
		return ReachField{}, errBadFrequency
	}
	h, err := s.hourFor([]LatLon{origin}, at)
	if err != nil {
		return ReachField{}, err
	}
	out := ReachField{Field: newField(), Limit: LimitMUF, MHz: float32(mhz)}
	read, sun := hourReader(h), newSunTrack(at)
	skip := math.Inf(1)
	for i := range out.Field.Values {
		lat, lon := grid.Centre(i)
		cell := LatLon{Lat: lat, Lon: lon}
		muf, ok := s.reachLimit(read, origin, cell, sun)
		if !ok {
			out.Field.NoData.Set(i)
			continue
		}
		if mhz < muf {
			out.Field.Values[i] = 1
			skip = math.Min(skip, limits.Distance(origin.Lat, origin.Lon, lat, lon))
		}
	}
	if out.Reaches = skip < math.Inf(1); out.Reaches {
		out.SkipKm = float32(skip)
	}
	if v, ok := s.reachLimit(read, origin, origin, sun); ok && mhz < v {
		out.SkipKm = 0
	}
	return out, nil
}

// reachLimit is the upper limit from an origin to a place: the path's basic
// MUF, or foF2 at the origin for a place too near for a path (R-2.2).
func (s Snapshot) reachLimit(read reader, origin, to LatLon, sun *sunTrack) (float64, bool) {
	if limits.Distance(origin.Lat, origin.Lon, to.Lat, to.Lon) < nearKm {
		fo, _, ok := read(origin)
		return fo, ok
	}
	return s.pathMUF(read, origin, to, sun)
}

// nearKm is the distance within which the reach takes foF2 at the origin:
// half a grid cell at the equator.
const nearKm = 111
