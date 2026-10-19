package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/hybrid"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
	"github.com/branden-thompson/go-ionomaps/ionomapstest"
)

// at is the fixtures' grid time; now is after it.
var (
	at  = time.Date(2026, 10, 14, 12, 5, 0, 0, time.UTC)
	now = time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
)

// flat is a field of one value in every cell.
func flat(v float32) []float32 {
	f := make([]float32, grid.Cells)
	for i := range f {
		f[i] = v
	}
	return f
}

// places are five places in week.py's mainland-US box, 400 to 1300 km apart.
var places = [][2]float64{{40, -105}, {37, -100}, {43, -95}, {35, -110}, {45, -88}}

// usStations are a station at each place, each reading foF2 fo and
// M(3000)F2 m at each offset from at.
func usStations(fo, m float32, offs ...time.Duration) []station {
	var out []station
	for i, p := range places {
		s := station{code: fmt.Sprintf("XX%02dX", i), lat: p[0], lon: p[1]}
		for _, off := range offs {
			s.readings = append(s.readings, giro.Reading{At: at.Add(off), Score: 90, FoF2: fo, M3000: m, MUF3000: fo * m, HasM300: true})
		}
		out = append(out, s)
	}
	return out
}

// fixed are backgrounds of one value each: the hybrid's 10 MHz and 3, the
// climatology's 9 MHz and 3.2.
var fixed = moment{at: at,
	hybrid: background.Fields{FoF2: flat(10), M3000: flat(3)},
	clim:   background.Fields{FoF2: flat(9), M3000: flat(3.2)}}

// TestAHeldOutStationIsPredictedFromTheOthers is G10.4: every station with
// a sounding at the grid's time is held out in turn and predicted from the
// others by the library's recipe. With every residual the same, the others'
// fit is that residual everywhere, so the hybrid predicts each station
// exactly; a station reading apart is predicted from the others, never from
// itself.
func TestAHeldOutStationIsPredictedFromTheOthers(t *testing.T) {
	ps, err := heldOut(fixed, usStations(10.5, 3, -4*time.Minute), asShipped)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 5 {
		t.Fatalf("%d stations held out of 5", len(ps))
	}
	for _, p := range ps {
		if math.Abs(p.hyFo-10.5) > 1e-4 || math.Abs(p.hyM-3) > 1e-4 || p.climFo != 9 || math.Abs(p.climM-3.2) > 1e-6 {
			t.Errorf("%s: hybrid %.4f x %.4f, climatology %.2f x %.2f", p.code, p.hyFo, p.hyM, p.climFo, p.climM)
		}
		if !(p.nearKm > 300 && p.nearKm < 1500) {
			t.Errorf("%s: the nearest other station %.0f km away", p.code, p.nearKm)
		}
	}
	hy, cl := errorsOf(ps, true), errorsOf(ps, false)
	if hy.foRMS > 1e-4 || hy.mRMS > 1e-3 || math.Abs(cl.foRMS-1.5) > 1e-6 || math.Abs(cl.mBias-(9*3.2-10.5*3)) > 1e-4 {
		t.Errorf("hybrid %+v, climatology %+v", hy, cl)
	}
	odd := usStations(10.5, 3, -4*time.Minute)
	odd[0].readings[0].FoF2, odd[0].readings[0].MUF3000 = 12.5, 37.5
	ps, err = heldOut(fixed, odd, asShipped)
	if err != nil {
		t.Fatal(err)
	}
	if ps[0].code != "XX00X" || math.Abs(ps[0].hyFo-10.5) > 1e-4 || ps[0].obsFo != 12.5 {
		t.Errorf("the odd station held out: %+v", ps[0])
	}
	// A station whose sounding carries no M(3000)F2 has no MUF(3000) to be
	// scored on: the other four are.
	noM := usStations(10.5, 3, -4*time.Minute)
	noM[0].readings[0].HasM300, noM[0].readings[0].M3000, noM[0].readings[0].MUF3000 = false, 0, 0
	if ps, _ := heldOut(fixed, noM, asShipped); len(ps) != 4 || ps[0].code == "XX00X" {
		t.Errorf("a sounding with no M(3000)F2 was scored: %d held out", len(ps))
	}
	// With three stations each is predicted from two: none is scored.
	if ps, _ := heldOut(fixed, usStations(10.5, 3, -4*time.Minute)[:3], asShipped); len(ps) != 0 {
		t.Errorf("three stations gave %d held out", len(ps))
	}
	// A field that is no number at a station fails the moment, never a NaN
	// carried into every RMS it enters.
	bad := fixed
	bad.clim = background.Fields{FoF2: flat(float32(math.NaN())), M3000: flat(3.2)}
	if _, err := heldOut(bad, usStations(10.5, 3, 0), asShipped); err == nil {
		t.Error("a climatology of NaN was scored")
	}
}

