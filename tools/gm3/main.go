// Command gm3 is go-ionomaps' G-M3 instrument (plan task G10.4; D-108,
// D-75, D-132): over recorded days, each GIRO station with a sounding at a
// GloTEC grid's time is held out in turn, the others' readings are
// assimilated by the library's own recipe (internal/hybrid, as the update
// makes its field), and the result at the held-out station is scored
// against what it measured, beside the climatology the library ships
// (NRL's CCIR refit, the 30-day F10.7).
//
//	gm3 -inputs DIR -solar FILE [-from YYYY-MM-DD] [-pairing shipped|week]
//
// DIR holds dry_fetch.py's output, read through ionomapstest; FILE is
// NOAA's daily-solar-indices.txt covering the 30 days before each day. The
// inputs stay outside the tree (GIRO's terms); nothing is fetched.
//
// -pairing shipped, the default, takes each other station's newest reading
// at or before the grid, at most an hour old, as the library assimilates
// what its burst holds (watchpost D-237); -pairing week takes the soundings
// within ten minutes either side, as week.py did, and -from scores only the
// grids from a day on: together, the replica check against week.py.
//
// It prints the figures by region and by distance to the nearest station
// assimilated (D-116, D-129), then G-M3's targets and D-75's floor (1)
// judged, and exits 1 when one is missed, 2 when the inputs cannot be
// scored.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
	"github.com/branden-thompson/go-ionomaps/ionomapstest"
)

// minF107Days is the fewest observed days the 30-day mean is taken over
// (week.py's MIN_F107_DAYS).
const minF107Days = 20

// pairings are -pairing's words.
var pairings = map[string]pairing{"shipped": asShipped, "week": asWeekPy}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now()))
}

func run(args []string, out, errOut io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("gm3", flag.ContinueOnError)
	fs.SetOutput(errOut)
	inputs := fs.String("inputs", "", "folder of the days' GloTEC grids and GIRO replies (outside the tree)")
	solarFile := fs.String("solar", "", "the days' daily-solar-indices.txt")
	from := fs.String("from", "", "score only the grids from this UTC day on, YYYY-MM-DD")
	pairWord := fs.String("pairing", "shipped", "the other stations' readings: shipped (as the library takes them) or week (week.py's)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, known := pairings[*pairWord]
	if *inputs == "" || *solarFile == "" || !known {
		_, _ = fmt.Fprintln(errOut, "gm3: -inputs and -solar are required, and -pairing is shipped or week")
		return 2
	}
	var since time.Time
	if *from != "" {
		t, err := time.Parse(time.DateOnly, *from)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, "gm3: -from is a day, YYYY-MM-DD")
			return 2
		}
		since = t
	}
	report, missed, err := score(*inputs, *solarFile, since, p, now)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "gm3:", err)
		return 2
	}
	if _, err := fmt.Fprint(out, report); err != nil {
		return 2
	}
	if missed > 0 {
		return 1
	}
	return 0
}

// methods are the report's words for each pairing.
var methods = map[pairing]string{
	asShipped: "the other stations' newest readings at or before the grid, at most an hour old, as the library assimilates what its burst holds (its best case: a burst just before the grid)",
	asWeekPy:  "the other stations' soundings within 10 minutes of the grid, either side, as week.py took them",
}

