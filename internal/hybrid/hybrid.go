// Package hybrid is the hour's field as the library ships it (R-9.2,
// watchpost D-101): the station readings held, those fresh enough,
// assimilated onto the backgrounds. The update and the G-M3 instrument
// (tools/gm3) both make it here, so the instrument scores what ships
// (watchpost D-237).
package hybrid

import (
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/assimilate"
	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
)

// For is how old a reading may be and still be assimilated (D-64,
// watchpost D-184): an hour, past which it does worse than GloTEC alone
// (week.py section 12).
const For = time.Hour

// Readings are the held readings assimilated at now - taken at or before
// it and no more than For before - as the assimilation takes them, and the
// oldest and newest of those; none, and no times, when none is.
//
// Source: D-64; watchpost D-184.
func Readings(held []giro.Held, now time.Time) (rs []assimilate.Reading, oldest, newest time.Time) {
	for _, h := range held {
		age := now.Sub(h.Reading.At)
		fresh := age >= 0 && age <= For
		if !fresh {
			continue
		}
		rs = append(rs, assimilate.Reading{Lat: h.Station.Lat, Lon: h.Station.Lon, FoF2: float64(h.Reading.FoF2),
			M3000: float64(h.Reading.M3000), HasM: h.Reading.HasM300, Unscored: h.Reading.Unscored})
		if oldest.IsZero() || h.Reading.At.Before(oldest) {
			oldest = h.Reading.At
		}
		if h.Reading.At.After(newest) {
			newest = h.Reading.At
		}
	}
	return rs, oldest, newest
}

// Field is the backgrounds with the readings assimilated onto them; the
// backgrounds as they were, nothing used, when the readings cannot be
// (fewer than three, or a fit that does not solve).
//
// Source: R-9.2; watchpost D-101.
func Field(b background.Fields, rs []assimilate.Reading) assimilate.Result {
	res, err := assimilate.Onto(b.FoF2, b.M3000, rs)
	if err != nil || res.Used == 0 {
		return assimilate.Result{FoF2: b.FoF2, M3000: b.M3000}
	}
	return res
}
