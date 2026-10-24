package giro

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// The statuses a burst reads.
const (
	statusOK           = 200
	statusUnauthorized = 401
	statusForbidden    = 403
	statusNotFound     = 404 // a station GIRO has no page for: that station's failure
	statusGone         = 410
	statusRefused      = 429
	statusFailed       = 500 // and above: the station's ask failed on GIRO's side
)

// failuresInARow is how many stations failing one after another stop a
// burst as GIRO itself down (watchpost D-247).
const failuresInARow = 3

// movedKm is how far from where the list places a station its reply may
// put it and still be read (watchpost D-248).
const movedKm = 50

// Outcome is how the last burst ended (watchpost D-247).
type Outcome uint8

// The outcomes.
const (
	Answered    Outcome = iota // the burst ran, or none has been made; stations may have failed one by one
	Unavailable                // a 429, or failuresInARow stations failing in a row: paused, D-39's back-off
	Denied                     // a 401, 403 or 410: GIRO refused this host, paused throttle.DeniedFor
)

// result is how one station's ask ended.
type result uint8

// The results.
const (
	answered  result = iota // an answer, whatever it held
	failed                  // the station's own: a server error, a 404, no answer
	refused                 // GIRO's 429
	denied                  // GIRO refused this host: a 401, 403 or 410
	cancelled               // the host's cancel
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
	Rejected int     // the last burst's rows refused by range checks
	Failed   int     // the last burst's stations whose own ask failed, the burst going on (watchpost D-247)
	Outcome  Outcome // how the last burst ended, carried until the next
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
	if a.thr.Denied(now) {
		a.Outcome = Denied // a pause only a denial sets: said so across the restart
	}
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
// 40, each from its last reading held. A station whose own ask fails - a
// server error, a 404, no answer - is counted and the burst goes on; a 429,
// or failuresInARow failures in a row, stops it and pauses GIRO by D-39's
// back-off; a 401, 403 or 410 stops it and pauses GIRO for a day (watchpost
// D-247); a cancel stops it, the hour spent once anything was asked - a
// burst whose context ended before its first ask spends none. The stations
// a stopped burst did not reach are asked first at the next. Each
// station's latest reading is held.
//
// Source: watchpost D-39, D-51, D-87, D-247; go-ionomaps R-5.2, R-5.3.
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
		a.Rejected, a.Failed, a.Outcome = 0, 0, Answered // the counts are this burst's
	}
	inARow := 0
	for i, code := range asks[:granted] {
		stopped := ctx.Err() != nil
		if stopped && i == 0 {
			a.thr = before // nothing asked: GIRO saw no burst
		}
		if stopped {
			a.list.Unasked(asks[i:granted], now)
			return
		}
		res, retryAfter := a.ask(ctx, code, now, fetch)
		inARow = a.count(res, inARow)
		if a.stops(res, retryAfter, inARow, now) {
			a.list.Unasked(asks[i+1:granted], now)
			return
		}
	}
}

// count is the failures in a row after one ask: one more for a station's
// own failure, none after any other end.
func (a *Asker) count(res result, inARow int) int {
	if res != failed {
		return 0
	}
	a.Failed++
	return inARow + 1
}

// stops is whether the burst ends after an ask, pausing GIRO as the ask's
// end asks (watchpost D-39, D-247).
func (a *Asker) stops(res result, retryAfter string, inARow int, now time.Time) bool {
	switch {
	case res == cancelled:
		return true // the host's cancel: no refusal of GIRO's
	case res == denied:
		a.thr.Deny(now)
		a.Outcome = Denied
		return true
	case res == refused || inARow >= failuresInARow:
		a.thr.Refused(now, retryAfter)
		a.Outcome = Unavailable
		return true
	}
	return false
}

// ask is one station's ask, how it ended, and the Retry-After GIRO sent.
func (a *Asker) ask(ctx context.Context, code string, now time.Time, fetch Fetch) (result, string) {
	since := a.list.Since(code, now)
	u, err := AskURL(code, since, now)
	if err != nil {
		return answered, "" // unreachable: the code is the seed's, checked at Seed, and since is never after now
	}
	status, retryAfter, body, err := fetch(ctx, u)
	if ctx.Err() != nil {
		return cancelled, ""
	}
	if r, ended := ended(status, err); ended {
		return r, retryAfter
	}
	a.thr.Answered()
	if status != statusOK {
		return answered, ""
	}
	rep, err := ParseReply(body, code, since, now, now)
	if err != nil {
		a.Rejected++
		return answered, ""
	}
	a.Rejected += rep.Rejected
	a.hold(code, rep)
	return answered, ""
}

// ended is an ask that gave no answer to read, and how: the station's own
// failure, GIRO's refusal of the request, or its refusal of this host.
func ended(status int, err error) (result, bool) {
	switch {
	case err != nil || status >= statusFailed || status == statusNotFound:
		return failed, true
	case status == statusRefused:
		return refused, true
	case status == statusUnauthorized || status == statusForbidden || status == statusGone:
		return denied, true
	}
	return answered, false
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
	if at, placed := a.list.At(code); placed && limits.Distance(at.Lat, at.Lon, rep.Station.Lat, rep.Station.Lon) > movedKm {
		a.Rejected++ // a placed station keeps its place: a reply putting it elsewhere is refused (watchpost D-248)
		return
	}
	if at, placed := a.list.At(code); placed {
		rep.Station.Lat, rep.Station.Lon = at.Lat, at.Lon
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

// Readings are the readings held, one a station, in the stations' codes'
// order, so the field they make is the same however the map is walked
// (RC-13: a sum's order moves its last bits).
//
// Source: go-ionomaps R-5.2.
func (a *Asker) Readings() []Held {
	out := make([]Held, 0, len(a.held))
	for _, h := range a.held {
		out = append(out, h)
	}
	slices.SortFunc(out, func(x, y Held) int { return strings.Compare(x.Station.Code, y.Station.Code) })
	return out
}
