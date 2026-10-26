package ionomaps_test

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
)

// giroSide is GIRO's answers, built here (R-4.3): every station answers
// with one reading five minutes old unless told otherwise; asks recorded.
type giroSide struct {
	asks     []string               // the URLs asked, in order
	rows     map[string][]time.Time // a station's readings, when set
	refuse   int                    // the ask, counting from 1, answered 429
	retry    string                 // the Retry-After that 429 carries, when set
	cancel   func()                 // called at the ask refuse names, in place of refusing
	silent   map[string]bool        // stations that answer with no reading
	speak    bool                   // stations answer with readings; NOAA's own tests leave it off
	score    map[string]string      // a station's confidence score, 95 when unset
	speakers int                    // when set, only the first this many asks give readings
	at       time.Time              // when set, every station's one reading is at this time
	truncate bool                   // every reply cut off partway through its last reading
	reformat bool                   // every reply a page that is no reply
	status   map[string]int         // a station's status in place of its reply, when set
	all      int                    // every station's status in place of its reply, when set
	fo       map[string]string      // a station's foF2 reading, 7.500 when unset
}

// giroAnswer answers one GIRO ask: the station's reply in FastChar's
// two-characteristic layout, built from pieces.
func (n *noaa) giroAnswer(ctx context.Context, raw string) (ionomaps.Response, error) {
	g := &n.giro
	g.asks = append(g.asks, raw)
	if g.refuse == len(g.asks) {
		if g.cancel != nil {
			g.cancel()
			return ionomaps.Response{}, context.Canceled
		}
		return ionomaps.Response{Status: 429, Rate: ionomaps.RateHeaders{RetryAfter: g.retry}}, nil
	}
	if err := ctx.Err(); err != nil {
		return ionomaps.Response{}, err
	}
	u, _ := url.Parse(raw)
	code := u.Query().Get("ursiCode")
	if s, ok := g.status[code]; ok {
		return ionomaps.Response{Status: s}, nil
	}
	if g.all != 0 {
		return ionomaps.Response{Status: g.all}, nil
	}
	body := "# Location: " + "GEO" + " ( " + placeOf(code) + " ), URSI-Code " + code + ", A STATION\n" +
		"# Time                    " + "CS" + "   foF2 " + "QD M(D) QD\n"
	times := []time.Time{n.clock.Now().Add(-5 * time.Minute)}
	if !g.at.IsZero() {
		times = []time.Time{g.at}
	}
	if t, ok := g.rows[code]; ok {
		times = t
	}
	score := "95"
	if v, ok := g.score[code]; ok {
		score = v
	}
	fo := "7.500"
	if v, ok := g.fo[code]; ok {
		fo = v
	}
	if g.speak && !g.silent[code] && (g.speakers == 0 || len(g.asks) <= g.speakers) {
		for _, at := range times {
			body += at.UTC().Format("2006-01-02T15:04:05.000Z") + "  " + score + "  " + fo + " // 3.40 //\n"
		}
	}
	switch {
	case g.reformat:
		body = "<html><body>This page has moved.</body></html>\n"
	case g.truncate:
		body = body[:len(body)-20] // through the last row's foF2: a reply cut short, as a dropped transfer leaves it
	}
	return ionomaps.Response{Status: 200, Body: []byte(body)}, nil
}

// placeOf is where the fake says a station is, in FastChar's form: where
// the seed places it, as GIRO's own location line does (a reply putting a
// placed station elsewhere is refused, watchpost D-248); a station the seed
// does not place spread by its code.
func placeOf(code string) string {
	list, err := stations.Seed(start)
	if err != nil {
		panic(err) // the shipped seed is broken: its own test fails first
	}
	if at, placed := list.At(code); placed {
		ns, lat, lon := "N", at.Lat, at.Lon
		if lat < 0 {
			ns, lat = "S", -lat
		}
		if lon < 0 {
			lon += 360 // GIRO writes longitudes east, 0 to 360
		}
		return fmt.Sprintf("%.1f %s   %.1f E", lat, ns, lon)
	}
	h := 0
	for _, r := range code {
		h = h*31 + int(r)
	}
	lat, lon := h%120-60, (h/7)%360
	ns := "N"
	if lat < 0 {
		ns, lat = "S", -lat
	}
	return fmt.Sprintf("%d.0 %s   %d.0 E", lat, ns, lon)
}

