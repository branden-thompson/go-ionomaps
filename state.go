package ionomaps

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// ErrBadState is the error, wrapped, for a state Restore will not take: not
// one this library wrote, or one that would hold GIRO back longer than any
// refusal does, or ask it from the future. The library is left as new.
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
}

// State is GIRO's throttle state, for the host to keep across restarts
// (watchpost D-213): when the last burst began, when a pause ends, the
// back-off, and when each station last gave data, with the rotation's
// places - never a reading, which GIRO's terms keep from being stored.
// The bytes are the library's own; a host stores them as they are and hands
// them to Restore.
//
// Source: watchpost D-213.
func (l *Library) State() []byte {
	l.mu.Lock()
	g := l.giro.Export()
	l.mu.Unlock()
	k := keptState{V: stateVersion, Burst: unix(g.Throttle.BurstAt), Pause: unix(g.Throttle.PauseTo), BackOff: int64(g.Throttle.BackOff / time.Second)}
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
// burst asks GIRO nothing new and a refusal's pause still holds (watchpost
// D-213). An empty state, a first run's, restores nothing. A state that is
// not this library's, or that fails its checks against the clock, is
// refused with ErrBadState and the library is left as it was: the host
// carries on without it.
//
// Source: watchpost D-213; R-3.3.
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
	if err := l.giro.Import(g, l.clock().UTC()); err != nil {
		return fmt.Errorf("%w: %v", ErrBadState, err)
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
