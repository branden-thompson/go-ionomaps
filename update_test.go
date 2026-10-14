package ionomaps_test

// update_test.go — plan task G4.2 (watchpost D-151, UAT-1): an update that
// asks NOAA alone - GloTEC's newest grid through its directory index, the
// daily solar file - and makes the hour's fields from the background, with
// D-39's limits on NOAA.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/background"
)

const (
	indexURL = "https://services.swpc.noaa.gov/products/glotec/geojson_2d_urt/"
	solarURL = "https://services.swpc.noaa.gov/text/daily-solar-indices.txt"
)

var start = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

// clock is a fake clock the test moves.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// noaa is NOAA's answers, built here (R-4.3): an index listing grids, the
// grids by name, the solar file. Every ask is recorded.
type noaa struct {
	mu        sync.Mutex
	asked     []string
	validator []ionomaps.Validators
	clock     *clock
	names     []string          // listed in the index
	grids     map[string][]byte // served by name
	solar     []byte
	status    map[string]int // a status in place of 200
	down      map[string]bool
	giro      giroSide

	drap           []byte // D-RAP's table; none served when nil
	drapAsks       []string
	drapValidators []ionomaps.Validators
}

func newNOAA(c *clock) *noaa {
	return &noaa{clock: c, grids: map[string][]byte{}, status: map[string]int{}, down: map[string]bool{}, solar: solarFile(start, 30, 140)}
}

func (n *noaa) Fetch(ctx context.Context, url string, v ionomaps.Validators) (ionomaps.Response, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if strings.Contains(url, "lgdc.uml.edu") {
		return n.giroAnswer(ctx, url)
	}
	if url == drapURL {
		return n.drapAnswer(ctx, v)
	}
	n.asked = append(n.asked, url)
	n.validator = append(n.validator, v)
	if err := ctx.Err(); err != nil {
		return ionomaps.Response{}, err
	}
	key := url
	if strings.HasPrefix(url, indexURL) && url != indexURL {
		key = "grid"
	}
	if n.down[key] {
		return ionomaps.Response{}, errors.New("no route")
	}
	if s, ok := n.status[key]; ok {
		return ionomaps.Response{Status: s}, nil
	}
	switch {
	case url == indexURL:
		return ionomaps.Response{Status: 200, Body: index(n.names)}, nil
	case url == solarURL:
		return ionomaps.Response{Status: 200, Body: n.solar, Validators: ionomaps.Validators{ETag: `"solar-1"`}}, nil
	}
	body, ok := n.grids[strings.TrimPrefix(url, indexURL)]
	if !ok {
		return ionomaps.Response{Status: 404}, nil
	}
	return ionomaps.Response{Status: 200, Body: body}, nil
}

// drapAnswer answers D-RAP's ask, recorded apart from GloTEC's and the solar
// file's: the table when one is set, with its validators, else 404.
func (n *noaa) drapAnswer(ctx context.Context, v ionomaps.Validators) (ionomaps.Response, error) {
	n.drapAsks = append(n.drapAsks, drapURL)
	n.drapValidators = append(n.drapValidators, v)
	if err := ctx.Err(); err != nil {
		return ionomaps.Response{}, err
	}
	if s, ok := n.status[drapURL]; ok {
		return ionomaps.Response{Status: s}, nil
	}
	if n.drap == nil {
		return ionomaps.Response{Status: 404}, nil
	}
	return ionomaps.Response{Status: 200, Body: n.drap, Validators: ionomaps.Validators{ETag: `"drap-1"`}}, nil
}

// list puts a grid valid at t in the index and serves it.
func (n *noaa) list(t testing.TB, valid time.Time, foF2 float64) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	name := "glotec_icao_" + valid.UTC().Format("20060102T150405Z") + ".geojson"
	n.names = append(n.names, name)
	n.grids[name] = grid(t, valid, foF2)
	return name
}

func (n *noaa) take() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	a := n.asked
	n.asked = nil
	return a
}

// index is a directory listing in the layout NOAA's server writes.
func index(names []string) []byte {
	var b strings.Builder
	b.WriteString("<html><head><title>Index of /products/glotec/geojson_2d_urt</title></head><body><pre>\n")
	for _, n := range names {
		fmt.Fprintf(&b, "<a href=\"%s\">%s</a> 2026-10-09 12:00  2.4M\n", n, n)
	}
	b.WriteString("</pre></body></html>\n")
	return []byte(b.String())
}

