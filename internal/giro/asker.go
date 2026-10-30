package giro

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/deadline"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// The statuses a burst reads; every other is a miss.
const (
	statusOK           = 200
	statusUnauthorized = 401
	statusForbidden    = 403
	statusRefused      = 429
)

// askTimeout is how long one station's ask may take; an ask is whole or
// none, so none starts without it and deadline.Margin left in the host's
// update (watchpost D-275, D-301, D-316). A burst of 39 took about 36 s in
// PLAN, under a second an ask.
var askTimeout = 15 * time.Second

// movedKm is how far from where the list places a station its reply may
// put it and still be read (watchpost D-248).
const movedKm = 50

// Outcome is how the last burst ended (watchpost D-247).
type Outcome uint8

// The outcomes.
const (
	Answered    Outcome = iota // the burst ran, or none has been made; stations may have missed one by one
	Unavailable                // GIRO refused the burst, a 429: paused, D-39's back-off
	Denied                     // a 401 or 403: GIRO refused this host, paused throttle.DeniedFor
)

// result is how one station's ask ended.
type result uint8

// The results.
const (
	data      result = iota // a reply holding at least one reading
	missed                  // no data: a timeout, an error, a status not named, a reply with no reading (watchpost D-316)
	refused                 // GIRO's 429
	denied                  // GIRO refused this host: a 401 or 403
	cancelled               // the host's cancel
)

// heldFor is how long a reading is kept after it was taken (watchpost
// D-51).
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
// throttle, the station list, the latest reading held a station, and the
// last burst's counts.
type Asker struct {
	thr      throttle.GIRO
	list     *stations.List
	held     map[string]Held
	Rejected int     // the last burst's rows refused by range checks
	Failed   int     // the last burst's stations not flagged that gave no data, the burst going on (watchpost D-247, D-316, D-321)
	NoRoom   bool    // the last burst found no room for its first ask: nothing asked, no hour spent (watchpost D-298)
	Outcome  Outcome // how the last burst ended, carried until the next
}

