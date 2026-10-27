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
// the globe.
func TestTheSeedIsGIROsList(t *testing.T) {
	l, err := Seed(t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.known) != 118 {
		t.Errorf("the seed holds %d stations; GIRO's form listed 118", len(l.known))
	}
	placed := 0
	for _, s := range l.known {
		if s.Placed {
			placed++
			if s.Lat < -90 || s.Lat > 90 || s.Lon < -180 || s.Lon > 180 {
				t.Errorf("%s is off the globe: %v, %v", s.Code, s.Lat, s.Lon)
			}
		}
	}
	if placed != 39 || len(l.Live(t0)) != 39 {
		t.Errorf("%d placed and %d live at a cold start; want 39 and 39", placed, len(l.Live(t0)))
	}
}

// TestAColdStartUsesTheSeedWithoutABurst is D-51: the first update polls the
// seed's live stations at once and probes one other, never a burst of
// probes.
func TestAColdStartUsesTheSeedWithoutABurst(t *testing.T) {
	l, _ := Seed(t0)
	poll, probe, _ := l.Plan(t0, 40)
	if len(poll) != 39 || probe == "" || slices.Contains(poll, probe) {
		t.Errorf("a cold start polls %d and probes %q; want the 39 live and one other", len(poll), probe)
	}
}

// TestADarkStationIsDroppedAfterThreeDays is D-51: a station that gives no
// data for three days is no longer polled; one that gives data again is.
func TestADarkStationIsDroppedAfterThreeDays(t *testing.T) {
	l, _ := Seed(t0)
	dark := l.Live(t0)[0]
	for _, s := range l.Live(t0)[1:] {
		l.Heard(s, t0.Add(70*time.Hour), Place{})
	}
	if poll, _, _ := l.Plan(t0.Add(71*time.Hour), 40); !slices.Contains(poll, dark) {
		t.Errorf("%s, silent 71 hours, is no longer polled", dark)
	}
	if poll, _, _ := l.Plan(t0.Add(73*time.Hour), 40); slices.Contains(poll, dark) {
		t.Errorf("%s, silent 73 hours, is still polled", dark)
	}
	l.Heard(dark, t0.Add(74*time.Hour), Place{})
	if poll, _, _ := l.Plan(t0.Add(74*time.Hour), 40); !slices.Contains(poll, dark) {
		t.Errorf("%s gave data again and is not polled", dark)
	}
}

// TestOneProbePerUpdate is D-51: each update probes one station that is not
// live, the next in rotation, so every one is probed in turn; a probe that
// gives data makes the station live, with the position its reply gave.
func TestOneProbePerUpdate(t *testing.T) {
	l, _ := Seed(t0)
	seen := map[string]bool{}
	at := t0
	for range 79 {
		_, probe, _ := l.Plan(at, 40)
		seen[probe] = true
		at = at.Add(time.Minute) // well inside three days: the seed's live stay live
	}
	if len(seen) != 79 {
		t.Errorf("79 updates probed %d different stations; the 79 not live, each once", len(seen))
	}
	_, probe, _ := l.Plan(at, 40)
	l.Heard(probe, at, Place{Lat: 10, Lon: 20, Known: true})
	if !slices.Contains(l.Live(at), probe) {
		t.Errorf("%s gave data on its probe and is not live", probe)
	}
	if s := l.station(probe); !s.Placed || s.Lat != 10 || s.Lon != 20 {
		t.Errorf("%s's position from its reply is not kept: %+v", probe, s)
	}
}

// TestTheProbeCountsInsideTheBurst is D-87: with 40 live, an update polls 39
// and probes one - never more than 40 asks.
func TestTheProbeCountsInsideTheBurst(t *testing.T) {
	l, _ := Seed(t0)
	_, probe, _ := l.Plan(t0, 40)
	l.Heard(probe, t0, Place{Lat: 1, Lon: 1, Known: true}) // the 40th live
	poll, next, _ := l.Plan(t0.Add(time.Hour), 40)
	if len(l.Live(t0.Add(time.Hour))) != 40 || len(poll) != 39 || next == "" {
		t.Errorf("40 live: %d polled and probe %q; want 39 and one", len(poll), next)
	}
	if poll, probe, _ := l.Plan(t0.Add(2*time.Hour), 10); len(poll)+min(len(probe), 1) > 10 {
		t.Errorf("a budget of 10 made %d asks", len(poll)+1)
	}
}

// TestAboveFortyStationsRotate is D-51 and D-87: above 40 live, 39 are polled
// in rotation, and those left out are polled first next time, even when
// the live list has changed between.
func TestAboveFortyStationsRotate(t *testing.T) {
	l, _ := Seed(t0)
	at := t0
	for i := range 6 { // six more live: 45
		_, probe, _ := l.Plan(at, 40)
		l.Heard(probe, at, Place{Lat: float64(i), Lon: 1, Known: true})
		at = at.Add(time.Minute)
	}
	live := l.Live(at)
	if len(live) != 45 {
		t.Fatalf("%d live; want 45", len(live))
	}
	first, _, _ := l.Plan(at, 40)
	var left []string
	for _, s := range live {
		if !slices.Contains(first, s) {
			left = append(left, s)
		}
	}
	later := at.Add(73 * time.Hour) // the first live station falls dark, so every place in the live list moves up one
	for _, s := range live[1:] {
		l.Heard(s, later, Place{})
	}
	second, _, _ := l.Plan(later, 40)
	for _, s := range left {
		if !slices.Contains(second, s) {
			t.Errorf("%s, left out of one update, is not polled in the next", s)
		}
	}
	if len(left) != 6 {
		t.Errorf("%d left out of the first update; want 45 - 39 = 6", len(left))
	}
}

