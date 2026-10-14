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

// Status is a band's state on a path or over an area (R-2.6). Absorbed,
// the regular daytime lower limit, comes with G6.2.
type Status uint8

// The statuses.
const (
	StatusNoData Status = iota // no value where it was asked
	Open                       // below the upper limit
	AboveUpper                 // its lower edge at or past the upper limit
	Disturbed                  // below the upper limit, its lower edge under D-RAP's 1 dB frequency: degraded, not closed (watchpost D-73)
)

// Limit is which limit decided a band's status.
type Limit uint8

// The limits.
const (
	LimitNone Limit = iota
	LimitMUF        // the path's basic MUF (P.533)
	LimitFoF2       // foF2, for the near-vertical area
	LimitDRAP       // D-RAP's 1 dB frequency (R-2.5)
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
// no value there (R-2.1); and its nearest station (R-2.11).
type Reading struct {
	FoF2, MUF3000 float32
	NoData        bool
	Nearest       Nearest
}

// Nearest is how far an answer's place is from the nearest station whose
// readings were assimilated, from the cell it falls in, and whether that
// station's readings are unscored (R-2.11, D-65): the host says its
// confidence, and tells the operator to verify on air where they are
// unscored (watchpost D-181).
type Nearest struct {
	Km       float32
	Unscored bool
	Known    bool // false when no station was assimilated
}

// Answer is each band's status at one place or on one path, and its
// nearest station: a path's, from its control point furthest from any
// (D-65, watchpost D-185).
type Answer struct {
	Bands   []BandStatus
	Nearest Nearest
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
		out[i] = Reading{FoF2: fo, MUF3000: muf, NoData: !okF || !okM, Nearest: s.nearestAt(h.FoF2, p)}
	}
	return out, nil
}

// Bands is each band's status over the near-vertical area around each place,
// within the radius (km), its upper limit foF2 there, and the hours of the
// UTC day each is typically open, from the climatology (R-2.2, R-2.8); and
// the place's nearest station (R-2.11).
//
// Source: watchpost D-24, D-73, D-135, D-185; this library's foF2; the climatology.
func (s Snapshot) Bands(origins []LatLon, radiusKm float64, at time.Time) ([]Answer, error) {
	if !(radiusKm > 0) || radiusKm > 2000 {
		return nil, errBadRadius
	}
	h, err := s.hourFor(origins, at)
	if err != nil {
		return nil, err
	}
	day := s.typicalDay(at)
	out := make([]Answer, len(origins))
	for i, p := range origins {
		fo, ok := cellAt(h.FoF2, p)
		mhz, disturbed := cellAt(s.Disturbance, p)
		bands := disturb(statuses(float64(fo), ok, LimitFoF2), float64(mhz), disturbed)
		out[i] = Answer{Bands: withOpenHours(bands, openHours(day, areaLimit(p))), Nearest: s.nearestAt(h.FoF2, p)}
	}
	return out, nil
}

// Path is each band's status on the path from one place to each other, its
// upper limit the path's basic MUF by ITU-R P.533-14, and the hours of the
// UTC day each is typically open, from the climatology (R-2.2, R-2.7, R-2.8);
// and the nearest station to the control point furthest from any (R-2.11).
//
// Source: ITU-R P.533-14 §3.5 (02-analysis/evidence/p533-equations.md);
// foE from P.1239-4 §4; the gyrofrequency from IGRF-14; watchpost D-135, D-185.
func (s Snapshot) Path(from LatLon, to []LatLon, at time.Time) ([]Answer, error) {
	h, err := s.hourFor(append([]LatLon{from}, to...), at)
	if err != nil {
		return nil, err
	}
	day, sun := s.typicalDay(at), newSunTrack(at)
	out := make([]Answer, len(to))
	for i, p := range to {
		muf, frac, ok := s.pathMUF(hourReader(h), from, p, sun)
		places := controlPlaces(from, p, frac)
		mhz, disturbed := s.pathDisturbance(places)
		bands := disturb(statuses(muf, ok, LimitMUF), mhz, disturbed)
		out[i] = Answer{Bands: withOpenHours(bands, openHours(day, s.pathLimit(from, p))), Nearest: s.pathNearest(h.FoF2, places)}
	}
	return out, nil
}

