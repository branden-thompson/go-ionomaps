package ionomaps

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/climatology"
	"github.com/branden-thompson/go-ionomaps/internal/grid"
	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// Span is a stretch of time, From up to To.
type Span struct{ From, To time.Time }

// typicalHour is the climatology for one hour of the day at the snapshot's
// F10.7 mean, and the subsolar point's quasi-dipole longitude then.
type typicalHour struct {
	at     time.Time
	hour   climatology.Hour
	subQD  float64
	sun    *sunTrack
	long   longMemo      // the day's §5.3 work by path, shared by its hours
	cells  *typicalCells // the climatology at each 2° cell, as worked out
	filled *atomic.Int64 // the cells of the typical day worked out, for the tests of what is kept
}

// openHour is the typical hour h of a kept day, made as it is opened.
func openHour(kept *typicalDayCells, h int, at time.Time, f107 float32, long longMemo) typicalHour {
	cells, sun := kept.hour(h)
	return typicalHour{at: at, hour: climatology.NewHour(climatology.Refits(), at, float64(f107)), subQD: magcoords.SubsolarQD(at), sun: sun, long: long,
		cells: cells, filled: &kept.filled}
}

// typicalDay is the climatology for each hour of the UTC day holding a
// moment (watchpost D-135).
func (s Snapshot) typicalDay(at time.Time) [24]typicalHour {
	var day [24]typicalHour
	start := at.UTC().Truncate(24 * time.Hour)
	kept := s.typical.day(start, s.Inputs.F107Mean)
	memo := longMemo{}
	for h := range day {
		day[h] = openHour(kept, h, start.Add(time.Duration(h)*time.Hour), s.Inputs.F107Mean, memo)
	}
	return day
}

// typicalHourAt is the typical hour starting at a UTC hour's start, its
// cells the typical day's, so an hour ahead and the answers read one
// climatology (R-8.1, watchpost D-222).
func (s Snapshot) typicalHourAt(at time.Time) typicalHour {
	start := at.UTC().Truncate(24 * time.Hour)
	kept := s.typical.day(start, s.Inputs.F107Mean)
	return openHour(kept, int(at.Sub(start)/time.Hour), at, s.Inputs.F107Mean, nil)
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

// read is the climatology in this hour at the 2° cell a place falls in -
// the resolution the snapshot's own field is read at (R-1.1) - from the
// cell centre's quasi-dipole latitude and magnetic local time, each cell
// worked out once a typical day and kept (watchpost D-205, P-4).
func (t *typicalHour) read(p LatLon) (float64, float64, bool) {
	i, ok := grid.CellOf(p.Lat, p.Lon)
	if !ok {
		return 0, 0, false
	}
	if t.cells == nil {
		return 0, 0, false // an hour outside its day: no climatology
	}
	slot := t.cells.cell(i)
	w := slot.Load()
	if w == 0 {
		lat, lon := grid.Centre(i)
		qdlat, qdlon, _ := magcoords.QD(lat, lon, t.at)
		w = packCell(t.hour.At(qdlat, magcoords.MLTFrom(qdlon, t.subQD)))
		slot.Store(w)
		if t.filled != nil {
			t.filled.Add(1)
		}
	}
	fo, m3 := unpackCell(w)
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
func openHours(day *[24]typicalHour, limit func(*typicalHour, *[24]typicalHour) hourLimits) [][]Span {
	var limits [24]hourLimits
	for h := range day {
		limits[h] = limit(&day[h], day)
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
func (s Snapshot) pathLimit(from, to LatLon) func(*typicalHour, *[24]typicalHour) hourLimits {
	return func(t *typicalHour, day *[24]typicalHour) hourLimits {
		muf, _, ok := s.pathMUF(t.read, from, to, t.sun)
		if !ok {
			return hourLimits{upper: math.NaN(), absorbed: notAbsorbed}
		}
		return hourLimits{upper: muf, absorbed: s.absorbedAt(t, day, from, to, muf)}
	}
}

// areaLimit is the near-vertical area's limits in one typical hour: foF2 at
// the place (NaN where there is none) and its representative path's
// absorption.
func (s Snapshot) areaLimit(p LatLon, radiusKm float64) func(*typicalHour, *[24]typicalHour) hourLimits {
	return func(t *typicalHour, day *[24]typicalHour) hourLimits {
		fo, _, ok := t.read(p)
		if !ok {
			return hourLimits{upper: math.NaN(), absorbed: notAbsorbed}
		}
		west, east := areaEnds(p, radiusKm)
		return hourLimits{upper: fo, absorbed: s.absorbedAt(t, day, west, east, fo)}
	}
}

// absorbedAt is whether a band is absorbed on a path in a typical hour;
// nothing is where no circuit is made.
func (s Snapshot) absorbedAt(t *typicalHour, day *[24]typicalHour, from, to LatLon, upper float64) func(Band) bool {
	c, ok := s.newCircuit(t.read, from, to, t.sun, day)
	if !ok {
		return notAbsorbed
	}
	return func(b Band) bool { return c.lowerStatus(b, upper) != Open } // the lower limit closes the band (R-2.4, D-70)
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
