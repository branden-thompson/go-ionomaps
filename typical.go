package ionomaps

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// typicalCells is one typical hour's climatology at each 2° cell, worked
// out as it is first read: foF2 and M(3000)F2 as float32 bits in one word, 0
// for a cell not yet worked out. Answers asked at once read and fill it
// safely (atomics; two filling one cell write the same value).
type typicalCells []atomic.Uint64

// typicalDayCells are a UTC day's typical hours at an F10.7 mean: each
// hour's cells and the sun's track to it.
type typicalDayCells struct {
	hours [24]typicalCells
	suns  [24]*sunTrack
}

// typicalKey names a typical day: its UTC midnight and the F10.7 mean.
type typicalKey struct {
	day  time.Time
	f107 float32
}

// keptDays is how many typical days a library keeps: today's, and the next
// for the hours ahead past midnight.
const keptDays = 2

// typicalDays are the typical days a library's snapshots share (watchpost
// P-4, D-205), so the climatology at a cell in an hour is worked out once a
// day, not once an answer.
type typicalDays struct {
	mu   sync.Mutex
	days map[typicalKey]*typicalDayCells
}

// day is the typical day starting at a UTC midnight at an F10.7 mean, kept
// or made; with no keeper (a snapshot made by hand), made for the caller
// alone.
func (t *typicalDays) day(start time.Time, f107 float32) *typicalDayCells {
	if t == nil {
		return newTypicalDay(start)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := typicalKey{day: start, f107: f107}
	if d, ok := t.days[k]; ok {
		return d
	}
	if t.days == nil || len(t.days) >= keptDays {
		t.days = map[typicalKey]*typicalDayCells{} // a new day, or a new mean: the old ones are not asked again
	}
	d := newTypicalDay(start)
	t.days[k] = d
	return d
}

// newTypicalDay is a typical day with no cell worked out yet.
func newTypicalDay(start time.Time) *typicalDayCells {
	d := &typicalDayCells{}
	for h := range d.hours {
		d.hours[h] = make(typicalCells, grid.Cells)
		d.suns[h] = newSunTrack(start.Add(time.Duration(h) * time.Hour))
	}
	return d
}

// packCell is a cell's foF2 and M(3000)F2 in one word.
func packCell(fo, m3 float64) uint64 {
	return uint64(math.Float32bits(float32(fo)))<<32 | uint64(math.Float32bits(float32(m3)))
}

// unpackCell is a word's foF2 and M(3000)F2.
func unpackCell(w uint64) (fo, m3 float64) {
	return float64(math.Float32frombits(uint32(w >> 32))), float64(math.Float32frombits(uint32(w)))
}
