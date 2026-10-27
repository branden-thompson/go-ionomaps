package stations

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

var t0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

// TestTheSeedIsGIROsList is D-51: the shipped seed holds every station in
// GIRO's form, each code checked, the 39 that gave data with a position on
// the globe; the rest, never heard and with no place, start flagged.
func TestTheSeedIsGIROsList(t *testing.T) {
	l, err := Seed()
	if err != nil {
		t.Fatal(err)
	}
	if len(l.known) != 118 {
		t.Errorf("the seed holds %d stations; GIRO's form listed 118", len(l.known))
	}
	placed, flagged := 0, 0
	for _, s := range l.known {
		if s.Placed {
			placed++
			if s.Lat < -90 || s.Lat > 90 || s.Lon < -180 || s.Lon > 180 {
				t.Errorf("%s is off the globe: %v, %v", s.Code, s.Lat, s.Lon)
			}
		}
		if l.Flagged(s.Code) {
			flagged++
		}
	}
	if placed != 39 || flagged != 118-39 {
		t.Errorf("%d placed and %d flagged at a cold start; want 39 and %d", placed, flagged, 118-39)
	}
}

// placedCodes are the stations the seed places, in its order.
func placedCodes(l *List) []string {
	var out []string
	for _, s := range l.known {
		if s.Placed {
			out = append(out, s.Code)
		}
	}
	return out
}

// TestAColdStartAsksThePlacedStationsFirst is the update's timing model's
// rule 1 (watchpost D-316): the first burst asks the 39 placed stations,
// then the flagged ones due, within the budget.
func TestAColdStartAsksThePlacedStationsFirst(t *testing.T) {
	l, _ := Seed()
	plan := l.Plan(t0, 40)
	if len(plan) != 40 || !slices.Equal(plan[:39], placedCodes(l)) || !l.Flagged(plan[39]) {
		t.Errorf("a cold start asks %d: %v; want the 39 placed, then one flagged", len(plan), plan)
	}
	if l.Plan(t0, 0) != nil || l.Plan(time.Time{}, 40) != nil {
		t.Error("a plan with no budget or no time asked something")
	}
}

// TestAStationIsFlaggedAfterThreeMissesInARow is the model's rule 3
// (watchpost D-316, D-317): two misses leave a station in the rotation, a
// reading between misses starts the count again, and the third miss in a
// row flags it.
func TestAStationIsFlaggedAfterThreeMissesInARow(t *testing.T) {
	l, _ := Seed()
	s := placedCodes(l)[4]
	l.Asked(s, t0, false)
	l.Asked(s, t0, false)
	l.Asked(s, t0, true) // it answers: the count starts again
	l.Asked(s, t0, false)
	l.Asked(s, t0, false)
	if l.Flagged(s) {
		t.Fatal("flagged after two misses in a row")
	}
	l.Asked(s, t0, false)
	if !l.Flagged(s) || slices.Contains(l.Plan(t0, 40), s) {
		t.Errorf("after three misses in a row: flagged %v, asked at once %v; want flagged and left until due", l.Flagged(s), slices.Contains(l.Plan(t0, 40), s))
	}
	if l.Flagged("NOPE1") {
		t.Error("a code the seed does not name is flagged")
	}
}

// TestAFlaggedStationIsAskedEverySixHours is the model's rules 1 and 3
// (watchpost D-317): a flagged station is asked again 6 hours after its
// last ask, behind every station not flagged, the one asked longest ago
// first; one reading clears its flag.
func TestAFlaggedStationIsAskedEverySixHours(t *testing.T) {
	l, _ := Seed()
	placed := placedCodes(l)
	a, b := placed[2], placed[9]
	for range 3 {
		l.Asked(a, t0, false)
		l.Asked(b, t0.Add(time.Hour), false)
	}
	plan := l.Plan(t0.Add(6*time.Hour), 200)
	if slices.Contains(plan, b) || slices.Index(plan, a) != len(placed)-2 {
		t.Errorf("at 6 h: %s due %v, %s at %d; want %s first behind the 37 not flagged, ahead of the codes never heard, %s not yet", b, slices.Contains(plan, b), a, slices.Index(plan, a), a, b)
	}
	if plan := l.Plan(t0.Add(7*time.Hour), 200); slices.Index(plan, a) > slices.Index(plan, b) {
		t.Errorf("at 7 h both are due: %s at %d, %s at %d; want the one asked longest ago first", a, slices.Index(plan, a), b, slices.Index(plan, b))
	}
	l.Asked(a, t0.Add(7*time.Hour), true)
	if l.Flagged(a) || !slices.Contains(l.Plan(t0.Add(8*time.Hour), 40), a) {
		t.Errorf("after a reading: flagged %v; want its flag cleared and it asked in the rotation", l.Flagged(a))
	}
}

