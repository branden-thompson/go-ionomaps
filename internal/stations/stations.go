// Package stations is GIRO's live-station list (plan task G9.3, watchpost
// D-51, D-87): a shipped seed of every station's code and, for the live
// ones, position; a station dropped after three days with no data; one
// station that is not live probed each update, in rotation; with more live
// than the burst holds, the live polled in rotation, those left out first
// next time, the probe always inside the burst. Station metadata, never
// readings (R-4.3).
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

// darkAfter is how long a station may give no data and still be polled
// (D-51).
const darkAfter = 72 * time.Hour

// firstWindow is how far back a station with no reading held is asked
// from (R-5.2): the last hour, which holds its latest sounding.
const firstWindow = time.Hour

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

// List is the live-station list's state: every station, when each last gave
// data, when the list was seeded, and the rotations' places.
type List struct {
	known   []Station
	index   map[string]int
	heard   map[string]time.Time
	seeded  time.Time
	pollAt  int
	probeAt int
	leftOut []string
	probe   string   // the last burst's probe, as Plan chose it
	keep    string   // a probe a burst did not reach: it keeps its turn, asked first next (watchpost D-303)
	last    []string // stations known to hang - a full ask ran out - asked last in the bursts after, until an ask of one ends any other way (watchpost D-307, D-309)
}

var (
	errSeed    = errors.New("stations: the shipped seed is not a station list")
	errNoClock = errors.New("stations: a cold start needs the time now")
	codeRE     = regexp.MustCompile(`^[A-Z0-9]{5}$`)
)