// withLate are the stations' readings at offs, each with a sounding 5
// minutes after the grid too, which scores it but which the library, at
// the grid's time, would not yet hold.
func withLate(offs ...time.Duration) []station {
	sts := usStations(10.5, 3, offs...)
	for i := range sts {
		sts[i].readings = append(sts[i].readings, giro.Reading{At: at.Add(5 * time.Minute), Score: 90, FoF2: 10.5, M3000: 3, MUF3000: 31.5, HasM300: true})
	}
	return sts
}

// TestTheOthersAreTakenAsThePairingSays is watchpost D-237: as shipped, the
// other stations' readings are those the library would hold at the grid's
// time - at or before it, at most an hour old - so a sounding minutes after
// the grid scores its own station but feeds no other; as week.py, every
// sounding within ten minutes either side does both.
func TestTheOthersAreTakenAsThePairingSays(t *testing.T) {
	after := usStations(10.5, 3, 4*time.Minute)
	if ps, _ := heldOut(fixed, after, asShipped); len(ps) != 0 {
		t.Errorf("as shipped, soundings 4 minutes after the grid fed %d predictions", len(ps))
	}
	if ps, _ := heldOut(fixed, after, asWeekPy); len(ps) != 5 {
		t.Errorf("as week.py, soundings 4 minutes after the grid gave %d of 5", len(ps))
	}
	if ps, _ := heldOut(fixed, withLate(-70*time.Minute), asShipped); len(ps) != 0 {
		t.Errorf("readings 70 minutes old were assimilated: %d held out", len(ps))
	}
	if ps, _ := heldOut(fixed, withLate(-50*time.Minute), asShipped); len(ps) != 5 {
		t.Errorf("readings 50 minutes old gave %d of 5", len(ps))
	}
}

// TestTheNearestSoundingIsPaired: of a station's soundings within ten
// minutes of the grid, the nearest is the one scored; the newest at or
// before the grid is the one held.
func TestTheNearestSoundingIsPaired(t *testing.T) {
	s := station{readings: []giro.Reading{
		{At: at.Add(-9 * time.Minute), FoF2: 1, M3000: 3, MUF3000: 3, HasM300: true},
		{At: at.Add(2 * time.Minute), FoF2: 2, M3000: 3, MUF3000: 6, HasM300: true},
		{At: at.Add(-11 * time.Minute), FoF2: 3, M3000: 3, MUF3000: 9, HasM300: true},
	}}
	if r, ok := sounding(s, at); !ok || r.FoF2 != 2 {
		t.Errorf("paired %+v, %v", r, ok)
	}
	if r, ok := newest(s, at); !ok || r.FoF2 != 1 {
		t.Errorf("held %+v, %v", r, ok)
	}
	if r, ok := sounding(station{readings: s.readings[2:]}, at); ok {
		t.Errorf("a sounding 11 minutes off was paired: %+v", r)
	}
}

// pairsWith are n mainland-US pairs, the hybrid and the climatology each
// off by a constant foF2 and MUF(3000), the nearest station near km away.
func pairsWith(n int, hyOff, clOff, km float64) []pair {
	var out []pair
	for i := range n {
		out = append(out, pair{at: at, code: fmt.Sprintf("XX%02dX", i), lat: 40, lon: -100, nearKm: km,
			obsFo: 8, obsMUF: 24, hyFo: 8 + hyOff, hyM: (24 + 3*hyOff) / (8 + hyOff), climFo: 8 + clOff, climM: (24 + 3*clOff) / (8 + clOff)})
	}
	return out
}

