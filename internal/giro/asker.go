package giro

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// The statuses a burst reads; every other is its station's failure.
const (
	statusOK           = 200
	statusUnauthorized = 401
	statusForbidden    = 403
	statusRefused      = 429
)

// failuresInARow is how many stations failing one after another, before
// any has answered, stop a burst as GIRO itself down (watchpost D-247,
// D-271); and how many asks running out of the full askTimeout one after
// another stop it whatever answered before (watchpost D-286).
const failuresInARow = 3

// timeoutsAtTheEnd is how many asks running out of the full askTimeout one
// after another call GIRO down when the burst reaches the end of the host's
// update: one is that station's own failure (watchpost D-296).
const timeoutsAtTheEnd = 2

// askTimeout is how long one station's ask may take while the host's
// update has room for it: a GIRO that stops answering is called down and
// backed off by asks running out in a row (watchpost D-275, D-286, D-296).
// A burst of 39 took about 36 s in PLAN, under a second an ask.
//
// askMargin is the time an ask leaves the host's update after it, and
// askFloor the least an ask is given: past the room for a full askTimeout,
// an ask gets the time left less askMargin, and none starts with less than
// askFloor to give it (watchpost D-295).
var askTimeout, askMargin, askFloor = 15 * time.Second, time.Second, 2 * time.Second

// movedKm is how far from where the list places a station its reply may
// put it and still be read (watchpost D-248).
const movedKm = 50

// Outcome is how the last burst ended (watchpost D-247).
type Outcome uint8

// The outcomes.
const (
	Answered    Outcome = iota // the burst ran, or none has been made; stations may have failed one by one
	Unavailable                // a 429, failuresInARow failures in a row before any answer, failuresInARow full asks run out in a row, or timeoutsAtTheEnd of them as the host's update ends: paused, D-39's back-off
	Denied                     // a 401 or 403: GIRO refused this host, paused throttle.DeniedFor
)

// result is how one station's ask ended.
type result uint8

// The results.
const (
	answered   result = iota // an answer, whatever it held
	failed                   // the station's own: a server error, a 404, an error that is no timeout
	timedOut                 // no answer within the full askTimeout: the station's failure, and GIRO down when it runs on
	notReached               // no answer within a shortened ask, the host's update ending: asked first next hour, no failure (watchpost D-295)
	refused                  // GIRO's 429
	denied                   // GIRO refused this host: a 401 or 403
	cancelled                // the host's cancel
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
	thr        throttle.GIRO
	list       *stations.List
	held       map[string]Held
	Rejected   int     // the last burst's rows refused by range checks
	Failed     int     // the last burst's stations whose own ask failed, the burst going on (watchpost D-247)
	NotReached int     // the last burst's stations not asked, or not answered within a shortened ask: asked first next hour (watchpost D-297)
	NoRoom     bool    // the last burst found no room for its first ask: nothing asked, no hour spent (watchpost D-298)
	Outcome    Outcome // how the last burst ended, carried until the next
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
// server error, a 404, a 410, any status not named, no answer - is counted
// and the burst goes on; a 429, failuresInARow failures in a row before any
// station has answered, or failuresInARow full asks run out in a row
// whatever answered, stops it and pauses GIRO by D-39's back-off; a 401 or
// 403 stops it and pauses GIRO for a day (watchpost D-247, D-271, D-272,
// D-286). Past the room for a full ask, each ask gets the time the host's
// update leaves; when even askFloor does not fit, or a shortened ask runs
// out, the burst ends there, its stations not reached asked first next hour,
// and GIRO is called down only if its last timeoutsAtTheEnd full asks ran
// out (watchpost D-291, D-295, D-296). A burst with no room for its first
// ask, or whose context ended before it, asks nothing and spends no hour
// (watchpost D-298). A run of failures that stops it moves the rotation's
// head past the station that ended it (watchpost D-287). Each station's
// latest reading is held.
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
		a.Rejected, a.Failed, a.NotReached, a.NoRoom, a.Outcome = 0, 0, 0, false, Answered // the counts are this burst's
	}
	var t tally
	for i, code := range asks[:granted] {
		limit, full := askLimit(ctx)
		if ctx.Err() != nil || limit == 0 { // the host's cancel, or no room left in its update
			a.endEarly(ctx, before, i == 0, t, asks[i:granted], now)
			return
		}
		res, retryAfter := a.ask(ctx, code, now, fetch, limit, full)
		if res == notReached { // a shortened ask ran out: the update's end, not the station's failure (watchpost D-295)
			a.endEarly(ctx, before, false, t, asks[i:granted], now)
			return
		}
		if t.add(res) {
			a.Failed++
		}
		if a.stops(res, retryAfter, t, now) {
			a.NotReached = granted - i - 1
			a.list.Unasked(asks[i+1:granted], movedPast(res, code), now)
			return
		}
	}
}

// tally is how a burst's asks have ended so far (watchpost D-271, D-286,
// D-296): the stations failed one after another, the full asks run out one
// after another, and whether any station answered.
type tally struct {
	inARow, timeouts int
	anyAnswered      bool
}

// add counts one ask's end, and is whether it was the station's failure.
func (t *tally) add(res result) bool {
	t.anyAnswered = t.anyAnswered || res == answered
	t.timeouts++
	if res != timedOut {
		t.timeouts = 0 // the run of asks run out ends
	}
	if res != failed && res != timedOut {
		t.inARow = 0
		return false
	}
	t.inARow++
	return true
}

