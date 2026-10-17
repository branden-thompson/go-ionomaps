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
	truncate bool                   // every reply cut in half
	reformat bool                   // every reply a page that is no reply
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
	if g.speak && !g.silent[code] && (g.speakers == 0 || len(g.asks) <= g.speakers) {
		for _, at := range times {
			body += at.UTC().Format("2006-01-02T15:04:05.000Z") + "  " + score + "  7.500 // 3.40 //\n"
		}
	}
	switch {
	case g.reformat:
		body = "<html><body>This page has moved.</body></html>\n"
	case g.truncate:
		body = body[:len(body)/2]
	}
	return ionomaps.Response{Status: 200, Body: []byte(body)}, nil
}

// placeOf is where the fake says a station is, in FastChar's form: Alpena
// and Dourbes where they are, every other station spread by its code.
func placeOf(code string) string {
	switch code {
	case "AL945":
		return "45.1 N   276.4 E"
	case "DB049":
		return "50.1 N     4.6 E"
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