// grid is a GloTEC-shaped grid of one foF2 everywhere, valid at t.
func grid(t testing.TB, valid time.Time, foF2 float64) []byte {
	t.Helper()
	type feature struct {
		Geometry struct {
			Coordinates [2]float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			NmF2    float64 `json:"NmF2"`
			HmF2    float64 `json:"hmF2"`
			Quality int     `json:"quality_flag"`
		} `json:"properties"`
	}
	doc := struct {
		Features []feature `json:"features"`
		TimeTag  string    `json:"time_tag"`
	}{TimeTag: valid.UTC().Format(time.RFC3339)}
	for i := range 72 {
		for j := range 72 {
			var f feature
			f.Geometry.Coordinates = [2]float64{-177.5 + 5*float64(i), -88.75 + 2.5*float64(j)}
			f.Properties.NmF2, f.Properties.HmF2, f.Properties.Quality = math.Pow(foF2/8.98e-6, 2), 300, 5
			doc.Features = append(doc.Features, f)
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// solarFile is NOAA's daily solar layout, built here: days flux each, ending
// the day before end.
func solarFile(end time.Time, days int, flux int) []byte {
	var b strings.Builder
	b.WriteString(":Product: Daily Solar Data            DSD.txt\n#\n")
	for i := days; i > 0; i-- {
		d := end.AddDate(0, 0, -i)
		fmt.Fprintf(&b, "%d %02d %02d  %3d     83      570      2    -999      *   2  0  0  1  1  0  0\n", d.Year(), int(d.Month()), d.Day(), flux)
	}
	return []byte(b.String())
}

func library(t testing.TB) (*ionomaps.Library, *noaa, *clock) {
	t.Helper()
	c := &clock{now: start}
	n := newNOAA(c)
	l, err := ionomaps.New(n, ionomaps.Options{Clock: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	return l, n, c
}

// TestAnUpdateMakesTheFieldsFromNOAA is G4.2: the newest grid in the index
// is asked, then the solar file; the hour's foF2 is GloTEC's, M(3000)F2 the
// climatology's, and MUF(3000) their product in every cell, every cell
// with data (foF2 times the climatology's M(3000)F2 at the same moment and
// F10.7); the snapshot says what fed it.
func TestAnUpdateMakesTheFieldsFromNOAA(t *testing.T) {
	l, n, _ := library(t)
	n.list(t, start.Add(-50*time.Minute), 6)
	newest := n.list(t, start.Add(-30*time.Minute), 7)
	n.list(t, start.Add(-40*time.Minute), 5) // listed out of order
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(n.take(), " "), indexURL+" "+indexURL+newest+" "+solarURL; got != want {
		t.Errorf("asked %s; want %s", got, want)
	}
	if s.Early != ionomaps.Fresh || !s.Computed.Equal(start) || len(s.Hours) != 1 {
		t.Fatalf("early %v, computed %v, %d hours", s.Early, s.Computed, len(s.Hours))
	}
	if s.Background.FoF2 != ionomaps.GloTEC || s.Background.M3000 != ionomaps.Climatology {
		t.Errorf("backgrounds %v and %v", s.Background.FoF2, s.Background.M3000)
	}
	in := s.Inputs
	if !in.GloTECValid.Equal(start.Add(-30*time.Minute)) || in.F107Mean != 140 || in.F107Days != 30 || in.F107Age != 0 {
		t.Errorf("inputs %+v", in)
	}
	h := s.Hours[0]
	climatology := background.Build(start, 140, nil)
	if !h.At.Equal(start) {
		t.Errorf("the hour is at %v", h.At)
	}
	for i, fo := range h.FoF2.Values {
		if h.FoF2.NoData.Has(i) || h.MUF3000.NoData.Has(i) {
			t.Fatalf("cell %d has no data", i)
		}
		if math.Abs(float64(fo)-7) > 1e-4 {
			t.Fatalf("cell %d: foF2 %v; GloTEC's is 7", i, fo)
		}
		if want := float64(fo) * float64(climatology.M3000[i]); math.Abs(float64(h.MUF3000.Values[i])-want) > 1e-4 {
			t.Fatalf("cell %d: MUF(3000) %v; foF2 %v times the climatology's M(3000)F2 is %v", i, h.MUF3000.Values[i], fo, want)
		}
	}
	if s.NearestKm != nil || s.Inputs.Stations != 0 {
		t.Error("no stations are assimilated before G5, yet the snapshot says some are")
	}
}

// TestAnUpdateTooSoonAnswersFromTheLast is R-5.4 for NOAA (D-39: grids at
// most 6 an hour): within ten minutes of the last ask nothing is asked; the
// last snapshot comes back, said too soon, in fresh slices (P-6).
func TestAnUpdateTooSoonAnswersFromTheLast(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	first, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	n.take()
	first.Hours[0].FoF2.Values[0] = -1
	first.Hours[0].FoF2.NoData.Set(1)
	c.Add(9 * time.Minute)
	second, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if a := n.take(); len(a) != 0 || len(n.drapAsks) != 1 {
		t.Errorf("asked %v and D-RAP %d times within ten minutes; want D-RAP's first ask alone", a, len(n.drapAsks))
	}
	if second.Early != ionomaps.TooSoon || !second.Computed.Equal(first.Computed) {
		t.Errorf("early %v, computed %v", second.Early, second.Computed)
	}
	if v := second.Hours[0].FoF2.Values[0]; v == -1 || second.Hours[0].FoF2.NoData.Has(1) {
		t.Error("a change to one snapshot reached the next")
	}
	second.Hours[0].FoF2.Values[2] = -1
	if third, _ := settled(l, context.Background(), ionomaps.UpdateOptions{}); third.Hours[0].FoF2.Values[2] == -1 {
		t.Error("a change to an early answer reached the next")
	}
}

// TestAnUnchangedIndexAsksForNoGrid is D-111: a grid already held is not
// asked again; the solar file is asked once a day, with its validators.
func TestAnUnchangedIndexAsksForNoGrid(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	n.take()
	c.Add(10 * time.Minute)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if a := n.take(); len(a) != 1 || a[0] != indexURL {
		t.Errorf("asked %v; only the index, the grid unchanged and the solar file read today", a)
	}
	if s.Early != ionomaps.Fresh || !s.Computed.Equal(start.Add(10*time.Minute)) || s.Inputs.F107Age != 10*time.Minute {
		t.Errorf("early %v, computed %v, F10.7 age %v", s.Early, s.Computed, s.Inputs.F107Age)
	}
	c.Add(12 * time.Hour)
	n.mu.Lock()
	n.validator = nil
	n.mu.Unlock()
	if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	a := n.take()
	if len(a) != 2 || a[1] != solarURL || n.validator[1].ETag != `"solar-1"` {
		t.Errorf("the next day asked %v with %+v; the solar file, with its validators", a, n.validator)
	}
}

// TestWithoutGloTECTheClimatologyIsNamed is FR-5.2 (watchpost D-101): with no
// index, or a grid more than an hour old, foF2 is the climatology's, named,
// and no GloTEC time is given.
func TestWithoutGloTECTheClimatologyIsNamed(t *testing.T) {
	l, n, c := library(t)
	n.status[indexURL] = 404
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Background.FoF2 != ionomaps.Climatology || !s.Inputs.GloTECValid.IsZero() || len(s.Hours) != 1 {
		t.Errorf("no index: background %v, GloTEC valid %v", s.Background.FoF2, s.Inputs.GloTECValid)
	}
	delete(n.status, indexURL)
	n.list(t, start.Add(-30*time.Minute), 7)
	c.Add(10 * time.Minute)
	if s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{}); s.Background.FoF2 != ionomaps.GloTEC {
		t.Fatalf("with the grid listed the background is %v", s.Background.FoF2)
	}
	n.status[indexURL] = 404
	c.Add(61 * time.Minute)
	if s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{}); s.Background.FoF2 != ionomaps.Climatology || !s.Inputs.GloTECValid.IsZero() {
		t.Errorf("a grid over an hour old: background %v, GloTEC valid %v", s.Background.FoF2, s.Inputs.GloTECValid)
	}
}

// TestARefusalBacksOff is D-39: a 429 stops the update, and it or no answer
// stops the asking for 60 s,
// doubling to 15 minutes; an answer ends the back-off.
func TestARefusalBacksOff(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.status[indexURL] = 429
	if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err == nil {
		t.Error("a refusal with nothing held gave no error")
	}
	if a := n.take(); len(a) != 1 {
		t.Errorf("after a refusal the update asked on: %v", a)
	}
	for i, wait := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		c.Add(wait - time.Second)
		if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err == nil || len(n.take()) != 0 {
			t.Fatalf("refusal %d: asked again before %v", i+1, wait)
		}
		c.Add(time.Second)
		settled(l, context.Background(), ionomaps.UpdateOptions{})
		if a := n.take(); len(a) != 1 {
			t.Fatalf("refusal %d: after %v asked %v", i+1, wait, a)
		}
	}
	delete(n.status, indexURL)
	c.Add(15 * time.Minute)
	if s, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err != nil || s.Early != ionomaps.Fresh {
		t.Fatalf("after the refusals ended: %v, %v", s.Early, err)
	}
	n.down[indexURL] = true
	c.Add(10 * time.Minute)
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	n.take()
	c.Add(time.Minute)
	if s, _ := settled(l, context.Background(), ionomaps.UpdateOptions{}); len(n.take()) != 1 || s.Early != ionomaps.Offline {
		t.Errorf("after an answer, the first back-off is 60 s again, and an update without NOAA is offline: %v", s.Early)
	}
}

// TestACancelledUpdateIsNoRefusal: an update the host cancels (closing the
// mode, watchpost FR-4.2) is no refusal by NOAA, so it starts no back-off
// and spends no ten minutes; the next update asks at once.
func TestACancelledUpdateIsNoRefusal(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	settled(l, ctx, ionomaps.UpdateOptions{})
	n.take()
	c.Add(time.Second)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil || s.Early != ionomaps.Fresh || len(n.take()) != 3 {
		t.Errorf("after a cancelled update: %v, %v", s.Early, err)
	}
}

// TestGridsAreAskedAtMostSixAnHour is D-39: however the asks fail and back
// off, no hour holds more than six asks for a grid.
func TestGridsAreAskedAtMostSixAnHour(t *testing.T) {
	l, n, c := library(t)
	n.down["grid"] = true
	var asks []time.Time
	for range 360 { // three hours, every 30 s, a new grid listed each time
		n.list(t, c.Now().Add(-25*time.Minute), 7)
		settled(l, context.Background(), ionomaps.UpdateOptions{})
		for _, a := range n.take() {
			if strings.HasSuffix(a, ".geojson") {
				asks = append(asks, c.Now())
			}
		}
		c.Add(30 * time.Second)
	}
	for i := range asks {
		in := 0
		for _, b := range asks[i:] {
			if b.Sub(asks[i]) < time.Hour {
				in++
			}
		}
		if in > 6 {
			t.Fatalf("%d grid asks in the hour from %v", in, asks[i])
		}
	}
	if len(asks) < 12 {
		t.Errorf("%d grid asks in three hours; the limit is six an hour, not fewer", len(asks))
	}
}

// TestNothingHeldAndNoNOAAIsAnErrorWithAPath is D-124 for the library: with
// nothing held and NOAA unreachable there is no field; the error says when
// it will be asked again, and the snapshot is said offline.
func TestNothingHeldAndNoNOAAIsAnErrorWithAPath(t *testing.T) {
	l, n, _ := library(t)
	n.down[indexURL], n.down[solarURL] = true, true
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err == nil || len(s.Hours) != 0 || s.Early != ionomaps.Offline {
		t.Fatalf("hours %d, early %v, error %v", len(s.Hours), s.Early, err)
	}
	if !strings.Contains(err.Error(), "asked again") {
		t.Errorf("the error gives no path: %v", err)
	}
}

// TestAnUpdateIsSafeForConcurrentUse is R-5.6: updates from several
// goroutines at once (run with the race detector) ask NOAA once.
func TestAnUpdateIsSafeForConcurrentUse(t *testing.T) {
	l, n, _ := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if a := n.take(); len(a) != 3 {
		t.Errorf("eight updates at once asked %d times; once each of index, grid and solar file", len(a))
	}
}

// TestASolarFileNotReadKeepsNoValidators: a solar file that answers but does
// not read gives no mean, and its validators are not kept, so the next ask
// is a whole one - never a 304 against a file that gave nothing.
func TestASolarFileNotReadKeepsNoValidators(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	good := n.solar
	n.solar = []byte("not the solar file")
	if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err == nil {
		t.Fatal("a solar file that does not read gave a field")
	}
	n.solar = good
	c.Add(10 * time.Minute)
	n.take()
	n.mu.Lock()
	n.validator = nil
	n.mu.Unlock()
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil || s.Inputs.F107Mean != 140 {
		t.Fatalf("the next update: mean %v, error %v", s.Inputs.F107Mean, err)
	}
	for i, url := range n.take() {
		if url == solarURL && n.validator[i] != (ionomaps.Validators{}) {
			t.Errorf("the solar file was asked again with %+v, the validators of a file that gave nothing", n.validator[i])
		}
	}
}

// settled is an update as a host makes it (D-63): when NOAA's snapshot says
// the stations are next, the host asks again at once, and that is the
// snapshot the NOAA tests read.
func settled(l *ionomaps.Library, ctx context.Context, o ionomaps.UpdateOptions) (ionomaps.Snapshot, error) {
	s, err := l.Update(ctx, o)
	if err == nil && s.Early == ionomaps.StationsNext {
		return l.Update(ctx, o)
	}
	return s, err
}
