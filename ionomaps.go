package ionomaps

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/drap"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/scales"
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
// whole header (I-1) - the validators for the next ask, and
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
// ruling, so it is not an option; nor is the grid step,
// which is 2°.
type Options struct {
	Clock func() time.Time // the time now; time.Now when nil
}

// Source is one source the library reads or is built from (R-4.1). Its
// fields, which go doc does not show through the alias: Name; Hosts, the
// hosts it is asked on at run time (none for one used only at build time);
// Terms; Cite, the citation it asks for; and MaxBytes, the largest reply
// from it a host's fetcher should read (0 for none asked at run time).
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
	drap      *drap.Grid // D-RAP's last table read (R-2.5)
	drapVal   Validators
	scales    *scales.Levels // NOAA's scales, the last file read (R-2.10)
	scalesVal Validators

	giro         giro.Asker // GIRO's throttle, stations and readings held (G9)
	stationsNext bool       // the last update was NOAA's, GIRO due: the next makes the burst (D-63)

	unread  []string       // NOAA's files the last update asked for and did not read: unanswered, refused or unreadable (L-F5, watchpost D-239)
	heard   bool           // whether any NOAA file the last update asked for came back (watchpost D-239)
	trouble bool           // whether any NOAA ask the last update made was refused, failed on NOAA's side or not answered: the next update backs off (watchpost D-260)
	typical *typicalDays   // the typical days the snapshots share, the answers' and the hours ahead's (P-4, R-8.1, watchpost D-222)
	bg      lastBackground // the last background, built on again when its inputs are the same (R-8.1)

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
	return &Library{fetch: f, clock: clock, offset: seedOffset(), typical: &typicalDays{}}, nil
}

// Sources is every source the library reads or is built from, with its
// hosts, terms and citation: a host credits them, and allows only those hosts
// (R-4.1). Each call builds the list afresh, so it is the
// host's own copy; no library is needed to read it.
//
// Source: this library; each entry's terms and citation as NOTICE gives them.
func Sources() []Source { return terms.All() }
