package giro

import (
	"context"
	"errors"
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
		<-ctx.Done() // GIRO never answers
		return 0, "", nil, ctx.Err()
	})
	if took := time.Since(started); asked != failuresInARow || a.Outcome != Unavailable || !a.Paused(now) || took > 10*askTimeout*failuresInARow {
		t.Errorf("a hung GIRO: %d asked in %v, %v, paused %v; want %d asks, each ended at its timeout, and Unavailable", asked, took, a.Outcome, a.Paused(now), failuresInARow)
	}
	if failuresInARow*was >= 60*time.Second {
		t.Errorf("%d asks of %v do not fit a 60-second update", failuresInARow, was)
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
