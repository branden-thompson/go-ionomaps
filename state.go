package ionomaps

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/solar"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// ErrBadState is the error, wrapped, for a state Restore will not take: not
// one this library wrote, or one that would hold GIRO back longer than any
// refusal does, or ask it from the future. The library is left as it was.
var ErrBadState = errors.New("ionomaps: the state given is not one this library keeps")

// stateVersion is the state's layout; another is refused.
const stateVersion = 1

// maxState is the most a state may be: the seed's stations and their times
// fit in far less.
const maxState = 64 << 10

// keptState is the state as kept: times as Unix seconds, the back-off in
// seconds, and the live-station list's times and rotation once seeded.
type keptState struct {
	V       int              `json:"v"`
	Burst   int64            `json:"burst"`
	Pause   int64            `json:"pause"`
	BackOff int64            `json:"backoff"`
	Seeded  int64            `json:"seeded"`
	Heard   map[string]int64 `json:"heard"`
	Poll    int              `json:"poll"`
	Probe   int              `json:"probe"`
	Out     []string         `json:"out"`
	F107    float64          `json:"f107,omitempty"`     // the F10.7 mean held (watchpost D-249)
	F107Day int              `json:"f107days,omitempty"` // the days behind it
	F107At  int64            `json:"f107at,omitempty"`   // when it was taken
}

// State is what the library keeps across restarts, for the host to store
// (watchpost D-213, D-249): GIRO's throttle - when the last burst began,
// when a pause ends, the back-off, and when each station last gave data,
// with the rotation's places, never a reading, which GIRO's terms keep from
// being stored - and NOAA's F10.7 mean held, public domain, with its days
// and when it was taken, so a restart while the solar file cannot be read
// still builds a field for up to 7 days. The bytes are the library's own; a
// host stores them as they are and hands them to Restore.
//
// Source: watchpost D-213, D-249.
func (l *Library) State() []byte {
	l.mu.Lock()
	g, f := l.giro.Export(), l.solar
	l.mu.Unlock()
	k := keptState{V: stateVersion, Burst: unix(g.Throttle.BurstAt), Pause: unix(g.Throttle.PauseTo), BackOff: int64(g.Throttle.BackOff / time.Second)}
	if f.OK {
		k.F107, k.F107Day, k.F107At = f.Mean, f.Days, unix(f.Taken)
	}
	if st := g.Stations; st != nil {
		k.Seeded, k.Poll, k.Probe, k.Out, k.Heard = unix(st.Seeded), st.PollAt, st.ProbeAt, st.LeftOut, map[string]int64{}
		for code, at := range st.Heard {
			k.Heard[code] = unix(at)
		}
	}
	b, _ := json.Marshal(k) // a struct of numbers, strings and maps of them always encodes
	return b
}

// Restore takes a state State wrote, so a restart within the hour of a
// burst asks GIRO nothing new, a refusal's pause still holds (watchpost
// D-213), and the F10.7 mean held comes back with its age (D-249). An empty
// state, a first run's, restores nothing. A state that is not this
// library's, or that fails its checks against the clock or a flux's
// bounds, is refused with ErrBadState and the library is left as it was:
// the host carries on without it.
//
// Source: watchpost D-213, D-249; R-3.3.
func (l *Library) Restore(state []byte) error {
	if len(state) == 0 {
		return nil
	}
	if len(state) > maxState {
		return fmt.Errorf("%w: it is over %d bytes", ErrBadState, maxState)
	}
	var k keptState
	if err := json.Unmarshal(state, &k); err != nil || k.V != stateVersion {
		return fmt.Errorf("%w: not a version %d state", ErrBadState, stateVersion)
	}
	g := giro.State{Throttle: throttle.State{BurstAt: fromUnix(k.Burst), PauseTo: fromUnix(k.Pause), BackOff: time.Duration(k.BackOff) * time.Second}}
	if k.Seeded != 0 {
		st := stations.State{Seeded: fromUnix(k.Seeded), PollAt: k.Poll, ProbeAt: k.Probe, LeftOut: k.Out, Heard: map[string]time.Time{}}
		for code, at := range k.Heard {
			st.Heard[code] = fromUnix(at)
		}
		g.Stations = &st
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock().UTC()
	f, err := solar.Resume(k.F107, k.F107Day, fromUnix(k.F107At), now)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadState, err)
	}
	if err := l.giro.Import(g, now); err != nil {
		return fmt.Errorf("%w: %v", ErrBadState, err)
	}
	if f.OK {
		l.solar = f
	}
	return nil
}

// unix is a time as Unix seconds; 0 for no time.
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// fromUnix is Unix seconds as a time; no time for 0.
func fromUnix(s int64) time.Time {
	if s == 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}
