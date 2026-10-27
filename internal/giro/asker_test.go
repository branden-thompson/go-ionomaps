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
	if asked != 1 || a.BurstDue(now) || a.Failed != 0 {
		t.Errorf("%d asks, %d missed; GIRO due after a burst that asked: %v; want the hour spent and the cancel no miss", asked, a.Failed, a.BurstDue(now))
	}
}

// TestAReplyIsHeldOnlyForItsStation is R-5.2: a reply is held under the
// station it says it is, never another's code.
func TestAReplyIsHeldOnlyForItsStation(t *testing.T) {
	var a Asker
	a.list, _ = stations.Seed()
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
// kept for three days and let go after; without the time, nothing is
// judged stale.
func TestAReadingIsLetGoAfterThreeDays(t *testing.T) {
	var a Asker
	a.list, _ = stations.Seed()
	at, _ := a.list.At("AS00Q")
	a.hold("AS00Q", Reply{Station: Station{Code: "AS00Q", Lat: at.Lat, Lon: at.Lon}, Readings: []Reading{{At: now, FoF2: 7}, {At: now.Add(-time.Hour), FoF2: 6}}})
	if r := a.Readings(); len(r) != 1 || !r[0].Reading.At.Equal(now) {
		t.Fatalf("held %+v; want the latest reading alone", r)
	}
	a.LetGo(time.Time{})
	a.LetGo(now.Add(71 * time.Hour))
	if len(a.Readings()) != 1 {
		t.Error("a reading was let go before three days")
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

// withData is a reply holding one reading for a station, taken five
// minutes before at, at the place the seed gives it - a reply hold keeps -
// or a 404 for a station the seed does not place, as most of those give no
// data. It is built from pieces, as giro_test.go's header is, so no
// signature of a reply stands whole in the source.
func withData(code string, at time.Time) (int, string, []byte, error) {
	list, _ := stations.Seed()
	p, placed := list.At(code)
	if !placed {
		return 404, "", nil, nil
	}
	ns, lat, lon := "N", p.Lat, p.Lon
	if lat < 0 {
		ns, lat = "S", -lat
	}
	if lon < 0 {
		lon += 360 // GIRO writes longitudes east, 0 to 360
	}
	body := "# Location: " + "GEO" + fmt.Sprintf(" ( %.1f %s   %.1f E ), URSI-Code %s, A STATION\n", lat, ns, lon, code) +
		"# Time                    " + "CS" + "   foF2 " + "QD M(D) QD\n" +
		at.Add(-5*time.Minute).UTC().Format("2006-01-02T15:04:05.000Z") + "  95  7.500 // 3.40 //\n"
	return 200, "", []byte(body), nil
}

// noReading is GIRO's reply with no reading in it: the header alone.
func noReading() (int, string, []byte, error) {
	return 200, "", []byte("# Location: " + "GEO" + " ( 45.1 N   276.4 E ), URSI-Code AL945, A STATION\n# Time                    " + "CS" + "   foF2 " + "QD M(D) QD\n"), nil
}

// withDataAt answers every ask with a reading taken just before at.
func withDataAt(at time.Time) func(context.Context, string) (int, string, []byte, error) {
	return func(_ context.Context, code string) (int, string, []byte, error) { return withData(code, at) }
}

// TestAStationThatGivesNoDataIsAMiss is the update's timing model's rule 2
// (watchpost D-247, D-272, D-316): a server error, a 404, a 410, a status
// no rule names, no answer, a timeout, a reply with no reading, or one GIRO
// does not write, is that station's miss - counted, and the burst goes on
// to every other - never GIRO paused.
func TestAStationThatGivesNoDataIsAMiss(t *testing.T) {
	was := askTimeout
	askTimeout = 20 * time.Millisecond
	defer func() { askTimeout = was }()
	for name, miss := range map[string]func(context.Context) (int, string, []byte, error){
		"a server error": func(context.Context) (int, string, []byte, error) { return 503, "", nil, nil },
		"a 404":          func(context.Context) (int, string, []byte, error) { return 404, "", nil, nil },
		"a 410":          func(context.Context) (int, string, []byte, error) { return 410, "", nil, nil },
		"a 451":          func(context.Context) (int, string, []byte, error) { return 451, "", nil, nil },
		"a redirect":     func(context.Context) (int, string, []byte, error) { return 302, "", nil, nil },
		"no answer":      func(context.Context) (int, string, []byte, error) { return 0, "", nil, errors.New("no route") },
		"a timeout":      hang,
		"no reply":       func(context.Context) (int, string, []byte, error) { return 200, "", nil, nil },
		"no reading":     func(context.Context) (int, string, []byte, error) { return noReading() },
		"not GIRO's reply": func(context.Context) (int, string, []byte, error) {
			return 200, "", []byte("<html>moved</html>\n"), nil
		},
	} {
		var a Asker
		var asked []string
		a.Burst(context.Background(), now, func(ctx context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			asked = append(asked, code)
			if len(asked) == 1 {
				return miss(ctx)
			}
			return withData(code, now)
		})
		if len(asked) != throttle.MaxBurst || a.Failed != 2 || a.Paused(now) || a.Outcome != Answered || len(a.Readings()) != throttle.MaxBurst-2 {
			t.Errorf("%s: %d asked, %d missed, %d held, paused %v, %v; want all %d asked, two misses - it and the one flagged station - the rest held, no pause", name, len(asked), a.Failed, len(a.Readings()), a.Paused(now), a.Outcome, throttle.MaxBurst)
		}
	}
}

// TestNothingAStationDoesStopsTheBurst is the model's rule 2 (watchpost
// D-316): with every station failing - server errors and hangs side by
// side, before any answer - the burst still asks every one, counts each a
// miss, and never says GIRO is unavailable.
func TestNothingAStationDoesStopsTheBurst(t *testing.T) {
	was := askTimeout
	askTimeout = 10 * time.Millisecond
	defer func() { askTimeout = was }()
	var a Asker
	asked := 0
	a.Burst(context.Background(), now, func(ctx context.Context, _ string) (int, string, []byte, error) {
		asked++
		if asked%2 == 0 {
			return hang(ctx)
		}
		return 500, "", nil, nil
	})
	if asked != throttle.MaxBurst || a.Failed != throttle.MaxBurst || a.Paused(now) || a.Outcome != Answered {
		t.Errorf("every station failing: %d asked, %d missed, paused %v, %v; want all %d asked and missed, GIRO not called down", asked, a.Failed, a.Paused(now), a.Outcome, throttle.MaxBurst)
	}
}

// hang answers nothing until the ask's context ends - or a guard of 2 s,
// when the ask had no timeout.
func hang(ctx context.Context) (int, string, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, "", nil, ctx.Err()
	case <-time.After(2 * time.Second):
		return 0, "", nil, errors.New("the guard ran out: the ask had no timeout")
	}
}

// TestAStationMissingThreeInARowIsAskedLessUntilItReturns is the model's
// rules 1 and 3 (watchpost D-316, D-317) over hours of bursts: a station
// that misses three hours running is flagged - asked again only 6 hours
// after its last ask, behind every other - and one reading puts it back in
// the rotation.
func TestAStationMissingThreeInARowIsAskedLessUntilItReturns(t *testing.T) {
	var a Asker
	list, _ := stations.Seed()
	down := list.Plan(now, throttle.MaxBurst)[3]
	returned := false
	askedIn := map[int]int{} // hour: the station's place in the burst
	for hour := range 12 {
		at := now.Add(time.Duration(hour) * time.Hour)
		n := 0
		a.Burst(context.Background(), at, func(_ context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			n++
			if code != down {
				return withData(code, at)
			}
			askedIn[hour] = n
			if returned {
				return withData(code, at)
			}
			return 503, "", nil, nil
		})
		returned = returned || hour == 7
	}
	for hour := range 12 {
		_, asked := askedIn[hour]
		want := hour < 3 || hour >= 8
		if asked != want {
			t.Errorf("hour %d: the station down asked %v; want %v - each hour until three misses, then 6 hours after its last, then each hour once it answers", hour, asked, want)
		}
	}
	if askedIn[8] < throttle.MaxBurst-1 {
		t.Errorf("flagged and due, the station was asked %dth; want behind every station not flagged", askedIn[8])
	}
}

// TestARefusalStopsTheBurst is the model's rule 4 (watchpost D-39, D-247,
// D-318): a 429 stops the burst and backs GIRO off, said Unavailable; a
// 401 or 403 stops it and leaves GIRO alone a day, said Denied, across a
// restart too - never an answer that resets the back-off.
func TestARefusalStopsTheBurst(t *testing.T) {
	var refused Asker
	asked := 0
	refused.Burst(context.Background(), now, func(context.Context, string) (int, string, []byte, error) {
		asked++
		return 429, "120", nil, nil
	})
	if asked != 1 || refused.Outcome != Unavailable || !refused.Paused(now.Add(time.Minute)) {
		t.Errorf("a 429: %d asked, %v, paused %v; want one ask, Unavailable and backed off", asked, refused.Outcome, refused.Paused(now.Add(time.Minute)))
	}
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
	a.list, _ = stations.Seed()
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
// 150 ms, its margin 10 ms - for a test of the end of the host's update,
// whose deadline the test scales alike: 600 ms for 60 s.
func scaledAsks(t *testing.T) {
	t.Helper()
	full, margin := askTimeout, deadline.Margin
	askTimeout, deadline.Margin = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { askTimeout, deadline.Margin = full, margin })
}

// burstUnder is one burst under a host's update of the given length: the
// stations asked, in order, and whether the burst ended before the update's
// deadline, each ask checked to start with its whole time and margin left.
func burstUnder(t *testing.T, a *Asker, at time.Time, update time.Duration, answer func(ctx context.Context, code string) (int, string, []byte, error)) (asked []string, beat bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), update)
	defer cancel()
	end, _ := ctx.Deadline()
	a.Burst(ctx, at, func(ctx context.Context, u string) (int, string, []byte, error) {
		if left := time.Until(end); left < askTimeout+deadline.Margin-5*time.Millisecond {
			t.Errorf("an ask began with %v left; none starts without %v", left, askTimeout+deadline.Margin)
		}
		code := codeOf(t, u)
		asked = append(asked, code)
		return answer(ctx, code)
	})
	return asked, ctx.Err() == nil
}

