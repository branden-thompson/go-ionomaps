// Package giro reads GIRO's FastChar replies (plan task G2.1): the header,
// which carries the requester's address, dropped before any row is read
// (R-3.4); every value held to physical bounds and every refusal counted
// (R-3.3); a reading taken by its confidence score (R-9.4, D-61). No error
// quotes the input, and nothing returned holds the reply's text.
package giro

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

// The physical bounds a reading is held to (R-3.3): foF2 and MUF(3000) in
// MHz, M(3000)F2 a ratio - the bounds PLAN's week was scored inside.
const (
	minFoF2, maxFoF2   = 0.5, 20.0
	minM3000, maxM3000 = 1.5, 5.0
)

// The confidence scores (R-9.4, D-61): taken at minScore or more, manual,
// or unknown; anything else under minScore dropped.
const (
	minScore     = 70
	manualScore  = 999
	unknownScore = -1
)

// maxRows bounds the readings a reply gives (R-3.3): a week of rows a
// minute; the rest are counted as rejected.
const maxRows = 7 * 24 * 60

// maxBytes bounds a reply: GIRO's reply bound; a station's week of
// five-minute rows is about 170 KB.
const maxBytes = terms.GIROReplyBytes

// Station is where a reply's station is: its code, and its place in
// degrees, longitude from -180 to 180.
type Station struct {
	Code     string
	Lat, Lon float64
}

// Reading is one sounding: its time, its confidence score, foF2 and, where
// GIRO gives it, M(3000)F2 and MUF(3000), their product.
type Reading struct {
	At             time.Time
	Score          int
	Unscored       bool // the score is unknown (-1): taken, and said (D-61)
	FoF2           float32
	MUF3000, M3000 float32
	HasM300        bool
}

// Reply is a station's reply, read: where it is, the readings taken, how
// many rows were refused as malformed or out of bounds, how many were
// dropped for their confidence, and how many GIRO gave no foF2 for.
type Reply struct {
	Station       Station
	Readings      []Reading
	Rejected      int
	LowConfidence int
	Missing       int
}

// The errors: each says what was wrong, never what the reply held.
var (
	ErrBadCode   = errors.New("giro: a station code is five capital letters or digits")
	errTooLarge  = errors.New("giro: the reply is larger than any FastChar reply")
	errNoPlace   = errors.New("giro: the reply has no station location line")
	errOtherCode = errors.New("giro: the reply is for another station than the one asked")
	errWindow    = errors.New("giro: the window asked ends before it starts, or starts after the clock")
)

// fastChar is GIRO's FastChar service (D-39 governs every ask of it).
const fastChar = "https://lgdc.uml.edu/fastchar/getbest"

// askTime is how FastChar takes a time: UTC, a space before the clock.
const askTime = "2006/01/02 15:04:05"

// AskURL is the ask for a station's readings in a window: foF2 and
// M(3000)F2 alone (R-5.2, D-62), the code reaching the address only through
// url.Values (R-3.3).
//
// Source: GIRO's FastChar service, as asked on 2026-10-14 (watchpost D-182).
func AskURL(code string, from, to time.Time) (string, error) {
	codeOK := CodeOK(code)
	if !codeOK {
		return "", ErrBadCode
	}
	backwards := to.Before(from)
	if backwards {
		return "", errWindow
	}
	q := url.Values{"ursiCode": {code}, "charName": {"foF2,M(D)"}, "DMUF": {"3000"},
		"fromDate": {from.UTC().Format(askTime)}, "toDate": {to.UTC().Format(askTime)}}
	return fastChar + "?" + q.Encode(), nil
}

// codeRE is a station's URSI code (R-3.3).
var codeRE = regexp.MustCompile(`^[A-Z0-9]{5}$`)

// CodeOK reports whether a station code is five capital letters or digits.
//
// Source: URSI's station codes as GIRO publishes them (R-3.3).
func CodeOK(code string) bool { return codeRE.MatchString(code) }

// locationRE reads the one header line the parser keeps anything from: the
// station's place and code, and nothing else of it.
var locationRE = regexp.MustCompile(`^# Location: GEO \(\s*(\d{1,2}(?:\.\d+)?)\s*([NS])\s+(\d{1,3}(?:\.\d+)?)\s*E\s*\),\s*URSI-Code\s+([A-Z0-9]{5})\b`)

