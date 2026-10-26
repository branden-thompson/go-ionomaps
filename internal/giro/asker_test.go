package giro

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

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

// TestABurstEndsBeforeTheHostsDeadline is watchpost D-291: before each ask
// the burst reads the time left on the host's update and ends while one
// more ask's askTimeout would not fit - GIRO called down if its last ask ran
// out or three did, otherwise the stations not reached first next hour -
// so it never runs into the host's deadline. Times are scaled: askTimeout
// 200 ms, the host's deadline 700 ms, an answer 10 ms or 40 ms.
func TestABurstEndsBeforeTheHostsDeadline(t *testing.T) {
	was := askTimeout
	askTimeout = 200 * time.Millisecond
	defer func() { askTimeout = was }()
	type answerFunc func(n int, code string, ctx context.Context) (int, string, []byte, error)
	run := func(t *testing.T, a *Asker, at time.Time, answer answerFunc) (asked []string, beat bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
		defer cancel()
		a.Burst(ctx, at, func(ctx context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			asked = append(asked, code)
			return answer(len(asked), code, ctx)
		})
		return asked, ctx.Err() == nil
	}
	answerIn := func(d time.Duration) (int, string, []byte, error) {
		time.Sleep(d)
		return 200, "", nil, nil
	}
	t.Run("a hang after 20 answers", func(t *testing.T) {
		var a Asker
		asked, beat := run(t, &a, now, func(n int, _ string, ctx context.Context) (int, string, []byte, error) {
			if n <= 20 {
				return answerIn(10 * time.Millisecond)
			}
			return hang(ctx)
		})
		if !beat || a.Outcome != Unavailable || !a.Paused(now) || len(asked) > 23 {
			t.Errorf("%d asked, before the deadline %v, %v, paused %v; want GIRO called down before the host's deadline", len(asked), beat, a.Outcome, a.Paused(now))
		}
	})
	t.Run("hangs alternating with a server error", func(t *testing.T) {
		var a Asker
		_, beat := run(t, &a, now, func(n int, _ string, ctx context.Context) (int, string, []byte, error) {
			switch {
			case n == 1:
				return 200, "", nil, nil
			case n%2 == 0:
				return hang(ctx)
			}
			return 500, "", nil, nil
		})
		if !beat || a.Outcome != Unavailable || !a.Paused(now) {
			t.Errorf("before the deadline %v, %v, paused %v; want GIRO called down, though never three hangs in a row", beat, a.Outcome, a.Paused(now))
		}
	})
	t.Run("slow answers, no hang", func(t *testing.T) {
		var a Asker
		asked, beat := run(t, &a, now, func(int, string, context.Context) (int, string, []byte, error) {
			return answerIn(40 * time.Millisecond)
		})
		if !beat || a.Outcome != Answered || a.Paused(now) || len(asked) >= 30 {
			t.Fatalf("%d asked, before the deadline %v, %v, paused %v; want the burst cut short, Answered, nothing paused", len(asked), beat, a.Outcome, a.Paused(now))
		}
		next, _ := run(t, &a, now.Add(time.Hour), func(int, string, context.Context) (int, string, []byte, error) { return 200, "", nil, nil })
		if len(next) == 0 || slices.Contains(asked, next[0]) {
			t.Errorf("the next burst began at %v, a station already asked; want one not reached", next[:min(1, len(next))])
		}
	})
	t.Run("three hanging stations", func(t *testing.T) {
		// Three station hosts that hang look like GIRO hanging, whichever
		// way the rotation orders them: GIRO is called down each hour, the
		// other stations still asked first (D-291's counter-argument).
		var a Asker
		var trio []string
		for hour := range 3 {
			at := now.Add(time.Duration(hour) * time.Hour)
			asked, beat := run(t, &a, at, func(_ int, code string, ctx context.Context) (int, string, []byte, error) {
				if hour == 0 && len(trio) < failuresInARow {
					trio = append(trio, code)
				}
				if slices.Contains(trio, code) {
					return hang(ctx)
				}
				return 200, "", nil, nil
			})
			if !beat || a.Outcome != Unavailable || (hour > 0 && len(asked) <= failuresInARow) {
				t.Errorf("hour %d: %d asked, before the deadline %v, %v; want the others asked and GIRO called down, inside the deadline", hour, len(asked), beat, a.Outcome)
			}
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
