package ionomaps

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// hoursADay are a typical day's hours.
const hoursADay = 24

// typicalCells is one typical hour's climatology at each 2° cell, worked
// out as it is first read: foF2 and M(3000)F2 as float32 bits in one word, 0
// for a cell not yet worked out. A row of cells is made with its first
// read, so an hour read at a path's few points holds those rows, not the
// globe's (watchpost R2P-1). Answers asked at once read and fill it safely
// (atomics; two filling one cell write the same value, two making one row
// keep the first made).
type typicalCells struct {
	rows [grid.Rows]atomic.Pointer[typicalRow]
}

// typicalRow is one row of a typical hour's cells.
type typicalRow [grid.Cols]atomic.Uint64

// cell is cell i's word, its row made at the row's first read.
func (c *typicalCells) cell(i int) *atomic.Uint64 {
	row := &c.rows[i/grid.Cols]
	r := row.Load()
	if r != nil {
		return &r[i%grid.Cols]
	}
	fresh := new(typicalRow)
	if row.CompareAndSwap(nil, fresh) {
		return &fresh[i%grid.Cols]
	}
	return &row.Load()[i%grid.Cols]
}

// typicalDayCells are a UTC day's typical hours at an F10.7 mean: each
// hour's cells and the sun's track to it, made the first time the hour is
// read (watchpost D-251), so an hour ahead holds its own hour, not the day's.
type typicalDayCells struct {
	start  time.Time
	made   [hoursADay]sync.Once
	hours  [hoursADay]*typicalCells
	suns   [hoursADay]*sunTrack
	filled atomic.Int64 // cells worked out, for the tests of what is kept
}

// hour is an hour's cells and the sun's track to it, made at its first
// read; safe for the answers asked at once. An hour outside the day has
// none, read as no data, never an index past the day's.
func (d *typicalDayCells) hour(h int) (*typicalCells, *sunTrack) {
	if h < 0 || h >= hoursADay {
		return nil, nil
	}
	d.made[h].Do(func() {
		d.hours[h] = new(typicalCells)
		d.suns[h] = newSunTrack(d.start.Add(time.Duration(h) * time.Hour))
	})
	return d.hours[h], d.suns[h]
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
	if t.days == nil {
		t.days = map[typicalKey]*typicalDayCells{}
	}
	if len(t.days) >= keptDays {
		old := t.oldest()
		if start.Before(old.day) {
			return newTypicalDay(start) // older than every day kept - a late answer of a snapshot before midnight - is made for it alone, never evicting today's (RP-2)
		}
		delete(t.days, old) // the oldest day goes alone, so a snapshot still answered across midnight keeps its own (RP-2)
	}
	d := newTypicalDay(start)
	t.days[k] = d
	return d
}

// oldest is the key of the earliest day kept; with two of one day (a new
// mean), the first found. Called with t.mu held.
func (t *typicalDays) oldest() typicalKey {
	var out typicalKey
	first := true
	for k := range t.days {
		if first || k.day.Before(out.day) {
			out, first = k, false
		}
	}
	return out
}

// newTypicalDay is a typical day with no hour made yet.
func newTypicalDay(start time.Time) *typicalDayCells {
	return &typicalDayCells{start: start}
}

// packCell is a cell's foF2 and M(3000)F2 in one word.
func packCell(fo, m3 float64) uint64 {
	return uint64(math.Float32bits(float32(fo)))<<32 | uint64(math.Float32bits(float32(m3)))
}

// unpackCell is a word's foF2 and M(3000)F2.
func unpackCell(w uint64) (fo, m3 float64) {
	return float64(math.Float32frombits(uint32(w >> 32))), float64(math.Float32frombits(uint32(w)))
}
