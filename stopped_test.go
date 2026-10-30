package ionomaps_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// stopAfter is NOAA's answers with the host's context ended once the ask
// named has been answered, while armed: a deadline passing, or a cancel,
// mid-update.
type stopAfter struct {
	*noaa
	url   string
	stop  context.CancelFunc
	armed bool
}

func (s *stopAfter) Fetch(ctx context.Context, url string, v ionomaps.Validators) (ionomaps.Response, error) {
	r, err := s.noaa.Fetch(ctx, url, v)
	if s.armed && url == s.url {
		s.stop()
	}
	return r, err
}

// stoppingLibrary is a library whose fetches end ctx after the ask named,
// once armed.
func stoppingLibrary(t *testing.T, url string, armed bool) (*ionomaps.Library, *noaa, *clock, context.Context, *stopAfter) {
	t.Helper()
	c := &clock{now: start}
	n := newNOAA(c)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := &stopAfter{noaa: n, url: url, stop: cancel, armed: armed}
	l, err := ionomaps.New(f, ionomaps.Options{Clock: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	return l, n, c, ctx, f
}

// TestAStoppedUpdateSaysWhy is L-F4: an update whose deadline passes, or
// that the host cancels, before any field can be made is no "too soon" and
// gives no time it never set: the error is ErrNoField wrapping the
// context's own, so a host tells a slow link from a closed mode, and the
// snapshot is said Unfinished.
func TestAStoppedUpdateSaysWhy(t *testing.T) {
	for name, c := range map[string]struct {
		ctx   func() (context.Context, context.CancelFunc)
		cause error
		words string
	}{
		"a deadline passed": {func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, context.DeadlineExceeded, "its deadline passed"},
		"the host cancelled": {func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, context.Canceled, "cancelled"},
	} {
		l, n, _ := library(t)
		n.list(t, start.Add(-30*time.Minute), 7)
		ctx, cancel := c.ctx()
		s, err := l.Update(ctx, ionomaps.UpdateOptions{})
		cancel()
		if !errors.Is(err, ionomaps.ErrNoField) || !errors.Is(err, c.cause) {
			t.Errorf("%s: the error %v; want ErrNoField wrapping %v", name, err, c.cause)
			continue
		}
		if words := err.Error(); !strings.Contains(words, c.words) || strings.Contains(words, "00:00:00") || strings.Contains(words, "too soon") {
			t.Errorf("%s: the error says %q", name, words)
		}
		if s.Reason != ionomaps.Unfinished || len(s.Now.FoF2.Values) != 0 {
			t.Errorf("%s: reason %v, %d cells", name, s.Reason, len(s.Now.FoF2.Values))
		}
	}
}

// TestTheSolarFileIsAskedFirst is L-F4: without the solar file there is no
// field at all, so it is asked before GloTEC's index and 2.5 MB grid; an
// update stopped once it is read still makes a field, the climatology's,
// named, and said Unfinished - and the next update asks again at once.
func TestTheSolarFileIsAskedFirst(t *testing.T) {
	l, n, c, ctx, _ := stoppingLibrary(t, solarURL, true)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := l.Update(ctx, ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if asked := n.take(); len(asked) == 0 || asked[0] != solarURL {
		t.Errorf("the first ask was %v; want the solar file", asked)
	}
	if s.Reason != ionomaps.Unfinished || s.Background.FoF2 != ionomaps.Climatology || len(s.Now.FoF2.Values) == 0 {
		t.Errorf("reason %v, background %v, %d cells; want the climatology's field, unfinished", s.Reason, s.Background.FoF2, len(s.Now.FoF2.Values))
	}
	c.Add(time.Second)
	s, err = l.Update(context.Background(), ionomaps.UpdateOptions{})
	if err != nil || s.Reason == ionomaps.TooSoon || s.Background.FoF2 != ionomaps.GloTEC {
		t.Errorf("the next update: %v, background %v (%v); want GloTEC's, asked at once", s.Reason, s.Background.FoF2, err)
	}
}

// TestAStoppedUpdateBuildsFromWhatIsHeld is L-F4: once a field is held, an
// update stopped part-way gives the snapshot built now from what is held,
// said Unfinished, with no error - never the last one said too soon.
func TestAStoppedUpdateBuildsFromWhatIsHeld(t *testing.T) {
	l, n, c, ctx, f := stoppingLibrary(t, indexURL, false)
	n.list(t, start.Add(-30*time.Minute), 7)
	if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	c.Add(11 * time.Minute)
	f.armed = true
	n.list(t, c.Now().Add(-25*time.Minute), 8)
	s, err := l.Update(ctx, ionomaps.UpdateOptions{})
	if err != nil || s.Reason != ionomaps.Unfinished || !s.Computed.Equal(c.Now()) {
		t.Errorf("reason %v, computed %v (%v); want unfinished, built now", s.Reason, s.Computed, err)
	}
	if r, _ := s.Readings([]ionomaps.LatLon{{Lat: 0, Lon: 0}}); r[0].FoF2 > 7.5 {
		t.Errorf("foF2 %v: the grid stopped before it arrived was used", r[0].FoF2)
	}
}

// TestTheAnswersSayWhenNoFieldIsHeld is L-F4 for the answers: a snapshot
// with no field refuses with ErrNoField.
func TestTheAnswersSayWhenNoFieldIsHeld(t *testing.T) {
	var s ionomaps.Snapshot
	if _, err := s.Readings([]ionomaps.LatLon{{Lat: 0, Lon: 0}}); !errors.Is(err, ionomaps.ErrNoField) {
		t.Errorf("Readings: %v", err)
	}
	if _, err := s.Ahead(1); !errors.Is(err, ionomaps.ErrNoField) {
		t.Errorf("Ahead: %v", err)
	}
}
