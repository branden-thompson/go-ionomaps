package ionomaps

import (
	"context"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/assimilate"
	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/hybrid"
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

// assimilated is the backgrounds with the readings held up to an hour old
// assimilated onto them, and the oldest and newest of those (R-9.2, D-64),
// made by internal/hybrid as the G-M3 instrument makes it (watchpost
// D-237); the backgrounds as they were, and no times, when fewer than three
// are that fresh.
func (l *Library) assimilated(b background.Fields, now time.Time) (res assimilate.Result, oldest, newest time.Time) {
	rs, oldest, newest := hybrid.Readings(l.giro.Readings(), now)
	res = hybrid.Field(b, rs)
	if res.Used == 0 {
		return res, time.Time{}, time.Time{}
	}
	return res, oldest, newest
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
			out.set(i)
		}
	}
	return out
}
