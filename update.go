package ionomaps

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/assimilate"
	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/drap"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/scales"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
)

// The addresses an update asks: GloTEC's directory index, its grids by the
// names the index gives (watchpost D-111, D-141), and the daily solar file
// (D-104).
const (
	glotecIndex = "https://services.swpc.noaa.gov/products/glotec/geojson_2d_urt/"
	solarFile   = "https://services.swpc.noaa.gov/text/daily-solar-indices.txt"
	drapTable   = "https://services.swpc.noaa.gov/text/drap_global_frequencies.txt"
	scalesFile  = "https://services.swpc.noaa.gov/products/noaa-scales.json"
)

// NOAA's limits (D-39): asked at most every ten minutes, the interval GloTEC
// publishes at; grids at most six an hour; after a refusal or no answer, 60 s
// doubling to 15 minutes.
const (
	noaaEvery         = 10 * time.Minute
	gridsAnHour       = 6
	firstBackOff      = time.Minute
	longestPause      = 15 * time.Minute
	statusOK          = 200
	statusNotModified = 304
	statusRefused     = 429
)

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
// so a change needs no new library and no new burst (R-9.6, A-30).
type UpdateOptions struct {
	// CorrectNoReadings moves GloTEC's foF2 by the typical offset when no
	// station readings are assimilated (watchpost D-105 c); off, GloTEC is
	// shown as it is. The snapshot's Offset.Corrected says when it applied.
	CorrectNoReadings bool

	// Noise is the man-made noise the reference circuit's receiver hears
	// (watchpost D-187), Residential by default; the band statuses' lower
	// limit moves with it. A change needs no request.
	Noise Noise
}

// Update asks NOAA for what is new and returns the hour's snapshot (G4.2,
// watchpost D-151): GloTEC's newest grid, found through its index, and the
// daily solar file, at most once a day; foF2 from GloTEC (the climatology,
// named, without it or past an hour old), M(3000)F2 from the climatology,
// MUF(3000) their product, with GIRO's readings up to an hour old
// assimilated onto both (G5, D-64), NearestKm and NearestUnscored saying
// where, and the stations' offset against GloTEC (R-9.6). When GIRO is due it returns NOAA's snapshot said StationsNext, and
// the next call makes GIRO's burst (D-63). Within ten minutes of the last
// ask, or while backing off, the last snapshot comes back said too soon
// (R-5.4), rebuilt from what is held when the options have changed, nothing
// asked. With nothing held and no F10.7 mean to be had there is no field:
// the error says when NOAA is asked again.
//
// Source: watchpost D-39, D-63, D-64, D-101, D-104, D-105, D-111, D-151; this library.
func (l *Library) Update(ctx context.Context, o UpdateOptions) (Snapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock().UTC()
	if l.stationsNext && l.made { // NOAA's snapshot went out: now the burst (D-63)
		l.stationsNext = false
		l.giro.Burst(ctx, now, l.giroFetch())
		l.giro.LetGo(now)
		s := l.build(now, o) // NOAA's fields held, the stations' readings now on them
		l.last, l.opts = s, o
		return s.clone(), nil
	}
	early := now.Before(l.notBefore)
	if early {
		return l.early(now, o)
	}
	answered, refused := l.refreshGrid(ctx, now)
	if !refused {
		refused = l.refreshSolar(ctx, now) // a refusal stops the update (D-39)
	}
	if !refused {
		refused = l.refreshDRAP(ctx, now)
	}
	if !refused {
		refused = l.refreshScales(ctx, now)
	}
	cancelled := ctx.Err() != nil
	if cancelled {
		return l.early(now, o) // the host's, not NOAA's: no back-off, no ten minutes spent
	}
	l.notBefore, l.backOff = nextAsk(now, l.backOff, refused)
	if !l.solar.OK {
		return Snapshot{Computed: now, Early: Offline}, l.notYet()
	}
	s := l.build(now, o)
	l.giro.LetGo(now)
	switch {
	case !answered:
		s.Early = Offline
	case l.giro.BurstDue(now):
		s.Early, l.stationsNext = StationsNext, true // shown at once; the host updates again for the stations (D-63)
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
		return Snapshot{Computed: now, Early: TooSoon}, l.notYet()
	}
	if o != l.opts {
		l.last, l.opts = l.build(l.last.Computed, o), o
	}
	s := l.last.clone()
	s.Early = TooSoon
	return s, nil
}

// notYet is the error for an update with no field to give.
func (l *Library) notYet() error {
	return fmt.Errorf("ionomaps: no field yet: NOAA's solar data could not be read; it is asked again at the first update after %s", l.notBefore.Format("15:04:05 UTC"))
}

// nextAsk is when NOAA may next be asked, and the back-off then: ten
// minutes on after an update without a refusal, which ends the back-off;
// after one, 60 s doubling to 15 minutes (D-39).
func nextAsk(now time.Time, backOff time.Duration, refused bool) (time.Time, time.Duration) {
	if !refused {
		return now.Add(noaaEvery), 0
	}
	backOff = min(max(2*backOff, firstBackOff), longestPause)
	return now.Add(backOff), backOff
}

// ask is one request: the answer, whether it is one to use (200, or 304
// when validators were sent), and whether it was refused or not answered,
// which backs off (D-39).
func (l *Library) ask(ctx context.Context, url string, v Validators) (r Response, ok, refused bool) {
	r, err := l.fetch.Fetch(ctx, url, v)
	if err != nil {
		return Response{}, false, true
	}
	if r.Status == statusRefused {
		return r, false, true
	}
	return r, r.Status == statusOK || r.Status == statusNotModified && v != (Validators{}), false
}

