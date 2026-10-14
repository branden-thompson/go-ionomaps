package ionomaps

import (
	"errors"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/background"
)

// maxAhead is the furthest hour ahead (R-1.3).
const maxAhead = 24

var errNotAhead = errors.New("ionomaps: an hour ahead is from 1 to 24")

// Ahead is the hour k hours after the snapshot's, 1 to 24, built when asked
// (D-60): the climatology's foF2 and M(3000)F2 at the snapshot's F10.7 mean,
// MUF(3000) their product, marked typical - never a forecast, and never a
// field from now (R-1.3). About 35 ms an hour; a host stepping through the
// hours keeps the ones it has.
//
// Source: the climatology (PyIRI's method, watchpost D-114); R-1.3.
func (s Snapshot) Ahead(k int) (Hour, error) {
	if k < 1 || k > maxAhead {
		return Hour{}, errNotAhead
	}
	if len(s.Hours) == 0 {
		return Hour{}, errNoHour
	}
	at := s.Hours[0].At.Add(time.Duration(k) * time.Hour)
	b := background.Build(at, float64(s.Inputs.F107Mean), nil)
	fo, muf := newField(), newField()
	for i, f := range b.FoF2 {
		fo.put(i, f)
		muf.put(i, f*b.M3000[i])
	}
	return Hour{At: at, FoF2: fo, MUF3000: muf, Typical: true}, nil
}

// typicalError is the hours ahead's typical error by distance to the
// nearest reporting station, and where it was measured (watchpost D-134),
// with the stations whose scores are unknown taken (D-61).
func typicalError() TypicalError {
	return TypicalError{Bands: []BandError{
		{UpToKm: 500, FoF2MHz: 0.77, MUF3000MHz: 2.91},
		{UpToKm: 1000, FoF2MHz: 0.87, MUF3000MHz: 3.25},
		{UpToKm: 2000, FoF2MHz: 1.22, MUF3000MHz: 4.33},
		{UpToKm: 0, FoF2MHz: 1.37, MUF3000MHz: 4.73},
	}, Basis: "the climatology against GIRO's stations over PLAN's week, readings of unknown score taken (watchpost D-134, D-181; go-ionomaps 02-analysis/evidence/week.py section 11, WEEK_TAKE_UNKNOWN=1)"}
}
