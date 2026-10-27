package giro

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/deadline"
	"github.com/branden-thompson/go-ionomaps/internal/stations"
	"github.com/branden-thompson/go-ionomaps/internal/throttle"
)

// TestABurstNeedsTheClock is R-5.4: with no time there is no burst - the
// throttle cannot time one - and nothing is asked.
func TestABurstNeedsTheClock(t *testing.T) {
	var a Asker
	asked := 0
	a.Burst(context.Background(), time.Time{}, func(context.Context, string) (int, string, []byte, error) {
		asked++
		return 200, "", nil, nil
	})
	if asked != 0 {
		t.Errorf("%d asks made with no clock", asked)
	}
}

// TestABurstStoppedBeforeItsFirstAskSpendsNoHour is R-5.3 and L-F4: a
// burst whose context has already ended asks nothing and leaves GIRO due;
// one stopped after an ask has spent the hour.
func TestABurstStoppedBeforeItsFirstAskSpendsNoHour(t *testing.T) {
	var a Asker
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	asked := 0
	ask := func(context.Context, string) (int, string, []byte, error) { asked++; return 200, "", nil, nil }
	a.Burst(ctx, now, ask)
	if asked != 0 || !a.BurstDue(now) {
		t.Errorf("%d asks; GIRO due after a burst that asked nothing: %v", asked, a.BurstDue(now))
	}
	ctx, cancel = context.WithCancel(context.Background())
	a.Burst(ctx, now, func(c context.Context, code string) (int, string, []byte, error) { cancel(); return ask(c, code) })
	if asked != 1 || a.BurstDue(now) {
		t.Errorf("%d asks; GIRO due after a burst that asked: %v", asked, a.BurstDue(now))
	}
}

// TestAReplyIsHeldOnlyForItsStation is R-5.2: a reply is held under the
// station it says it is, never another's code.
func TestAReplyIsHeldOnlyForItsStation(t *testing.T) {
	var a Asker
	a.list, _ = stations.Seed(now)
	at, _ := a.list.At("AS00Q")
	rep := Reply{Station: Station{Code: "AS00Q", Lat: at.Lat, Lon: at.Lon}, Readings: []Reading{{At: now.Add(-time.Hour), FoF2: 7}}}
	a.hold("AL945", rep)
	if len(a.Readings()) != 0 {
		t.Error("a reply for AS00Q was held under AL945")
	}
	a.hold("AS00Q", rep)
	if len(a.Readings()) != 1 {
		t.Error("a reply for AS00Q was not held under its own code")
	}
}

// TestAReadingIsLetGoAfterThreeDays is R-5.7 and D-51: a reading held is
// kept while its station stays live, let go after; without the time,
// nothing is judged stale.
func TestAReadingIsLetGoAfterThreeDays(t *testing.T) {
	var a Asker
	a.list, _ = stations.Seed(now)
	at, _ := a.list.At("AS00Q")
	a.hold("AS00Q", Reply{Station: Station{Code: "AS00Q", Lat: at.Lat, Lon: at.Lon}, Readings: []Reading{{At: now, FoF2: 7}, {At: now.Add(-time.Hour), FoF2: 6}}})
	if r := a.Readings(); len(r) != 1 || !r[0].Reading.At.Equal(now) {
		t.Fatalf("held %+v; want the latest reading alone", r)
	}
	a.LetGo(time.Time{})
	a.LetGo(now.Add(71 * time.Hour))
	if len(a.Readings()) != 1 {
		t.Error("a reading was let go before its station could fall dark")
	}
	a.LetGo(now.Add(73 * time.Hour))
	if len(a.Readings()) != 0 {
		t.Error("a reading 73 hours old is still held")
	}
}

// codeOf is the station an ask is for.
func codeOf(t *testing.T, u string) string {
	t.Helper()
	q, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	return q.Query().Get("ursiCode")
}

// TestAStationsFailureIsItsOwn is watchpost D-247 and D-272: one station's
// server error, 404, 410, a status no rule names, or no answer is that
// station's - counted, and the burst goes on to every other - never GIRO
// paused, and never taken as an answer.
func TestAStationsFailureIsItsOwn(t *testing.T) {
	for name, fail := range map[string]func() (int, error){
		"a server error": func() (int, error) { return 503, nil },
		"a 404":          func() (int, error) { return 404, nil },
		"a 410":          func() (int, error) { return 410, nil },
		"a 400":          func() (int, error) { return 400, nil },
		"a 451":          func() (int, error) { return 451, nil },
		"a redirect":     func() (int, error) { return 302, nil },
		"no answer":      func() (int, error) { return 0, errors.New("no route") },
	} {
		var a Asker
		var asked []string
		a.Burst(context.Background(), now, func(_ context.Context, u string) (int, string, []byte, error) {
			asked = append(asked, codeOf(t, u))
			if len(asked) == 1 {
				s, err := fail()
				return s, "", nil, err
			}
			return 200, "", nil, nil
		})
		live, _ := stations.Seed(now)
		if len(asked) != len(live.Live(now))+1 || a.Failed != 1 || a.Paused(now) || a.Outcome != Answered {
			t.Errorf("%s: %d asked of %d, %d failed, paused %v, %v; want every station asked, one failed, no pause", name, len(asked), len(live.Live(now))+1, a.Failed, a.Paused(now), a.Outcome)
		}
	}
}

