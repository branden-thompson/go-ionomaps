package ionomaps

import (
	"errors"
	"math"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// LatLon is a place, degrees north and east.
type LatLon struct{ Lat, Lon float64 }

// Band is an amateur HF band, 160 metres to 10.
type Band uint8

// The bands, lowest first.
const (
	Band160m Band = iota + 1
	Band80m
	Band60m
	Band40m
	Band30m
	Band20m
	Band17m
	Band15m
	Band12m
	Band10m
)

// bandEdges are each band's lower edge, MHz, by Band.
var bandEdges = [...]float64{Band160m: 1.8, Band80m: 3.5, Band60m: 5.3, Band40m: 7, Band30m: 10.1, Band20m: 14, Band17m: 18.068,
	Band15m: 21, Band12m: 24.89, Band10m: 28}

// LowMHz is a band's lower edge, MHz; 0 for a value that is no band.
//
// Source: the amateur HF allocations (watchpost D-144).
func (b Band) LowMHz() float64 {
	if b < Band160m || b > Band10m {
		return 0
	}
	return bandEdges[b]
}

// AllBands are the bands, lowest first.
//
// Source: this library.
func AllBands() []Band {
	return []Band{Band160m, Band80m, Band60m, Band40m, Band30m, Band20m, Band17m, Band15m, Band12m, Band10m}
}

// Status is a band's state on a path or over an area (R-2.6). The lower
// limit's states - absorbed, disturbed - come with UAT-3 (watchpost D-175).
type Status uint8

// The statuses.
const (
	StatusNoData Status = iota // no value where it was asked
	Open                       // below the upper limit
	AboveUpper                 // its lower edge at or past the upper limit
)

// Limit is which limit decided a band's status.
type Limit uint8

// The limits.
const (
	LimitNone Limit = iota
	LimitMUF        // the path's basic MUF (P.533)
	LimitFoF2       // foF2, for the near-vertical area
)

// BandStatus is one band's answer: its status and the limit that decided
// it, and the spans of the UTC day it is typically open (R-2.8).
type BandStatus struct {
	Band      Band
	Status    Status
	Limit     Limit
	LimitMHz  float32
	OpenHours []Span
}

// Reading is foF2 and MUF(3000) at a place, MHz; NoData where the field has
// no value there (R-2.1).
type Reading struct {
	FoF2, MUF3000 float32
	NoData        bool
}

// The answers' refusals (R-3.5): each says what was wrong, never what to do
// with the library's own data.
var (
	errTooManyPlaces = errors.New("ionomaps: more than 64 places in one call")
	errBadPlace      = errors.New("ionomaps: a place is off the globe or no number")
	errBadRadius     = errors.New("ionomaps: the radius is not above 0 and at most 2000 km")
	errNoHour        = errors.New("ionomaps: the snapshot holds no hour for that moment")
)

// maxPlaces is the most places one call answers (R-3.5).
const maxPlaces = 64

// At is foF2 and MUF(3000) at each place, in the hour that holds the moment
// (R-2.1, R-2.3).
//
// Source: this library's fields.
func (s Snapshot) At(points []LatLon, at time.Time) ([]Reading, error) {
	h, err := s.hourFor(points, at)
	if err != nil {
		return nil, err
	}
	out := make([]Reading, len(points))
	for i, p := range points {
		fo, okF := cellAt(h.FoF2, p)
		muf, okM := cellAt(h.MUF3000, p)
		out[i] = Reading{FoF2: fo, MUF3000: muf, NoData: !okF || !okM}
	}
	return out, nil
}

// Bands is each band's status over the near-vertical area around each place,
// within the radius (km), its upper limit foF2 there, and the hours of the
// UTC day each is typically open, from the climatology (R-2.2, R-2.8).
//
// Source: watchpost D-24, D-73, D-135; this library's foF2; the climatology.
func (s Snapshot) Bands(origins []LatLon, radiusKm float64, at time.Time) ([][]BandStatus, error) {
	if !(radiusKm > 0) || radiusKm > 2000 {
		return nil, errBadRadius
	}
	h, err := s.hourFor(origins, at)
	if err != nil {
		return nil, err
	}
	day := s.typicalDay(at)
	out := make([][]BandStatus, len(origins))
	for i, p := range origins {
		fo, ok := cellAt(h.FoF2, p)
		out[i] = withOpenHours(statuses(float64(fo), ok, LimitFoF2), openHours(day, areaLimit(p)))
	}
	return out, nil
}

// Path is each band's status on the path from one place to each other, its
// upper limit the path's basic MUF by ITU-R P.533-14, and the hours of the
// UTC day each is typically open, from the climatology (R-2.2, R-2.7, R-2.8).
//
// Source: ITU-R P.533-14 §3.5 (02-analysis/evidence/p533-equations.md);
// foE from P.1239-4 §4; the gyrofrequency from IGRF-14; watchpost D-135.
func (s Snapshot) Path(from LatLon, to []LatLon, at time.Time) ([][]BandStatus, error) {
	h, err := s.hourFor(append([]LatLon{from}, to...), at)
	if err != nil {
		return nil, err
	}
	day, sun := s.typicalDay(at), newSunTrack(at)
	out := make([][]BandStatus, len(to))
	for i, p := range to {
		muf, ok := s.pathMUF(hourReader(h), from, p, sun)
		out[i] = withOpenHours(statuses(muf, ok, LimitMUF), openHours(day, s.pathLimit(from, p)))
	}
	return out, nil
}

// pathMUF is one path's basic MUF from what a reader gives: control points
// at the middle and d₀/2 from each end.
func (s Snapshot) pathMUF(read reader, from, to LatLon, sun *sunTrack) (float64, bool) {
	km := limits.Distance(from.Lat, from.Lon, to.Lat, to.Lon)
	middle, ok := s.controlPoint(read, from, to, 0.5, sun)
	if !ok || !(km > 0) {
		return 0, false
	}
	frac := limits.Hops(km, limits.Dmax(middle.FoF2, middle.M3000, middle.FoE))
	frac = 1 / (2 * frac) // d₀/2 of the path, as a part of it
	near, okN := s.controlPoint(read, from, to, frac, sun)
	far, okF := s.controlPoint(read, from, to, 1-frac, sun)
	if !okN || !okF {
		return 0, false
	}
	muf := limits.PathMUF(km, middle, near, far)
	return muf, muf == muf
}

// controlPoint is what the basic MUF reads at a part of the way along a
// path: foF2 and M(3000)F2 from the reader, foE from the sun there and the
// snapshot's F10.7 mean, the gyrofrequency at 300 km.
func (s Snapshot) controlPoint(read reader, from, to LatLon, frac float64, sun *sunTrack) (limits.Point, bool) {
	lat, lon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, frac)
	return s.pointAt(read, LatLon{Lat: lat, Lon: lon}, sun)
}