// giroLibrary is the library with NOAA serving a grid, ready to update.
func giroLibrary(t *testing.T) (*ionomaps.Library, *noaa, *clock) {
	t.Helper()
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.giro.speak = true
	return l, n, c
}

// update is one Update that must succeed.
func update(t *testing.T, l *ionomaps.Library) ionomaps.Snapshot {
	t.Helper()
	s, err := l.Update(context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestNOAAComesFirstThenTheStations is D-63: with GIRO due, the first update
// is NOAA's alone, said StationsNext; the next, at once, makes GIRO's burst
// - 39 stations polled and one probed - and is said fresh.
func TestNOAAComesFirstThenTheStations(t *testing.T) {
	l, n, _ := giroLibrary(t)
	first := update(t, l)
	if first.Reason != ionomaps.StationsNext || len(n.giro.asks) != 0 {
		t.Fatalf("the first update: reason %v and %d GIRO asks; want StationsNext and none", first.Reason, len(n.giro.asks))
	}
	second := update(t, l)
	if second.Reason != ionomaps.Fresh || len(n.giro.asks) != 40 {
		t.Errorf("the second update: reason %v and %d GIRO asks; want fresh and 40 (39 and the probe)", second.Reason, len(n.giro.asks))
	}
	if held, _ := ionomaps.HeldReadings(l); held != 40 {
		t.Errorf("%d stations' readings held; every station asked answered one", held)
	}
}

// TestGIROIsNeverAskedMoreThanHourly is R-5.7: within the hour of a burst an
// update asks NOAA alone, never GIRO; an hour on, GIRO is due again.
func TestGIROIsNeverAskedMoreThanHourly(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	asked := len(n.giro.asks)
	for range 5 {
		c.Add(10 * time.Minute)
		if s := update(t, l); s.Reason == ionomaps.StationsNext {
			t.Fatal("GIRO said due within the hour of its burst")
		}
	}
	if len(n.giro.asks) != asked {
		t.Errorf("%d GIRO asks within the hour after the burst", len(n.giro.asks)-asked)
	}
	c.Add(11 * time.Minute)
	if s := update(t, l); s.Reason != ionomaps.StationsNext {
		t.Errorf("an hour after the burst the update is %v; GIRO is due", s.Reason)
	}
}

// TestAnUpdateBetweenGIROAsksReusesTheLastReadings is R-5.7: an update
// between bursts keeps the readings held, with their age.
func TestAnUpdateBetweenGIROAsksReusesTheLastReadings(t *testing.T) {
	l, _, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	n, oldest := ionomaps.HeldReadings(l)
	c.Add(10 * time.Minute)
	update(t, l)
	if m, o := ionomaps.HeldReadings(l); m != n || !o.Equal(oldest) || n == 0 {
		t.Errorf("between bursts: %d readings, oldest %v; after the burst %d, oldest %v", m, o, n, oldest)
	}
}

// TestOnlyTheLastReadingPerStationIsHeld is R-5.2 (P-11): a reply of several
// readings leaves the latest held, no more.
func TestOnlyTheLastReadingPerStationIsHeld(t *testing.T) {
	l, n, c := giroLibrary(t)
	n.giro.rows = map[string][]time.Time{}
	for _, code := range []string{"AL945", "AS00Q"} {
		n.giro.rows[code] = []time.Time{start.Add(-50 * time.Minute), start.Add(-30 * time.Minute), start.Add(-10 * time.Minute)}
	}
	update(t, l)
	update(t, l)
	if n, oldest := ionomaps.HeldReadings(l); n != 40 || oldest.Before(c.Now().Add(-10*time.Minute)) {
		t.Errorf("%d readings held, the oldest %v; one a station, the latest", n, oldest)
	}
}

// TestAnUpdateAsksOnlySinceTheLastReading is R-5.2: the next burst asks each
// station from its last reading held.
func TestAnUpdateAsksOnlySinceTheLastReading(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	last := c.Now().Add(-5 * time.Minute) // the reading each station gave
	c.Add(time.Hour)
	update(t, l)
	n.giro.asks = nil
	update(t, l)
	want := url.Values{"fromDate": {last.UTC().Format("2006/01/02 15:04:05")}}.Encode()
	for _, a := range n.giro.asks {
		if strings.Contains(a, "ursiCode=AL945") && !strings.Contains(a, want) {
			t.Errorf("AL945 is asked %q; want from its last reading, %s", a, want)
		}
	}
}

// TestA429IsSeenByTheLibraryNotRetried is R-5.3: a refusal stops the burst
// at once, never retried; GIRO is not due until the pause ends.
func TestA429IsSeenByTheLibraryNotRetried(t *testing.T) {
	l, n, c := giroLibrary(t)
	n.giro.refuse = 3
	update(t, l)
	update(t, l)
	if len(n.giro.asks) != 3 {
		t.Errorf("a refusal at the third ask left %d asked; the burst stops there", len(n.giro.asks))
	}
	c.Add(10 * time.Minute)
	if s := update(t, l); s.Reason == ionomaps.StationsNext {
		t.Error("GIRO said due within its hour after a refused burst")
	}
}

// TestACancelledBurstSpendsTheHoursBudget is R-5.3 (round 2, LN1): a burst
// the host cancels part way still spends GIRO's hour.
func TestACancelledBurstSpendsTheHoursBudget(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	ctx, cancel := context.WithCancel(context.Background())
	n.giro.refuse, n.giro.cancel = 5, cancel
	_, _ = l.Update(ctx, ionomaps.UpdateOptions{})
	asked := len(n.giro.asks)
	if asked > 5 {
		t.Errorf("the burst went on after the host cancelled it: %d asks", asked)
	}
	c.Add(10 * time.Minute)
	if s := update(t, l); s.Reason == ionomaps.StationsNext || len(n.giro.asks) != asked {
		t.Error("a cancelled burst gave its hour back")
	}
}

// TestABurstStoppedBeforeItsFirstAskKeepsTheHour is R-5.3 and L-F4: a
// burst whose context has ended before its first ask asks GIRO nothing, so
// it spends no hour - the next update makes the burst - and the snapshot is
// said Unfinished.
func TestABurstStoppedBeforeItsFirstAskKeepsTheHour(t *testing.T) {
	l, n, _ := giroLibrary(t)
	update(t, l)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := l.Update(ctx, ionomaps.UpdateOptions{})
	if err != nil || s.Reason != ionomaps.Unfinished || len(n.giro.asks) != 0 {
		t.Fatalf("a burst stopped before it began: reason %v, %d asks (%v)", s.Reason, len(n.giro.asks), err)
	}
	if s := update(t, l); s.Reason != ionomaps.Fresh || len(n.giro.asks) != 40 {
		t.Errorf("the next update: reason %v, %d GIRO asks; want the burst", s.Reason, len(n.giro.asks))
	}
}

// TestNoStationsAsksGIRONothing is R-9.9 (watchpost D-214): with the "no
// stations" option GIRO is never asked and no station's reading is on the
// field - NOAA's and the climatology's alone, said by choice - however many
// hours go by; set after a burst, the readings held leave the field with no
// request, and cleared, they come back with none.
func TestNoStationsAsksGIRONothing(t *testing.T) {
	none := ionomaps.UpdateOptions{NoStations: true}
	l, n, c := giroLibrary(t)
	for range 13 {
		s, err := l.Update(context.Background(), none)
		if err != nil {
			t.Fatal(err)
		}
		if s.Reason == ionomaps.StationsNext || !s.Inputs.NoStations || s.Inputs.Stations != 0 || len(s.NearestKm) != 0 {
			t.Fatalf("with no stations: reason %v, said %v, %d stations, %d distances", s.Reason, s.Inputs.NoStations, s.Inputs.Stations, len(s.NearestKm))
		}
		c.Add(10 * time.Minute)
	}
	if len(n.giro.asks) != 0 {
		t.Fatalf("GIRO asked %d times with no stations", len(n.giro.asks))
	}

	l, n, _ = giroLibrary(t)
	if s := update(t, l); s.Reason != ionomaps.StationsNext {
		t.Fatalf("control: the first update's reason is %v", s.Reason)
	}
	if _, err := l.Update(context.Background(), none); err != nil || len(n.giro.asks) != 0 {
		t.Fatalf("no stations set after StationsNext: %d GIRO asks (%v)", len(n.giro.asks), err)
	}

	l, n, _ = giroLibrary(t)
	first := update(t, l)
	with := update(t, l)
	asked := len(n.giro.asks)
	if with.Inputs.Stations == 0 || with.Inputs.NoStations {
		t.Fatalf("control: the burst assimilated %d stations", with.Inputs.Stations)
	}
	without, err := l.Update(context.Background(), none)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := without.Readings([]ionomaps.LatLon{seededAt})
	g, _ := first.Readings([]ionomaps.LatLon{seededAt})
	if without.Inputs.Stations != 0 || !without.Inputs.NoStations || r[0].FoF2 != g[0].FoF2 || len(n.giro.asks) != asked {
		t.Errorf("no stations set after a burst: %d stations, foF2 %v at the seeded station (GloTEC's %v), %d new asks", without.Inputs.Stations, r[0].FoF2, g[0].FoF2, len(n.giro.asks)-asked)
	}
	back := update(t, l)
	if back.Inputs.Stations == 0 || back.Inputs.NoStations || len(n.giro.asks) != asked {
		t.Errorf("no stations cleared: %d stations, %d new asks", back.Inputs.Stations, len(n.giro.asks)-asked)
	}
}

// TestTheLibraryStartsNoGoroutines is R-5.4: an update with GIRO's burst
// leaves no goroutine of the library's running.
func TestTheLibraryStartsNoGoroutines(t *testing.T) {
	l, _, _ := giroLibrary(t)
	before := runtime.NumGoroutine()
	update(t, l)
	update(t, l)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("%d goroutines before the updates and %d after", before, after)
	}
}

// TestOldReadingsAreLetGo is R-5.7 and D-51: a reading held three days is
// let go at the next update, before any new burst.
func TestOldReadingsAreLetGo(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	c.Add(73 * time.Hour)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	if s := update(t, l); s.Reason != ionomaps.StationsNext {
		t.Fatalf("three days on, the update is %v; GIRO is due", s.Reason)
	}
	if held, _ := ionomaps.HeldReadings(l); held != 0 {
		t.Errorf("%d readings three days old still held", held)
	}
}

// seededAt is a seeded live station's place: Alpena, AL945.
var seededAt = ionomaps.LatLon{Lat: 45.1, Lon: -83.6}

// TestStationsAreAssimilatedIntoTheField is R-9.2 (D-101): after GIRO's
// burst the field moves toward the stations' readings - foF2 7.5 over
// GloTEC's 7 - the snapshot says how many were used and how old the oldest
// is, and every cell how far its nearest station is.
func TestStationsAreAssimilatedIntoTheField(t *testing.T) {
	l, _, _ := giroLibrary(t)
	before := update(t, l)
	after := update(t, l)
	b, _ := before.Readings([]ionomaps.LatLon{seededAt})
	a, _ := after.Readings([]ionomaps.LatLon{seededAt})
	if !(a[0].FoF2 > b[0].FoF2+0.2) {
		t.Errorf("foF2 at Alpena is %v before the burst and %v after; its reading is 7.5 over GloTEC's 7", b[0].FoF2, a[0].FoF2)
	}
	if after.Inputs.Stations != 40 || after.Inputs.OldestReading.IsZero() || len(after.NearestKm) != len(after.Now.FoF2.Values) {
		t.Errorf("after the burst: %d stations, oldest %v, %d distances", after.Inputs.Stations, after.Inputs.OldestReading, len(after.NearestKm))
	}
	if before.Inputs.Stations != 0 || before.NearestKm != nil {
		t.Error("NOAA's snapshot before the burst says stations")
	}
}

// TestOnlyReadingsAnHourOldAreAssimilated is D-64 (watchpost D-184): a
// reading over an hour old is not assimilated - the field is the
// background alone, said with no stations - though it is still held.
func TestOnlyReadingsAnHourOldAreAssimilated(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	c.Add(61 * time.Minute)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	s := update(t, l) // NOAA's, GIRO due: built before the burst, the readings 66 minutes old
	if s.Inputs.Stations != 0 || s.NearestKm != nil {
		t.Errorf("readings 66 minutes old: %d stations assimilated", s.Inputs.Stations)
	}
	if held, _ := ionomaps.HeldReadings(l); held == 0 {
		t.Error("the readings were let go; they are kept until three days")
	}
}

// TestAnUnscoredStationIsSaid is R-2.11 (D-61, watchpost D-181): a cell
// whose nearest station's readings are unscored is marked, and an answer
// there says so, so the host can tell the operator to verify; one whose
// nearest is scored is not.
func TestAnUnscoredStationIsSaid(t *testing.T) {
	l, n, _ := giroLibrary(t)
	n.giro.score = map[string]string{"AL945": "-1"}
	update(t, l)
	s := update(t, l)
	cell := func(p ionomaps.LatLon) int {
		f := s.Now.FoF2
		row := int((f.North - p.Lat) / (f.North - f.South) * float64(f.Rows))
		col := int((p.Lon - f.West) / (f.East - f.West) * float64(f.Cols))
		return row*f.Cols + col
	}
	if !s.NearestUnscored.Has(cell(seededAt)) {
		t.Error("Alpena's cell, its nearest station unscored, is not marked")
	}
	if s.NearestUnscored.Has(cell(ionomaps.LatLon{Lat: 50.1, Lon: 4.6})) {
		t.Error("Dourbes' cell, its nearest station scored, is marked")
	}
	r, err := s.Readings([]ionomaps.LatLon{seededAt, {Lat: 50.1, Lon: 4.6}})
	if err != nil || !r[0].Nearest.Unscored || r[1].Nearest.Unscored || !r[0].Nearest.Known || r[0].Nearest.Km > 200 {
		t.Errorf("the answers at Alpena and Dourbes: %+v, %+v (%v); want Alpena's unscored and near, Dourbes' scored", r[0].Nearest, r[1].Nearest, err)
	}
}

// TestGIROsOutcomeIsSaidAllHour is watchpost D-247: a station that fails is
// counted and the others assimilated; three failing in a row are GIRO
// unavailable, and a 403 GIRO's denial - each carried in every snapshot of
// the hour that follows, the denial said GIRO paused for a day, never a
// quiet hour said as no readings.
func TestGIROsOutcomeIsSaidAllHour(t *testing.T) {
	l, n, _ := giroLibrary(t)
	n.giro.status = map[string]int{"AL945": 503}
	update(t, l)
	s := update(t, l)
	if s.Inputs.StationsFailed != 1 || s.Inputs.GIRO != ionomaps.GIROAnswered || s.Inputs.Stations == 0 {
		t.Errorf("one station failing: %d failed, %v, %d assimilated; want 1, answered, the rest assimilated", s.Inputs.StationsFailed, s.Inputs.GIRO, s.Inputs.Stations)
	}
	for _, c := range []struct {
		status int
		want   ionomaps.GIROOutcome
	}{{503, ionomaps.GIROUnavailable}, {403, ionomaps.GIRODenied}} {
		l, n, clk := giroLibrary(t)
		n.giro.all = c.status
		update(t, l)
		s := update(t, l)
		clk.Add(40 * time.Minute)
		later := update(t, l)
		if s.Inputs.GIRO != c.want || later.Inputs.GIRO != c.want || s.Inputs.Stations != 0 {
			t.Errorf("every station %d: %v, 40 minutes on %v, %d stations; want %v all hour", c.status, s.Inputs.GIRO, later.Inputs.GIRO, s.Inputs.Stations, c.want)
		}
		if c.status == 403 && later.Reason != ionomaps.GIROPaused {
			t.Errorf("a denial, 40 minutes on: %v; want GIRO paused", later.Reason)
		}
	}
}

// TestAnImplausibleReadingIsCounted is watchpost D-248 at the library: a
// placed station reading foF2 far past its residual's bound over the grid
// is refused, counted in Inputs.Implausible apart from the burst's own
// refused rows, and leaves the field where the other stations put it.
func TestAnImplausibleReadingIsCounted(t *testing.T) {
	l, n, _ := giroLibrary(t)
	n.giro.fo = map[string]string{"AL945": "20.000"} // 13 MHz over GloTEC's 7
	update(t, l)
	s := update(t, l)
	if s.Inputs.Implausible != 1 || s.Inputs.Rejected != 0 || s.Inputs.Stations != 39 {
		t.Errorf("one station reading 20 MHz over a 7 MHz grid: %d implausible, %d rejected, %d stations; want 1, 0, 39", s.Inputs.Implausible, s.Inputs.Rejected, s.Inputs.Stations)
	}
}

// TestAStalledNOAAStopsItsRoundAndGIROIsStillAsked is watchpost D-285: a
// NOAA host that accepts and never answers costs one ask's time, stops
// NOAA's round as a refusal does, backs NOAA off, 10 minutes then 15, and
// leaves the update inside its deadline, so GIRO, healthy, is still asked.
// Each update runs under a guard deadline, so a round that waited on the
// stall fails here rather than hanging the package.
func TestAStalledNOAAStopsItsRoundAndGIROIsStillAsked(t *testing.T) {
	defer ionomaps.SetNOAAAskTimeout(20 * time.Millisecond)()
	l, n, c := giroLibrary(t)
	n.stall[indexURL] = true
	run := func() ionomaps.Snapshot {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s, _ := settled(l, ctx, ionomaps.UpdateOptions{})
		return s
	}
	s := run()
	if s.Reason == ionomaps.Unfinished || len(n.giro.asks) == 0 {
		t.Fatalf("a stalled index: reason %v, %d GIRO asks; want the update finished and GIRO asked", s.Reason, len(n.giro.asks))
	}
	if len(n.drapAsks) != 0 || n.scalesAsks != 0 {
		t.Errorf("after the stalled index the round went on: %d D-RAP asks, %d scales asks; want it stopped", len(n.drapAsks), n.scalesAsks)
	}
	n.take()
	for i, wait := range []time.Duration{10 * time.Minute, 15 * time.Minute} {
		c.Add(wait - time.Second)
		run()
		if a := n.take(); len(a) != 0 {
			t.Fatalf("stall %d: NOAA asked again before %v: %v", i+1, wait, a)
		}
		c.Add(time.Second)
		run()
		if a := n.take(); len(a) == 0 {
			t.Fatalf("stall %d: NOAA not asked again after %v", i+1, wait)
		}
	}
}
