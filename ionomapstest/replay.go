package ionomapstest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

// The statuses a replay answers with: a file served, or none held.
const (
	statusOK      = 200
	statusMissing = 404
)

// askTime is how FastChar takes a time.
const askTime = "2006/01/02 15:04:05"

// ErrUnknownAsk is an address the replay holds no answer for: the library
// asks only the addresses it lists (internal/terms), so a moved address
// fails the run, never answered with nothing.
var ErrUnknownAsk = errors.New("ionomapstest: the replay holds no answer for this address")

// Options are a replay's settings; the zero value serves every recorded
// file.
type Options struct {
	// WithoutGloTEC withholds GloTEC's index and grids, so the library's
	// background is the climatology alone.
	WithoutGloTEC bool
}

// Replay is the library's Fetcher answering from recorded days at its
// clock, which the caller sets; its Now is the library's Clock. Safe for
// the library's goroutines.
type Replay struct {
	in    *Recorded
	grids bool

	mu      sync.Mutex
	now     time.Time
	unknown []string
}

var _ ionomaps.Fetcher = (*Replay)(nil)

// Replay is a fetcher over the recorded days, its clock at the zero time
// until SetNow.
//
// Source: this library.
func (r *Recorded) Replay(o Options) *Replay {
	return &Replay{in: r, grids: !o.WithoutGloTEC}
}

// SetNow moves the replay's clock.
//
// Source: this library.
func (p *Replay) SetNow(t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = t
}

// Now is the replay's clock: the library's Clock option.
//
// Source: this library.
func (p *Replay) Now() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.now
}

// Unknown are the addresses asked that the replay holds no answer for.
//
// Source: this library.
func (p *Replay) Unknown() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.unknown)
}

// Fetch answers an ask as the service would have at the clock: GloTEC's
// index and its grids valid by then; the solar file's rows before the
// clock's day; a station's readings in the window asked and not after the
// clock; D-RAP and the scales, none of which dry_fetch.py keeps, missing.
//
// Source: this library; the addresses in internal/terms.
func (p *Replay) Fetch(_ context.Context, u string, _ ionomaps.Validators) (ionomaps.Response, error) {
	now := p.Now()
	switch {
	case u == terms.GloTECIndex:
		return p.index(now), nil
	case strings.HasPrefix(u, terms.GloTECIndex):
		return p.grid(strings.TrimPrefix(u, terms.GloTECIndex), now)
	case u == terms.SolarFile:
		return answer(p.in.Solar(now)), nil
	case u == terms.DRAPTable, u == terms.ScalesFile:
		return ionomaps.Response{Status: statusMissing}, nil
	case strings.HasPrefix(u, terms.FastChar+"?"):
		return p.station(u, now)
	}
	p.mu.Lock()
	p.unknown = append(p.unknown, u)
	p.mu.Unlock()
	return ionomaps.Response{}, ErrUnknownAsk
}

// answer is a 200 with a body.
func answer(body []byte) ionomaps.Response {
	return ionomaps.Response{Status: statusOK, Body: body}
}

// index is GloTEC's directory index at the clock: the grids valid by then,
// as links, as NOAA's index names them.
func (p *Replay) index(now time.Time) ionomaps.Response {
	if !p.grids {
		return ionomaps.Response{Status: statusMissing}
	}
	var names []string
	for name, valid := range p.in.grids {
		if !valid.After(now) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var b bytes.Buffer
	for _, name := range names {
		fmt.Fprintf(&b, "<a href=%q>%s</a>\n", name, name)
	}
	return answer(b.Bytes())
}

// grid is a grid's file; missing when withheld, not recorded, or valid
// after the clock.
func (p *Replay) grid(name string, now time.Time) (ionomaps.Response, error) {
	valid, known := p.in.grids[name]
	if !p.grids || !known || valid.After(now) {
		return ionomaps.Response{Status: statusMissing}, nil
	}
	body, err := os.ReadFile(filepath.Join(p.in.dir, name))
	if err != nil {
		return ionomaps.Response{}, err
	}
	return answer(body), nil
}

// station is a station's reply to the library's ask; missing for a
// station not recorded.
func (p *Replay) station(u string, now time.Time) (ionomaps.Response, error) {
	q, err := url.Parse(u)
	if err != nil {
		return ionomaps.Response{}, err
	}
	v := q.Query()
	from, errF := time.Parse(askTime, v.Get("fromDate"))
	to, errT := time.Parse(askTime, v.Get("toDate"))
	if errF != nil || errT != nil || to.Before(from) {
		return ionomaps.Response{}, ErrUnknownAsk
	}
	body, err := p.in.Reply(v.Get("ursiCode"), from, to, now)
	if errors.Is(err, ErrNoStation) {
		return ionomaps.Response{Status: statusMissing}, nil
	}
	if err != nil {
		return ionomaps.Response{}, err
	}
	return answer(body), nil
}