// sunSteps is how many ten-minute steps back a sun track holds: a day.
const sunSteps = 24 * 6

// sunTrack is where the sun stood at a moment and at every ten minutes of
// the day before it, the subsolar latitude as its sine and cosine: worked out
// once a moment, where every control point reads it.
type sunTrack struct {
	at            time.Time
	decl          float64
	sin, cos, lon [sunSteps + 1]float64
}

// newSunTrack is the sun's track up to a moment.
func newSunTrack(at time.Time) *sunTrack {
	t := &sunTrack{at: at}
	r := math.Pi / 180
	for i := range sunSteps + 1 {
		slon, slat := magcoords.Subsolar(at.Add(-time.Duration(i) * 10 * time.Minute))
		t.sin[i], t.cos[i], t.lon[i] = math.Sin(slat*r), math.Cos(slat*r), slon
	}
	t.decl = math.Asin(t.sin[0]) / r
	return t
}

// sunAt is the sun's declination and zenith angle at a place, degrees, and
// at night the hours since it set (to ten minutes, within a day).
func (t *sunTrack) sunAt(lat, lon float64) (decl, chi, sinceSunset float64) {
	r := math.Pi / 180
	sl, cl := math.Sincos(lat * r)
	cosZenith := func(i int) float64 { return sl*t.sin[i] + cl*t.cos[i]*math.Cos((lon-t.lon[i])*r) }
	chi = math.Acos(math.Max(-1, math.Min(1, cosZenith(0)))) / r
	if chi < 90 {
		return t.decl, chi, 0
	}
	for i := 1; i <= sunSteps; i++ { // a day back, ten minutes at a time (P10-02)
		if cosZenith(i) > 0 {
			return t.decl, chi, float64(i) / 6
		}
	}
	return t.decl, chi, 24 // a polar night: equation (17e) decides
}

// statuses are each band's status under an upper limit, MHz.
func statuses(limitMHz float64, ok bool, by Limit) []BandStatus {
	out := make([]BandStatus, 0, len(bandEdges)-1)
	for _, b := range AllBands() {
		st := BandStatus{Band: b, Status: StatusNoData}
		if ok {
			st = BandStatus{Band: b, Status: AboveUpper, Limit: by, LimitMHz: float32(limitMHz)}
			if b.LowMHz() < limitMHz {
				st.Status = Open
			}
		}
		out = append(out, st)
	}
	return out
}

// hourFor is the hour that holds a moment, the places checked (R-3.5).
func (s Snapshot) hourFor(places []LatLon, at time.Time) (Hour, error) {
	if len(places) > maxPlaces {
		return Hour{}, errTooManyPlaces
	}
	for _, p := range places {
		finite := p.Lat == p.Lat && p.Lon == p.Lon && p.Lon >= -540 && p.Lon <= 540
		if !finite || p.Lat < -90 || p.Lat > 90 {
			return Hour{}, errBadPlace
		}
	}
	for _, h := range s.Hours {
		if !at.Before(h.At) && at.Before(h.At.Add(time.Hour)) {
			return h, nil
		}
	}
	return Hour{}, errNoHour
}

// cellAt is a field's value at a place, from the cell it falls in; false
// where the field has no cells or no value there.
func cellAt(f Field, p LatLon) (float32, bool) {
	if f.Cols <= 0 || f.Rows <= 0 || len(f.Values) != f.Cols*f.Rows {
		return 0, false
	}
	lon := math.Mod(math.Mod(p.Lon+180, 360)+360, 360) - 180
	col := int((lon - f.West) / (f.East - f.West) * float64(f.Cols))
	row := int((f.North - p.Lat) / (f.North - f.South) * float64(f.Rows))
	col, row = min(max(col, 0), f.Cols-1), min(max(row, 0), f.Rows-1)
	i := row*f.Cols + col
	if f.NoData.Has(i) {
		return 0, false
	}
	return f.Values[i], true
}
