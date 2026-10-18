package ionomaps

// aheadcache.go - what a library keeps rather than works out again (R-8.1,
// watchpost D-222): the day's climatology hours, which the hours ahead are,
// and the last background, which an update with the same inputs - new
// options at the snapshot's moment - is built on.

import (
	"sync"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
)

// aheadKey names an hour ahead's climatology: the start of its UTC hour and
// the F10.7 mean.
type aheadKey struct {
	at   time.Time
	f107 float32
}

// keptAhead is how many hours ahead a library keeps: a day's, twice over, so
// the hours of a snapshot after midnight find the day's before it.
const keptAhead = 2 * (maxAhead + 1)

// aheadHours are the climatology hours a library's snapshots share, so an
// hour ahead is worked out once a day rather than once an ask. Built outside
// the lock: two asks of one hour at once both build it, and keep the same.
type aheadHours struct {
	mu    sync.Mutex
	hours map[aheadKey]background.Fields
	built int // hours worked out, for the tests
}

// fields is the climatology at the start of an hour at an F10.7 mean, kept
// or worked out; with no keeper (a snapshot made by hand), worked out for
// the caller alone.
func (h *aheadHours) fields(at time.Time, f107 float32) background.Fields {
	if h == nil {
		return background.Build(at, float64(f107), nil)
	}
	k := aheadKey{at: at, f107: f107}
	h.mu.Lock()
	f, kept := h.hours[k]
	h.mu.Unlock()
	if kept {
		return f
	}
	f = background.Build(at, float64(f107), nil)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hours == nil || len(h.hours) >= keptAhead {
		h.hours = map[aheadKey]background.Fields{} // a new day, or a new mean: the old hours are not asked again
	}
	h.hours[k] = f
	h.built++
	return f
}

// lastBackground is the background the last build was made on, and what it
// was made from: an update whose moment, F10.7 mean and grid are the same -
// the host's options changed at the snapshot's moment - is built on it.
type lastBackground struct {
	at     time.Time
	f107   float64
	grid   *glotec.Grid
	fields background.Fields
	built  int // backgrounds worked out, for the tests
}

// background is the background for a moment, mean and grid: the last one
// when they are its own, else worked out and kept. Its slices are never
// written: the assimilation and the correction copy them (internal/assimilate).
func (b *lastBackground) background(at time.Time, f107 float64, g *glotec.Grid) background.Fields {
	if b.built > 0 && b.at.Equal(at) && b.f107 == f107 && b.grid == g {
		return b.fields
	}
	b.at, b.f107, b.grid = at, f107, g
	b.fields = background.Build(at, f107, g)
	b.built++
	return b.fields
}