// TestFailuresInARowStopTheBurst is watchpost D-247: three stations failing
// one after another is GIRO itself down - the burst stops and GIRO backs
// off as D-39 rules for a 429; the stations not reached are asked first at
// the next burst, so it moves past the ones that failed.
func TestFailuresInARowStopTheBurst(t *testing.T) {
	var a Asker
	var asked []string
	a.Burst(context.Background(), now, func(_ context.Context, u string) (int, string, []byte, error) {
		asked = append(asked, codeOf(t, u))
		return 503, "", nil, nil
	})
	if len(asked) != failuresInARow || !a.Paused(now) || a.Outcome != Unavailable {
		t.Fatalf("all failing: %d asked, paused %v, %v; want %d and a pause", len(asked), a.Paused(now), a.Outcome, failuresInARow)
	}
	first := asked
	asked = nil
	later := now.Add(throttle.Every)
	a.Burst(context.Background(), later, func(_ context.Context, u string) (int, string, []byte, error) {
		asked = append(asked, codeOf(t, u))
		return 200, "", nil, nil
	})
	if len(asked) == 0 || slices.Contains(first, asked[0]) {
		t.Errorf("the next burst began at %v, one of the stations that failed %v; want one not reached", asked[:min(1, len(asked))], first)
	}
}

// TestStationsDeadAtTheHeadStopOneBurstOnly is watchpost D-287: three dead
// stations at the head of the rotation stop the first burst, and go behind
// the rest - the rotation's head moved past them - so for the next seven
// hours every burst asks every station and stays Answered, never paused,
// where the head would otherwise have stopped every other burst.
func TestStationsDeadAtTheHeadStopOneBurstOnly(t *testing.T) {
	for _, deadAtHead := range []int{failuresInARow, failuresInARow + 1} { // four: the head moves past the last that failed, not the first (R4C-8)
		var a Asker
		var dead []string
		for hour := range 8 {
			var asked []string
			at := now.Add(time.Duration(hour) * time.Hour)
			a.Burst(context.Background(), at, func(_ context.Context, u string) (int, string, []byte, error) {
				code := codeOf(t, u)
				asked = append(asked, code)
				if hour == 0 && len(dead) < deadAtHead {
					dead = append(dead, code) // the head of the rotation: dead from now on
				}
				if slices.Contains(dead, code) {
					return 500, "", nil, nil
				}
				return 200, "", nil, nil
			})
			stopped := a.Outcome == Unavailable
			if hour == 0 && (!stopped || len(asked) != failuresInARow) {
				t.Fatalf("%d dead, hour 0: %d asked, %v; want the first three to stop the burst", deadAtHead, len(asked), a.Outcome)
			}
			if hour > 0 && (stopped || a.Paused(at) || len(asked) <= failuresInARow) {
				t.Errorf("%d dead, hour %d: %d asked, %v, paused %v; want every station asked and the burst Answered", deadAtHead, hour, len(asked), a.Outcome, a.Paused(at))
			}
		}
	}
}

// TestFailuresAfterAnAnswerAreTheStations is watchpost D-271: three
// stations failing one after another once another has answered is not GIRO
// down - each failure is its station's own, counted, and the burst goes on;
// nothing is paused and the outcome stays Answered.
func TestFailuresAfterAnAnswerAreTheStations(t *testing.T) {
	var a Asker
	var asked []string
	a.Burst(context.Background(), now, func(_ context.Context, u string) (int, string, []byte, error) {
		asked = append(asked, codeOf(t, u))
		if n := len(asked); n >= 2 && n <= 1+failuresInARow {
			return 503, "", nil, nil
		}
		return 200, "", nil, nil
	})
	live, _ := stations.Seed(now)
	if len(asked) != len(live.Live(now))+1 || a.Failed != failuresInARow || a.Paused(now) || a.Outcome != Answered {
		t.Errorf("%d asked of %d, %d failed, paused %v, %v; want every station asked, %d failed, no pause", len(asked), len(live.Live(now))+1, a.Failed, a.Paused(now), a.Outcome, failuresInARow)
	}
}

