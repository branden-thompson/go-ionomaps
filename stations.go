package ionomaps

import (
	"context"

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
