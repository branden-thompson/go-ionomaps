package ionomaps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/assimilate"
	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/deadline"
	"github.com/branden-thompson/go-ionomaps/internal/drap"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/scales"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

// The addresses an update asks, each defined once in internal/terms.
const (
	glotecIndex = terms.GloTECIndex
	solarFile   = terms.SolarFile
	drapTable   = terms.DRAPTable
	scalesFile  = terms.ScalesFile
)

// NOAA's limits (D-39): asked at most every ten minutes, the interval GloTEC
// publishes at; grids at most six an hour; after a refusal, a server error
// or no answer, never sooner than the ten minutes, then 15 (watchpost D-260).
const (
	noaaEvery         = 10 * time.Minute
	gridsAnHour       = 6
	longestPause      = 15 * time.Minute
	statusOK          = 200
	statusNotModified = 304
	statusRefused     = 429
	statusServerError = 500
)

// solarMaxAge is how old the F10.7 mean held may be and still build a
// field, its age said: the 30-day mean moves a few solar flux units a day
// (watchpost D-249).
const solarMaxAge = 7 * 24 * time.Hour

// gridMaxAge is how old a grid may be and still be the background: GloTEC
// publishes every ten minutes about half an hour behind, so a grid over an
// hour old means several missed, and foF2 has moved with the hour (A-5).
const gridMaxAge = time.Hour

// drapMaxAge is how old D-RAP's table may be and still say a disturbance:
// it is remade every few minutes, and a flare's absorption lasts about an
// hour (A-9).
const drapMaxAge = time.Hour

// scalesMaxAge is how old NOAA's scales file may be and still be carried:
// NOAA rewrites it within minutes of a change.
const scalesMaxAge = time.Hour

// UpdateOptions are one update's settings, the host's to give each time,
// so a change needs no new library and no new burst.
type UpdateOptions struct {
	// CorrectNoReadings moves GloTEC's foF2 by the typical offset when no
	// station readings are assimilated; off, GloTEC is
	// shown as it is. The snapshot's Offset.Corrected says when it applied.
	CorrectNoReadings bool

	// Noise is the man-made noise the reference circuit's receiver hears,
	// Residential by default; the band statuses' lower
	// limit moves with it. A change needs no request.
	Noise Noise

	// NoStations asks GIRO nothing and puts no station's reading on the
	// field: NOAA's and the climatology's alone, for a
	// host whose use GIRO's terms do not cover, or that wants none. The
	// snapshot's Inputs.NoStations says so. A change needs no request:
	// readings already held leave the field, and come back when it is
	// cleared.
	NoStations bool
}