// askLimit is how long the next ask may take, and whether that is the full
// askTimeout: the full one while the host's update has room for it and
// askMargin after; past that, the time left less askMargin; zero when that
// would be under askFloor (watchpost D-291, D-295). With no deadline set,
// every ask is a full one.
func askLimit(ctx context.Context) (limit time.Duration, full bool) {
	left, set := TimeLeft(ctx)
	if !set {
		return askTimeout, true // no deadline: the burst runs to its end
	}
	left -= askMargin
	if left >= askTimeout {
		return askTimeout, true
	}
	if left < askFloor {
		return 0, false // not even a shortened ask fits
	}
	return left, false
}

// endEarly ends a burst before its last ask: the host's cancel, or the end
// of its update. With nothing asked, GIRO saw no burst and the hour is not
// spent (watchpost D-298). At the update's end GIRO is called down when its
// last timeoutsAtTheEnd full asks ran out; otherwise nothing is backed off
// (watchpost D-296). The stations not reached are asked first next hour,
// and the rotation's head stays: the stations that ran out were the burst's
// last already.
func (a *Asker) endEarly(ctx context.Context, before throttle.GIRO, nothingAsked bool, t tally, rest []string, now time.Time) {
	if nothingAsked {
		a.thr, a.NoRoom = before, ctx.Err() == nil // nothing asked: GIRO saw no burst
	}
	if ctx.Err() == nil && t.timeouts >= timeoutsAtTheEnd {
		a.thr.Refused(now, "")
		a.Outcome = Unavailable
	}
	a.NotReached = len(rest)
	a.list.Unasked(rest, "", now)
}

// movedPast is the station a stopped burst's rotation head moves past: the
// one whose failure ended a run of failures or of asks run out (watchpost
// D-287); none when GIRO refused or denied the burst.
func movedPast(res result, code string) string {
	if res != failed && res != timedOut {
		return ""
	}
	return code
}

// stops is whether the burst ends after an ask, pausing GIRO as the ask's
// end asks (watchpost D-39, D-247): failures in a row are GIRO itself down
// only while nothing in the burst has answered - after an answer, each
// failure is its station's own (watchpost D-271) - but full asks running
// out in a row are GIRO down whatever answered before (watchpost D-286).
func (a *Asker) stops(res result, retryAfter string, t tally, now time.Time) bool {
	switch {
	case res == cancelled:
		return true // the host's cancel: no refusal of GIRO's
	case res == denied:
		a.thr.Deny(now)
		a.Outcome = Denied
		return true
	case res == refused || (t.inARow >= failuresInARow && !t.anyAnswered) || t.timeouts >= failuresInARow:
		a.thr.Refused(now, retryAfter)
		a.Outcome = Unavailable
		return true
	}
	return false
}

// ask is one station's ask, how it ended, and the Retry-After GIRO sent.
func (a *Asker) ask(ctx context.Context, code string, now time.Time, fetch Fetch, limit time.Duration, full bool) (result, string) {
	since := a.list.Since(code, now)
	u, err := AskURL(code, since, now)
	if err != nil {
		return answered, "" // unreachable: the code is the seed's, checked at Seed, and since is never after now
	}
	askCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	status, retryAfter, body, err := fetch(askCtx, u)
	if ctx.Err() != nil {
		return cancelled, ""
	}
	ranOut := err != nil && (askCtx.Err() == context.DeadlineExceeded || IsTimeout(err))
	if ranOut && !full {
		return notReached, "" // a shortened ask: the update's end, not the station's (watchpost D-295)
	}
	if ranOut {
		return timedOut, "" // no answer within askTimeout, or the host's client gave up first (watchpost D-286, R4L-3)
	}
	if r, ended := ended(status, err); ended {
		return r, retryAfter
	}
	a.thr.Answered()
	rep, err := ParseReply(body, code, since, now, now)
	if err != nil {
		a.Rejected++
		return answered, ""
	}
	a.Rejected += rep.Rejected
	a.hold(code, rep)
	return answered, ""
}

// ended is an ask that gave no answer to read, and how: GIRO's refusal of
// the request (a 429), its refusal of this host (a 401 or 403), or else the
// station's own failure - a server error, a 404, a 410 for a station gone,
// any status not named, or no answer (watchpost D-247, D-272).
func ended(status int, err error) (result, bool) {
	if err != nil {
		return failed, true
	}
	if status == statusRefused {
		return refused, true
	}
	if status == statusUnauthorized || status == statusForbidden {
		return denied, true
	}
	if status != statusOK {
		return failed, true
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

// TimeLeft is the time the context's deadline leaves, and whether it has
// one; with none set, there is no limit to read. GIRO's burst and NOAA's
// round both read it before each ask (watchpost D-291, D-298).
//
// Source: watchpost D-291, D-298.
func TimeLeft(ctx context.Context) (left time.Duration, set bool) {
	at, set := ctx.Deadline()
	if !set {
		return 0, false // no deadline: the caller's own limits alone
	}
	return time.Until(at), true
}

// IsTimeout is whether an error is a timeout by its own account - the
// host's client giving up before the library's own limit does - told by the
// Timeout method the standard library's network errors carry, never by
// importing them (the library opens no connection of its own). GIRO's burst
// and NOAA's round both read it.
//
// Source: watchpost R4L-3.
func IsTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}