// State is GIRO's side as a host keeps it across restarts (watchpost
// D-213): the throttle, and the station list once seeded - never a
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
		if list, err = stations.Seed(); err != nil {
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

// Burst is one hourly burst (D-39, D-87; the update's timing model, rules 1
// to 6): the stations in turn within the budget, each ask whole or none
// inside the host's update. A station that gives no data is a miss, counted
// in Failed, and the burst moves on; a 429 stops it and backs GIRO off, a
// 401 or 403 stops it and pauses GIRO a day (watchpost D-247, D-318); a
// flagged station's miss is not counted (watchpost D-321). The
// host's cancel, or no room for the next ask, ends it there, and the next
// burst carries on from where it stopped; with nothing asked, no hour is
// spent and the last burst's counts stand (watchpost D-298, D-302). Each
// station's latest reading is held.
//
// Source: watchpost D-39, D-51, D-87, D-247, D-316; go-ionomaps R-5.2, R-5.3.
func (a *Asker) Burst(ctx context.Context, now time.Time, fetch Fetch) {
	a.NoRoom = false // this burst's own, whatever ends it
	if fetch == nil {
		return
	}
	if now.IsZero() {
		return // the throttle cannot time a burst without the time
	}
	if a.list == nil {
		list, err := stations.Seed()
		if err != nil {
			return // the shipped seed is broken: its own test fails first
		}
		a.list = list
	}
	asks := a.list.Plan(now, throttle.MaxBurst)
	before, counts := a.thr, a.counts()
	granted := a.thr.Start(now, len(asks))
	if granted > 0 {
		a.Rejected, a.Failed, a.Outcome = 0, 0, Answered // the counts are this burst's
	}
	for i, code := range asks[:granted] {
		limit, _ := deadline.Room(ctx, askTimeout, askTimeout) // whole or none
		if ctx.Err() != nil || limit == 0 {
			if i == 0 {
				a.restore(before, counts, ctx.Err() == nil) // nothing asked: GIRO saw no burst
			}
			return
		}
		flagged := a.list.Flagged(code)
		res, retryAfter := a.ask(ctx, code, now, fetch, limit)
		if a.stops(res, retryAfter, now) {
			return
		}
		a.list.Asked(code, now, res == data)
		if res == missed && !flagged {
			a.Failed++ // a flagged station giving nothing is no news (watchpost D-321)
		}
	}
}

// burstCounts is what a burst says of itself, kept across a burst that asks
// nothing (watchpost D-302).
type burstCounts struct {
	rejected, failed int
	outcome          Outcome
}

// counts is the last burst's counts.
func (a *Asker) counts() burstCounts {
	return burstCounts{a.Rejected, a.Failed, a.Outcome}
}

// restore puts back what a burst that asked nothing changed: GIRO's throttle
// and the last burst's counts; noRoom says the deadline, not a cancel, left
// it no room (watchpost D-298).
func (a *Asker) restore(thr throttle.GIRO, c burstCounts, noRoom bool) {
	a.thr, a.NoRoom = thr, noRoom
	a.Rejected, a.Failed, a.Outcome = c.rejected, c.failed, c.outcome
}

// stops is whether the burst ends after an ask: the host's cancel, or GIRO
// refusing it, paused as the refusal asks (watchpost D-39, D-247, D-318).
func (a *Asker) stops(res result, retryAfter string, now time.Time) bool {
	switch res {
	case cancelled:
		return true
	case refused:
		a.thr.Refused(now, retryAfter)
		a.Outcome = Unavailable
		return true
	case denied:
		a.thr.Deny(now)
		a.Outcome = Denied
		return true
	}
	return false
}

// ask is one station's ask, how it ended, and the Retry-After GIRO sent.
func (a *Asker) ask(ctx context.Context, code string, now time.Time, fetch Fetch, limit time.Duration) (result, string) {
	since := a.list.Since(code, now)
	u, err := AskURL(code, since, now)
	if err != nil {
		return missed, "" // unreachable: the code is the seed's, checked at Seed, and since is never after now
	}
	askCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	status, retryAfter, body, err := fetch(askCtx, u)
	if ctx.Err() != nil {
		return cancelled, ""
	}
	if r, ended := ended(status, err); ended {
		return r, retryAfter
	}
	a.thr.Answered()
	rep, err := ParseReply(body, code, since, now, now)
	if err != nil {
		a.Rejected++
		return missed, ""
	}
	a.Rejected += rep.Rejected
	if !a.hold(code, rep) {
		return missed, "" // no reading, or a reply refused - another station's, or one placed elsewhere: no data (watchpost D-316)
	}
	return data, ""
}

// ended is an ask that gave no answer to read, and how: GIRO's refusal of
// the request (a 429), its refusal of this host (a 401 or 403), or else a
// miss - a server error, a 404, a 410 for a station gone, any status not
// named, or no answer, a timeout among them (watchpost D-247, D-272, D-316).
func ended(status int, err error) (result, bool) {
	if err != nil {
		return missed, true
	}
	if status == statusRefused {
		return refused, true
	}
	if status == statusUnauthorized || status == statusForbidden {
		return denied, true
	}
	if status != statusOK {
		return missed, true
	}
	return data, false // a 200: read it to know whether it holds data
}

// hold keeps a reply's latest reading for its station, tells the list when
// and where the station gave it, and is whether it was kept.
func (a *Asker) hold(code string, rep Reply) bool {
	if len(rep.Readings) == 0 {
		return false
	}
	if rep.Station.Code != code {
		return false // never held under another station's code
	}
	if at, placed := a.list.At(code); placed && limits.Distance(at.Lat, at.Lon, rep.Station.Lat, rep.Station.Lon) > movedKm {
		a.Rejected++ // a placed station keeps its place: a reply putting it elsewhere is refused (watchpost D-248)
		return false
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
	return true
}

// LetGo lets go of readings older than heldFor (D-51): a reading is reused
// between bursts with its age (R-5.7), not for ever.
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