// Update asks NOAA for what is new and returns the hour's snapshot: the
// daily solar file first, at most once a day, then GloTEC's newest grid,
// found through its index; foF2 from GloTEC (the climatology, named,
// without it or past an hour old), M(3000)F2 from the climatology,
// MUF(3000) their product, with GIRO's readings up to an hour old
// assimilated onto both, NearestKm and NearestUnscored saying where, and
// the stations' offset against GloTEC. When GIRO is due it returns NOAA's
// snapshot said StationsNext, and the next call makes GIRO's burst. Within
// ten minutes of the last ask, or while backing off, the last snapshot
// comes back said too soon, rebuilt from what is held when the options have
// changed, nothing asked. An update the host cancels is built from what is
// held and said Unfinished, with no back-off. With no F10.7 mean to be had
// there is no field: the error wraps ErrNoField, and the context's own
// error when it stopped the update, and says when NOAA is asked again.
//
// An update never runs into ctx's deadline, which is to leave it at least
// MinDeadline. Each NOAA ask has 20 s and each GIRO ask 15 s, in real time
// whatever Options.Clock says, and every ask leaves a 1 s margin after it.
// A NOAA ask that runs out, or that the host's client gives up on first,
// stops that round and backs NOAA off. A GIRO station that gives no data -
// its ask run out among the ways - is a miss, counted in
// Inputs.StationsFailed, and the burst moves on; one that misses three
// bursts running is asked every 6 hours until it gives data again, and
// GIRO itself is paused only when it refuses. No ask starts without its
// whole time and the margin left: a NOAA round ends there, nothing backed
// off, and Inputs.Unread names that file and any input the snapshot lacks -
// a young file held stands in; a GIRO burst ends there and the next carries
// on from where it stopped. With no room for any ask, nothing is asked and
// the error wraps ErrNoRoom. Inputs.StationsHeld, StationsRecent and
// NewestReading say how many stations report and how recently, for a host
// to say when the map rests on too few.
//
// Source: NOAA SWPC's GloTEC and F10.7; GIRO's DIDBase; this library.
func (l *Library) Update(ctx context.Context, o UpdateOptions) (Snapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock().UTC()
	if o.NoStations {
		l.stationsNext = false // no burst is made while the host wants none
	}
	if l.stationsNext && l.made { // NOAA's snapshot went out: now the burst (D-63)
		l.giro.Burst(ctx, now, l.giroFetch())
		if l.giro.NoRoom {
			return l.roomless(ctx, now, o) // no room for the burst's first ask: it is still to make (watchpost D-298)
		}
		l.stationsNext = false
		l.giro.LetGo(now)
		s := l.build(now, o) // NOAA's fields held, the stations' readings now on them
		switch {
		case ctx.Err() != nil:
			s.Reason = Unfinished                 // the host cancelled it mid-burst: the readings that came are on the field
			l.stationsNext = l.giro.BurstDue(now) // cancelled before its first ask: the burst is still to make
		case l.giro.Paused(now):
			s.Reason = GIROPaused // GIRO refused or did not answer the burst: the readings that came are on the field
		case len(l.unread) > 0:
			s.Reason = Partial // NOAA's round left files unread: Unread names them (watchpost D-239)
		}
		l.last, l.opts = s, o
		return s.clone(), nil
	}
	early := now.Before(l.notBefore)
	if early {
		return l.early(now, o)
	}
	l.unread, l.heard, l.trouble, l.asked, l.noRoom = l.unread[:0], false, false, false, false
	endsRound := l.refreshSolar(ctx, now) // first: with no F10.7 mean there is no field at all (L-F4)
	fieldable := l.solarUsable(now)       // without one, no grid is fetched to be thrown away (RL-2)
	if !endsRound && fieldable {          // a 429, an ask run out, or no room for the next, stops the round (D-39, watchpost D-285, D-298); a failed file does not (watchpost D-270)
		endsRound = l.refreshGrid(ctx, now)
	}
	if !endsRound && fieldable {
		endsRound = l.refreshDRAP(ctx, now)
	}
	if !endsRound && fieldable {
		endsRound = l.refreshScales(ctx, now)
	}
	stopped := ctx.Err() != nil
	if stopped {
		return l.unfinished(ctx, now, o) // the host's, not NOAA's: no back-off, no ten minutes spent
	}
	if l.noRoom && !l.asked {
		return l.roomless(ctx, now, o) // nothing asked: nothing backed off, nothing spent (watchpost D-298)
	}
	l.notBefore, l.backOff = nextAsk(now, l.backOff, l.trouble)
	l.short(now)
	if !fieldable {
		reason := Partial // NOAA answered, and the solar file did not read: Unread names it (watchpost D-239)
		if !l.heard {
			reason = Offline
		}
		return Snapshot{Computed: now, Reason: reason, Inputs: Inputs{Unread: slices.Clone(l.unread)}}, l.notYet()
	}
	s := l.build(now, o)
	l.giro.LetGo(now)
	switch { // GIRO is asked whatever NOAA answered: they are separate services (watchpost D-239)
	case !o.NoStations && l.giro.BurstDue(now):
		s.Reason, l.stationsNext = StationsNext, true // shown at once; the host updates again for the stations (D-63)
	case !l.heard:
		s.Reason = Offline // no NOAA file came back, and GIRO is not due: what is held, Unread naming each file
	case !o.NoStations && l.giro.Paused(now):
		s.Reason = GIROPaused // the stations' readings are the last held until the pause ends
	case len(l.unread) > 0:
		s.Reason = Partial // some of NOAA's files did not come back or could not be read: Unread names them
	}
	l.last, l.made, l.opts = s, true, o
	return s.clone(), nil
}

