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

// hourLimits are one typical hour's limits: the upper limit, MHz (NaN
// where there is none), and whether a band is absorbed under the lower.
type hourLimits struct {
	upper    float64
	absorbed func(Band) bool
}

// notAbsorbed is the lower limit where no circuit is made: nothing absorbed.
func notAbsorbed(Band) bool { return false }

// openHours are the spans of the day each band is open between the
// hour-by-hour limits, by band in AllBands order.
func openHours(day [24]typicalHour, limit func(typicalHour) hourLimits) [][]Span {
	var limits [24]hourLimits
	for h := range day {
		limits[h] = limit(day[h])
	}
	out := make([][]Span, 0, len(bandEdges)-1)
	for _, b := range AllBands() {
		var spans []Span
		for h := range day {
			if !(b.LowMHz() < limits[h].upper) || limits[h].absorbed(b) {
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

// pathLimit is a path's limits in one typical hour: its basic MUF (NaN
// where there is none) and its circuit's absorption.
func (s Snapshot) pathLimit(from, to LatLon) func(typicalHour) hourLimits {
	return func(t typicalHour) hourLimits {
		muf, _, ok := s.pathMUF(t.read, from, to, t.sun)
		if !ok {
			return hourLimits{upper: math.NaN(), absorbed: notAbsorbed}
		}
		return hourLimits{upper: muf, absorbed: s.lowerAt(t, from, to, muf)}
	}
}

// areaLimit is the near-vertical area's limits in one typical hour: foF2 at
// the place (NaN where there is none) and its representative path's
// absorption.
func (s Snapshot) areaLimit(p LatLon, radiusKm float64) func(typicalHour) hourLimits {
	return func(t typicalHour) hourLimits {
		fo, _, ok := t.read(p)
		if !ok {
			return hourLimits{upper: math.NaN(), absorbed: notAbsorbed}
		}
		west, east := areaEnds(p, radiusKm)
		return hourLimits{upper: fo, absorbed: s.lowerAt(t, west, east, fo)}
	}
}

// lowerAt is whether a band is absorbed on a path in a typical hour; nothing
// is where no circuit is made.
func (s Snapshot) lowerAt(t typicalHour, from, to LatLon, upper float64) func(Band) bool {
	c, ok := s.circuitAt(t, from, to)
	if !ok {
		return notAbsorbed
	}
	return func(b Band) bool { return c.closedBelow(b, upper) }
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
