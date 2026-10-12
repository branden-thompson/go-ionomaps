package ionomaps

import (
	"math"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/climatology"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// Span is a stretch of time, From up to To.
type Span struct{ From, To time.Time }

// typicalHour is the climatology for one hour of the day at the snapshot's
// F10.7 mean, and the subsolar point's quasi-dipole longitude then.
type typicalHour struct {
	at    time.Time
	hour  climatology.Hour
	subQD float64
	sun   *sunTrack
}

// typicalDay is the climatology for each hour of the UTC day holding a
// moment (watchpost D-135).
func (s Snapshot) typicalDay(at time.Time) [24]typicalHour {
	var day [24]typicalHour
	start := at.UTC().Truncate(24 * time.Hour)
	for h := range day {
		t := start.Add(time.Duration(h) * time.Hour)
		day[h] = typicalHour{at: t, hour: climatology.NewHour(climatology.Refits(), t, float64(s.Inputs.F107Mean)), subQD: magcoords.SubsolarQD(t), sun: newSunTrack(t)}
	}
	return day
}

// reader is foF2 and M(3000)F2 at a place in one hour; false where there is
// no value.
type reader func(p LatLon) (foF2, m3000 float64, ok bool)

// hourReader reads the snapshot's fields for an hour.
func hourReader(h Hour) reader {
	return func(p LatLon) (float64, float64, bool) {
		fo, okF := cellAt(h.FoF2, p)
		muf, okM := cellAt(h.MUF3000, p)
		if !okF || !okM || !(fo > 0) {
			return 0, 0, false
		}
		return float64(fo), float64(muf / fo), true
	}
}

// read is the climatology at a place in this hour, at its quasi-dipole
// latitude and magnetic local time.
func (t typicalHour) read(p LatLon) (float64, float64, bool) {
	qdlat, qdlon, _ := magcoords.QD(p.Lat, p.Lon, t.at)
	fo, m3 := t.hour.At(qdlat, magcoords.MLTFrom(qdlon, t.subQD))
	return fo, m3, fo > 0 && m3 > 0
}

// openHours are the spans of the day each band is open under the hour-by-hour
// upper limits, MHz (NaN where there is none), by band in AllBands order.
func openHours(day [24]typicalHour, limit func(typicalHour) float64) [][]Span {
	var limits [24]float64
	for h := range day {
		limits[h] = limit(day[h])
	}
	out := make([][]Span, 0, len(bandEdges)-1)
	for _, b := range AllBands() {
		var spans []Span
		for h := range day {
			if !(b.LowMHz() < limits[h]) {
				continue
			}
			end := day[h].at.Add(time.Hour)
			if n := len(spans); n > 0 && spans[n-1].To.Equal(day[h].at) {
				spans[n-1].To = end
				continue
			}
			spans = append(spans, Span{From: day[h].at, To: end})
		}
		out = append(out, spans)
	}
	return out
}

// withOpenHours sets each band's open hours.
func withOpenHours(bands []BandStatus, open [][]Span) []BandStatus {
	for i := range bands {
		bands[i].OpenHours = open[i]
	}
	return bands
}

// pathLimit is a path's basic MUF in one typical hour; NaN where there is
// none.
func (s Snapshot) pathLimit(from, to LatLon) func(typicalHour) float64 {
	return func(t typicalHour) float64 {
		muf, ok := s.pathMUF(t.read, from, to, t.sun)
		if !ok {
			return math.NaN()
		}
		return muf
	}
}

// areaLimit is foF2 at a place in one typical hour; NaN where there is none.
func areaLimit(p LatLon) func(typicalHour) float64 {
	return func(t typicalHour) float64 {
		fo, _, ok := t.read(p)
		if !ok {
			return math.NaN()
		}
		return fo
	}
}

// pointAt is a control point's limits.Point from what a reader gives there.
func (s Snapshot) pointAt(read reader, p LatLon, sun *sunTrack) (limits.Point, bool) {
	fo, m3, ok := read(p)
	if !ok {
		return limits.Point{}, false
	}
	decl, chi, since := sun.sunAt(p.Lat, p.Lon)
	return limits.Point{FoF2: fo, M3000: m3, FoE: limits.FoE(p.Lat, decl, chi, float64(s.Inputs.F107Mean), since),
		FH: magcoords.Gyro(p.Lat, p.Lon, sun.at)}, true
}