// early is the answer to an update asked before NOAA may be: the last
// snapshot, said too soon, rebuilt at its own time from what is held when
// the host's options differ from the ones it was built with (A-30); with
// none, the error that says when.
func (l *Library) early(now time.Time, o UpdateOptions) (Snapshot, error) {
	if !l.made {
		return Snapshot{Computed: now, Reason: TooSoon}, l.notYet()
	}
	if o != l.opts {
		l.last, l.opts = l.build(l.last.Computed, o), o
	}
	s := l.last.clone()
	s.Reason = TooSoon
	return s, nil
}

// unfinished is the answer to an update its context stopped: the snapshot
// built now from what is held, said Unfinished; with no F10.7 mean held,
// no field, and an error that wraps the context's own and says which.
func (l *Library) unfinished(ctx context.Context, now time.Time, o UpdateOptions) (Snapshot, error) {
	if !l.solarUsable(now) {
		why := "the host cancelled it"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			why = "its deadline passed"
		}
		return Snapshot{Computed: now, Reason: Unfinished}, fmt.Errorf("%w yet: the update stopped before NOAA's solar data arrived (%s); update again: %w", ErrNoField, why, ctx.Err())
	}
	s := l.build(now, o)
	s.Reason = Unfinished
	l.last, l.made, l.opts = s, true, o
	return s.clone(), nil
}

// roomless is the answer to an update whose deadline left no room for its
// first ask: nothing asked, nothing backed off, a burst due still to make;
// the last snapshot, said Unfinished, or none; and an error that names the
// time left and the least an update needs (watchpost D-298).
func (l *Library) roomless(ctx context.Context, now time.Time, o UpdateOptions) (Snapshot, error) {
	left, _ := deadline.Left(ctx)
	err := fmt.Errorf("%w: it left %s, and an update needs at least %s; nothing was asked, and nothing backed off", ErrNoRoom, max(left, 0).Truncate(time.Millisecond), MinDeadline)
	s, _ := l.early(now, o) // the last snapshot, rebuilt when the options differ; none made, an empty one
	s.Reason = Unfinished
	return s, err
}

// notYet is the error for an update with no field to give, once NOAA's
// next ask is set: a stopped update gives its own (unfinished).
func (l *Library) notYet() error {
	return fmt.Errorf("%w yet: NOAA's solar data could not be read, and no F10.7 mean under %d days old is held; it is asked again at the first update after %s", ErrNoField, int(solarMaxAge/(24*time.Hour)), l.notBefore.Format("15:04:05 UTC"))
}

// solarUsable is whether an F10.7 mean is held that a field may be built
// from: one taken up to solarMaxAge ago, its age said (watchpost D-249).
func (l *Library) solarUsable(now time.Time) bool {
	if !l.solar.OK {
		return false // no mean held: nothing to build from
	}
	return l.solar.Age(now) <= solarMaxAge
}

// gridUsable is whether a grid valid at a time may be the background: one
// listed, or held, up to gridMaxAge old (A-29 (1): one rule, read wherever
// a grid's age is asked). It takes a time, not the library, because the
// index's newest grid is not held; each held input's own rule is a method -
// solarUsable, heldGrid, drapUsable, scalesUsable.
func gridUsable(valid time.Time, ok bool, now time.Time) bool {
	if !ok {
		return false // none listed or held
	}
	return now.Sub(valid) <= gridMaxAge
}

// heldGrid is the grid held when it is usable at now; nil otherwise.
func (l *Library) heldGrid(now time.Time) *glotec.Grid {
	if l.grid == nil {
		return nil // none held
	}
	if !gridUsable(l.grid.Valid, true, now) {
		return nil // past its age: the climatology, as Background says
	}
	return l.grid
}