// TestABurstEndsInsideItsDeadline is the model's rule 5 (watchpost D-298,
// D-301): every ask is whole or none, so a GIRO hanging on every ask is
// asked three times in a 60 s update and the burst ends before its
// deadline, nothing called down; the next hour carries on from the station
// after the last asked; an update with no room for an ask asks nothing,
// spends no hour and keeps the last burst's counts. Times are a hundredth
// of their own.
func TestABurstEndsInsideItsDeadline(t *testing.T) {
	scaledAsks(t)
	var a Asker
	first, beat := burstUnder(t, &a, now, 600*time.Millisecond, func(ctx context.Context, _ string) (int, string, []byte, error) { return hang(ctx) })
	if len(first) != 3 || !beat || a.Failed != 3 || a.Outcome != Answered || a.Paused(now) {
		t.Fatalf("GIRO hung: %d asked, before the deadline %v, %d missed, %v, paused %v; want three asks inside it, each a miss, nothing called down", len(first), beat, a.Failed, a.Outcome, a.Paused(now))
	}
	plan, _ := stations.Seed()
	order := plan.Plan(now, throttle.MaxBurst)
	next := now.Add(time.Hour)
	second, _ := burstUnder(t, &a, next, 600*time.Millisecond, withDataAt(next))
	if len(second) != throttle.MaxBurst || second[0] != order[3] {
		t.Errorf("the next hour asked %d, from %s; want all, carrying on from %s", len(second), second[0], order[3])
	}
	held := a.Failed
	later := now.Add(2 * time.Hour)
	if asked, _ := burstUnder(t, &a, later, 100*time.Millisecond, withDataAt(later)); len(asked) != 0 || !a.NoRoom || !a.BurstDue(later) || a.Failed != held {
		t.Errorf("no room for an ask: %d asked, no room %v, still due %v, counts %d; want nothing asked, the hour not spent, the last counts kept", len(asked), a.NoRoom, a.BurstDue(later), a.Failed)
	}
	if burstUnder(t, &a, later, 600*time.Millisecond, withDataAt(later)); a.NoRoom {
		t.Error("a burst with room still says the last one's no room")
	}
}

