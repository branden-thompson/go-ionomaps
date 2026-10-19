package ionomapstest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/terms"
	"github.com/branden-thompson/go-ionomaps/ionomapstest"
)

// day is the tests' first day.
var day = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)

// stations are four of the library's seeded live stations (D-83), which an
// update asks GIRO for, at their published places in the mainland US, each
// reading foF2 6 MHz and M(3000)F2 3.
var stations = []ionomapstest.Station{
	{Code: "AL945", Lat: 45.1, Lon: -83.6, FoF2: 6, M3000: 3},
	{Code: "EG931", Lat: 30.5, Lon: -86.5, FoF2: 6, M3000: 3},
	{Code: "IF843", Lat: 43.8, Lon: -112.7, FoF2: 6, M3000: 3},
	{Code: "MHJ45", Lat: 42.6, Lon: -71.5, FoF2: 6, M3000: 3},
}

// recorded writes two grids, 00:05 and 03:05, the stations reading every 15
// minutes through the first two hours, and opens them.
func recorded(t *testing.T) *ionomapstest.Recorded {
	t.Helper()
	dir := t.TempDir()
	var readings []time.Time
	for m := 0; m <= 120; m += 15 {
		readings = append(readings, day.Add(time.Duration(m)*time.Minute))
	}
	solar, err := ionomapstest.Write(dir, ionomapstest.Synthetic{
		Grids:    []time.Time{day.Add(3*time.Hour + 5*time.Minute), day.Add(5 * time.Minute)},
		GridFoF2: 6, Stations: stations, Readings: readings, F107: 100})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ionomapstest.Open(dir, solar)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// TestTheReplayServesNothingAfterItsClock is the replay's one promise: at
// its clock it answers as the services would have then - the grids valid by
// then and no later, the solar rows of the days before, the readings in the
// window asked and not after the clock, in the columns the library asks,
// without a line naming the requester.
func TestTheReplayServesNothingAfterItsClock(t *testing.T) {
	r := recorded(t).Replay(ionomapstest.Options{})
	ctx, none := context.Background(), ionomaps.Validators{}
	r.SetNow(day.Add(2*time.Hour + 35*time.Minute))
	idx, _ := r.Fetch(ctx, terms.GloTECIndex, none)
	if !strings.Contains(string(idx.Body), "T000500Z") || strings.Contains(string(idx.Body), "T030500Z") {
		t.Errorf("the index at 02:35 names other than the 00:05 grid:\n%s", idx.Body)
	}
	if g, _ := r.Fetch(ctx, terms.GloTECIndex+"glotec_icao_20260929T030500Z.geojson", none); g.Status != 404 {
		t.Errorf("the 03:05 grid at 02:35: %d; want missing", g.Status)
	}
	r.SetNow(day.Add(3*time.Hour + 35*time.Minute))
	if g, _ := r.Fetch(ctx, terms.GloTECIndex+"glotec_icao_20260929T030500Z.geojson", none); g.Status != 200 || len(g.Body) == 0 {
		t.Errorf("the 03:05 grid at 03:35: %d; want served", g.Status)
	}
	sol, _ := r.Fetch(ctx, terms.SolarFile, none)
	if !strings.Contains(string(sol.Body), "2026 09 28") || strings.Contains(string(sol.Body), "2026 09 29") || !strings.Contains(string(sol.Body), ":Product:") {
		t.Errorf("the solar file on the 29th holds the 29th's row, or lacks the 28th's or its header:\n%s", sol.Body)
	}
	r.SetNow(day.Add(50 * time.Minute))
	u := terms.FastChar + "?ursiCode=AL945&charName=foF2%2CM%28D%29&DMUF=3000&fromDate=2026%2F09%2F29+00%3A10%3A00&toDate=2026%2F09%2F29+00%3A50%3A00"
	st, err := r.Fetch(ctx, u, none)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for line := range strings.Lines(string(st.Body)) {
		if !strings.HasPrefix(line, "#") {
			rows = append(rows, strings.Join(strings.Fields(line), " "))
		}
	}
	want := []string{"2026-09-29T00:15:00.000Z 90 6.000 // 3.00 //", "2026-09-29T00:30:00.000Z 90 6.000 // 3.00 //", "2026-09-29T00:45:00.000Z 90 6.000 // 3.00 //"}
	if !slices.Equal(rows, want) {
		t.Errorf("AL945 from 00:10 to 00:50 at 00:50:\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(string(st.Body), "URSI-Code AL945") {
		t.Errorf("the reply lost the station's location line:\n%s", st.Body)
	}
	r.SetNow(day.Add(20 * time.Minute))
	if st, _ := r.Fetch(ctx, u, none); strings.Contains(string(st.Body), "T00:30:00") {
		t.Errorf("at 00:20 the reply holds a reading from 00:30:\n%s", st.Body)
	}
}

// TestTheReplayAnswersOnlyWhatItHolds is the replay's edges: without
// GloTEC no grid is given; D-RAP, the scales and a station not recorded are
// missing; any other address fails, and is kept for the run to report.
func TestTheReplayAnswersOnlyWhatItHolds(t *testing.T) {
	r := recorded(t).Replay(ionomapstest.Options{WithoutGloTEC: true})
	r.SetNow(day.Add(4 * time.Hour))
	ctx, none := context.Background(), ionomaps.Validators{}
	for _, u := range []string{terms.GloTECIndex, terms.GloTECIndex + "glotec_icao_20260929T000500Z.geojson", terms.DRAPTable, terms.ScalesFile,
		terms.FastChar + "?ursiCode=ZZ999&fromDate=2026%2F09%2F29+00%3A00%3A00&toDate=2026%2F09%2F29+01%3A00%3A00"} {
		if got, err := r.Fetch(ctx, u, none); err != nil || got.Status != 404 {
			t.Errorf("%s: %d, %v; want missing", u, got.Status, err)
		}
	}
	if _, err := r.Fetch(ctx, "https://example.org/other", none); !errors.Is(err, ionomapstest.ErrUnknownAsk) || len(r.Unknown()) != 1 {
		t.Errorf("an unknown address: %v, kept %v", err, r.Unknown())
	}
	if _, err := ionomapstest.Open(t.TempDir(), filepath.Join(t.TempDir(), "none")); err == nil {
		t.Error("an empty folder opened")
	}
}

// TestTheRecordedFilesAreRead is the reader a scorer uses beside the
// replay: grids by time, stations by code, every file named for the
// checksums, and a row of another shape than dry_fetch.py's refused.
func TestTheRecordedFilesAreRead(t *testing.T) {
	rec := recorded(t)
	if g := rec.GridTimes(); len(g) != 2 || !g[0].Equal(day.Add(5*time.Minute)) {
		t.Errorf("grid times %v", g)
	}
	if b, err := rec.Grid(day.Add(5 * time.Minute)); err != nil || len(b) == 0 {
		t.Errorf("the 00:05 grid: %d bytes, %v", len(b), err)
	}
	if _, err := rec.Grid(day.Add(time.Hour)); !errors.Is(err, ionomapstest.ErrNoGrid) {
		t.Errorf("a grid not recorded: %v", err)
	}
	if s := rec.Stations(); !slices.Equal(s, []string{"AL945", "EG931", "IF843", "MHJ45"}) {
		t.Errorf("stations %v", s)
	}
	if f := rec.Files(); len(f) != 7 || !slices.IsSorted(f) {
		t.Errorf("files %v", f)
	}
	if _, err := rec.Reply("ZZ999", day, day.Add(time.Hour), day.Add(time.Hour)); !errors.Is(err, ionomapstest.ErrNoStation) {
		t.Errorf("a station not recorded: %v", err)
	}
	dir := t.TempDir()
	solar, err := ionomapstest.Write(dir, ionomapstest.Synthetic{Grids: []time.Time{day}, GridFoF2: 6, F107: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fc_XX09X.txt"), []byte("2026-09-29T00:15:00.000Z  90  6.000 // 3.00 //\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	odd, err := ionomapstest.Open(dir, solar)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := odd.Reply("XX09X", day, day.Add(time.Hour), day.Add(time.Hour)); err == nil {
		t.Error("a row in the library's own six columns was read as a recorded one")
	}
	// A reply as GIRO sends it names who asked: the replay leaves that out.
	// Built from pieces, with a documentation address (RFC 5737), so no
	// FastChar signature stands whole here (R-4.3).
	asked := "# Location: " + "GEO" + " ( 45.1 N   276.4 E ), URSI-Code XX08X, SYNTHETIC\n# Requested by unknown guest from IP:192.0.2.1\n" +
		"2026-09-29T00:15:00.000Z  90  6.000 // 18.000 // 3.00 //  250.0 //\n"
	if err := os.WriteFile(filepath.Join(dir, "fc_XX08X.txt"), []byte(asked), 0o600); err != nil {
		t.Fatal(err)
	}
	withAsker, err := ionomapstest.Open(dir, solar)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := withAsker.Reply("XX08X", day, day.Add(time.Hour), day.Add(time.Hour)); err != nil || strings.Contains(string(b), "Requested by") || !strings.Contains(string(b), "URSI-Code XX08X") {
		t.Errorf("the reply as served: %v\n%s", err, b)
	}
}

// TestALibraryRunsOnRecordedDays is the package's use: the library, on the
// replay's clock, updates from the recorded days - GloTEC's grid as its
// background, the solar file's mean, the four stations assimilated, nothing
// asked that the replay does not hold - and without GloTEC, the
// climatology, the stations still asked, GloTEC named unread. D-RAP and the
// scales are never recorded, so they are named unread too and every replayed
// snapshot is Partial (watchpost D-239).
func TestALibraryRunsOnRecordedDays(t *testing.T) {
	rec := recorded(t)
	for _, without := range []bool{false, true} {
		r := rec.Replay(ionomapstest.Options{WithoutGloTEC: without})
		lib, err := ionomaps.New(r, ionomaps.Options{Clock: r.Now})
		if err != nil {
			t.Fatal(err)
		}
		r.SetNow(day.Add(35 * time.Minute))
		s, err := lib.Update(context.Background(), ionomaps.UpdateOptions{})
		if err == nil && s.Reason == ionomaps.StationsNext { // NOAA's files first, the stations' burst on the next update
			s, err = lib.Update(context.Background(), ionomaps.UpdateOptions{})
		}
		if err != nil {
			t.Fatalf("without GloTEC %v: %v", without, err)
		}
		wantBG, wantUnread := ionomaps.GloTEC, []string{"D-RAP", "scales"}
		if without {
			wantBG, wantUnread = ionomaps.Climatology, []string{"GloTEC", "D-RAP", "scales"}
		}
		if s.Background.FoF2 != wantBG || s.Reason != ionomaps.Partial || !slices.Equal(s.Inputs.Unread, wantUnread) || s.Inputs.Stations != 4 || s.Inputs.F107Mean != 100 || len(r.Unknown()) != 0 {
			t.Errorf("without GloTEC %v: background %v, said %v, unread %v, stations %d, F10.7 %v, unknown %v", without, s.Background.FoF2, s.Reason, s.Inputs.Unread, s.Inputs.Stations, s.Inputs.F107Mean, r.Unknown())
		}
	}
}