// pathMUF is one path's basic MUF from what a reader gives, and where its
// control points lie: at the middle, and frac (d₀/2) of the way from each
// end - the middle too on a path of one hop; frac is a half where the
// middle has no value.
func (s Snapshot) pathMUF(read reader, from, to LatLon, sun *sunTrack) (muf, frac float64, ok bool) {
	km := limits.Distance(from.Lat, from.Lon, to.Lat, to.Lon)
	middle, ok := s.controlPoint(read, from, to, 0.5, sun)
	if !ok || !(km > 0) {
		return 0, 0.5, false
	}
	frac = limits.Hops(km, limits.Dmax(middle.FoF2, middle.M3000, middle.FoE))
	frac = 1 / (2 * frac) // d₀/2 of the path, as a part of it
	near, okN := s.controlPoint(read, from, to, frac, sun)
	far, okF := s.controlPoint(read, from, to, 1-frac, sun)
	if !okN || !okF {
		return 0, frac, false
	}
	muf = limits.PathMUF(km, middle, near, far)
	return muf, frac, muf == muf
}

// controlPlaces are where a path's control points lie: frac of the way
// from each end, and the middle.
func controlPlaces(from, to LatLon, frac float64) [3]LatLon {
	var out [3]LatLon
	for i, at := range [...]float64{frac, 0.5, 1 - frac} {
		lat, lon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, at)
		out[i] = LatLon{Lat: lat, Lon: lon}
	}
	return out
}

// pathNearest is a path's nearest station: that of its control point
// furthest from any (D-65); none known when no station was assimilated.
func (s Snapshot) pathNearest(f Field, places [3]LatLon) Nearest {
	var worst Nearest
	for _, p := range places {
		n := s.nearestAt(f, p)
		if n.Known && (!worst.Known || n.Km > worst.Km) {
			worst = n
		}
	}
	return worst
}

// pathDisturbance is D-RAP's highest 1 dB frequency at a path's control
// points (A-9); false when the snapshot holds no D-RAP.
func (s Snapshot) pathDisturbance(places [3]LatLon) (float64, bool) {
	most, held := 0.0, false
	for _, p := range places {
		if v, ok := cellAt(s.Disturbance, p); ok {
			most, held = max(most, float64(v)), true
		}
	}
	return most, held
}

// disturb marks each band open under its upper limit whose lower edge is
// under D-RAP's 1 dB frequency as disturbed, named as D-RAP's (R-2.5).
func disturb(bands []BandStatus, mhz float64, held bool) []BandStatus {
	if !held {
		return bands
	}
	for i, b := range bands {
		if b.Status == Open && b.Band.LowMHz() < mhz {
			bands[i] = BandStatus{Band: b.Band, Status: Disturbed, Limit: LimitDRAP, LimitMHz: float32(mhz)}
		}
	}
	return bands
}

// nearestAt is a place's nearest station, from the cell of f it falls in;
// none known when the snapshot holds no distances on f's grid.
func (s Snapshot) nearestAt(f Field, p LatLon) Nearest {
	i, ok := cellIndex(f, p)
	held := len(s.NearestKm) == len(f.Values)
	if !ok || !held {
		return Nearest{}
	}
	return Nearest{Km: s.NearestKm[i], Unscored: s.NearestUnscored.Has(i), Known: true}
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
	i, ok := cellIndex(f, p)
	if !ok || f.NoData.Has(i) {
		return 0, false
	}
	return f.Values[i], true
}

// cellIndex is the cell of a field a place falls in; false where the field
// has no cells.
func cellIndex(f Field, p LatLon) (int, bool) {
	if f.Cols <= 0 || f.Rows <= 0 || len(f.Values) != f.Cols*f.Rows {
		return 0, false
	}
	lon := math.Mod(math.Mod(p.Lon+180, 360)+360, 360) - 180
	col := int((lon - f.West) / (f.East - f.West) * float64(f.Cols))
	row := int((f.North - p.Lat) / (f.North - f.South) * float64(f.Rows))
	col, row = min(max(col, 0), f.Cols-1), min(max(row, 0), f.Rows-1)
	return row*f.Cols + col, true
}