// refreshGrid asks the index and, when it names a grid not held, that grid,
// within six grid asks an hour. answered is whether the index came back;
// refused whether either ask was refused or unanswered.
func (l *Library) refreshGrid(ctx context.Context, now time.Time) (answered, refused bool) {
	idx, ok, refused := l.ask(ctx, glotecIndex, Validators{})
	if !ok {
		return false, refused
	}
	name, _, listed := glotec.Newest(idx.Body, now)
	if !listed {
		return true, false
	}
	if name == l.gridName {
		return true, false // held already: a new grid is never a 304 (D-111)
	}
	may := l.mayAskGrid(now)
	if !may {
		return true, false
	}
	r, ok, refused := l.ask(ctx, glotecIndex+name, Validators{})
	if !ok {
		return true, refused
	}
	g, err := glotec.Decode(r.Body, now)
	if err != nil {
		return true, false // a changed format: the grid held stays, and its age says so
	}
	l.grid, l.gridName = &g, name
	return true, false
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
// update. refused is whether it was refused or unanswered.
func (l *Library) refreshSolar(ctx context.Context, now time.Time) (refused bool) {
	today := now.Truncate(24 * time.Hour)
	readToday := l.solar.OK && l.solarDay.Equal(today)
	if readToday {
		return false
	}
	r, ok, refused := l.ask(ctx, solarFile, l.solarVal)
	if !ok {
		return refused
	}
	l.solarDay = today
	if r.Status != statusOK {
		return false // unchanged since it was read: the mean held stands
	}
	l.solar = solar.Hold(l.solar, r.Body, now)
	if l.solar.Taken.Equal(now) {
		l.solarVal = r.Validators // the mean is this file's
	}
	return false
}

// refreshDRAP asks for D-RAP's table at each update, with the validators of
// the table held (R-2.5, R-5.7); a table that cannot be read leaves the one
// held, its age saying so. refused is whether it was refused or unanswered.
func (l *Library) refreshDRAP(ctx context.Context, now time.Time) (refused bool) {
	r, ok, refused := l.ask(ctx, drapTable, l.drapVal)
	if !ok || r.Status != statusOK {
		return refused
	}
	g, err := drap.ParseTable(r.Body, now)
	if err != nil {
		return false
	}
	l.drap, l.drapVal = &g, r.Validators
	return false
}

// refreshScales asks for NOAA's scales at each update, with D-RAP and the
// validators of the file held (R-2.10, watchpost D-88); a file that cannot
// be read leaves the one held. refused is whether it was refused or
// unanswered.
func (l *Library) refreshScales(ctx context.Context, now time.Time) (refused bool) {
	r, ok, refused := l.ask(ctx, scalesFile, l.scalesVal)
	if !ok || r.Status != statusOK {
		return refused
	}
	s, err := scales.Read(r.Body, now)
	if err != nil {
		return false
	}
	l.scales, l.scalesVal = &s, r.Validators
	return false
}

// scalesOf are the scales held as the snapshot carries them: none, said by
// a zero valid time, when none was read or the last is over scalesMaxAge
// old (watchpost D-88: never level 0).
func scalesOf(held *scales.Levels, now time.Time) Scales {
	if held == nil || now.Sub(held.Valid) > scalesMaxAge {
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
	g := l.grid
	if g != nil && now.Sub(g.Valid) > gridMaxAge {
		g = nil
	}
	b := background.Build(now, l.solar.Mean, g)
	res, oldest, newest := l.assimilated(b, now)
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
	s := Snapshot{
		Computed:   now,
		Hours:      []Hour{{At: now, FoF2: fo, MUF3000: muf}},
		Background: Backgrounds{FoF2: backgroundOf(b.FoF2From), M3000: backgroundOf(b.M3000From)},
		Inputs: Inputs{F107Mean: float32(l.solar.Mean), F107Days: l.solar.Days, F107Age: l.solar.Age(now),
			CoordsExtrapolated: b.CoordsExtrapolated, Stations: res.Used, OldestReading: oldest, Rejected: l.giro.Rejected},
		NearestKm:       res.NearestKm,
		NearestUnscored: unscoredCells(res.NearestUnscored),
		TypicalError:    typicalError(),
		Offset:          l.offset.offset(res.MeanResidual, measured, corrected),
		Noise:           o.Noise,
		Scales:          scalesOf(l.scales, now),
	}
	if s.Background.FoF2 == GloTEC {
		s.Inputs.GloTECValid = g.Valid
	}
	if d := l.drap; d != nil && now.Sub(d.Valid) <= drapMaxAge {
		s.Disturbance = newField()
		copy(s.Disturbance.Values, d.MHz)
		s.Inputs.DRAPValid = d.Valid
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
		f.NoData.Set(i)
		return
	}
	f.Values[i] = v
}

// clone is a snapshot in slices of its own (P-6).
func (s Snapshot) clone() Snapshot {
	c := s
	c.Hours = make([]Hour, len(s.Hours))
	for i, h := range s.Hours {
		c.Hours[i] = Hour{At: h.At, MUF3000: h.MUF3000.owned(), FoF2: h.FoF2.owned()}
	}
	c.Disturbance = s.Disturbance.owned()
	c.NearestKm = slices.Clone(s.NearestKm)
	c.NearestUnscored = Bitset{bits: slices.Clone(s.NearestUnscored.bits), n: s.NearestUnscored.n}
	c.Scales.Outlook = slices.Clone(s.Scales.Outlook)
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
