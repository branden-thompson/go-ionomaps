package ionomaps

import (
	"math"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/scales"
)

// A Snapshot is one hour's fields and what they were made from, and answers
// for that hour. It is immutable: every update returns fresh slices (P-6).
// An update's snapshot is now; Ahead gives an hour ahead as a snapshot of
// its own, typical (R-1.3, watchpost D-109).
type Snapshot struct {
	Computed        time.Time    // the age is the host's clock minus this
	Now             Hour         // the hour it answers for: now, or from Ahead an hour ahead
	NearestKm       []float32    // per cell: distance to the nearest assimilated station (R-2.11); measured within 500 km (R-3.2)
	NearestUnscored Bitset       // per cell: whether that nearest station's readings are unscored - verify on air (D-61)
	Background      Backgrounds  // per field (watchpost D-101)
	Inputs          Inputs       // what fed it, and when (R-3.3, R-9.5, R-9.7)
	Reason          Reason       // why this is not from new data (R-5.4)
	Scales          Scales       // NOAA's space-weather levels (watchpost D-88)
	Offset          Offset       // stations against GloTEC (watchpost D-105)
	Disturbance     Field        // D-RAP's highest frequency absorbed by 1 dB, MHz a cell (R-2.5); no cells without D-RAP
	Noise           Noise        // the man-made noise the reference circuit hears, as the update asked (D-187)
	TypicalError    TypicalError // the hours ahead's error, with its basis (watchpost D-109)

	typical *typicalDays // the library's typical days, which the answers and the hours ahead share (P-4, R-8.1); none for a snapshot made by hand
}

// Hour is one hour's fields, for the moment At.
type Hour struct {
	At      time.Time
	MUF3000 Field
	FoF2    Field
	Typical bool // an hour ahead: the climatology's, typical, never a forecast (R-1.3)
}

// Field is a value a cell over a box: rows from the north, each west to east,
// Cols times Rows of them, every one finite (R-3.3). A cell with no known
// value is marked in NoData; its value is then not to be read.
type Field struct {
	West, South, East, North float64
	Cols, Rows               int
	Values                   []float32
	NoData                   Bitset
}

// WithNoData is a copy of the field with the cells given marked as having
// no data; a cell outside the field is passed over. The field itself is
// unchanged: a host's tests mark cells this way.
//
// Source: this library.
func (f Field) WithNoData(cells ...int) Field {
	c := f.owned()
	for _, i := range cells {
		c.NoData.set(i)
	}
	return c
}

// Bitset is a set of cells by index.
type Bitset struct {
	bits []uint64
	n    int
}

func newBitset(n int) Bitset { return Bitset{bits: make([]uint64, (n+63)/64), n: n} }

// Len is how many cells the set covers.
//
// Source: this library.
func (b Bitset) Len() int { return b.n }

// Has reports whether cell i is in the set; a cell outside it never is.
//
// Source: this library.
func (b Bitset) Has(i int) bool {
	if i < 0 || i >= b.n {
		return false
	}
	return b.bits[i/64]&(1<<(i%64)) != 0
}

// set puts cell i in the set; a cell outside it is passed over.
func (b Bitset) set(i int) {
	if i < 0 || i >= b.n {
		return
	}
	b.bits[i/64] |= 1 << (i % 64)
}

// Background is what a field's background was made from (watchpost D-101).
type Background uint8

// The backgrounds.
const (
	NoBackground Background = iota // nothing yet
	GloTEC                         // NOAA SWPC's GloTEC, foF2 from NmF2
	Climatology                    // the climatology, the PyIRI method
)

// String names a background.
//
// Source: this library.
func (b Background) String() string {
	switch b {
	case GloTEC:
		return "GloTEC"
	case Climatology:
		return "climatology"
	}
	return "none"
}

// Backgrounds are each field's background: foF2 over GloTEC, else the
// climatology; M(3000)F2 over the climatology (watchpost D-101).
type Backgrounds struct{ FoF2, M3000 Background }

// Inputs are what fed a snapshot, and when (R-3.3, R-9.5, R-9.7).
type Inputs struct {
	Stations, Rejected int           // readings used, and the last burst's rows refused: past range checks, or from a station placed over 50 km from where it is (watchpost D-248)
	Implausible        int           // readings refused whole this build, their foF2 past its residual's bound against the background (watchpost D-248)
	ImplausibleM       int           // readings whose M(3000)F2 alone was refused this build, past its bound; their foF2 was used
	GloTECValid        time.Time     // the grid's own valid time; zero when GloTEC was missing
	OldestReading      time.Time     // the oldest GIRO reading assimilated
	DRAPValid          time.Time     // zero when D-RAP was missing: no "disturbed" state
	F107Mean           float32       // the 30-day mean used (watchpost D-104)
	F107Days           int           // observed days in it
	F107Age            time.Duration // since the solar file was last fetched
	CoordsExtrapolated bool          // past IGRF-14's last year
	NoStations         bool          // the host asked for no stations (UpdateOptions.NoStations): GIRO not asked, no reading on the field
	GloTECFilled       int           // cells GloTEC gave no value for, filled from the climatology, when GloTEC is the background
	GIRO               GIROOutcome   // how the last burst ended, carried until the next (watchpost D-247)
	StationsFailed     int           // stations whose own ask failed in the last burst - a server error, a 404, no answer - the burst going on
	Unread             []string      // NOAA's files this update asked for and did not read - unanswered, refused or unreadable: "GloTEC", "solar", "D-RAP", "scales" - each the one held used instead where one is, its age saying so; Background says what foF2 was drawn from
}

