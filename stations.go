package ionomaps

import (
	"context"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/assimilate"
	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
)

// giroFetch is the host's Fetcher as GIRO's asker takes it: one ask, no
// validators (FastChar sends none), only the status, the Retry-After and the
// body read.
func (l *Library) giroFetch() giro.Fetch {
	return func(ctx context.Context, url string) (int, string, []byte, error) {
		r, err := l.fetch.Fetch(ctx, url, Validators{})
		return r.Status, r.Rate.RetryAfter, r.Body, err
	}
}

// assimilateFor is how old a reading may be and still be assimilated
// (D-64, watchpost D-184): an hour, past which it does worse than GloTEC
// alone (week.py section 12).
const assimilateFor = time.Hour

// assimilated is the backgrounds with the readings held up to an hour old
// assimilated onto them, and the oldest and newest of those (R-9.2, D-64);
// the backgrounds as they were when fewer than three are that fresh.
func (l *Library) assimilated(b background.Fields, now time.Time) (res assimilate.Result, oldest, newest time.Time) {
	var rs []assimilate.Reading
	for _, h := range l.giro.Readings() {
		if now.Sub(h.Reading.At) > assimilateFor {
			continue
		}
		rs = append(rs, assimilate.Reading{Lat: h.Station.Lat, Lon: h.Station.Lon, FoF2: float64(h.Reading.FoF2),
			M3000: float64(h.Reading.M3000), HasM: h.Reading.HasM300, Unscored: h.Reading.Unscored})
		if oldest.IsZero() || h.Reading.At.Before(oldest) {
			oldest = h.Reading.At
		}
		newest = later(newest, h.Reading.At)
	}
	res, err := assimilate.Onto(b.FoF2, b.M3000, rs)
	if err != nil || res.Used == 0 {
		return assimilate.Result{FoF2: b.FoF2, M3000: b.M3000}, time.Time{}, time.Time{}
	}
	return res, oldest, newest
}

// later is the later of two times.
func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// unscoredCells are the cells whose nearest station is unscored, as a
// snapshot carries them; none when nothing was assimilated.
func unscoredCells(cells []bool) Bitset {
	if len(cells) == 0 {
		return Bitset{}
	}
	out := newBitset(len(cells))
	for i, u := range cells {
		if u {
			out.Set(i)
		}
	}
	return out
}