// Seed is the list at a cold start: every station the seed names, the ones
// with a position live from now (D-51).
//
// Source: GIRO's DIDBase station form, as asked on 2026-10-05, and the
// positions GIRO's replies state (seed.txt).
func Seed(now time.Time) (*List, error) {
	noClock := now.IsZero()
	if noClock {
		return nil, errNoClock // every seeded station would read as dark
	}
	l := &List{index: map[string]int{}, heard: map[string]time.Time{}, seeded: now}
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

// Live are the stations live now, in the seed's order: each that gave data
// within three days, or, placed at a cold start, has not been silent three
// days since; none with no clock.
//
// Source: watchpost D-51.
func (l *List) Live(now time.Time) []string {
	noClock := now.IsZero()
	if noClock {
		return nil
	}
	var out []string
	for _, s := range l.known {
		if l.isLive(s, now) {
			out = append(out, s.Code)
		}
	}
	return out
}

// isLive reports whether a station is live now.
func (l *List) isLive(s Station, now time.Time) bool {
	last, heard := l.heard[s.Code]
	if !heard && !s.Placed {
		return false
	}
	if !heard {
		last = l.seeded
	}
	return now.Sub(last) <= darkAfter
}

// Plan is an update's asks within a budget (D-87): the live stations to
// poll, at most one fewer than the budget, those left out last time first
// and those whose full ask ran out last (watchpost D-307); and one station
// that is not live to probe, the next in rotation - or the probe a burst did
// not reach, which kept its turn, and then first is true: it is asked before
// the stations polled (watchpost D-303).
//
// Source: watchpost D-51, D-87, D-303, D-307.
func (l *List) Plan(now time.Time, budget int) (poll []string, probe string, first bool) {
	if budget <= 0 {
		return nil, "", false
	}
	if now.IsZero() {
		return nil, "", false
	}
	live := l.Live(now)
	room := budget - 1 // the probe counts inside the burst (D-87)
	order := l.rotation(live)
	poll = order[:min(room, len(order))]
	l.leftOut = order[len(poll):]
	l.pollAt += len(poll)
	kept := l.keep
	l.keep = ""
	if kept != "" && !l.isLive(l.station(kept), now) {
		l.probe = kept
		return poll, kept, true
	}
	l.probe = l.nextProbe(now)
	return poll, l.probe, false
}

// RanOut keeps whether a station's full ask ran out: one that did is known
// to hang, asked last in the bursts after, behind every station that
// answered - even when a deadline left it unreached - until an ask of it
// ends any other way (watchpost D-307, D-309; the update's timing model,
// section 3).
//
// Source: watchpost D-307, D-309.
func (l *List) RanOut(code string, ranOut bool) {
	if code == "" {
		return // no station named: nothing to keep
	}
	l.last = slices.DeleteFunc(l.last, func(c string) bool { return c == code })
	if ranOut {
		l.last = append(l.last, code)
	}
}

// Hangs is whether a station is known to hang: its last full ask ran out
// and no ask of it has ended another way since (watchpost D-307, D-310).
//
// Source: watchpost D-307, D-310.
func (l *List) Hangs(code string) bool { return slices.Contains(l.last, code) }

// Unasked puts the stations a stopped burst did not reach first for the
// next, ahead of those left out by the budget, so a burst that stops at the
// same station each hour still moves on (watchpost D-247); a code the list
// does not poll now is passed over, but the burst's probe among the codes
// keeps its turn and is asked first next (watchpost D-303); a station known
// to hang among them stays last, as rotation places it (watchpost D-309).
// When a run of failures
// stopped it, the rotation's head moves past the station named past - the
// one whose failure ended the run - so stations dead at the head cannot stop
// a burst again while the rest still answer (watchpost D-287); a burst GIRO
// refused or denied, the host cancelled, or the update's end cut short
// names none and moves nothing.
//
// Source: watchpost D-247, D-287, D-303, D-309.
func (l *List) Unasked(codes []string, past string, now time.Time) {
	if l.probe != "" && slices.Contains(codes, l.probe) {
		l.keep = l.probe // the probe not reached keeps its turn (watchpost D-303)
	}
	live := l.Live(now)
	if len(live) == 0 {
		return // none live: none to put first, and no head to move
	}
	if at := slices.Index(live, past); at >= 0 {
		l.pollAt += (at + 1 - l.pollAt%len(live) + len(live)) % len(live) // the head past it
	}
	var first []string
	for _, c := range codes {
		if slices.Contains(live, c) && !slices.Contains(first, c) {
			first = append(first, c)
		}
	}
	rest := slices.DeleteFunc(slices.Clone(l.leftOut), func(c string) bool { return slices.Contains(first, c) })
	l.leftOut = append(first, rest...)
}

// rotation is the live stations in the order they are polled: those left
// out last time first, then the rest from where the rotation stands, and
// those known to hang last of all, in the order they ran out (watchpost
// D-307, D-309; the update's timing model, section 3).
func (l *List) rotation(live []string) []string {
	n := len(live)
	if n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for _, s := range l.leftOut {
		if slices.Contains(live, s) && !l.Hangs(s) { // known to hang: last, whatever left it out (watchpost D-309)
			out = append(out, s)
		}
	}
	for i := range n {
		c := live[(l.pollAt%n+i)%n]
		if !slices.Contains(out, c) && !l.Hangs(c) { // each once; those known to hang come after
			out = append(out, c)
		}
	}
	for _, c := range l.last { // known to hang: behind every station that answered, in the order they ran out
		if slices.Contains(live, c) && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// nextProbe is the next station that is not live, in the seed's order from
// where the probe rotation stands; "" when every station is live.
func (l *List) nextProbe(now time.Time) string {
	n := len(l.known)
	if n == 0 {
		return ""
	}
	for i := range n {
		s := l.known[(l.probeAt+i)%n]
		if !l.isLive(s, now) {
			l.probeAt = (l.probeAt + i + 1) % n
			return s.Code
		}
	}
	return ""
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
// reply gave: the station is live, and asked from then next time. A reading
// with no time is no sign of life; a position off the globe is not kept,
// nor one for a station already placed, which keeps its place (watchpost
// D-248).
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
// station back from dark is not asked for its whole silence.
//
// Source: go-ionomaps R-5.2.
func (l *List) Since(code string, now time.Time) time.Time {
	last, held := l.heard[code]
	if !held {
		return now.Add(-firstWindow)
	}
	if now.Sub(last) > darkAfter {
		return now.Add(-firstWindow)
	}
	return last
}

// State is the list's state as a host keeps it across restarts (watchpost
// D-213): when it was seeded, when each station last gave data, and the
// rotations' places - no reading, and no position, which the next reply
// gives again.
type State struct {
	Seeded          time.Time
	Heard           map[string]time.Time
	PollAt, ProbeAt int
	LeftOut         []string
	Keep            string   `json:",omitempty"` // a probe not reached, asked first next (watchpost D-303)
	Last            []string `json:",omitempty"` // stations whose full ask ran out, asked last (watchpost D-307)
}

// pollAtMost is the most polls a list reaches: a century of hourly bursts
// of at most 40 (watchpost D-39). A state past it is not one the list
// reached, and adding to it could overflow.
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
	return State{Seeded: l.seeded, Heard: maps.Clone(l.heard), PollAt: l.pollAt, ProbeAt: l.probeAt, LeftOut: slices.Clone(l.leftOut), Keep: l.keep, Last: slices.Clone(l.last)}
}

// Apply puts a state kept onto a seeded list, checked against now: every
// station one the seed names, none left out twice, no time more than
// throttle.ClockSkew after now (a time within it is taken as now, a clock
// stepped back), no rotation place below zero or past what a century of
// bursts reaches; a state that fails leaves the list as it was.
//
// Source: watchpost D-213; R-3.3.
func (l *List) Apply(s State, now time.Time) error {
	latest := now.Add(throttle.ClockSkew)
	if s.Seeded.IsZero() || s.Seeded.After(latest) || s.PollAt < 0 || s.ProbeAt < 0 || s.ProbeAt >= len(l.known) {
		return errBadState
	}
	if s.PollAt > pollAtMost {
		return errBadState // more polls than a century of bursts
	}
	if len(s.Heard) > len(l.known) || len(s.LeftOut) > len(l.known) || len(s.Last) > len(l.known) {
		return errBadState // more stations than the seed names
	}
	// A code the seed no longer names - a station retired in a later release
	// - is dropped, never the whole state refused over it (R2L-4).
	heard := make(map[string]time.Time, len(s.Heard))
	for code, at := range s.Heard {
		if at.After(latest) {
			return errBadState
		}
		if _, known := l.index[code]; known {
			heard[code] = notAfter(at, now)
		}
	}
	out := make(map[string]bool, len(s.LeftOut))
	var leftOut []string
	for _, code := range s.LeftOut {
		if out[code] {
			return errBadState // a station left out twice would be asked twice in one burst (RI-5)
		}
		out[code] = true
		if _, known := l.index[code]; known {
			leftOut = append(leftOut, code)
		}
	}
	if _, known := l.index[s.Keep]; s.Keep != "" && !known {
		s.Keep = "" // a probe the seed no longer names is dropped (R2L-4)
	}
	last := slices.DeleteFunc(slices.Clone(s.Last), func(code string) bool { _, known := l.index[code]; return !known })
	l.seeded, l.heard, l.pollAt, l.probeAt, l.leftOut, l.keep, l.last = notAfter(s.Seeded, now), heard, s.PollAt, s.ProbeAt, leftOut, s.Keep, last
	return nil
}

// station is a station by its code.
func (l *List) station(code string) Station {
	i, ok := l.index[code]
	if !ok {
		return Station{}
	}
	return l.known[i]
}
