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
// the rows refused.
type Asker struct {
	thr      throttle.GIRO
	list     *stations.List
	held     map[string]Held
	Rejected int
}

// BurstDue reports whether GIRO's burst is due now (D-39).
//
// Source: watchpost D-39.
func (a *Asker) BurstDue(now time.Time) bool { return a.thr.Due(now) }

// Burst is GIRO's burst: the live stations polled and one probed, at most
// 40, each from its last reading held; a refusal or no answer stops it and
// pauses GIRO; a cancel stops it, the hour spent. Each station's latest
// reading is held.
//
// Source: watchpost D-39, D-51, D-87; go-ionomaps R-5.2, R-5.3.
func (a *Asker) Burst(ctx context.Context, now time.Time, fetch Fetch) {
	noFetch := fetch == nil
	if noFetch {
		return
	}
	noClock := now.IsZero()
	if noClock {
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
	granted := a.thr.Start(now, len(asks))
	for _, code := range asks[:granted] {
		stop := a.ask(ctx, code, now, fetch)
		if stop {
			return
		}
	}
}

// ask is one station's ask; true when the burst stops: refused, no answer,
// or cancelled.
func (a *Asker) ask(ctx context.Context, code string, now time.Time, fetch Fetch) bool {
	cancelled := ctx.Err() != nil
	if cancelled {
		return true
	}
	since := a.list.Since(code, now)
	u, err := AskURL(code, since, now)
	if err != nil {
		return false
	}
	status, retryAfter, body, err := fetch(ctx, u)
	hostCancel := ctx.Err() != nil
	if hostCancel {
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
	none := len(rep.Readings) == 0
	if none {
		return
	}
	other := rep.Station.Code != code
	if other {
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
	noClock := now.IsZero()
	if noClock {
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