// TestTheRotationCarriesOnWhereTheBurstStopped is the model's rules 1 and 5
// (watchpost D-316): a burst the deadline stops after four asks is carried
// on next hour from the fifth.
func TestTheRotationCarriesOnWhereTheBurstStopped(t *testing.T) {
	l, _ := Seed()
	first := l.Plan(t0, 40)
	for _, s := range first[:4] {
		l.Asked(s, t0, true)
	}
	if next := l.Plan(t0.Add(time.Hour), 40); next[0] != first[4] {
		t.Errorf("the next burst starts at %s; want %s, where the last one stopped", next[0], first[4])
	}
}

// TestAboveFortyStationsRotate is D-87: with more stations not flagged than
// the burst holds, each is asked in turn, none left out two bursts running.
func TestAboveFortyStationsRotate(t *testing.T) {
	l, _ := Seed()
	at := t0
	var unplaced []string
	for _, s := range l.known {
		if !s.Placed {
			unplaced = append(unplaced, s.Code)
		}
	}
	for i, code := range unplaced[:6] { // six more placed by their replies: 45 not flagged
		l.Heard(code, at, Place{Lat: float64(i), Lon: 1, Known: true})
		l.Asked(code, at, true)
		at = at.Add(time.Minute)
	}
	asked := map[string]int{}
	for hour := range 2 {
		for _, s := range l.Plan(at.Add(time.Duration(hour)*time.Hour), 40) {
			asked[s]++
			l.Asked(s, at, true)
		}
	}
	for _, s := range placedCodes(l) {
		if asked[s] == 0 {
			t.Errorf("%s, not flagged, was left out of two bursts running", s)
		}
	}
	down := placedCodes(l)[0]
	for range 3 {
		l.Asked(down, at, false)
	}
	if plan := l.Plan(at.Add(7*time.Hour), 40); !slices.Contains(plan, down) {
		t.Errorf("with 44 not flagged, %s, flagged and due, was not asked; want the place kept for the flagged due", down)
	}
	if len(placedCodes(l)) != 45 {
		t.Errorf("%d placed; want 45", len(placedCodes(l)))
	}
}

// TestAnUpdateAsksOnlySinceTheLastReading is R-5.2: a station is asked from
// its last reading held, from the last hour when it has none or the last is
// three days old; a reading with no time changes nothing, and a position
// off the globe is not kept.
func TestAnUpdateAsksOnlySinceTheLastReading(t *testing.T) {
	l, _ := Seed()
	s := placedCodes(l)[0]
	if got := l.Since(s, t0); !got.Equal(t0.Add(-time.Hour)) {
		t.Errorf("with no reading held, %s is asked from %v; want the last hour", s, got)
	}
	l.Heard(s, t0.Add(-20*time.Minute), Place{})
	if got := l.Since(s, t0); !got.Equal(t0.Add(-20 * time.Minute)) {
		t.Errorf("%s is asked from %v; its last reading is 20 minutes old", s, got)
	}
	if got := l.Since(s, t0.Add(80*time.Hour)); !got.Equal(t0.Add(79 * time.Hour)) {
		t.Errorf("%s, silent three days, is asked from %v; want the last hour, not its whole silence", s, got)
	}
	before := l.Since(s, t0)
	l.Heard(s, time.Time{}, Place{})
	if got := l.Since(s, t0); !got.Equal(before) {
		t.Errorf("a reading with no time moved %s's last reading to %v", s, got)
	}
	unplaced := l.known[slices.IndexFunc(l.known, func(x Station) bool { return !x.Placed })].Code
	l.Heard(unplaced, t0, Place{Lat: 99, Lon: 1, Known: true})
	if _, placed := l.At(unplaced); placed {
		t.Errorf("a position off the globe placed %s", unplaced)
	}
	if got := l.Since(fmt.Sprint("NOPE1"), t0); !got.Equal(t0.Add(-time.Hour)) {
		t.Errorf("a station not in the list is asked from %v", got)
	}
}

