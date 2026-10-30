// Package solar reads NOAA SWPC's daily solar data and takes the
// climatology's F10.7: the mean of the observed fluxes over the 30 days
// before the day (R-9.5, watchpost D-104).
package solar

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"
)

// The bounds a day's flux is held to, in solar flux units (R-3.3; A-2): the
// quiet sun sits near 65, and the largest observed fluxes near 450.
const (
	minFlux, maxFlux = 50.0, 500.0
	maxLines         = 1000    // far past the file's 30 days
	maxBytes         = 1 << 18 // about eighty times the file's few kilobytes, as GloTEC, D-RAP and the scales bound theirs
	clockSkew        = 5 * time.Minute
	// The years a day may lie in: F10.7's record begins in 1947.
	firstYear, lastYear = 1947, 2200
	window              = 30 // days
)

// Daily is one day's observed flux.
type Daily struct {
	Date time.Time
	F107 float64
}

var (
	errNoDays   = errors.New("solar: the file holds no day with a flux: not NOAA's daily solar data")
	errTooLarge = errors.New("solar: the file is larger than NOAA's daily solar data is")
	errLine     = errors.New("solar: a line could not be read: not NOAA's daily solar data")
	errTooLong  = errors.New("solar: the file has more lines than NOAA's daily solar data has")
	errKept     = errors.New("solar: the mean kept is outside a flux's bounds, its days, or after now")
)

// Parse reads the file's dated lines: year, month, day, then the flux. A day
// whose flux is missing (-999) or past its bounds is skipped, and a day
// named twice keeps its first flux, so no day counts twice in a mean; a
// file over maxBytes or maxLines, a line the scanner cannot take, or a file
// with no day at all is an error - never the newest days dropped unsaid.
// No error quotes the input.
//
// Source: NOAA SWPC, text/daily-solar-indices.txt, as served.
func Parse(data []byte) ([]Daily, error) {
	if len(data) == 0 {
		return nil, errNoDays
	}
	if len(data) > maxBytes {
		return nil, errTooLarge
	}
	var out []Daily
	seen := map[time.Time]bool{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	lines := 0
	for ; lines < maxLines && sc.Scan(); lines++ {
		d, ok := parseLine(sc.Text())
		if ok && !seen[d.Date] {
			seen[d.Date] = true
			out = append(out, d)
		}
	}
	if lines == maxLines && sc.Scan() {
		return nil, errTooLong // the lines past the bound would be lost unsaid: the newest days, as the file runs oldest first
	}
	if sc.Err() != nil {
		return nil, errLine // a line past the scanner's buffer: the days after it would be lost unsaid
	}
	if len(out) == 0 {
		return nil, errNoDays
	}
	return out, nil
}

// parseLine reads one dated line; false for any other.
func parseLine(line string) (Daily, bool) {
	f := strings.Fields(line)
	if len(f) < 4 || len(f[0]) != 4 {
		return Daily{}, false
	}
	var n [4]int
	for i := range n {
		v, err := strconv.Atoi(f[i])
		if err != nil {
			return Daily{}, false
		}
		n[i] = v
	}
	if n[0] < firstYear || n[0] > lastYear {
		return Daily{}, false
	}
	date := time.Date(n[0], time.Month(n[1]), n[2], 0, 0, 0, 0, time.UTC)
	real := date.Year() == n[0] && int(date.Month()) == n[1] && date.Day() == n[2]
	if !real {
		return Daily{}, false // 2026-02-30 is no date
	}
	flux := float64(n[3])
	if !(flux >= minFlux && flux <= maxFlux) {
		return Daily{}, false // missing (-999), or past its bounds
	}
	return Daily{Date: date, F107: flux}, true
}

// Mean is the mean flux over the 30 days before a day, and how many days
// had one; false when none did.
//
// Source: watchpost D-104 (the climatology's F10.7 rule).
func Mean(days []Daily, day time.Time) (mean float64, n int, ok bool) {
	end := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -window)
	sum := 0.0
	for _, d := range days {
		if !d.Date.Before(start) && d.Date.Before(end) {
			sum += d.F107
			n++
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return sum / float64(n), n, true
}

// Held is the mean in use: its value, the days behind it, and when it was
// taken; OK once one has been.
type Held struct {
	Mean  float64
	Days  int
	Taken time.Time
	OK    bool
}

// Age is how long ago the held mean was taken; none while no mean is held.
//
// Source: this library.
func (h Held) Age(now time.Time) time.Duration {
	if !h.OK {
		return 0
	}
	return now.Sub(h.Taken)
}

// Resume is a mean kept across a restart, checked: a flux within its
// bounds, 1 to 30 days behind it, taken up to clockSkew after now (a clock
// stepped back a little); nothing kept is no mean, and no error.
//
// Source: watchpost D-249; R-3.3.
func Resume(mean float64, days int, taken, now time.Time) (Held, error) {
	if taken.IsZero() {
		return Held{}, nil
	}
	inBounds := mean >= minFlux && mean <= maxFlux && days >= 1 && days <= window
	if !inBounds || taken.After(now.Add(clockSkew)) {
		return Held{}, errKept
	}
	if taken.After(now) {
		taken = now // a clock stepped back: the mean is as new as can be, never from the future
	}
	return Held{Mean: mean, Days: days, Taken: taken, OK: true}, nil
}

// Hold is the mean to use at now: the file's, when it reads and holds days
// before now; else the mean held before, unchanged, so its age grows and is
// said (R-9.5).
//
// Source: watchpost D-104; this library.
func Hold(prev Held, data []byte, now time.Time) Held {
	days, err := Parse(data)
	if err != nil {
		return prev
	}
	mean, n, ok := Mean(days, now)
	if !ok {
		return prev
	}
	return Held{Mean: mean, Days: n, Taken: now, OK: true}
}
