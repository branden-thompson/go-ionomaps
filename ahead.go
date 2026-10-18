package ionomaps

import (
	"errors"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// maxAhead is the furthest hour ahead (R-1.3).
const maxAhead = 24

var errNotAhead = errors.New("ionomaps: an hour ahead is from 1 to 24")

// Ahead is the hour k hours after the snapshot's, 1 to 24, as a snapshot of
// its own (D-60): the climatology's foF2 and M(3000)F2 at the start of that
// UTC hour, at the snapshot's F10.7 mean, MUF(3000) their product, marked
// typical - never a forecast (R-1.3). It carries none of now's
// measurements: no D-RAP disturbance and no live offset, which hold for now
// alone (watchpost D-216, L-F2); the stations' distances stay, the hours
// ahead's typical error read by them. The hour's cells are the typical
// day's, which the answers share, so each is worked out once a day whichever
// snapshot or answer asks it (R-8.1, watchpost D-222).
//
// Source: the climatology (PyIRI's method, watchpost D-114); R-1.3.
func (s Snapshot) Ahead(k int) (Snapshot, error) {
	if k < 1 || k > maxAhead {
		return Snapshot{}, errNotAhead
	}
	if len(s.Now.FoF2.Values) == 0 {
		return Snapshot{}, errNoField
	}
	at := s.Now.At.Add(time.Duration(k) * time.Hour).Truncate(time.Hour)
	th := s.typicalHourAt(at)
	fo, muf := newField(), newField()
	for i := range fo.Values {
		lat, lon := grid.Centre(i)
		f, m3, ok := th.read(LatLon{Lat: lat, Lon: lon})
		if !ok {
			fo.NoData.set(i)
			muf.NoData.set(i)
			continue
		}
		fo.put(i, float32(f))
		muf.put(i, float32(f*m3))
	}
	a := s.clone()
	a.Now = Hour{At: at, FoF2: fo, MUF3000: muf, Typical: true}
	a.Background = Backgrounds{FoF2: Climatology, M3000: Climatology}
	a.Disturbance, a.Inputs.DRAPValid = Field{}, time.Time{}
	a.Offset = Offset{TypicalMHz: s.Offset.TypicalMHz, SpreadMHz: s.Offset.SpreadMHz, Basis: s.Offset.Basis, Source: s.Offset.Source}
	return a, nil
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
	}, Basis: "measured against GIRO's stations over a week in early October 2026",
		Source: "the climatology against GIRO's stations over PLAN's week, readings of unknown score taken (watchpost D-134, D-181; go-ionomaps 02-analysis/evidence/week.py section 11, WEEK_TAKE_UNKNOWN=1)"}
}
