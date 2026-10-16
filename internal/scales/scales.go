// Package scales reads NOAA SWPC's space-weather scales (plan task G2.5,
// R-2.10, watchpost D-88): the R, S and G levels now and the outlook for
// today and the next two days. Every level is checked from 0 to 5 and every
// probability from 0 to 100; a level NOAA leaves null is unset, never 0; a
// changed layout is an error, never a level (R-3.1, R-3.3). No error quotes
// the input (R-3.4).
package scales

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

// None is a level or probability the file leaves null: unset, never 0.
const None = -1

// maxBytes bounds a file: about sixty times the measured 1.1 KB.
const maxBytes = 64 << 10

// clockSkew is how far a file's time may lie past the host's clock.
const clockSkew = 5 * time.Minute

// The scales' bounds: NOAA's levels run 0 to 5, its probabilities are
// percentages.
const (
	topLevel = 5
	topPct   = 100
)

// outlookDays are the outlook's keys in the file, today first.
var outlookDays = []string{"1", "2", "3"}

// Levels are the scales now, the outlook, and the file's time.
type Levels struct {
	Valid   time.Time
	R, S, G int // None where the file gives no level
	Days    []Day
}

// Day is one day of the outlook: R's minor and major probabilities, S's,
// and the G level predicted; None where the file gives none.
type Day struct {
	Day                        time.Time
	RMinorPct, RMajorPct, SPct int
	G                          int
}

// The errors: each says what was wrong, never what the file held.
var (
	errTooLarge = errors.New("scales: the file is larger than any scales file")
	errLayout   = errors.New("scales: the file is not NOAA's scales layout")
	errNoTime   = errors.New("scales: the file has no time")
	errFuture   = errors.New("scales: the file's time is in the future")
	errRange    = errors.New("scales: a level or probability is outside its bounds or no number")
)

// entry is one day of the file as written: each value a quoted number or
// null.
type entry struct {
	DateStamp, TimeStamp string
	R                    struct{ Scale, MinorProb, MajorProb *string }
	S                    struct{ Scale, Prob *string }
	G                    struct{ Scale *string }
}

// Read reads a scales file: its current day, timed no later than the
// host's clock allows, and the outlook's days present, each value checked.
//
// Source: NOAA SWPC, products/noaa-scales.json (public domain); its layout
// read from the file on 2026-10-16.
func Read(body []byte, now time.Time) (Levels, error) {
	if len(body) > maxBytes {
		return Levels{}, errTooLarge
	}
	var days map[string]entry
	if err := json.Unmarshal(body, &days); err != nil {
		return Levels{}, errLayout
	}
	cur, ok := days["0"]
	if !ok {
		return Levels{}, errLayout
	}
	valid, err := time.Parse(time.DateTime, cur.DateStamp+" "+cur.TimeStamp)
	if err != nil {
		return Levels{}, errNoTime
	}
	future := valid.After(now.Add(clockSkew))
	if future {
		return Levels{}, errFuture
	}
	l := Levels{Valid: valid}
	l.R, l.S, l.G, err = levels(cur)
	if err != nil {
		return Levels{}, err
	}
	for _, k := range outlookDays {
		e, ok := days[k]
		if !ok {
			continue
		}
		d, err := outlook(e)
		if err != nil {
			return Levels{}, err
		}
		l.Days = append(l.Days, d)
	}
	return l, nil
}

// levels are a day's three levels.
func levels(e entry) (r, s, g int, err error) {
	r, err = value(e.R.Scale, topLevel)
	if err != nil {
		return 0, 0, 0, err
	}
	s, err = value(e.S.Scale, topLevel)
	if err != nil {
		return 0, 0, 0, err
	}
	g, err = value(e.G.Scale, topLevel)
	return r, s, g, err
}

// outlook is an outlook day: its date, probabilities and G level.
func outlook(e entry) (Day, error) {
	date, err := time.Parse(time.DateOnly, e.DateStamp)
	if err != nil {
		return Day{}, errNoTime
	}
	d := Day{Day: date}
	for _, v := range []struct {
		raw *string
		top int
		to  *int
	}{{e.R.MinorProb, topPct, &d.RMinorPct}, {e.R.MajorProb, topPct, &d.RMajorPct}, {e.S.Prob, topPct, &d.SPct}, {e.G.Scale, topLevel, &d.G}} {
		*v.to, err = value(v.raw, v.top)
		if err != nil {
			return Day{}, err
		}
	}
	return d, nil
}

// value is a quoted whole number from 0 to top, or None for null.
func value(raw *string, top int) (int, error) {
	if raw == nil {
		return None, nil
	}
	n, err := strconv.Atoi(*raw)
	if err != nil || n < 0 || n > top {
		return 0, errRange
	}
	return n, nil
}