// drapUsable is whether the D-RAP table held may say a disturbance: one
// held, up to drapMaxAge old.
func (l *Library) drapUsable(now time.Time) bool {
	if l.drap == nil {
		return false // none held
	}
	return now.Sub(l.drap.Valid) <= drapMaxAge
}

// scalesUsable is whether the scales held may be carried: ones held, up to
// scalesMaxAge old.
func (l *Library) scalesUsable(now time.Time) bool {
	if l.scales == nil {
		return false // none held
	}
	return now.Sub(l.scales.Valid) <= scalesMaxAge
}

// short names each of NOAA's inputs the snapshot will lack - none held, or
// the one held past its age - that the update has not named already, so a
// snapshot short of any is never said Fresh (watchpost D-239): whatever left
// it short, an index naming no usable grid as much as a file refused.
func (l *Library) short(now time.Time) {
	lacking := []struct {
		name  string
		lacks bool
	}{
		{UnreadSolar, !l.solarUsable(now)},
		{UnreadGloTEC, l.heldGrid(now) == nil},
		{UnreadDRAP, !l.drapUsable(now)},
		{UnreadScales, !l.scalesUsable(now)},
	}
	for _, in := range lacking {
		if in.lacks && !slices.Contains(l.unread, in.name) {
			l.unread = append(l.unread, in.name)
		}
	}
}

// nextAsk is when NOAA may next be asked, and the back-off then: ten
// minutes on after an update without trouble - no refusal, no failure on
// NOAA's side, no ask unanswered - which ends the back-off; after one with
// it, ten minutes, then 15 - never sooner than a healthy NOAA is asked
// (watchpost D-260, under D-39's cap).
func nextAsk(now time.Time, backOff time.Duration, trouble bool) (time.Time, time.Duration) {
	if !trouble {
		return now.Add(noaaEvery), 0
	}
	backOff = min(max(2*backOff, noaaEvery), longestPause)
	return now.Add(backOff), backOff
}

// MinDeadline is the least time an update's context is to leave it: room
// for a NOAA ask, or two of GIRO's, and the margin each keeps; an update
// given it asks. With less, an update may
// find no room for any ask, and then nothing is asked and Update's error
// wraps ErrNoRoom.
const MinDeadline = 32 * time.Second

// ErrNoRoom is the error, wrapped, when an update's deadline left no room
// for its first ask: nothing was asked and nothing backed off, and the
// update is to be made again with a longer deadline.
var ErrNoRoom = errors.New("ionomaps: the update's deadline left no room for an ask")

// noaaAskTimeout is how long one of NOAA's asks may take, in real time. One
// that runs out, or that the host's client gives up on first, stops the
// round and backs NOAA off, so a host that accepts and never answers costs
// one ask's time; and no ask starts without it and deadline.Margin left,
// the round ending there with nothing backed off, so the update ends inside
// the host's deadline (watchpost D-285, D-298, D-301, R4L-3).
var noaaAskTimeout = 20 * time.Second