// TestTheTargetsAreJudgedAsRuled is D-108 and D-75's floor (1): the hybrid
// at least 15 % below the climatology in the mainland US and 10 % overall,
// on foF2 and MUF(3000); within 500 km, at most 0.5 and 1.6 MHz; and below
// the climatology in the US at all.
func TestTheTargetsAreJudgedAsRuled(t *testing.T) {
	met := func(ps []pair) map[string]bool {
		out := map[string]bool{}
		for _, v := range judge(ps) {
			out[v.name] = v.met
		}
		return out
	}
	for name, ok := range met(pairsWith(10, 0.42, 0.5, 300)) { // 16 % below, near: every target met
		if !ok {
			t.Errorf("16 %% below the climatology, 300 km from a station: %q missed", name)
		}
	}
	v := met(pairsWith(10, 0.43, 0.5, 300)) // 14 % below: the US targets missed, the overall and the floor met
	if v["G-M3: foF2 RMS, mainland US"] || v["G-M3: MUF(3000) RMS, mainland US"] || !v["G-M3: foF2 RMS, all stations"] || !v["D-75 floor (1): foF2, mainland US"] {
		t.Errorf("14 %% below: %v", v)
	}
	if v := met(pairsWith(10, 0.5, 0.5, 300)); v["D-75 floor (1): foF2, mainland US"] || v["D-75 floor (1): MUF(3000), mainland US"] {
		t.Errorf("equal to the climatology meets the floor: %v", v)
	}
	if v := met(pairsWith(10, 0.6, 2, 300)); v["G-M3: foF2 RMS within 500 km of a reporting station"] || v["G-M3: MUF(3000) RMS within 500 km of a reporting station"] {
		t.Errorf("0.6 MHz near a station meets the near targets: %v", v)
	}
	if v := met(pairsWith(10, 0.1, 2, 600)); v["G-M3: foF2 RMS within 500 km of a reporting station"] {
		t.Errorf("no pair within 500 km meets the near target: %v", v)
	}
}

// synthetic writes recorded days with ionomapstest: one grid at at, foF2
// fo everywhere, and a station at each place reading fo + 0.5 and 3, a
// quarter-hour apart up to the grid; plus a reply with no location line.
func synthetic(t *testing.T, fo float64) (dir, solarFile string) {
	t.Helper()
	dir = t.TempDir()
	var sts []ionomapstest.Station
	for i, p := range places {
		sts = append(sts, ionomapstest.Station{Code: fmt.Sprintf("XX%02dX", i), Lat: p[0], Lon: p[1], FoF2: fo + 0.5, M3000: 3})
	}
	solarFile, err := ionomapstest.Write(dir, ionomapstest.Synthetic{Grids: []time.Time{at}, GridFoF2: fo, Stations: sts,
		Readings: []time.Time{at.Add(-30 * time.Minute), at.Add(-15 * time.Minute), at}, F107: 106})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fc_XX99X.txt"), []byte("# Global Ionospheric Radio Observatory\n# no data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, solarFile
}

// TestARecordedDayIsScored is G10.4 end to end: the recorded files are
// read through ionomapstest and the library's parsers, every station is
// held out, a reply with no location line is named, and the report gives
// the figures and the verdicts; the hybrid, with every residual the same,
// predicts each station's foF2 exactly.
func TestARecordedDayIsScored(t *testing.T) {
	dir, sol := synthetic(t, 10)
	var out, errOut bytes.Buffer
	if code := run([]string{"-inputs", dir, "-solar", sol}, &out, &errOut, now); code == 2 {
		t.Fatalf("exit 2: %s", errOut.String())
	}
	for _, want := range []string{
		"1 GloTEC grids scored of 1 read, 5 GIRO stations read, 5 held out; 5 pairs. F10.7, the 30-day mean: 106 to 106.",
		"Held out: XX00X, XX01X, XX02X, XX03X, XX04X.\n",
		"Replies not read: XX99X (",
		"as the library assimilates what its burst holds",
		"| mainland US | 5 | 0.00 / +0.00 |",
		"## G-M3 (D-108) and D-75's floor (1)",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, out.String())
		}
	}
	out.Reset()
	if code := run([]string{"-inputs", dir, "-solar", sol, "-pairing", "week"}, &out, &errOut, now); code == 2 || !strings.Contains(out.String(), "as week.py took them") {
		t.Errorf("-pairing week: exit %d:\n%s", code, out.String())
	}
	if code := run([]string{"-inputs", dir, "-solar", sol, "-from", "2026-10-15"}, &out, &errOut, now); code != 2 {
		t.Errorf("no grid from the day asked: exit %d", code)
	}
}

