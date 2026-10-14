package ionomaps

import (
	"math"
	"time"
)

// The typical offset's seed, measured over PLAN's week (watchpost D-105):
// the live offset's mean, -0.51 MHz, taken as -0.5; one standard deviation,
// the 2-SD band of 0.45 MHz halved; counted as the 56 updates it was
// measured on, so the library's own updates refine it, never replace it at
// once. A live offset needs five readings, as offset.py measured it.
const (
	seedOffsetMHz = -0.5
	seedSpreadMHz = 0.225
	seedUpdates   = 56
	liveReadings  = 5
	offsetBasis   = "the live offset over PLAN's week, 56 updates, mean -0.51 MHz, SD 0.23 (watchpost D-105; go-ionomaps 02-analysis/evidence/offset.py)"
)

// typicalOffset is the typical offset learned so far: a running mean and
// sum of squared differences (Welford's) over the seed's updates and this
// library's, and the newest reading learned from, so the same readings are
// learned from once however many grids they are assimilated over.
type typicalOffset struct {
	n, mean, sum float64
	learned      time.Time
}

// seedOffset is the typical offset before any update: the seed, counted as
// the week's updates.
func seedOffset() typicalOffset {
	return typicalOffset{n: seedUpdates, mean: seedOffsetMHz, sum: seedSpreadMHz * seedSpreadMHz * seedUpdates}
}

// learn takes one update's live offset, whose newest reading is newest,
// unless readings that new were learned from already.
func (o *typicalOffset) learn(live float64, newest time.Time) {
	if !newest.After(o.learned) {
		return
	}
	o.n++
	d := live - o.mean
	o.mean += d / o.n
	o.sum += d * (live - o.mean)
	o.learned = newest
}

// offset is the snapshot's Offset: the live offset when measured, the
// typical offset and its spread.
func (o typicalOffset) offset(live float64, measured, corrected bool) Offset {
	s := Offset{TypicalMHz: float32(o.mean), SpreadMHz: float32(math.Sqrt(o.sum / o.n)), Measured: measured, Corrected: corrected, Basis: offsetBasis}
	if measured {
		s.LiveMHz = float32(live)
	}
	return s
}
