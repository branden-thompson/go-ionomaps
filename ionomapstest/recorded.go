// Package ionomapstest serves recorded days to the library, as
// net/http/httptest serves a handler to a client: a folder of GloTEC grids
// and GIRO replies, as the evidence's dry_fetch.py fetches them, and NOAA's
// daily solar file, answered at a clock as the services would have answered
// then, nothing dated after it.
//
// dry_fetch.py keeps no D-RAP table and no scales file, so the replay
// answers them as missing unless Options.Answers serves them: the library
// names them in Inputs.Unread and says each snapshot Partial, as it would
// of a NOAA that did not serve them. DRAPTable and
// ScalesFile build them, so a replay can be Fresh; Answers also gives any
// input a status, a body or an error, so a host's tests reach every state
// the library says. A station not recorded answers as GIRO answers
// one with no data.
//
// It is for a host's own tests and for the instruments that grade the
// library on recorded days (G10.4's tools/gm3; watchpost's tools/reachscore,
// W10.3). The recorded files stay outside any repository: GIRO's replies
// carry the requester's address and its data is CC BY-NC-SA (R-4.3).
//
//	rec, err := ionomapstest.Open(dir, solarFile)
//	replay := rec.Replay(ionomapstest.Options{})
//	lib, err := ionomaps.New(replay, ionomaps.Options{Clock: replay.Now})
//	replay.SetNow(t)
//	snap, err := lib.Update(ctx, ionomaps.UpdateOptions{})
//	if err == nil && snap.Reason == ionomaps.StationsNext {
//		snap, err = lib.Update(ctx, ionomaps.UpdateOptions{}) // the stations' burst
//	}
package ionomapstest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// gridName is a GloTEC grid's file name, its stamp the valid time.
var gridName = regexp.MustCompile(`^glotec_icao_(\d{8}T\d{6}Z)\.geojson$`)

// giroName is a recorded GIRO reply's file name, by its station's code.
var giroName = regexp.MustCompile(`^fc_([A-Z0-9]{5})\.txt$`)

// Recorded is a folder of recorded days, read once: the grids by name and
// valid time, the stations' replies by code, and the solar file. It is
// never written after Open, so any number of replays may share it.
type Recorded struct {
	dir   string
	grids map[string]time.Time
	giro  map[string][]byte
	solar []byte
	files []string
}

// The errors Open and the readers return; none quotes what a file holds.
var (
	ErrNoInputs    = errors.New("ionomapstest: the folder holds no GloTEC grid (glotec_icao_*.geojson) or no GIRO reply (fc_*.txt), or the solar file is empty")
	ErrNoStation   = errors.New("ionomapstest: no reply was recorded for the station")
	ErrNoGrid      = errors.New("ionomapstest: no grid was recorded valid at that time")
	errRecordedRow = fmt.Errorf("ionomapstest: a recorded GIRO row is not the %d fields dry_fetch.py asks for", recordedFields)
)

// Open reads a folder's grids' names and replies, and the solar file.
//
// Source: this library; the evidence's dry_fetch.py, whose layout it reads.
func Open(dir, solarFile string) (*Recorded, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	in := &Recorded{dir: dir, grids: map[string]time.Time{}, giro: map[string][]byte{}}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if m := gridName.FindStringSubmatch(e.Name()); m != nil {
			t, err := time.Parse("20060102T150405Z", m[1])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.Name(), err)
			}
			in.grids[e.Name()], in.files = t, append(in.files, path)
		}
		if m := giroName.FindStringSubmatch(e.Name()); m != nil {
			body, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			in.giro[m[1]], in.files = body, append(in.files, path)
		}
	}
	if in.solar, err = os.ReadFile(solarFile); err != nil {
		return nil, err
	}
	in.files = append(in.files, solarFile)
	if len(in.grids) == 0 || len(in.giro) == 0 || len(in.solar) == 0 {
		return nil, ErrNoInputs
	}
	slices.Sort(in.files)
	return in, nil
}

// Files are every file read, in order: what a run's checksums are taken of.
//
// Source: this library.
func (r *Recorded) Files() []string { return slices.Clone(r.files) }

// GridTimes are the grids' valid times, in order.
//
// Source: this library; NOAA SWPC GloTEC's file names, as served.
func (r *Recorded) GridTimes() []time.Time {
	out := make([]time.Time, 0, len(r.grids))
	for _, t := range r.grids {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	return out
}

// Grid is the grid valid at a time, as NOAA served it.
//
// Source: this library.
func (r *Recorded) Grid(valid time.Time) ([]byte, error) {
	for name, t := range r.grids {
		if t.Equal(valid) {
			return os.ReadFile(filepath.Join(r.dir, name))
		}
	}
	return nil, ErrNoGrid
}

// Stations are the recorded stations' codes, in order.
//
// Source: this library.
func (r *Recorded) Stations() []string {
	out := make([]string, 0, len(r.giro))
	for code := range r.giro {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

// recordedFields are a recorded reply's row fields as dry_fetch.py asks
// them: the time, the score, then foF2, MUF(3000), M(3000)F2 and hmF2, each
// with its qualifier.
const recordedFields = 10

// Reply is a station's recorded reply as GIRO would have answered the
// library's ask at now: the rows in the window and not after now, in the
// columns the library asks for - the time, the score, foF2 and
// M(3000)F2 with their qualifiers - and the header without the line naming
// the requester, which is the fetch's, not the station's. A row of another
// shape than dry_fetch.py's fails the reply.
//
// Source: GIRO's FastChar text, as dry_fetch.py and the library ask it.
func (r *Recorded) Reply(code string, from, to, now time.Time) ([]byte, error) {
	body, ok := r.giro[code]
	if !ok {
		return nil, ErrNoStation
	}
	var b bytes.Buffer
	for line := range bytes.Lines(body) {
		text := strings.TrimRight(string(line), "\r\n")
		if strings.HasPrefix(text, "# Requested by") {
			continue
		}
		if strings.HasPrefix(text, "#") || strings.TrimSpace(text) == "" {
			b.WriteString(text + "\n")
			continue
		}
		f := strings.Fields(text)
		if len(f) != recordedFields {
			return nil, errRecordedRow
		}
		at, err := time.Parse("2006-01-02T15:04:05.000Z", f[0])
		if err != nil {
			return nil, errRecordedRow
		}
		if at.Before(from) || at.After(to) || at.After(now) {
			continue
		}
		fmt.Fprintf(&b, "%s %4s %6s %s %5s %s\n", f[0], f[1], f[2], f[3], f[6], f[7])
	}
	return b.Bytes(), nil
}

// solarDay is a daily row's leading date in the solar file.
var solarDay = regexp.MustCompile(`^(\d{4} \d{2} \d{2})\s`)

// Solar is the solar file as it stood at now: the rows of the days before
// now's UTC day, the headers kept.
//
// Source: NOAA SWPC, text/daily-solar-indices.txt, as served.
func (r *Recorded) Solar(now time.Time) []byte {
	today := now.UTC().Truncate(24 * time.Hour)
	var b bytes.Buffer
	for line := range bytes.Lines(r.solar) {
		if m := solarDay.FindSubmatch(line); m != nil {
			d, err := time.Parse("2006 01 02", string(m[1]))
			if err != nil || !d.Before(today) {
				continue
			}
		}
		b.Write(line)
	}
	return b.Bytes()
}
