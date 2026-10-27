package ionomaps_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// restarted is a new library on the same NOAA and GIRO, as after the
// host's restart, with the state given restored.
func restarted(t *testing.T, n *noaa, c *clock, state []byte) *ionomaps.Library {
	t.Helper()
	l, err := ionomaps.New(n, ionomaps.Options{Clock: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Restore(state); err != nil {
		t.Fatal(err)
	}
	return l
}

// TestTheThrottleOutlivesARestart is R-5.8 (watchpost D-213): a library
// restored from the state the last one exported asks GIRO nothing within
// the hour of the last burst; past it, its burst asks each station from
// the reading time held. A library given no state bursts at once.
func TestTheThrottleOutlivesARestart(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	asked, heardAt := len(n.giro.asks), c.Now().Add(-5*time.Minute)
	state := l.State()
	c.Add(5 * time.Minute)
	again := restarted(t, n, c, state)
	for range 5 {
		if s := update(t, again); s.Reason == ionomaps.StationsNext {
			t.Fatalf("restored within the hour, GIRO is due at %v", c.Now())
		}
		c.Add(10 * time.Minute)
	}
	if len(n.giro.asks) != asked {
		t.Fatalf("restored within the hour, GIRO was asked %d times", len(n.giro.asks)-asked)
	}
	c.Add(10 * time.Minute)
	if s := update(t, again); s.Reason != ionomaps.StationsNext {
		t.Fatalf("past the hour the restored library's reason is %v", s.Reason)
	}
	update(t, again)
	if len(n.giro.asks) != asked+40 {
		t.Fatalf("the restored library's burst asked %d", len(n.giro.asks)-asked)
	}
	u, _ := url.Parse(n.giro.asks[asked])
	if from := u.Query().Get("fromDate"); from != heardAt.UTC().Format("2006/01/02 15:04:05") {
		t.Errorf("the restored burst asks from %s; want the reading time held, %v", from, heardAt)
	}

	fresh := restarted(t, n, c, nil)
	if s := update(t, fresh); s.Reason != ionomaps.StationsNext {
		t.Errorf("a library given no state: reason %v; want GIRO due", s.Reason)
	}
}

// TestAFlaggedStationStaysFlaggedAcrossARestart is the update's timing
// model's rule 3 with D-213: a station that missed three bursts running is
// flagged, and the state the host keeps carries it, so a restart does not
// ask it again before it is due.
func TestAFlaggedStationStaysFlaggedAcrossARestart(t *testing.T) {
	l, n, c := giroLibrary(t)
	n.giro.status = map[string]int{"AL945": 503}
	for range 3 {
		update(t, l)
		update(t, l)
		c.Add(time.Hour)
	}
	again := restarted(t, n, c, l.State())
	asked := len(n.giro.asks)
	update(t, again)
	update(t, again)
	if len(n.giro.asks) == asked {
		t.Fatal("control: the restored library made no burst")
	}
	for _, raw := range n.giro.asks[asked:] {
		if u, _ := url.Parse(raw); u.Query().Get("ursiCode") == "AL945" {
			t.Errorf("restored, AL945, flagged by three misses an hour before, was asked again; want it left until due")
		}
	}
}

// TestTheStateHoldsNoReading is watchpost D-213 and brief GC-2: the state
// is times, a back-off and the rotation's places - never a reading, which
// GIRO's terms keep from being stored - and NOAA's F10.7 mean, public
// domain (D-249).
func TestTheStateHoldsNoReading(t *testing.T) {
	l, _, _ := giroLibrary(t)
	update(t, l)
	update(t, l)
	state := l.State()
	if len(state) == 0 || bytes.Contains(state, []byte("7.5")) || bytes.Contains(state, []byte("3.4")) {
		t.Errorf("the state is %s; it holds a reading's value", state)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(state, &keys); err != nil {
		t.Fatal(err)
	}
	for k := range keys {
		switch k {
		case "v", "burst", "pause", "backoff", "seeded", "heard", "poll", "probe", "out", "f107", "f107days", "f107at":
		default:
			t.Errorf("the state carries %q", k)
		}
	}
}

// TestABadStateIsRefused is watchpost D-213, R-3.3 and R2L-4: a state that
// is not one this library wrote is refused with ErrBadState and leaves the
// library as new; one whose GIRO part would hold GIRO back or ask it from
// the future is refused for that part alone, GIRO starting as new and the
// kept mean taken; one whose mean is out of bounds keeps GIRO's state; a
// station code the seed does not name is dropped, never the state refused
// over it; an empty one, a first run's, restores nothing and is no error.
func TestABadStateIsRefused(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l)
	good := l.State()
	edit := func(key string, v any) []byte {
		var m map[string]any
		if err := json.Unmarshal(good, &m); err != nil {
			t.Fatal(err)
		}
		m[key] = v
		b, _ := json.Marshal(m)
		return b
	}
	now := c.Now().Unix()
	restore := func(state []byte) (*ionomaps.Library, error) {
		again, err := ionomaps.New(n, ionomaps.Options{Clock: c.Now})
		if err != nil {
			t.Fatal(err)
		}
		return again, again.Restore(state)
	}
	for name, state := range map[string][]byte{
		"not JSON":        []byte("{held"),
		"another version": edit("v", 2),
		"too big":         append(edit("out", []string{}), bytes.Repeat([]byte(" "), 70_000)...),
	} {
		again, err := restore(state)
		if !errors.Is(err, ionomaps.ErrBadState) {
			t.Errorf("%s: restored with %v", name, err)
			continue
		}
		if s := update(t, again); s.Reason != ionomaps.StationsNext {
			t.Errorf("%s: after a refused state the reason is %v; want the library as new", name, s.Reason)
		}
	}
	for name, state := range map[string][]byte{
		"a burst in the future":            edit("burst", now+3600),
		"a pause past a day":               edit("pause", now+25*3600),
		"a back-off past fifteen":          edit("backoff", 3600),
		"a reading time past the skew":     edit("heard", map[string]int64{"AL945": now + 3600}),
		"a rotation place below zero":      edit("poll", -1),
		"a rotation place no list reaches": edit("poll", int64(9223372036854775807)),
	} {
		again, err := restore(state)
		if !errors.Is(err, ionomaps.ErrBadState) {
			t.Errorf("%s: restored with %v", name, err)
			continue
		}
		if !errors.Is(err, ionomaps.ErrBadGIROState) || errors.Is(err, ionomaps.ErrBadMean) || strings.Contains(err.Error(), "\n") {
			t.Errorf("%s: %v; want GIRO's part named, the mean not, on one line", name, err)
		}
		n.down[solarURL] = true // the solar file unread: a field only from the mean the state kept (R2L-4)
		s, err := again.Update(context.Background(), ionomaps.UpdateOptions{})
		delete(n.down, solarURL)
		if err != nil || s.Reason != ionomaps.StationsNext {
			t.Errorf("%s: GIRO's part refused: reason %v, %v; want GIRO as new and the kept mean drawn", name, s.Reason, err)
		}
	}
	for name, state := range map[string][]byte{
		"a mean past a flux's bounds": edit("f107", 900),
		"a mean taken past the skew":  edit("f107at", now+3600),
	} {
		again, err := restore(state)
		if !errors.Is(err, ionomaps.ErrBadState) {
			t.Errorf("%s: restored with %v", name, err)
			continue
		}
		if !errors.Is(err, ionomaps.ErrBadMean) || errors.Is(err, ionomaps.ErrBadGIROState) {
			t.Errorf("%s: %v; want the mean named, GIRO's part not", name, err)
		}
		if s := update(t, again); s.Reason == ionomaps.StationsNext {
			t.Errorf("%s: the mean refused threw GIRO's state away too: %v", name, s.Reason)
		}
	}
	for name, state := range map[string][]byte{
		"a station code not GIRO's":     edit("heard", map[string]int64{"al945": now - 60}),
		"a station never seeded":        edit("heard", map[string]int64{"ZZ999": now - 60}),
		"a left-out station not GIRO's": edit("out", []string{"../x"}),
	} {
		again, err := restore(state)
		if err != nil {
			t.Errorf("%s: refused with %v; want the unknown code dropped", name, err)
			continue
		}
		if s := update(t, again); s.Reason == ionomaps.StationsNext {
			t.Errorf("%s: GIRO's state was not taken: %v", name, s.Reason)
		}
	}
	both := edit("f107", 900)
	var m map[string]any
	if err := json.Unmarshal(both, &m); err != nil {
		t.Fatal(err)
	}
	m["burst"] = now + 3600
	both, _ = json.Marshal(m)
	if _, err := restore(both); !errors.Is(err, ionomaps.ErrBadGIROState) || !errors.Is(err, ionomaps.ErrBadMean) || strings.Contains(err.Error(), "\n") {
		t.Errorf("both parts refused: %v; want both named, on one line (R4L-6)", err)
	}
	if err := restarted(t, n, c, good).Restore(nil); err != nil {
		t.Errorf("an empty state: %v", err)
	}
}

// FuzzARestoredStateNeverPanics is watchpost D-124 and D-213 against any
// bytes a state file could hold: Restore refuses what the library could not
// have reached, and whatever it takes, the updates after it - NOAA's and the
// stations' burst - never panic.
func FuzzARestoredStateNeverPanics(f *testing.F) {
	f.Add([]byte(`{"v":1,"seeded":1760000000,"poll":9223372036854775807,"probe":0}`))
	f.Add([]byte(`{"v":1,"seeded":1760000000,"poll":3,"probe":2,"heard":{"AL945":1760000100}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, state []byte) {
		_, n, c := giroLibrary(t)
		again, err := ionomaps.New(n, ionomaps.Options{Clock: c.Now})
		if err != nil {
			t.Fatal(err)
		}
		if again.Restore(state) != nil {
			return
		}
		update(t, again)
		update(t, again)
	})
}