// TestAHungGIROIsCalledDown is watchpost D-275: an ask that is never
// answered ends at its own timeout, so three in a row before any answer
// call GIRO down - backed off, said Unavailable - inside a host's update,
// never run out to its deadline as a cancel.
func TestAHungGIROIsCalledDown(t *testing.T) {
	was := askTimeout
	askTimeout = 20 * time.Millisecond
	defer func() { askTimeout = was }()
	var a Asker
	asked := 0
	started := time.Now()
	a.Burst(context.Background(), now, func(ctx context.Context, _ string) (int, string, []byte, error) {
		asked++
		return hang(ctx) // GIRO never answers
	})
	if took := time.Since(started); asked != failuresInARow || a.Outcome != Unavailable || !a.Paused(now) || took > 10*askTimeout*failuresInARow {
		t.Errorf("a hung GIRO: %d asked in %v, %v, paused %v; want %d asks, each ended at its timeout, and Unavailable", asked, took, a.Outcome, a.Paused(now), failuresInARow)
	}
	if failuresInARow*was >= 60*time.Second {
		t.Errorf("%d asks of %v do not fit a 60-second update", failuresInARow, was)
	}
}

// hang is an ask GIRO never answers: it ends when its context does, and
// after a guard of two seconds if the context never does, so a missing
// timeout fails its test rather than hanging the package (R3C-7).
func hang(ctx context.Context) (int, string, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, "", nil, ctx.Err()
	case <-time.After(2 * time.Second):
		return 0, "", nil, errors.New("the guard ran out: the ask had no timeout")
	}
}

// TestAGIROHangingAfterAnAnswerIsCalledDown is watchpost D-286: asks that
// run out one after another call GIRO down whatever answered before - a
// station answers, then GIRO hangs: three asks run out, the burst stops,
// GIRO is backed off and said Unavailable, inside a host's update.
func TestAGIROHangingAfterAnAnswerIsCalledDown(t *testing.T) {
	was := askTimeout
	askTimeout = 20 * time.Millisecond
	defer func() { askTimeout = was }()
	var a Asker
	asked := 0
	a.Burst(context.Background(), now, func(ctx context.Context, u string) (int, string, []byte, error) {
		asked++
		if asked == 1 {
			return 200, "", nil, nil // the first station answers
		}
		return hang(ctx)
	})
	if asked != 1+failuresInARow || a.Outcome != Unavailable || !a.Paused(now) {
		t.Errorf("an answer, then GIRO hung: %d asked, %v, paused %v; want %d asked, Unavailable and paused", asked, a.Outcome, a.Paused(now), 1+failuresInARow)
	}
}

// TestADenialPausesForADay is watchpost D-247 and D-272: a 401 or 403 is
// GIRO refusing this host - the burst stops and GIRO is left alone for a
// day, said as Denied, across a restart too - never an answer that resets
// the back-off.
func TestADenialPausesForADay(t *testing.T) {
	for _, status := range []int{401, 403} {
		var a Asker
		asked := 0
		a.Burst(context.Background(), now, func(context.Context, string) (int, string, []byte, error) {
			asked++
			return status, "", nil, nil
		})
		if asked != 1 || a.Outcome != Denied || !a.Paused(now.Add(23*time.Hour)) || a.Paused(now.Add(throttle.DeniedFor+time.Second)) {
			t.Errorf("a %d: %d asked, %v, paused at 23 h %v; want one ask and a day's pause", status, asked, a.Outcome, a.Paused(now.Add(23*time.Hour)))
		}
		var back Asker
		if err := back.Import(a.Export(), now.Add(time.Hour)); err != nil || back.Outcome != Denied || !back.Paused(now.Add(time.Hour)) {
			t.Errorf("a %d across a restart: %v, %v, paused %v", status, err, back.Outcome, back.Paused(now.Add(time.Hour)))
		}
	}
}

// TestAPlacedStationKeepsItsPlace is watchpost D-248: a reply putting a
// station placed by the seed more than 50 km from its place is refused and
// counted, never held; one within reads at the seed's place.
func TestAPlacedStationKeepsItsPlace(t *testing.T) {
	var a Asker
	a.list, _ = stations.Seed(now)
	at, placed := a.list.At("AL945")
	if !placed {
		t.Fatal("AL945 is not placed by the seed")
	}
	away := Reply{Station: Station{Code: "AL945", Lat: 51.5, Lon: -0.1}, Readings: []Reading{{At: now, FoF2: 7}}}
	a.hold("AL945", away)
	if len(a.Readings()) != 0 || a.Rejected != 1 {
		t.Errorf("AL945 placed in London: %d held, %d rejected; want refused", len(a.Readings()), a.Rejected)
	}
	near := Reply{Station: Station{Code: "AL945", Lat: at.Lat + 0.1, Lon: at.Lon}, Readings: []Reading{{At: now, FoF2: 7}}}
	a.hold("AL945", near)
	if r := a.Readings(); len(r) != 1 || r[0].Station.Lat != at.Lat || r[0].Station.Lon != at.Lon {
		t.Errorf("AL945 11 km from its place: held %+v; want at the seed's place %+v", r, at)
	}
}