// ask is one request: the answer, whether it is one to use (200, or 304
// when validators were sent), and whether it stops the update's round - a
// 429 (D-39, watchpost D-270), an ask that ran out (watchpost D-285), or the
// host's update leaving no room for one more (watchpost D-298). It sets
// l.trouble when NOAA refused, failed on its side or did not answer, which
// backs the next update off (watchpost D-260); no room backs nothing off,
// and a failure, or an error that is no timeout, stops nothing, the file
// missed and the round going on (watchpost D-270).
func (l *Library) ask(ctx context.Context, url string, v Validators) (r Response, ok, endsRound bool) {
	if limit, _ := deadline.Room(ctx, noaaAskTimeout, noaaAskTimeout); limit == 0 {
		l.noRoom = true
		return Response{}, false, true // no room for one more ask: the round ends before the host's deadline (watchpost D-298)
	}
	l.asked = true
	askCtx, cancel := context.WithTimeout(ctx, noaaAskTimeout)
	r, err := l.fetch.Fetch(askCtx, url, v)
	timedOut := err != nil && ctx.Err() == nil && (askCtx.Err() == context.DeadlineExceeded || deadline.IsTimeout(err))
	cancel()
	l.trouble = l.trouble || err != nil || r.Status == statusRefused || r.Status >= statusServerError
	if timedOut {
		return Response{}, false, true // a stalled host: the round stops as a refusal stops it (watchpost D-285)
	}
	if err != nil {
		return Response{}, false, false
	}
	if r.Status == statusRefused {
		return r, false, true
	}
	if r.Status >= statusServerError {
		return r, false, false
	}
	ok = r.Status == statusOK || r.Status == statusNotModified && v != (Validators{})
	l.heard = l.heard || ok
	return r, ok, false
}

// refreshGrid asks the index and, when it names a grid not held and young
// enough to use, that grid, within six grid asks an hour; GloTEC is named
// unread when either does not come back or the grid cannot be read, and by
// short when the index names none young enough. endsRound is whether
// either ask stops the round: a 429, or an ask run out.
func (l *Library) refreshGrid(ctx context.Context, now time.Time) (endsRound bool) {
	idx, ok, endsRound := l.ask(ctx, glotecIndex, Validators{})
	if !ok {
		return l.missed(UnreadGloTEC, endsRound) // the grid held stays, and its age says so; else the climatology, as Background says
	}
	name, valid, listed := glotec.Newest(idx.Body, now)
	if !gridUsable(valid, listed, now) {
		return false // no grid to use: short names GloTEC unread, and none is fetched to be thrown away
	}
	if name == l.gridName {
		return false // held already: a new grid is never a 304 (D-111)
	}
	may := l.mayAskGrid(now)
	if !may {
		return false
	}
	r, ok, endsRound := l.ask(ctx, glotecIndex+name, Validators{})
	if !ok {
		return l.missed(UnreadGloTEC, endsRound)
	}
	g, err := glotec.Read(r.Body, now)
	if err != nil {
		l.unread = append(l.unread, UnreadGloTEC)
		return false // a changed format: the grid held stays, and its age says so
	}
	l.grid, l.gridName = &g, name
	return false
}

// missed names a NOAA file the update asked for and did not read -
// unanswered, refused, or not a 200 or a 304 - and passes on whether the ask
// stops the round (watchpost D-239, D-285).
func (l *Library) missed(file string, endsRound bool) bool {
	l.unread = append(l.unread, file)
	return endsRound
}

// mayAskGrid is whether a grid may be asked now, at most six in any hour
// (D-39); when it may, the ask is counted.
func (l *Library) mayAskGrid(now time.Time) bool {
	l.gridAsks = slices.DeleteFunc(l.gridAsks, func(t time.Time) bool { return now.Sub(t) >= time.Hour })
	if len(l.gridAsks) >= gridsAnHour {
		return false
	}
	l.gridAsks = append(l.gridAsks, now)
	return true
}

// refreshSolar reads the solar file once a UTC day, with the validators of
// the file the held mean was taken from; a failed ask is tried at the next
// update. endsRound is whether its ask stops the round: a 429, or run out.
func (l *Library) refreshSolar(ctx context.Context, now time.Time) (endsRound bool) {
	today := now.Truncate(24 * time.Hour)
	readToday := l.solar.OK && l.solarDay.Equal(today)
	if readToday {
		return false
	}
	r, ok, endsRound := l.ask(ctx, solarFile, l.solarVal)
	if !ok {
		return l.missed(UnreadSolar, endsRound) // the mean held stands, if one is
	}
	l.solarDay = today
	if r.Status != statusOK {
		return false // unchanged since it was read: the mean held stands
	}
	l.solar = solar.Hold(l.solar, r.Body, now)
	if !l.solar.Taken.Equal(now) {
		l.unread = append(l.unread, UnreadSolar) // the mean held stands
		return false
	}
	l.solarVal = r.Validators // the mean is this file's
	return false
}