// score reads the inputs, holds every station out at every grid from since,
// and renders the figures and the verdicts.
func score(dir, solarFile string, since time.Time, p pairing, now time.Time) (string, int, error) {
	rec, err := ionomapstest.Open(dir, solarFile)
	if err != nil {
		return "", 0, err
	}
	raw, err := os.ReadFile(solarFile)
	if err != nil {
		return "", 0, err
	}
	days, err := solar.Parse(raw)
	if err != nil {
		return "", 0, err
	}
	grids, err := readGrids(rec, now)
	if err != nil {
		return "", 0, err
	}
	stations, unread := readStations(rec, grids[0].Valid.Add(-2*time.Hour), grids[len(grids)-1].Valid.Add(time.Hour), now)
	if len(stations) == 0 {
		return "", 0, fmt.Errorf("%w (%s)", errNoStations, strings.Join(unread, "; "))
	}
	var pairs []pair
	scored, f107Lo, f107Hi := 0, 0.0, 0.0
	for i := range grids {
		g := &grids[i]
		if g.Valid.Before(since) {
			continue
		}
		f107, n, ok := solar.Mean(days, g.Valid)
		if !ok || n < minF107Days {
			return "", 0, fmt.Errorf("only %d days of F10.7 in the 30 before %s (need %d)", n, g.Valid.Format(time.DateOnly), minF107Days)
		}
		if scored == 0 || f107 < f107Lo {
			f107Lo = f107
		}
		f107Hi = max(f107Hi, f107)
		ps, err := heldOut(momentAt(g, f107), stations, p)
		if err != nil {
			return "", 0, err
		}
		pairs = append(pairs, ps...)
		scored++
	}
	if len(pairs) == 0 {
		return "", 0, errNoPairs
	}
	first, last, held := covered(pairs)
	var b strings.Builder
	fmt.Fprintf(&b, "# G-M3, held out: %s to %s\n\n", first.Format("2006-01-02 15:04"), last.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "Tool: gm3 at %s.\n", revision())
	fmt.Fprintf(&b, "Inputs: %d GloTEC grids scored of %d read, %d GIRO stations read, %d held out; %d pairs. F10.7, the 30-day mean: %.0f to %.0f.\n",
		scored, len(grids), len(stations), len(held), len(pairs), f107Lo, f107Hi)
	fmt.Fprintf(&b, "Held out: %s.\n", strings.Join(held, ", "))
	if len(unread) > 0 {
		fmt.Fprintf(&b, "Replies not read: %s.\n", strings.Join(unread, "; "))
	}
	fmt.Fprintf(&b, "Method: each station with a sounding within 10 minutes of a grid, carrying M(3000)F2, its score as the library takes it (70 or more, manual or unknown, D-61), held out; predicted from %s, at least three, by internal/hybrid over background.Build with the grid; the climatology background.Build without one; both read at the station by grid.Sample. MUF(3000) is foF2 times M(3000)F2.\n", methods[p])
	table(&b, "By region", pairs, regions)
	table(&b, "By distance to the nearest station assimilated", pairs, bands)
	missed := 0
	b.WriteString("\n## G-M3 (D-108) and D-75's floor (1)\n\n| Measure | Hybrid against the climatology | Target | Verdict |\n|---|---|---|---|\n")
	for _, v := range judge(pairs) {
		word := "met"
		if !v.met {
			word, missed = "**missed**", missed+1
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", v.name, v.measured, v.target, word)
	}
	return b.String(), missed, nil
}

// momentAt is a grid's time and its backgrounds: the hybrid's, built with
// the grid as the update builds it, and the climatology's alone.
func momentAt(g *glotec.Grid, f107 float64) moment {
	return moment{at: g.Valid, hybrid: background.Build(g.Valid, f107, g), clim: background.Build(g.Valid, f107, nil)}
}

// table writes the hybrid's and the climatology's errors for each group.
func table(b *strings.Builder, title string, ps []pair, groups []group) {
	fmt.Fprintf(b, "\n## %s\n\nRMS / bias, MHz.\n\n| Group | Pairs | Hybrid foF2 | Climatology foF2 | Hybrid MUF(3000) | Climatology MUF(3000) |\n|---|---|---|---|---|---|\n", title)
	for _, g := range groups {
		in := selected(ps, g)
		if len(in) == 0 {
			continue
		}
		hy, cl := errorsOf(in, true), errorsOf(in, false)
		fmt.Fprintf(b, "| %s | %d | %.2f / %+.2f | %.2f / %+.2f | %.2f / %+.2f | %.2f / %+.2f |\n",
			g.name, len(in), hy.foRMS, hy.foBias, cl.foRMS, cl.foBias, hy.mRMS, hy.mBias, cl.mRMS, cl.mBias)
	}
}

var errNoStations = errors.New("no GIRO reply with a reading in the inputs")

// readGrids reads every recorded grid by the library's parser, in time
// order.
func readGrids(rec *ionomapstest.Recorded, now time.Time) ([]glotec.Grid, error) {
	var out []glotec.Grid
	for _, valid := range rec.GridTimes() {
		raw, err := rec.Grid(valid)
		if err != nil {
			return nil, err
		}
		g, err := glotec.Read(raw, now)
		if err != nil {
			return nil, fmt.Errorf("the grid valid at %s: %w", valid.Format(time.RFC3339), err)
		}
		out = append(out, g)
	}
	return out, nil
}

// readStations reads every recorded reply in the library's columns
// (ionomapstest) by the library's parser, its readings kept to the window;
// a reply refused is named with why, never dropped unsaid (GIRO answers a
// station with no data with a reply that has no location line).
func readStations(rec *ionomapstest.Recorded, from, to, now time.Time) (stations []station, unread []string) {
	for _, code := range rec.Stations() {
		body, err := rec.Reply(code, from, to, now)
		if err != nil {
			unread = append(unread, code+" ("+err.Error()+")")
			continue
		}
		r, err := giro.ParseReply(body, code, from, to, now)
		if err != nil {
			unread = append(unread, code+" ("+err.Error()+")")
			continue
		}
		if len(r.Readings) > 0 {
			stations = append(stations, station{code: code, lat: r.Station.Lat, lon: r.Station.Lon, readings: r.Readings})
		}
	}
	return stations, unread
}

// revision is the commit the binary was built from and whether its tree
// was clean, as the build stamped it: a figure is cited only from the
// committed tool (watchpost E-19). `go run` stamps nothing, so its figures
// say so; build the tool with `go build` to cite them.
func revision() string {
	var rev, modified string
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				modified = s.Value
			}
		}
	}
	switch {
	case len(rev) < 12:
		return "an unknown revision (not built with go build from a repository): these figures are not citable"
	case modified != "false":
		return rev[:12] + ", built from a DIRTY tree: these figures are not citable"
	}
	return rev[:12] + ", built from a clean tree"
}