// TestAGIROThatStopsAnsweringIsReadAgainWhenItReturns is the model's
// limits (watchpost D-316): a GIRO hanging on every ask for two days costs
// each hour three timeouts inside the deadline, never a burst run past it
// and never GIRO said unavailable; when it answers again, every station
// placed is read within 6 hours. Times are a hundredth of their own.
func TestAGIROThatStopsAnsweringIsReadAgainWhenItReturns(t *testing.T) {
	scaledAsks(t)
	var a Asker
	for hour := range 48 {
		at := now.Add(time.Duration(hour) * time.Hour)
		asked, beat := burstUnder(t, &a, at, 600*time.Millisecond, func(ctx context.Context, _ string) (int, string, []byte, error) { return hang(ctx) })
		if len(asked) != 3 || !beat || a.Outcome != Answered || a.Paused(at) {
			t.Fatalf("hour %d of the outage: %d asked, before the deadline %v, %v, paused %v; want three asks inside it, nothing called down", hour, len(asked), beat, a.Outcome, a.Paused(at))
		}
	}
	seed, _ := stations.Seed()
	placed := seed.Plan(now, 39)
	read := map[string]int{}
	for hour := 48; hour < 48+7; hour++ {
		at := now.Add(time.Duration(hour) * time.Hour)
		burstUnder(t, &a, at, 600*time.Millisecond, func(_ context.Context, code string) (int, string, []byte, error) {
			if _, seen := read[code]; !seen {
				read[code] = hour
			}
			return withData(code, at)
		})
	}
	later := 0
	for _, code := range placed {
		h, seen := read[code]
		if !seen || h > 48+6 {
			t.Errorf("%s, placed, read in hour %d (%v); want within 6 hours of GIRO's return", code, h, seen)
		}
		if h > 48 {
			later++
		}
	}
	if later == 0 {
		t.Errorf("control: every placed station was read the first hour back; want the outage to have flagged some")
	}
}

// TestTheFlagsSurviveARestart is watchpost D-213: a station flagged, and
// when it was last asked, come back with the state, so a restart does not
// ask it again before it is due.
func TestTheFlagsSurviveARestart(t *testing.T) {
	var a Asker
	list, _ := stations.Seed()
	down := list.Plan(now, throttle.MaxBurst)[0]
	for hour := range 3 {
		at := now.Add(time.Duration(hour) * time.Hour)
		a.Burst(context.Background(), at, func(_ context.Context, u string) (int, string, []byte, error) {
			code := codeOf(t, u)
			if code == down {
				return 503, "", nil, nil
			}
			return withData(code, at)
		})
	}
	var back Asker
	restart := now.Add(3 * time.Hour)
	if err := back.Import(a.Export(), restart); err != nil {
		t.Fatal(err)
	}
	var asked []string
	back.Burst(context.Background(), restart, func(_ context.Context, u string) (int, string, []byte, error) {
		code := codeOf(t, u)
		asked = append(asked, code)
		return withData(code, restart)
	})
	if slices.Contains(asked, down) || len(asked) == 0 {
		t.Errorf("after a restart, %s, flagged an hour before, asked again %v (%d asked); want it left until due", down, slices.Contains(asked, down), len(asked))
	}
}