// refreshDRAP asks for D-RAP's table at each update, with the validators of
// the table held (R-2.5, R-5.7); a table that cannot be read leaves the one
// held, its age saying so. endsRound is whether its ask stops the round: a
// 429, or run out.
func (l *Library) refreshDRAP(ctx context.Context, now time.Time) (endsRound bool) {
	r, ok, endsRound := l.ask(ctx, drapTable, l.drapVal)
	if !ok {
		return l.missed(UnreadDRAP, endsRound)
	}
	if r.Status != statusOK {
		return false // unchanged since it was read: the table held stands
	}
	g, err := drap.ParseTable(r.Body, now)
	if err != nil {
		l.unread = append(l.unread, UnreadDRAP)
		return false
	}
	l.drap, l.drapVal = &g, r.Validators
	return false
}

// refreshScales asks for NOAA's scales at each update, with D-RAP and the
// validators of the file held (R-2.10, watchpost D-88); a file that cannot
// be read leaves the one held. endsRound is whether its ask stops the
// round: a 429, or run out.
func (l *Library) refreshScales(ctx context.Context, now time.Time) (endsRound bool) {
	r, ok, endsRound := l.ask(ctx, scalesFile, l.scalesVal)
	if !ok {
		return l.missed(UnreadScales, endsRound)
	}
	if r.Status != statusOK {
		return false // unchanged since it was read: the levels held stand
	}
	s, err := scales.Read(r.Body, now)
	if err != nil {
		l.unread = append(l.unread, UnreadScales)
		return false
	}
	l.scales, l.scalesVal = &s, r.Validators
	return false
}

// scalesOf are the scales held as the snapshot carries them: none, said by
// a zero valid time, when none was read or the last is over scalesMaxAge
// old (watchpost D-88: never level 0).
func (l *Library) scalesOf(now time.Time) Scales {
	held := l.scales
	if !l.scalesUsable(now) {
		return Scales{R: ScaleUnset, S: ScaleUnset, G: ScaleUnset}
	}
	s := Scales{R: held.R, S: held.S, G: held.G, Valid: held.Valid, Outlook: make([]DayScale, len(held.Days))}
	for i, d := range held.Days {
		s.Outlook[i] = DayScale{Day: d.Day, RMinorPct: d.RMinorPct, RMajorPct: d.RMajorPct, SPct: d.SPct, G: d.G}
	}
	return s
}

