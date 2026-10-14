// Package throttle is GIRO's limits (plan task G9.1, watchpost D-39): a
// burst of at most 40 asks, at most hourly - well inside 2 a minute
// sustained; a refusal pauses it 60 s, doubling to 15 minutes, or for a
// Retry-After when one is sent, held to 15 minutes. It is state and the
// clock it is given: no timer, no goroutine (R-5.4).
package throttle

import (
	"strconv"
	"strings"
	"time"
)

// The limits (D-39, R-5.3).
const (
	MaxBurst     = 40
	Every        = time.Hour
	firstBackOff = time.Minute
	MaxPause     = 15 * time.Minute
)

// httpDate is the date form a Retry-After may take (RFC 9110).
const httpDate = "Mon, 02 Jan 2006 15:04:05 GMT"

// GIRO is the throttle's state: when the last burst began, when a pause
// ends, and the back-off a next refusal doubles.
type GIRO struct {
	burstAt time.Time
	pauseTo time.Time
	backOff time.Duration
}

// Due reports whether GIRO may be asked now: an hour since the last burst
// began, and no pause running; never with no clock.
//
// Source: watchpost D-39.
func (g *GIRO) Due(now time.Time) bool {
	noClock := now.IsZero()
	if noClock {
		return false
	}
	paused := now.Before(g.pauseTo)
	if paused {
		return false
	}
	first := g.burstAt.IsZero()
	if first {
		return true
	}
	return !now.Before(g.burstAt.Add(Every))
}

// Start begins a burst of up to want asks, and says how many may be made:
// at most MaxBurst, none when GIRO is not due. A burst granted spends the
// hour, whether or not its asks are made (R-5.3).
//
// Source: watchpost D-39; go-ionomaps R-5.3.
func (g *GIRO) Start(now time.Time, want int) int {
	if want <= 0 {
		return 0
	}
	due := g.Due(now)
	if !due {
		return 0
	}
	g.burstAt = now
	return min(want, MaxBurst)
}

// Refused pauses GIRO after a 429 or no answer: for the Retry-After sent,
// held to MaxPause, or for the back-off, 60 s doubling to MaxPause.
//
// Source: watchpost D-39.
func (g *GIRO) Refused(now time.Time, retryAfter string) {
	g.backOff = min(max(2*g.backOff, firstBackOff), MaxPause)
	pause := g.backOff
	if d, ok := RetryAfter(retryAfter, now); ok {
		pause = d
	}
	g.pauseTo = now.Add(pause)
}

// Answered ends the back-off: the next refusal pauses 60 s again.
//
// Source: watchpost D-39.
func (g *GIRO) Answered() { g.backOff = 0 }

// RetryAfter is a Retry-After header as a pause: seconds, or a date, held
// to 0 to MaxPause; false for no header, or one that is no time.
//
// Source: RFC 9110 §10.2.3; watchpost D-39 and persona N10's 15 minutes.
func RetryAfter(s string, now time.Time) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	secs, errN := strconv.ParseInt(s, 10, 64)
	at, errD := time.Parse(httpDate, s)
	numeric, dated := errN == nil, errD == nil
	if numeric && secs < 0 {
		return 0, false // no time
	}
	if numeric && secs >= int64(MaxPause/time.Second) {
		return MaxPause, true // held to 15 minutes
	}
	if numeric {
		return time.Duration(secs) * time.Second, true
	}
	if !dated {
		return 0, false
	}
	past := at.Before(now)
	if past {
		return 0, true
	}
	return min(at.Sub(now), MaxPause), true
}