// ParseReply reads a FastChar reply for a station, the readings kept to
// the window asked and never after now.
//
// Source: GIRO's FastChar "Tabulated Rapid Access Ionospheric
// Characteristics" text, version 1.1, as PLAN's week received it.
func ParseReply(body []byte, code string, from, to, now time.Time) (Reply, error) {
	codeOK, size := CodeOK(code), len(body)
	backwards, early := to.Before(from), from.After(now)
	if !codeOK {
		return Reply{}, ErrBadCode
	}
	if size > maxBytes {
		return Reply{}, errTooLarge
	}
	if backwards || early {
		return Reply{}, errWindow
	}
	var r Reply
	located := false
	for line := range bytes.Lines(body) {
		text := strings.TrimRight(string(line), "\r\n")
		if strings.HasPrefix(text, "#") { // a header line: only the location is read, the rest never kept (R-3.4)
			if m := locationRE.FindStringSubmatch(text); m != nil {
				if m[4] != code {
					return Reply{}, errOtherCode
				}
				st, ok := place(m[1], m[2], m[3])
				if !ok {
					return Reply{}, errNoPlace
				}
				st.Code = code
				r.Station, located = st, true
			}
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		r.take(text, from, to, now)
	}
	if !located {
		return Reply{}, errNoPlace
	}
	return r, nil
}

// place is a location line's latitude and longitude, the longitude from
// 0-360°E to -180-180; false off the globe.
func place(lat, ns, lon string) (Station, bool) {
	la, errA := strconv.ParseFloat(lat, 64)
	lo, errO := strconv.ParseFloat(lon, 64)
	if errA != nil || errO != nil || la > 90 || lo > 360 {
		return Station{}, false
	}
	if ns == "S" {
		la = -la
	}
	if lo > 180 {
		lo -= 360
	}
	return Station{Lat: la, Lon: lo}, true
}

// take reads one row into the reply: kept, dropped for its confidence, or
// refused and counted.
func (r *Reply) take(text string, from, to, now time.Time) {
	if f := strings.Fields(text); len(f) == rowFields && f[2] == "---" {
		r.Missing++ // a sounding GIRO could not scale: no foF2, nothing to assimilate
		return
	}
	held := len(r.Readings)
	if held >= maxRows {
		r.Rejected++ // past the cap: counted, never kept (R-3.3)
		return
	}
	x, ok := readRow(text)
	outside := x.At.Before(from) || x.At.After(to) || x.At.After(now)
	if !ok || outside {
		r.Rejected++
		return
	}
	switch {
	case x.Score >= minScore && x.Score <= 100, x.Score == manualScore, x.Score == unknownScore:
		x.Unscored = x.Score == unknownScore
		r.Readings = append(r.Readings, x)
	case x.Score >= 0 && x.Score < minScore:
		r.LowConfidence++
	default:
		r.Rejected++ // a score that is no score
	}
}

// rowFields is a row's fields as the ask (askURL) has GIRO write them: the
// time, the score, foF2 and its qualifier, M(3000)F2 and its (D-62).
const rowFields = 6

// readRow is a row's reading; false for another shape, or a value out of
// its bounds.
func readRow(text string) (Reading, bool) {
	f := strings.Fields(text)
	fields := len(f)
	if fields != rowFields {
		return Reading{}, false
	}
	moved := len(f[3]) != 2 || len(f[5]) != 2
	if moved {
		return Reading{}, false // the qualifier columns moved: a changed format, never read across (R-3.1)
	}
	at, err := time.Parse("2006-01-02T15:04:05.000Z", f[0])
	score, errS := strconv.Atoi(f[1])
	fo, okF := value(f[2], minFoF2, maxFoF2)
	if err != nil || errS != nil || !okF {
		return Reading{}, false
	}
	x := Reading{At: at, Score: score, FoF2: fo}
	if f[5] != "__" && f[4] != "---" {
		m, ok := value(f[4], minM3000, maxM3000)
		if !ok {
			return Reading{}, false
		}
		x.M3000, x.MUF3000, x.HasM300 = m, fo*m, true
	}
	return x, true
}

// value is a number held to its bounds; false for no number, or one out of
// them.
func value(s string, lo, hi float64) (float32, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if !(v >= lo && v <= hi) { // NaN fails both
		return 0, false
	}
	return float32(v), true
}