// scaledAsks sets a burst's limits to a hundredth of their own - an ask
// 150 ms, its margin 10 ms, its floor 20 ms - for a test of the end of the
// host's update, whose deadline the test scales alike: 600 ms for 60 s.
func scaledAsks(t *testing.T) {
	t.Helper()
	full, margin, floor := askTimeout, deadline.Margin, askFloor
	askTimeout, deadline.Margin, askFloor = 150*time.Millisecond, 10*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { askTimeout, deadline.Margin, askFloor = full, margin, floor })
}

// answerIn answers after d, a reply with no rows - or gives up when the
// ask's context ends first, as a real client does.
func answerIn(ctx context.Context, d time.Duration) (int, string, []byte, error) {
	select {
	case <-time.After(d):
		return 200, "", nil, nil
	case <-ctx.Done():
		return 0, "", nil, ctx.Err()
	}
}

// burstUnder is one burst under a host's update of 600 ms, as a hundredth
// of watchpost's 60 s: the stations asked, in order, and whether the burst
// ended before the update's deadline. answer is given each ask's number
// from 1, its station, its context and the update's end.
func burstUnder(t *testing.T, a *Asker, at time.Time, answer func(n int, code string, ctx context.Context, end time.Time) (int, string, []byte, error)) (asked []string, beat bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	end, _ := ctx.Deadline()
	a.Burst(ctx, at, func(ctx context.Context, u string) (int, string, []byte, error) {
		code := codeOf(t, u)
		asked = append(asked, code)
		return answer(len(asked), code, ctx, end)
	})
	return asked, ctx.Err() == nil
}

