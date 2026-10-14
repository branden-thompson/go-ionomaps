package throttle

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

// TestAnUpdateNeverExceedsItsBurst is R-5.3 (D-39): one burst grants at
// most 40 asks, and GIRO is not due again until an hour after the burst
// began; within the hour nothing more is granted.
func TestAnUpdateNeverExceedsItsBurst(t *testing.T) {
	var g GIRO
	if !g.Due(t0) {
		t.Fatal("a fresh throttle is not due")
	}
	if n := g.Start(t0, 75); n != 40 {
		t.Errorf("a burst asked for 75 was granted %d; at most 40", n)
	}
	if g.Due(t0.Add(59*time.Minute)) || g.Start(t0.Add(59*time.Minute), 5) != 0 {
		t.Error("GIRO is due again within the hour of a burst")
	}
	if !g.Due(t0.Add(time.Hour)) {
		t.Error("GIRO is not due an hour after the burst began")
	}
	if n := g.Start(t0.Add(time.Hour), 12); n != 12 {
		t.Errorf("a burst of 12 was granted %d", n)
	}
}

// TestA429BacksOff is R-5.3: a refusal pauses GIRO 60 s, doubling with each
// refusal in a row, to 15 minutes at most; an answer ends the back-off. The
// pause is on top of the hour.
func TestA429BacksOff(t *testing.T) {
	var g GIRO
	g.Start(t0, 40)
	at := t0.Add(time.Hour)
	for i, want := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		g.Refused(at, "")
		if g.Due(at.Add(want-time.Second)) || !g.Due(at.Add(want)) {
			t.Errorf("refusal %d: the pause is not %v", i+1, want)
		}
		at = at.Add(want)
	}
	g.Answered()
	g.Start(at, 1)
	g.Refused(at.Add(time.Hour), "")
	if !g.Due(at.Add(time.Hour + time.Minute)) {
		t.Error("an answer did not end the back-off: the next refusal paused longer than 60 s")
	}
}

// TestRetryAfterIsHonouredWhenSent is R-5.3: a Retry-After, in seconds or as
// a date, sets the pause in place of the back-off.
func TestRetryAfterIsHonouredWhenSent(t *testing.T) {
	var g GIRO
	g.Refused(t0, "300")
	if g.Due(t0.Add(299*time.Second)) || !g.Due(t0.Add(300*time.Second)) {
		t.Error("Retry-After 300 is not a five-minute pause")
	}
	var h GIRO
	h.Refused(t0, t0.Add(7*time.Minute).Format(httpDate))
	if h.Due(t0.Add(6*time.Minute)) || !h.Due(t0.Add(7*time.Minute)) {
		t.Error("a Retry-After date seven minutes on is not honoured")
	}
}

// TestRetryAfterIsBoundedAt15Minutes is R-5.3 (persona N10): a Retry-After
// longer than 15 minutes is held to 15; a date already past is no pause;
// one that is no time is the back-off.
func TestRetryAfterIsBoundedAt15Minutes(t *testing.T) {
	var g GIRO
	g.Refused(t0, "86400")
	if !g.Due(t0.Add(15 * time.Minute)) {
		t.Error("a day's Retry-After paused GIRO past 15 minutes")
	}
	if d, ok := RetryAfter(t0.Add(-time.Hour).Format(httpDate), t0); !ok || d != 0 {
		t.Errorf("a Retry-After date already past is a pause of %v (%v); want none", d, ok)
	}
	var h GIRO
	h.Refused(t0, "soon")
	if h.Due(t0.Add(59*time.Second)) || !h.Due(t0.Add(time.Minute)) {
		t.Error("a Retry-After that is no time did not fall back to the 60 s back-off")
	}
}

// TestACancelledBurstSpendsTheHoursBudget is R-5.3 (round 2, LN1): a burst
// granted and then cancelled by the host still waits its hour.
func TestACancelledBurstSpendsTheHoursBudget(t *testing.T) {
	var g GIRO
	g.Start(t0, 40) // the host cancels before any ask is made
	if g.Due(t0.Add(10 * time.Minute)) {
		t.Error("a cancelled burst gave its hour back")
	}
}

// TestA429IsSeenByTheLibraryNotRetried is R-5.3: within a burst, a refusal
// stops it - nothing more is granted until the pause ends.
func TestA429IsSeenByTheLibraryNotRetried(t *testing.T) {
	var g GIRO
	g.Start(t0, 40)
	g.Refused(t0.Add(5*time.Second), "")
	if g.Start(t0.Add(10*time.Second), 40) != 0 {
		t.Error("a burst was granted again after a refusal inside the hour")
	}
}

// FuzzRateHeaders is R-3.3: whatever Retry-After says, the pause is between
// 0 and 15 minutes, and nothing panics.
func FuzzRateHeaders(f *testing.F) {
	for _, s := range []string{"", "0", "120", "-5", "999999999999999999999", "Wed, 21 Oct 2026 07:28:00 GMT", "1e9", " 60 "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if d, ok := RetryAfter(s, t0); ok && (d < 0 || d > MaxPause) {
			t.Fatalf("Retry-After %q is a pause of %v", s, d)
		}
	})
}