// TestAnUpdateAsksOnlySinceTheLastReading is R-5.2: a station is asked from
// its last reading held; one with none held, from the last hour.
func TestAnUpdateAsksOnlySinceTheLastReading(t *testing.T) {
	l, _ := Seed(t0)
	s := l.Live(t0)[0]
	if got := l.Since(s, t0); !got.Equal(t0.Add(-time.Hour)) {
		t.Errorf("with no reading held, %s is asked from %v; want the last hour", s, got)
	}
	l.Heard(s, t0.Add(-20*time.Minute), Place{})
	if got := l.Since(s, t0); !got.Equal(t0.Add(-20 * time.Minute)) {
		t.Errorf("%s is asked from %v; its last reading is 20 minutes old", s, got)
	}
	if got := l.Since(s, t0.Add(80*time.Hour)); !got.Equal(t0.Add(79 * time.Hour)) {
		t.Errorf("%s, back after three days dark, is asked from %v; want the last hour, not its whole silence", s, got)
	}
	l.Heard(s, t0, Place{Lat: 99, Lon: 1, Known: true})
	before := l.Since(s, t0)
	l.Heard(s, time.Time{}, Place{})
	if got := l.Since(s, t0); !got.Equal(before) || !slices.Contains(l.Live(t0), s) {
		t.Errorf("a reading with no time moved %s's last reading to %v", s, got)
	}
	fresh := l.Live(t0)[1] // live from the seed, never heard
	l.Heard(fresh, time.Time{}, Place{})
	if !slices.Contains(l.Live(t0), fresh) {
		t.Errorf("a reading with no time made %s, live from the seed, dark", fresh)
	}
	if st := l.station(s); st.Lat == 99 {
		t.Errorf("a position off the globe was kept for %s", s)
	}
	if _, err := Seed(time.Time{}); err == nil {
		t.Error("a cold start with no clock was taken")
	}
	if got := l.Since(fmt.Sprint("NOPE1"), t0); !got.Equal(t0.Add(-time.Hour)) {
		t.Errorf("a station not in the list is asked from %v", got)
	}
}

// TestAListStateIsCheckedOnApply is watchpost D-213: a list's state
// comes back onto a fresh seed as it was - each station asked from its
// reading time, the rotations where they stood - and is the list's own copy;
// one with a time after now, a station left out twice, or a rotation place
// below zero is refused, the list left as it was; a station the seed no
// longer names is dropped and the rest taken (R2L-4).
func TestAListStateIsCheckedOnApply(t *testing.T) {
	l, _ := Seed(t0)
	live := l.Live(t0)
	l.Heard(live[0], t0.Add(-20*time.Minute), Place{})
	l.Plan(t0, 10)
	kept := l.Record()
	now := t0.Add(30 * time.Minute)
	back, _ := Seed(now)
	if err := back.Apply(kept, now); err != nil {
		t.Fatal(err)
	}
	if got := back.Since(live[0], now); !got.Equal(t0.Add(-20 * time.Minute)) {
		t.Errorf("restored, %s is asked from %v; its reading was at %v", live[0], got, t0.Add(-20*time.Minute))
	}
	a, _, _ := back.Plan(now, 10)
	b, _, _ := l.Plan(now, 10)
	if !slices.Equal(a, b) {
		t.Errorf("the restored rotation polls %v; the kept list %v", a, b)
	}
	kept.Heard[live[1]] = t0
	if back.Since(live[1], now).Equal(t0) {
		t.Error("a change to the state kept reached the restored list")
	}
	for name, s := range map[string]State{
		"no seed time":                {Heard: map[string]time.Time{}},
		"seeded past the skew":        {Seeded: now.Add(throttle.ClockSkew + time.Second)},
		"heard past the skew":         {Seeded: t0, Heard: map[string]time.Time{live[0]: now.Add(throttle.ClockSkew + time.Second)}},
		"left out twice":              {Seeded: t0, LeftOut: []string{live[0], live[0]}},
		"a poll place below zero":     {Seeded: t0, PollAt: -1},
		"a probe place below zero":    {Seeded: t0, ProbeAt: -1},
		"a probe place past the list": {Seeded: t0, ProbeAt: 10000},
		"more left out than stations": {Seeded: t0, LeftOut: make([]string, 10000)},
	} {
		fresh, _ := Seed(now)
		if err := fresh.Apply(s, now); err == nil {
			t.Errorf("%s: restored", name)
		}
		if !fresh.seeded.Equal(now) || len(fresh.heard) != 0 {
			t.Errorf("%s: a refused state changed the list", name)
		}
	}
	retired, _ := Seed(now)
	if err := retired.Apply(State{Seeded: t0, Heard: map[string]time.Time{"ZZ999": t0, live[0]: t0}, LeftOut: []string{"ZZ999", live[1]}}, now); err != nil || len(retired.heard) != 1 || !slices.Equal(retired.leftOut, []string{live[1]}) {
		t.Errorf("a station the seed no longer names: %v, heard %v, left out %v; want it dropped and the rest taken", err, retired.heard, retired.leftOut)
	}
	ahead, _ := Seed(now)
	if err := ahead.Apply(State{Seeded: now.Add(time.Minute), Heard: map[string]time.Time{live[0]: now.Add(time.Minute)}}, now); err != nil || !ahead.seeded.Equal(now) || !ahead.heard[live[0]].Equal(now) {
		t.Errorf("times a minute ahead, a clock stepped back: %v, seeded %v, heard %v; want taken as now", err, ahead.seeded, ahead.heard[live[0]])
	}
}