// GIROOutcome is how GIRO's last burst ended (watchpost D-247), carried in
// every snapshot until the next burst, so the hour between says it too.
type GIROOutcome uint8

// The outcomes.
const (
	GIROAnswered    GIROOutcome = iota // the burst ran, or none has been made; Inputs.StationsFailed counts stations that failed one by one
	GIROUnavailable                    // GIRO refused (a 429), or three stations failed in a row: GIRO is backing off (watchpost D-39)
	GIRODenied                         // GIRO refused this host - a 401, 403 or 410 - and is left alone for a day
)

// String names an outcome.
//
// Source: watchpost D-247.
func (g GIROOutcome) String() string {
	switch g {
	case GIROUnavailable:
		return "unavailable"
	case GIRODenied:
		return "denied"
	}
	return "answered"
}

// Reason is why a snapshot is not from new data (R-5.4).
type Reason uint8

// The reasons.
const (
	Fresh        Reason = iota // from new data
	TooSoon                    // asked within ten minutes of NOAA's last ask, or while it backs off: the last snapshot, nothing asked
	GIROPaused                 // GIRO is backing off, or left alone after a denial: Inputs.GIRO says which
	Offline                    // no NOAA file came back and GIRO was not due: built from what is held; with ErrNoField, there is nothing yet to build from
	StationsNext               // NOAA's fields, GIRO due: update again at once for the stations (D-63)
	Unfinished                 // the update stopped before every file arrived - its deadline passed, or the host cancelled it - so it is built from what is held; NOAA may be asked again at once (L-F4)
	Partial                    // some of NOAA's files did not come back - Inputs.Unread names each - and what is held, or the climatology, stands in for them; the rest is new (watchpost D-239)
)

// TypicalError is the hours ahead's error by distance to the nearest
// reporting station, and where and when it was measured (watchpost D-109,
// D-134): Basis in words a listener can be told, Source the citation for a
// developer (watchpost AX-3).
type TypicalError struct {
	Bands         []BandError
	Basis, Source string
}

// BandError is the typical error up to a distance: 500, 1000, 2000 km, then
// no limit.
type BandError struct {
	UpToKm              float32
	FoF2MHz, MUF3000MHz float32
}

// Scales are NOAA's space-weather levels now and their outlook - today and
// the next two days - as published; Valid is zero when the feed is missing,
// and a level or probability NOAA leaves null is ScaleUnset: neither is
// ever read as level 0 (watchpost D-88).
type Scales struct {
	R, S, G int // 0 to 5, or ScaleUnset
	Outlook []DayScale
	Valid   time.Time
}

// ScaleUnset is a level or probability NOAA's file gives none for: -1,
// never read as level 0 (watchpost D-88).
const ScaleUnset = scales.None

// DayScale is one day of NOAA's outlook: R's minor and major probabilities
// and S's, in percent, and the G level; each ScaleUnset where none is given.
type DayScale struct {
	Day                        time.Time
	RMinorPct, RMajorPct, SPct int
	G                          int
}

// Offset is the stations' foF2 against GloTEC's (watchpost D-105, R-9.6):
// the live offset, this update's, when measured; the typical offset and its
// spread (one standard deviation), seeded from PLAN's week and learned from
// this library's own updates; whether the no-readings correction moved
// GloTEC's foF2 by the typical offset; and where the seed was measured.
type Offset struct {
	LiveMHz, TypicalMHz, SpreadMHz float32
	Measured                       bool   // a live offset: GloTEC the background, five or more readings assimilated
	Corrected                      bool   // the no-readings correction is in use
	Basis                          string // where the seed was measured, in words a listener can be told
	Source                         string // the seed's citation, for a developer
}

// Apart is whether the live offset is more than two spreads from the
// typical (watchpost D-105 d): the host's status says so; false when no live
// offset is measured.
//
// Source: watchpost D-105; go-ionomaps 02-analysis/evidence/offset.py.
func (o Offset) Apart() bool {
	if !o.Measured {
		return false
	}
	return math.Abs(float64(o.LiveMHz-o.TypicalMHz)) > 2*float64(o.SpreadMHz)
}
