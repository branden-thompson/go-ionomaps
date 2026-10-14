package ionomaps

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

// Fetcher is the host's network: the library opens no connection of its own
// (R-5.1). It asks one address, with the validators of the last answer, and
// hands back the status, the rate headers and the body. It must not retry a
// 429: the library alone decides a back-off.
type Fetcher interface {
	Fetch(ctx context.Context, url string, validators Validators) (Response, error)
}

// Response is one answer: its status, only the rate-related headers - never a
// whole header (watchpost D-83, I-1) - the validators for the next ask, and
// the body, empty on a 304.
type Response struct {
	Status     int
	Rate       RateHeaders
	Validators Validators
	Body       []byte
}

// RateHeaders are an answer's rate-related headers, as sent.
type RateHeaders struct{ RetryAfter, Limit, Remaining, Reset string }

// Validators let a host ask again for an answer only if it has changed.
type Validators struct{ ETag, LastModified string }

// Options are the library's settings. The reference circuit is fixed by
// ruling (watchpost D-73, D-91), so it is not an option; nor is the grid step,
// which is 2° (D-138).
type Options struct {
	Clock func() time.Time // the time now; time.Now when nil
}

// Source is one source the library reads or is built from: its name, the
// hosts it is asked on at run time (none for one used only at build time),
// its terms and the citation it asks for (R-4.1).
type Source = terms.Source

// Library makes snapshots through the host's fetcher. It is safe for
// concurrent use (R-5.6): one update runs at a time, and holds what the next
// needs.
type Library struct {
	fetch Fetcher
	clock func() time.Time

	mu        sync.Mutex
	last      Snapshot
	made      bool          // a snapshot has been made
	notBefore time.Time     // NOAA is not asked before this
	backOff   time.Duration // after a refusal (D-39); zero after an answer
	gridAsks  []time.Time   // the grid asks in the last hour (D-39)
	gridName  string        // the grid held
	grid      *glotec.Grid
	solar     solar.Held
	solarDay  time.Time // the UTC day the solar file was last read
	solarVal  Validators

	giro         giro.Asker // GIRO's throttle, stations and readings held (G9)
	stationsNext bool       // the last update was NOAA's, GIRO due: the next makes the burst (D-63)

	offset typicalOffset // the typical offset learned from this library's updates (D-105)
	opts   UpdateOptions // the options the last snapshot was built with
}

// errNoFetcher is New without the host's network.
var errNoFetcher = errors.New("ionomaps: New needs the host's Fetcher; the library opens no connection of its own")

// New is a library that asks through f. It fetches nothing until an update.
//
// Source: this library.
func New(f Fetcher, o Options) (*Library, error) {
	if f == nil {
		return nil, errNoFetcher
	}
	clock := o.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Library{fetch: f, clock: clock, offset: seedOffset()}, nil
}

// Sources is every source the library reads or is built from, with its
// hosts, terms and citation: a host credits them, and allows only those hosts
// (R-4.1, watchpost D-113). Each call builds the list afresh, so it is the
// host's own copy.
//
// Source: this library; each entry's terms and citation as NOTICE gives them.
func (l *Library) Sources() []Source { return terms.All() }
