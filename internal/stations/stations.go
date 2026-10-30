// Package stations is GIRO's station list (plan task G9.3, watchpost D-51,
// D-87, D-316): a shipped seed of every station's code and, for those GIRO
// gave a place, its position; each asked in turn, and one that misses three
// asks in a row flagged and asked less until it gives data again. Station
// metadata, never readings (R-4.3).
package stations

import (
	_ "embed"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

//go:embed seed.txt
var seedFile string

// flagAfter is how many asks in a row a station misses before it is flagged,
// and flaggedEvery how often a flagged station is asked (watchpost D-317).
const (
	flagAfter    = 3
	flaggedEvery = 6 * time.Hour
)

// sinceAtMost is the furthest back a station is asked from, and firstWindow
// how far back one with no reading held is asked from (R-5.2): the last
// hour, which holds its latest sounding.
const (
	sinceAtMost = 72 * time.Hour
	firstWindow = time.Hour
)

// Station is a GIRO station: its code, and its position once known.
type Station struct {
	Code     string
	Lat, Lon float64
	Placed   bool
}

// Place is a position a reply gave, when it gave one.
type Place struct {
	Lat, Lon float64
	Known    bool
}

// List is the station list's state: every station, when each last gave
// data, each one's misses in a row, when each flagged one was last asked,
// and the rotation's place.
type List struct {
	known  []Station
	index  map[string]int
	heard  map[string]time.Time
	misses map[string]int
	asked  map[string]time.Time
	pollAt int
}

var (
	errSeed = errors.New("stations: the shipped seed is not a station list")
	codeRE  = regexp.MustCompile(`^[A-Z0-9]{5}$`)
)

// Seed is the list at a cold start: every station the seed names, none
// flagged but those with no place (watchpost D-51, D-316).
//
// Source: GIRO's DIDBase station form, as asked on 2026-10-05, and the
// positions GIRO's replies state (seed.txt).
func Seed() (*List, error) {
	l := &List{index: map[string]int{}, heard: map[string]time.Time{}, misses: map[string]int{}, asked: map[string]time.Time{}}
	for line := range strings.Lines(seedFile) {
		f := strings.Fields(line)
		comment := len(f) == 0 || strings.HasPrefix(f[0], "#")
		if comment {
			continue
		}
		s, ok := parseStation(f)
		if !ok {
			return nil, errSeed
		}
		l.index[s.Code] = len(l.known)
		l.known = append(l.known, s)
	}
	none := len(l.known) == 0
	if none {
		return nil, errSeed
	}
	return l, nil
}

// parseStation is one seed line: a code, and a position on the globe or
// none; false for anything else.
func parseStation(f []string) (Station, bool) {
	fields := len(f)
	if fields != 1 && fields != 3 {
		return Station{}, false
	}
	if !codeRE.MatchString(f[0]) {
		return Station{}, false
	}
	if fields == 1 {
		return Station{Code: f[0]}, true
	}
	lat, errA := strconv.ParseFloat(f[1], 64)
	lon, errO := strconv.ParseFloat(f[2], 64)
	if errA != nil || errO != nil {
		return Station{}, false
	}
	onGlobe := lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
	if !onGlobe {
		return Station{}, false
	}
	return Station{Code: f[0], Lat: lat, Lon: lon, Placed: true}, true
}

// Flagged reports whether a station is flagged: it missed flagAfter asks in
// a row, or, with no place, has never given data (watchpost D-316, D-317).
//
// Source: watchpost D-316, D-317.
func (l *List) Flagged(code string) bool {
	i, known := l.index[code]
	if !known {
		return false
	}
	if l.misses[code] >= flagAfter {
		return true
	}
	_, heard := l.heard[code]
	return !heard && !l.known[i].Placed
}

// Plan is an hour's asks within a budget (D-87): the stations not flagged,
// in rotation from where the last burst stopped, then the flagged ones due
// again - each every flaggedEvery - those that have given data before ahead
// of those never heard, and among each the one asked longest ago first. One
// place is kept for the flagged due, so one can return however many others
// there are (D-87). The whole is the update's timing model, rule 1.
//
// Source: watchpost D-87, D-316, D-317.
func (l *List) Plan(now time.Time, budget int) []string {
	if budget <= 0 {
		return nil
	}
	if now.IsZero() {
		return nil // nothing is due without the time
	}
	var active, due []string
	for _, s := range l.known {
		switch {
		case !l.Flagged(s.Code):
			active = append(active, s.Code)
		case now.Sub(l.asked[s.Code]) >= flaggedEvery:
			due = append(due, s.Code)
		}
	}
	room := budget
	if len(due) > 0 {
		room-- // a place kept for the flagged due (D-87)
	}
	order := make([]string, 0, len(active)+len(due))
	for i := range min(room, len(active)) {
		order = append(order, active[(l.pollAt+i)%len(active)])
	}
	slices.SortStableFunc(due, func(a, b string) int {
		if l.reported(a) != l.reported(b) {
			return boolOrder(l.reported(b)) - boolOrder(l.reported(a)) // a station that has reported before first
		}
		return l.asked[a].Compare(l.asked[b])
	})
	order = append(order, due...)
	return order[:min(budget, len(order))]
}

// reported is whether a station has given data before, or the seed places
// it, as GIRO's form did for each station that had.
func (l *List) reported(code string) bool {
	_, heard := l.heard[code]
	return heard || l.known[l.index[code]].Placed
}

// boolOrder is 1 for true and 0 for false.
func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Asked records how a station's ask ended: with data, its misses and its
// flag cleared; without, one more miss, and when a flagged one was asked.
// A station not flagged moves the rotation on past it, so the next burst
// starts where this one stopped (watchpost D-316, D-317).
//
// Source: watchpost D-316, D-317.
func (l *List) Asked(code string, now time.Time, data bool) {
	if _, known := l.index[code]; !known {
		return
	}
	if !l.Flagged(code) {
		l.pollAt = (l.pollAt + 1) % pollAtMost
	}
	if data {
		delete(l.misses, code)
		delete(l.asked, code)
		return
	}
	l.misses[code] = min(l.misses[code]+1, flagAfter)
	if l.Flagged(code) {
		l.asked[code] = now
	}
}

// At is where the list places a station - its seed's position, or the
// first its replies gave - and whether it is placed (watchpost D-248).
//
// Source: watchpost D-248.
func (l *List) At(code string) (Place, bool) {
	i, known := l.index[code]
	if !known || !l.known[i].Placed {
		return Place{}, false
	}
	return Place{Lat: l.known[i].Lat, Lon: l.known[i].Lon, Known: true}, true
}

// Heard records that a station gave data at a time, with the position its
// reply gave: it is asked from then next time. A reading with no time is no
// data; a position off the globe is not kept, nor one for a station already
// placed, which keeps its place (watchpost D-248).
//
// Source: watchpost D-51; the station's reply.
func (l *List) Heard(code string, at time.Time, p Place) {
	i, ok := l.index[code]
	if !ok {
		return
	}
	if at.IsZero() {
		return
	}
	if last, held := l.heard[code]; !held || at.After(last) {
		l.heard[code] = at
	}
	onGlobe := p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
	if !p.Known || !onGlobe || l.known[i].Placed {
		return
	}
	l.known[i].Lat, l.known[i].Lon, l.known[i].Placed = p.Lat, p.Lon, true
}

// Since is when a station is asked from (R-5.2): its last reading held, or
// the last hour when none is, or when the last is three days old - a
// station back after a long silence is not asked for the whole of it.
//
// Source: go-ionomaps R-5.2.
func (l *List) Since(code string, now time.Time) time.Time {
	last, held := l.heard[code]
	if !held {
		return now.Add(-firstWindow)
	}
	if now.Sub(last) > sinceAtMost {
		return now.Add(-firstWindow)
	}
	return last
}

// State is the list's state as a host keeps it across restarts (watchpost
// D-213): when each station last gave data, the rotation's place, each
// station's misses in a row and when each flagged one was last asked - no
// reading, and no position, which the next reply gives again.
type State struct {
	Heard  map[string]time.Time
	PollAt int
	Misses map[string]int       `json:",omitempty"`
	Asked  map[string]time.Time `json:",omitempty"`
}

// pollAtMost is where the rotation's place wraps: a century of hourly
// bursts of at most 40 (watchpost D-39). A state past it is not one the
// list reached.
const pollAtMost = 40 * 24 * 366 * 100

// notAfter is t, or now when t lies after it: a time kept within the clock's
// skew is taken as now.
func notAfter(t, now time.Time) time.Time {
	if t.After(now) {
		return now
	}
	return t
}

// errBadState is a state this list could not have reached by now.
var errBadState = errors.New("stations: the state is not one the list reaches")

// Record is the list's state, to keep: copies, so the list's own are never
// the caller's.
//
// Source: watchpost D-213.
func (l *List) Record() State {
	return State{Heard: maps.Clone(l.heard), PollAt: l.pollAt, Misses: maps.Clone(l.misses), Asked: maps.Clone(l.asked)}
}

// Apply puts a state kept onto a seeded list, checked against now: no more
// stations than the seed names, no time more than throttle.ClockSkew after
// now (a time within it is taken as now, a clock stepped back), no miss
// count below one, no rotation place below zero or past where it wraps; a
// code the seed no longer names is dropped; a state that fails leaves the
// list as it was.
//
// Source: watchpost D-213; R-3.3.
func (l *List) Apply(s State, now time.Time) error {
	latest := now.Add(throttle.ClockSkew)
	if s.PollAt < 0 || s.PollAt >= pollAtMost {
		return errBadState
	}
	if len(s.Heard) > len(l.known) || len(s.Misses) > len(l.known) || len(s.Asked) > len(l.known) {
		return errBadState // more stations than the seed names
	}
	heard, err := knownTimes(l, s.Heard, now, latest)
	if err != nil {
		return err
	}
	asked, err := knownTimes(l, s.Asked, now, latest)
	if err != nil {
		return err
	}
	misses := make(map[string]int, len(s.Misses))
	for code, n := range s.Misses {
		if n < 1 {
			return errBadState
		}
		if _, known := l.index[code]; known {
			misses[code] = min(n, flagAfter)
		}
	}
	l.heard, l.asked, l.misses, l.pollAt = heard, asked, misses, s.PollAt
	return nil
}

// knownTimes is a kept map of times with the codes the seed no longer names
// dropped - a station retired in a later release, never the whole state
// refused over it (R2L-4) - and each time within the skew taken as now; an
// error for a time past it.
func knownTimes(l *List, kept map[string]time.Time, now, latest time.Time) (map[string]time.Time, error) {
	out := make(map[string]time.Time, len(kept))
	for code, at := range kept {
		if at.After(latest) {
			return nil, errBadState
		}
		if _, known := l.index[code]; known {
			out[code] = notAfter(at, now)
		}
	}
	return out, nil
}