// TestAListStateIsCheckedOnApply is watchpost D-213: a list's state comes
// back onto a fresh seed as it was - each station asked from its reading
// time, the rotation where it stood, the flags and when each flagged one
// was asked - and is the list's own copy; one with a time past the skew, a
// miss count below one, a rotation place out of range, or more stations
// than the seed names is refused, the list left as it was; a station the
// seed no longer names is dropped and the rest taken (R2L-4).
func TestAListStateIsCheckedOnApply(t *testing.T) {
	l, _ := Seed()
	placed := placedCodes(l)
	l.Heard(placed[0], t0.Add(-20*time.Minute), Place{})
	for _, s := range l.Plan(t0, 10)[:3] {
		l.Asked(s, t0, true)
	}
	for range 3 {
		l.Asked(placed[7], t0, false)
	}
	kept := l.Record()
	now := t0.Add(30 * time.Minute)
	back, _ := Seed()
	if err := back.Apply(kept, now); err != nil {
		t.Fatal(err)
	}
	if got := back.Since(placed[0], now); !got.Equal(t0.Add(-20 * time.Minute)) {
		t.Errorf("restored, %s is asked from %v; its reading was at %v", placed[0], got, t0.Add(-20*time.Minute))
	}
	if a, b := back.Plan(now, 10), l.Plan(now, 10); !slices.Equal(a, b) || !back.Flagged(placed[7]) {
		t.Errorf("the restored list asks %v, flagged %v; the kept list %v", a, back.Flagged(placed[7]), b)
	}
	kept.Heard[placed[1]] = t0
	if back.Since(placed[1], now).Equal(t0) {
		t.Error("a change to the state kept reached the restored list")
	}
	tooMany := map[string]time.Time{}
	for i := range 200 {
		tooMany[fmt.Sprintf("X%04d", i)] = t0
	}
	for name, s := range map[string]State{
		"heard past the skew":         {Heard: map[string]time.Time{placed[0]: now.Add(throttle.ClockSkew + time.Second)}},
		"asked past the skew":         {Asked: map[string]time.Time{placed[0]: now.Add(throttle.ClockSkew + time.Second)}},
		"a miss count below one":      {Misses: map[string]int{placed[0]: 0}},
		"a poll place below zero":     {PollAt: -1},
		"a poll place past its wrap":  {PollAt: pollAtMost},
		"more stations than the seed": {Heard: tooMany},
	} {
		fresh, _ := Seed()
		if err := fresh.Apply(s, now); err == nil {
			t.Errorf("%s: restored", name)
		}
		if len(fresh.heard) != 0 || len(fresh.misses) != 0 || fresh.pollAt != 0 {
			t.Errorf("%s: a refused state changed the list", name)
		}
	}
	retired, _ := Seed()
	if err := retired.Apply(State{Heard: map[string]time.Time{"ZZ999": t0, placed[0]: t0}, Misses: map[string]int{"ZZ999": 3, placed[1]: 9}}, now); err != nil || len(retired.heard) != 1 || len(retired.misses) != 1 || retired.misses[placed[1]] != flagAfter {
		t.Errorf("a station the seed no longer names: %v, heard %v, misses %v; want it dropped, the rest taken, a count held to the flag", err, retired.heard, retired.misses)
	}
	ahead, _ := Seed()
	if err := ahead.Apply(State{Heard: map[string]time.Time{placed[0]: now.Add(time.Minute)}}, now); err != nil || !ahead.heard[placed[0]].Equal(now) {
		t.Errorf("a time a minute ahead, a clock stepped back: %v, heard %v; want taken as now", err, ahead.heard[placed[0]])
	}
}