// TestABurstEndsBeforeTheHostsDeadline is watchpost D-291, D-295 and D-296:
// before each ask the burst reads the time left on the host's update - a
// full ask while there is room for one, then the time left, then none - so
// it never runs into the deadline; at the end GIRO is called down only when
// its last two full asks ran out, one hanging station being that station's
// own failure wherever the rotation puts it; the stations not reached are
// counted and asked first next hour. Times are a hundredth of their own, an
// answer 9 ms as PLAN's 0.92 s.
func TestABurstEndsBeforeTheHostsDeadline(t *testing.T) {
	scaledAsks(t)
	t.Run("a hang after 20 answers", func(t *testing.T) {
		var a Asker
		asked, beat := burstUnder(t, &a, now, func(n int, _ string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
			if n <= 20 {
				return answerIn(ctx, 9*time.Millisecond)
			}
			return hang(ctx)
		})
		if !beat || a.Outcome != Unavailable || !a.Paused(now) || len(asked) > 23 {
			t.Errorf("%d asked, before the deadline %v, %v, paused %v; want GIRO called down on two full asks run out, inside the update", len(asked), beat, a.Outcome, a.Paused(now))
		}
	})
	t.Run("one station hanging late, every hour", func(t *testing.T) {
		var a Asker
		var hanging string
		for hour := range 8 {
			at := now.Add(time.Duration(hour) * time.Hour)
			asked, beat := burstUnder(t, &a, at, func(n int, code string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
				if hour == 0 && n == 36 {
					hanging = code
				}
				if code == hanging {
					return hang(ctx)
				}
				return answerIn(ctx, 9*time.Millisecond)
			})
			if !beat || a.Outcome != Answered || a.Paused(at) || a.Failed > 1 || !slices.Contains(asked, hanging) {
				t.Errorf("hour %d: %d asked, before the deadline %v, %v, %d failed, paused %v; want one station's own failure, GIRO answering (R5L-1)", hour, len(asked), beat, a.Outcome, a.Failed, a.Paused(at))
			}
		}
	})
	t.Run("one station hanging early, the rest all asked", func(t *testing.T) {
		var a Asker
		asked, beat := burstUnder(t, &a, now, func(n int, _ string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
			if n == 5 {
				return hang(ctx)
			}
			return answerIn(ctx, 9*time.Millisecond)
		})
		if !beat || a.Outcome != Answered || a.NotReached != 0 || len(asked) < throttle.MaxBurst-1 {
			t.Errorf("%d asked, %d not reached, before the deadline %v, %v; want every station asked, the last asks given the time left (R5C-1)", len(asked), a.NotReached, beat, a.Outcome)
		}
	})
	t.Run("hangs alternating with a server error", func(t *testing.T) {
		var a Asker
		_, beat := burstUnder(t, &a, now, func(n int, _ string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
			switch {
			case n == 1:
				return 200, "", nil, nil
			case n%2 == 0:
				return hang(ctx)
			}
			return 500, "", nil, nil
		})
		if !beat || a.Outcome != Answered || a.Paused(now) || a.NotReached == 0 {
			t.Errorf("before the deadline %v, %v, paused %v, %d not reached; want the burst cut short and never two full asks run out in a row: GIRO not called down", beat, a.Outcome, a.Paused(now), a.NotReached)
		}
	})
	t.Run("one hang as the update ends", func(t *testing.T) {
		var a Asker
		hung := false
		_, beat := burstUnder(t, &a, now, func(_ int, _ string, ctx context.Context, end time.Time) (int, string, []byte, error) {
			if !hung && time.Until(end) < 180*time.Millisecond {
				hung = true
				return hang(ctx) // a full ask, its station's own: the update ends after it
			}
			return answerIn(ctx, 15*time.Millisecond)
		})
		if !hung || !beat || a.Outcome != Answered || a.Paused(now) || a.Failed != 1 {
			t.Errorf("hung %v, before the deadline %v, %v, paused %v, %d failed; want one full ask run out as the update ends taken as its station's failure", hung, beat, a.Outcome, a.Paused(now), a.Failed)
		}
	})
	t.Run("slow answers, no hang", func(t *testing.T) {
		var a Asker
		asked, beat := burstUnder(t, &a, now, func(_ int, _ string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
			return answerIn(ctx, 30*time.Millisecond)
		})
		if !beat || a.Outcome != Answered || a.Paused(now) || a.NotReached == 0 || a.Failed != 0 {
			t.Fatalf("%d asked, %d not reached, %d failed, before the deadline %v, %v, paused %v; want the burst cut short, its stations not reached counted, nothing failed or paused", len(asked), a.NotReached, a.Failed, beat, a.Outcome, a.Paused(now))
		}
		next, _ := burstUnder(t, &a, now.Add(time.Hour), func(int, string, context.Context, time.Time) (int, string, []byte, error) { return 200, "", nil, nil })
		if len(next) == 0 || slices.Contains(asked[:len(asked)-1], next[0]) {
			t.Errorf("the next burst began at %v, a station already answered; want one not reached", next[:min(1, len(next))])
		}
	})
	t.Run("three hanging stations", func(t *testing.T) {
		var a Asker
		var trio []string
		for hour := range 4 {
			at := now.Add(time.Duration(hour) * time.Hour)
			asked, beat := burstUnder(t, &a, at, func(_ int, code string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
				if hour == 0 && len(trio) < failuresInARow {
					trio = append(trio, code)
				}
				if slices.Contains(trio, code) {
					return hang(ctx)
				}
				return answerIn(ctx, 9*time.Millisecond)
			})
			want := Answered
			if hour == 0 {
				want = Unavailable // three in a row before any answer (D-286)
			}
			if !beat || a.Outcome != want || (hour > 0 && len(asked) <= failuresInARow) {
				t.Errorf("hour %d: %d asked, before the deadline %v, %v; want %v, the others asked, inside the update", hour, len(asked), beat, a.Outcome, want)
			}
		}
	})
	t.Run("a hung GIRO at the least deadline", func(t *testing.T) {
		// MinDeadline is room for two full asks and their margins: a GIRO
		// hung on every ask is called down within it (D-296, D-301; R7C-7).
		var a Asker
		ctx, cancel := context.WithTimeout(context.Background(), 2*(askTimeout+deadline.Margin)+5*time.Millisecond)
		defer cancel()
		a.Burst(ctx, now, func(ctx context.Context, _ string) (int, string, []byte, error) { return hang(ctx) })
		if a.Outcome != Unavailable || !a.Paused(now) || ctx.Err() != nil {
			t.Errorf("%v, paused %v, before the deadline %v; want a hung GIRO called down at the least deadline", a.Outcome, a.Paused(now), ctx.Err() == nil)
		}
	})
	t.Run("just under a full ask's room", func(t *testing.T) {
		// With the full ask and the margin not quite left, the ask is a
		// shortened one, and its running out is the update's end, never the
		// station's failure (D-295, D-301).
		var a Asker
		ctx, cancel := context.WithTimeout(context.Background(), askTimeout+deadline.Margin-5*time.Millisecond)
		defer cancel()
		a.Burst(ctx, now, func(ctx context.Context, _ string) (int, string, []byte, error) { return hang(ctx) })
		if a.Failed != 0 || a.NotReached == 0 || a.Outcome != Answered {
			t.Errorf("%d failed, %d not reached, %v; want the shortened ask not reached, no failure", a.Failed, a.NotReached, a.Outcome)
		}
	})
	t.Run("no room for the first ask", func(t *testing.T) {
		var a Asker
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		n := 0
		a.Burst(ctx, now, func(context.Context, string) (int, string, []byte, error) {
			n++
			return 200, "", nil, nil
		})
		if n != 0 || !a.NoRoom || !a.BurstDue(now) {
			t.Errorf("25 ms left: %d asked, no room %v, due %v; want nothing asked and the hour not spent (D-298)", n, a.NoRoom, a.BurstDue(now))
		}
	})
}