// build is the snapshot at now from what is held, with the options asked:
// the typical offset learns from a live offset over readings not yet
// learned from, and the no-readings correction moves GloTEC's foF2 when no
// reading is assimilated (R-9.6).
func (l *Library) build(now time.Time, o UpdateOptions) Snapshot {
	g := l.heldGrid(now)
	b := l.bg.background(now, l.solar.Mean, g)
	res, oldest, newest := assimilate.Result{FoF2: b.FoF2, M3000: b.M3000}, time.Time{}, time.Time{}
	if !o.NoStations {
		res, oldest, newest = l.assimilated(b, now)
	}
	overGloTEC := b.FoF2From == background.GloTEC
	measured := overGloTEC && res.Used >= liveReadings
	if measured {
		l.offset.learn(res.MeanResidual, newest)
	}
	corrected := false
	if o.CorrectNoReadings && overGloTEC && res.Used == 0 {
		var shifted []float32
		shifted, corrected = assimilate.Shifted(res.FoF2, l.offset.mean)
		if corrected {
			res.FoF2 = shifted
		}
	}
	fo, muf := newField(), newField()
	for i, f := range res.FoF2 {
		fo.put(i, f)
		muf.put(i, f*res.M3000[i])
	}
	held, recent, newestHeld := 0, 0, time.Time{}
	if !o.NoStations {
		held, recent, newestHeld = reporting(l.giro.Readings(), now)
	}
	s := Snapshot{
		Computed:   now,
		Now:        Hour{At: now, FoF2: fo, MUF3000: muf},
		Background: Backgrounds{FoF2: backgroundOf(b.FoF2From), M3000: backgroundOf(b.M3000From)},
		Inputs: Inputs{F107Mean: float32(l.solar.Mean), F107Days: l.solar.Days, F107Age: l.solar.Age(now),
			CoordsExtrapolated: b.CoordsExtrapolated, Stations: res.Used, OldestReading: oldest, Rejected: l.giro.Rejected, Implausible: res.Implausible, ImplausibleM: res.ImplausibleM, NoStations: o.NoStations,
			GIRO: GIROOutcome(l.giro.Outcome), StationsFailed: l.giro.Failed, StationsHeld: held, StationsRecent: recent, NewestReading: newestHeld, Unread: slices.Clone(l.unread)},
		NearestKm:       res.NearestKm,
		NearestUnscored: unscoredCells(res.NearestUnscored),
		TypicalError:    typicalError(),
		Offset:          l.offset.offset(res.MeanResidual, measured, corrected),
		Noise:           o.Noise,
		Scales:          l.scalesOf(now),
		typical:         l.typical,
	}
	if s.Background.FoF2 == GloTEC {
		s.Inputs.GloTECValid, s.Inputs.GloTECFilled = g.Valid, b.Filled
	}
	if l.drapUsable(now) {
		s.Disturbance = newField()
		copy(s.Disturbance.Values, l.drap.MHz)
		s.Inputs.DRAPValid = l.drap.Valid
	}
	return s
}

// backgroundOf names a background's source.
func backgroundOf(s background.Source) Background {
	switch s {
	case background.GloTEC:
		return GloTEC
	case background.Climatology:
		return Climatology
	}
	return NoBackground
}

// newField is an empty field over the global grid (R-1.1): 2° cells over the
// whole globe, edges at -180..180 and -90..90, no antimeridian crossing, the
// only step in v0.1.0 (D-138); every cell's data to be put.
func newField() Field {
	return Field{West: -180, South: -90, East: 180, North: 90, Cols: grid.Cols, Rows: grid.Rows,
		Values: make([]float32, grid.Cells), NoData: newBitset(grid.Cells)}
}

// put sets a cell's value; one not finite and above zero is marked as no
// data, never handed on (R-3.3).
func (f Field) put(i int, v float32) {
	usable := v > 0 && !math.IsInf(float64(v), 0)
	if !usable {
		f.NoData.set(i)
		return
	}
	f.Values[i] = v
}

// clone is a snapshot in slices of its own (P-6).
func (s Snapshot) clone() Snapshot {
	c := s.withTables()
	c.Now = Hour{At: s.Now.At, MUF3000: s.Now.MUF3000.owned(), FoF2: s.Now.FoF2.owned(), Typical: s.Now.Typical}
	c.Disturbance = s.Disturbance.owned()
	return c
}

// withTables is the snapshot with its tables in slices of its own and its
// fields still the original's: what an hour ahead starts from, replacing
// the fields, so nothing is copied to be thrown away (watchpost D-251).
func (s Snapshot) withTables() Snapshot {
	c := s
	c.NearestKm = slices.Clone(s.NearestKm)
	c.NearestUnscored = Bitset{bits: slices.Clone(s.NearestUnscored.bits), n: s.NearestUnscored.n}
	c.Scales.Outlook = slices.Clone(s.Scales.Outlook)
	c.Inputs.Unread = slices.Clone(s.Inputs.Unread)
	c.TypicalError.Bands = slices.Clone(s.TypicalError.Bands)
	return c
}

// owned is a field in slices of its own.
func (f Field) owned() Field {
	c := f
	c.Values = slices.Clone(f.Values)
	c.NoData = Bitset{bits: slices.Clone(f.NoData.bits), n: f.NoData.n}
	return c
}