// TestTheRunFailsClosed: inputs that cannot be scored exit 2, never a report
// with nothing in it.
func TestTheRunFailsClosed(t *testing.T) {
	dir, sol := synthetic(t, 10)
	short := filepath.Join(t.TempDir(), "dsi.txt")
	if err := os.WriteFile(short, []byte(fmt.Sprintf("%d %02d %02d   106\n", at.Year(), at.Month(), at.Day()-1)), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"no flags":           {},
		"no grids":           {"-inputs", t.TempDir(), "-solar", sol},
		"one day of F10.7":   {"-inputs", dir, "-solar", short},
		"a day that is none": {"-inputs", dir, "-solar", sol, "-from", "Tuesday"},
		"a pairing unknown":  {"-inputs", dir, "-solar", sol, "-pairing", "hourly"},
	} {
		if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}, now); code != 2 {
			t.Errorf("%s: exit %d", name, code)
		}
	}
}

// TestTheInstrumentScoresTheFieldTheLibraryShips is watchpost D-237's
// guarantee: on the same recorded day, the field gm3 builds with no station
// held out - its backgrounds, its readings as shipped, internal/hybrid - is
// the field the library's own update hands a host, cell for cell. Were the
// update's recipe to change and the instrument's not, this fails.
func TestTheInstrumentScoresTheFieldTheLibraryShips(t *testing.T) {
	dir := t.TempDir()
	sts := []ionomapstest.Station{ // four of the library's seeded live stations (D-83), which its burst asks
		{Code: "AL945", Lat: 45.1, Lon: -83.6, FoF2: 7.2, M3000: 3.1},
		{Code: "EG931", Lat: 30.5, Lon: -86.5, FoF2: 8.4, M3000: 2.9},
		{Code: "IF843", Lat: 43.8, Lon: -112.7, FoF2: 6.1, M3000: 3.3},
		{Code: "MHJ45", Lat: 42.6, Lon: -71.5, FoF2: 7.7, M3000: 3.0},
	}
	sol, err := ionomapstest.Write(dir, ionomapstest.Synthetic{Grids: []time.Time{at}, GridFoF2: 7, Stations: sts,
		Readings: []time.Time{at.Add(-30 * time.Minute), at.Add(-15 * time.Minute), at}, F107: 106})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ionomapstest.Open(dir, sol)
	if err != nil {
		t.Fatal(err)
	}
	replay := rec.Replay(ionomapstest.Options{})
	lib, err := ionomaps.New(replay, ionomaps.Options{Clock: replay.Now})
	if err != nil {
		t.Fatal(err)
	}
	replay.SetNow(at)
	ctx := context.Background()
	snap, err := lib.Update(ctx, ionomaps.UpdateOptions{})
	if err == nil && snap.Reason == ionomaps.StationsNext {
		snap, err = lib.Update(ctx, ionomaps.UpdateOptions{})
	}
	if err != nil || snap.Inputs.Stations != 4 {
		t.Fatalf("the library's update: %d stations, %v", snap.Inputs.Stations, err)
	}

	grids, err := readGrids(rec, now)
	if err != nil {
		t.Fatal(err)
	}
	stations, unread := readStations(rec, at.Add(-2*time.Hour), at.Add(time.Hour), now)
	raw, err := os.ReadFile(sol)
	if err != nil {
		t.Fatal(err)
	}
	days, err := solar.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	f107, _, _ := solar.Mean(days, at)
	m := momentAt(&grids[0], f107)
	rs, _, _ := hybrid.Readings(held(m, stations, -1, asShipped))
	res := hybrid.Field(m.hybrid, rs)
	if len(unread) != 0 || res.Used != 4 {
		t.Fatalf("the instrument read %d stations (%v), used %d", len(stations), unread, res.Used)
	}
	// Compared bit for bit, each product rounded to float32 by an explicit
	// conversion, as the library stores it: Go may fuse a multiply into a
	// subtraction, and round it differently, where the conversion is left out.
	differ := 0
	for i, v := range snap.Now.FoF2.Values {
		if v != res.FoF2[i] || snap.Now.MUF3000.Values[i] != float32(res.FoF2[i]*res.M3000[i]) {
			differ++
		}
	}
	if differ != 0 {
		t.Errorf("the instrument's field differs from the library's in %d of %d cells", differ, len(res.FoF2))
	}
}
