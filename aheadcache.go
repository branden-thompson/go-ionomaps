package ionomaps

// aheadcache.go - the last background a library keeps rather than works out
// again (R-8.1, watchpost D-222): an update with the same inputs - new
// options at the snapshot's moment - is built on it. The hours ahead read
// the typical day's cells (typical.go).

import (
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/background"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
)

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
