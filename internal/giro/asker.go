package giro

import (
	"context"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// The statuses a burst reads.
const (
	statusOK      = 200
	statusRefused = 429
)

// heldFor is how long a reading is kept after it was taken: as long as a
// station stays live (watchpost D-51).
const heldFor = 72 * time.Hour

// Fetch is one ask through the host's network: the status, the
// Retry-After sent, the body; an error for no answer.
type Fetch func(ctx context.Context, url string) (status int, retryAfter string, body []byte, err error)

// Held is the one reading held for a station (R-5.2, P-11): its latest, and
// where the station is.
type Held struct {
	Reading Reading
	Station Station
}

// Asker is GIRO's side of an update (plan task G9, D-39, D-51, D-87): the
// throttle, the live-station list, the latest reading held a station, and
// the rows the last burst refused.
type Asker struct {
	thr      throttle.GIRO
	list     *stations.List
	held     map[string]Held
	Rejected int
}

// State is GIRO's side as a host keeps it across restarts (watchpost
// D-213): the throttle, and the live-station list once seeded - never a
// reading.
type State struct {
	Throttle throttle.State
	Stations *stations.State // nil before the first burst seeded the list
}

// Export is GIRO's side, to keep.
//
// Source: watchpost D-213.
func (a *Asker) Export() State {
	s := State{Throttle: a.thr.Kept()}
	if a.list != nil {
		st := a.list.Record()
		s.Stations = &st
	}
	return s
}

// Import puts a state kept onto an asker, checked against now; one that
// fails leaves the asker as it was.
//
// Source: watchpost D-213.
func (a *Asker) Import(s State, now time.Time) error {
	thr, err := throttle.Resume(s.Throttle, now)
	if err != nil {
		return err
	}
	list := a.list
	if s.Stations != nil {
		if list, err = stations.Seed(now); err != nil {
			return err
		}
		if err = list.Apply(*s.Stations, now); err != nil {
			return err
		}
	}
	a.thr, a.list = thr, list
	return nil
}

// Paused reports whether GIRO is backing off after a refusal now (D-39).
//
// Source: watchpost D-39.
func (a *Asker) Paused(now time.Time) bool { return a.thr.Holding(now) }

// BurstDue reports whether GIRO's burst is due now (D-39).
//
// Source: watchpost D-39.
func (a *Asker) BurstDue(now time.Time) bool { return a.thr.Due(now) }

// Burst is GIRO's burst: the live stations polled and one probed, at most
// 40, each from its last reading held; a refusal or no answer stops it and
// pauses GIRO; a cancel stops it, the hour spent once anything was asked -
// a burst whose context ended before its first ask spends none. Each
// station's latest reading is held.
//
// Source: watchpost D-39, D-51, D-87; go-ionomaps R-5.2, R-5.3.
func (a *Asker) Burst(ctx context.Context, now time.Time, fetch Fetch) {
	if fetch == nil {
		return
	}
	if now.IsZero() {
		return // the throttle cannot time a burst without the time
	}
	if a.list == nil {
		list, err := stations.Seed(now)
		if err != nil {
			return // the shipped seed is broken: its own test fails first
		}
		a.list = list
	}
	poll, probe := a.list.Plan(now, throttle.MaxBurst)
	asks := poll
	if probe != "" {
		asks = append(asks, probe)
	}
	before := a.thr
	granted := a.thr.Start(now, len(asks))
	if granted > 0 {
		a.Rejected = 0 // the rows refused are this burst's
	}
	for i, code := range asks[:granted] {
		stopped := ctx.Err() != nil
		if stopped && i == 0 {
			a.thr = before // nothing asked: GIRO saw no burst
		}
		if stopped {
			return
		}
		if a.ask(ctx, code, now, fetch) {
			return
		}
	}
}

// ask is one station's ask; true when the burst stops: refused, no answer,
// or cancelled during the ask.
func (a *Asker) ask(ctx context.Context, code string, now time.Time, fetch Fetch) bool {
	since := a.list.Since(code, now)
	u, err := AskURL(code, since, now)
	if err != nil {
		return false // unreachable: the code is the seed's, checked at Seed, and since is never after now
	}
	status, retryAfter, body, err := fetch(ctx, u)
	if ctx.Err() != nil {
		return true // the host's cancel: no refusal of GIRO's
	}
	refused := err != nil || status == statusRefused
	if refused {
		a.thr.Refused(now, retryAfter)
		return true
	}
	a.thr.Answered()
	if status != statusOK {
		return false
	}
	rep, err := ParseReply(body, code, since, now, now)
	if err != nil {
		a.Rejected++
		return false
	}
	a.Rejected += rep.Rejected
	a.hold(code, rep)
	return false
}

// hold keeps a reply's latest reading for its station, and tells the list
// the station is live, where it is.
func (a *Asker) hold(code string, rep Reply) {
	if len(rep.Readings) == 0 {
		return
	}
	if rep.Station.Code != code {
		return // never held under another station's code
	}
	latest := rep.Readings[0]
	for _, x := range rep.Readings[1:] {
		if x.At.After(latest.At) {
			latest = x
		}
	}
	if a.held == nil {
		a.held = map[string]Held{}
	}
	a.held[code] = Held{Reading: latest, Station: rep.Station}
	a.list.Heard(code, latest.At, stations.Place{Lat: rep.Station.Lat, Lon: rep.Station.Lon, Known: true})
}

// LetGo lets go of readings older than a station stays live (D-51): a
// reading is reused between bursts with its age (R-5.7), not for ever.
//
// Source: watchpost D-51; go-ionomaps R-5.7.
func (a *Asker) LetGo(now time.Time) {
	if now.IsZero() {
		return // nothing is judged stale without the time
	}
	for code, h := range a.held {
		if now.Sub(h.Reading.At) > heldFor {
			delete(a.held, code)
		}
	}
}

// Readings are the readings held, one a station.
//
// Source: go-ionomaps R-5.2.
func (a *Asker) Readings() []Held {
	out := make([]Held, 0, len(a.held))
	for _, h := range a.held {
		out = append(out, h)
	}
	return out
}