// TestTimeoutsAreFailuresAndARunOfThem is watchpost D-271 and D-286, the
// counts underneath: an ask that runs out counts as its station's failure,
// so a server error, a timeout and a server error before any answer are
// three failures in a row and stop the burst; asks that run out with answers
// between them are no run, and the burst goes on.
func TestTimeoutsAreFailuresAndARunOfThem(t *testing.T) {
	was := askTimeout
	askTimeout = 20 * time.Millisecond
	defer func() { askTimeout = was }()
	var before Asker
	n := 0
	before.Burst(context.Background(), now, func(ctx context.Context, _ string) (int, string, []byte, error) {
		n++
		if n == 2 {
			return hang(ctx)
		}
		return 500, "", nil, nil
	})
	if n != failuresInARow || before.Failed != failuresInARow || before.Outcome != Unavailable {
		t.Errorf("500, a timeout, 500 before any answer: %d asked, %d failed, %v; want three failures in a row, GIRO down", n, before.Failed, before.Outcome)
	}
	var scattered Asker
	n = 0
	scattered.Burst(context.Background(), now, func(ctx context.Context, _ string) (int, string, []byte, error) {
		n++
		if n <= 5 && n%2 == 1 {
			return hang(ctx) // asks 1, 3 and 5 run out, answers between them
		}
		return 200, "", nil, nil
	})
	if scattered.Outcome != Answered || scattered.Paused(now) || n <= 5 {
		t.Errorf("three timeouts with answers between: %d asked, %v, paused %v; want the burst run on", n, scattered.Outcome, scattered.Paused(now))
	}
}

// clientTimeout is the error a host's HTTP client gives when its own
// timeout runs out first: a timeout by its own account.
type clientTimeout struct{}

func (clientTimeout) Error() string { return "the client's timeout ran out" }
func (clientTimeout) Timeout() bool { return true }

// TestAHostClientsTimeoutIsATimeout is watchpost R4L-3: a host whose client
// gives up before askTimeout does returns its own timeout error, and the
// burst takes it as an ask run out - three in a row after an answer call
// GIRO down (D-286) - never as a station's other failure.
func TestAHostClientsTimeoutIsATimeout(t *testing.T) {
	var a Asker
	n := 0
	a.Burst(context.Background(), now, func(context.Context, string) (int, string, []byte, error) {
		n++
		if n == 1 {
			return 200, "", nil, nil
		}
		return 0, "", nil, fmt.Errorf("asking: %w", clientTimeout{})
	})
	if n != 1+failuresInARow || a.Outcome != Unavailable || !a.Paused(now) {
		t.Errorf("an answer, then the host's client timing out: %d asked, %v, paused %v; want three timeouts calling GIRO down", n, a.Outcome, a.Paused(now))
	}
}

// TestStationsTimingOutGoBehindTheRest is watchpost D-287 for asks that run
// out: three stations hanging after an answer stop the burst (D-286), and
// every later burst begins past them, the rest asked first - once the
// stations not reached are spent, the rotation's head is what keeps them
// behind.
func TestStationsTimingOutGoBehindTheRest(t *testing.T) {
	was := askTimeout
	askTimeout = 20 * time.Millisecond
	defer func() { askTimeout = was }()
	var a Asker
	var trio []string
	for hour := range 4 {
		var asked []string
		a.Burst(context.Background(), now.Add(time.Duration(hour)*time.Hour), func(ctx context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			asked = append(asked, code)
			if hour == 0 && len(asked) > 1 && len(trio) < failuresInARow {
				trio = append(trio, code)
			}
			if slices.Contains(trio, code) {
				return hang(ctx)
			}
			return 200, "", nil, nil
		})
		if hour == 0 && (a.Outcome != Unavailable || len(asked) != 1+failuresInARow) {
			t.Fatalf("an answer, then three hangs: %d asked, %v; want the burst stopped at the third", len(asked), a.Outcome)
		}
		if hour > 0 && (len(asked) <= 1+failuresInARow || slices.Contains(trio, asked[0])) {
			t.Errorf("hour %d asked %d, first %s; want it begun past the stations that hung %v", hour, len(asked), asked[0], trio)
		}
	}
}

// TestOnlyTheUpdatesEndCountsStationsNotReached is watchpost D-302: the
// count of stations not reached holds only those the update's end left - a
// cancel or GIRO's refusal counts none, though the stations still go first
// next hour, the one a cancel interrupted among them, and no cancel is a
// station's failure or calls GIRO down; a burst that asked nothing leaves
// the last burst's counts as they were. Times are a hundredth of their own.
func TestOnlyTheUpdatesEndCountsStationsNotReached(t *testing.T) {
	scaledAsks(t)
	t.Run("a cancel", func(t *testing.T) {
		var a Asker
		ctx, cancel := context.WithCancel(context.Background())
		var asked []string
		a.Burst(ctx, now, func(ctx context.Context, u string) (int, string, []byte, error) {
			asked = append(asked, codeOf(t, u))
			switch len(asked) {
			case 1:
				return 200, "", nil, nil
			case 2, 3:
				return hang(ctx) // two full asks run out
			}
			cancel() // the host leaves during the fourth ask
			return 0, "", nil, ctx.Err()
		})
		if a.NotReached != 0 || a.Outcome != Answered || a.Paused(now) || a.Failed != 2 {
			t.Fatalf("cancelled during ask 4 after two hangs: %d not reached, %v, paused %v, %d failed; want none counted, GIRO not called down, the cancel no failure", a.NotReached, a.Outcome, a.Paused(now), a.Failed)
		}
		kept := a.list.Record().Keep // the probe not reached goes first of all (D-303)
		var next []string
		a.Burst(context.Background(), now.Add(time.Hour), func(_ context.Context, u string) (int, string, []byte, error) {
			if code := codeOf(t, u); code != kept {
				next = append(next, code)
			}
			return 200, "", nil, nil
		})
		if len(next) == 0 || next[0] != asked[3] {
			t.Errorf("the next burst's first live station was %v; want the one the cancel interrupted, %s", next[:min(1, len(next))], asked[3])
		}
	})
	t.Run("GIRO's refusal", func(t *testing.T) {
		var a Asker
		n := 0
		a.Burst(context.Background(), now, func(context.Context, string) (int, string, []byte, error) {
			n++
			if n == 3 {
				return 429, "", nil, nil
			}
			return 200, "", nil, nil
		})
		if a.NotReached != 0 || a.Outcome != Unavailable {
			t.Errorf("a 429 at ask 3: %d not reached, %v; want none counted, GIRO unavailable", a.NotReached, a.Outcome)
		}
	})
	t.Run("a burst that asked nothing", func(t *testing.T) {
		var a Asker
		burstUnder(t, &a, now, func(_ int, _ string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
			return answerIn(ctx, 30*time.Millisecond)
		})
		cut := a.NotReached
		if cut == 0 {
			t.Fatal("control: the slow burst was not cut short")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		later := now.Add(time.Hour)
		a.Burst(ctx, later, func(context.Context, string) (int, string, []byte, error) { return 200, "", nil, nil })
		if a.NotReached != cut || !a.NoRoom || !a.BurstDue(later) {
			t.Errorf("no room the hour after: %d not reached, no room %v, due %v; want the last burst's %d kept, the hour not spent", a.NotReached, a.NoRoom, a.BurstDue(later), cut)
		}
		a.Burst(context.Background(), later, nil)
		if a.NoRoom {
			t.Error("a burst with nothing to ask with kept the last burst's no room (R6L-6)")
		}
	})
}

// TestAProbeNotReachedIsAskedFirst is watchpost D-303: a burst cut short
// before its probe - the one station not live it asks, last - leaves the
// probe its turn, asked first next hour, and counts only the live stations
// not reached - those never asked, and the one whose shortened ask ran out
// (D-295).
func TestAProbeNotReachedIsAskedFirst(t *testing.T) {
	scaledAsks(t)
	var a Asker
	asked, _ := burstUnder(t, &a, now, func(_ int, _ string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
		return answerIn(ctx, 30*time.Millisecond)
	})
	state := a.list.Record()
	kept, want := state.Keep, throttle.MaxBurst-1-len(asked) // the probe apart
	if slices.Contains(state.LeftOut, asked[len(asked)-1]) {
		want++ // the last ask ran out of its shortened limit: not reached (D-295)
	}
	if kept == "" || slices.Contains(asked, kept) || a.NotReached != want {
		t.Fatalf("cut short at %d asked: probe kept %q, %d not reached; want the probe kept and only the live stations counted, %d", len(asked), kept, a.NotReached, want)
	}
	var next []string
	a.Burst(context.Background(), now.Add(time.Hour), func(_ context.Context, u string) (int, string, []byte, error) {
		next = append(next, codeOf(t, u))
		return 200, "", nil, nil
	})
	if len(next) == 0 || next[0] != kept {
		t.Errorf("the next burst began at %v; want the probe not reached, %s", next[:min(1, len(next))], kept)
	}
}

// TestAStationThatRanOutIsAskedLast is watchpost D-307: a station whose full
// ask ran out is asked last in the bursts after - behind every station that
// answered, the probe apart - and back in its place once it answers.
func TestAStationThatRanOutIsAskedLast(t *testing.T) {
	was := askTimeout
	askTimeout = 20 * time.Millisecond
	defer func() { askTimeout = was }()
	var a Asker
	var hanging string
	answering := false
	burst := func(hour int) []string {
		var asked []string
		a.Burst(context.Background(), now.Add(time.Duration(hour)*time.Hour), func(ctx context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			asked = append(asked, code)
			if hour == 0 && len(asked) == 5 {
				hanging = code
			}
			if code == hanging && !answering {
				return hang(ctx)
			}
			return 200, "", nil, nil
		})
		return asked
	}
	burst(0)
	polled := func(asked []string) []string { return asked[:len(asked)-1] } // the probe is the last ask
	if p := polled(burst(1)); len(p) == 0 || p[len(p)-1] != hanging {
		t.Fatalf("the hour after: polled ending %v; want the station that ran out, %s, last", p[max(len(p)-2, 0):], hanging)
	}
	answering = true
	burst(2) // it answers, last
	if p := polled(burst(3)); len(p) == 0 || p[len(p)-1] == hanging {
		t.Errorf("after it answered: it is still polled last; want it back in its place")
	}
	var failing string // a station's own server error is no run out: it keeps its place (R7L-2)
	var b Asker
	for hour := range 2 {
		var asked []string
		b.Burst(context.Background(), now.Add(time.Duration(hour)*time.Hour), func(_ context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			asked = append(asked, code)
			if hour == 0 && len(asked) == 5 {
				failing = code
			}
			if code == failing {
				return 500, "", nil, nil
			}
			return 200, "", nil, nil
		})
		if p := polled(asked); hour == 1 && (len(p) == 0 || p[len(p)-1] == failing) {
			t.Errorf("a station that answered a server error is polled last; want only a run out to put it there")
		}
	}
}

// TestHangingStationsNeverCallAHealthyGIRODown is watchpost D-309 and D-310,
// the update's timing model's sections 2 and 3, over ten hours under a
// deadline: stations that hang are known to after the first hour, asked
// last however a deadline cut them, and their running out again counts
// toward no run - so with two, three or four of them, a dark probe that
// hangs too, or GIRO answering fast, every station that answers is read
// each hour and GIRO is never called down after the first. Times are a
// hundredth of their own, an answer 9 ms as PLAN's 0.92 s.
func TestHangingStationsNeverCallAHealthyGIRODown(t *testing.T) {
	scaledAsks(t)
	for _, c := range []struct {
		name      string
		hang      []int // the asks of the first hour, from 1, whose stations hang
		pace      time.Duration
		darkHangs bool // every station not live hangs too, so the probe does
	}{
		{"two hanging stations", []int{10, 20}, 9 * time.Millisecond, false},
		{"three side by side", []int{2, 3, 4}, 9 * time.Millisecond, false},
		{"four hanging stations", []int{5, 15, 25, 35}, 9 * time.Millisecond, false},
		{"two and a hanging probe", []int{10, 20}, 9 * time.Millisecond, true},
		{"two, GIRO answering fast", []int{10, 20}, 3 * time.Millisecond, false},
		{"two and a hanging probe, GIRO fast", []int{10, 20}, 3 * time.Millisecond, true},
		{"two, GIRO slower", []int{10, 20}, 11 * time.Millisecond, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var a Asker
			hanging := map[string]bool{}
			healthy := map[string]bool{}
			for hour := range 10 {
				at := now.Add(time.Duration(hour) * time.Hour)
				read := map[string]bool{}
				var opened []string
				_, beat := burstUnder(t, &a, at, func(n int, code string, ctx context.Context, _ time.Time) (int, string, []byte, error) {
					if hour == 0 && slices.Contains(c.hang, n) {
						hanging[code] = true
					}
					if len(opened) < 3 {
						opened = append(opened, code)
					}
					if hanging[code] || c.darkHangs && !slices.Contains(a.list.Live(at), code) {
						return hang(ctx) // a station that hangs, or a dark probe that does
					}
					read[code] = true
					return answerIn(ctx, c.pace)
				})
				if !beat {
					t.Fatalf("hour %d ran into its deadline", hour)
				}
				if hour == 0 {
					continue
				}
				liveRead := 0
				for code := range read {
					if slices.Contains(a.list.Live(at), code) {
						healthy[code] = true
						liveRead++
					}
				}
				if a.Outcome != Answered || a.Paused(at) {
					t.Errorf("hour %d: %v, paused %v, the burst opening %v; want GIRO answering, never called down by stations known to hang", hour, a.Outcome, a.Paused(at), opened)
				}
				for _, code := range opened {
					if hanging[code] && hour > 1 {
						t.Errorf("hour %d opened with %s, a station known to hang; want it last", hour, code)
					}
				}
				if hour > 1 && liveRead < len(healthy) {
					t.Errorf("hour %d read %d of the %d stations that answer; want every one", hour, liveRead, len(healthy))
				}
				if hour > 1 && a.NotReached != 0 {
					t.Errorf("hour %d counted %d stations not reached, every answering one read; want the stations known to hang and the probe uncounted", hour, a.NotReached)
				}
			}
		})
	}
}
