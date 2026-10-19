package ionomaps_test

import (
	"slices"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// TestAGloTECOutageIsSaidAndTheStationsStillAsked is watchpost D-239 ("we
// should never have data mislabelled"): GloTEC's index unanswered while
// NOAA's other files and GIRO answer. The update names GloTEC unread and is
// never said Offline, since NOAA's other files came back; GIRO, a separate
// service, is still asked, so the stations correct the climatology; and the
// snapshot is Partial, never Fresh, since its foF2 background is not new.
func TestAGloTECOutageIsSaidAndTheStationsStillAsked(t *testing.T) {
	for _, down := range []struct {
		name string
		set  func(n *noaa)
	}{
		{"no answer", func(n *noaa) { n.down[indexURL] = true }},
		{"a 503", func(n *noaa) { n.status[indexURL] = 503 }},
	} {
		l, n, _ := library(t) // no grid ever held: the climatology is the background
		n.giro.speak = true
		down.set(n)
		first := update(t, l)
		if first.Reason != ionomaps.StationsNext || !slices.Contains(first.Inputs.Unread, "GloTEC") {
			t.Errorf("%s: the first update said %v, unread %v; want GIRO asked next and GloTEC named", down.name, first.Reason, first.Inputs.Unread)
		}
		s := update(t, l)
		if s.Inputs.Stations == 0 || s.Background.FoF2 != ionomaps.Climatology {
			t.Errorf("%s: after the burst, %d stations over %v; want the stations on the climatology", down.name, s.Inputs.Stations, s.Background.FoF2)
		}
		if s.Reason != ionomaps.Partial || !slices.Contains(s.Inputs.Unread, "GloTEC") {
			t.Errorf("%s: after the burst, said %v, unread %v; want Partial with GloTEC named", down.name, s.Reason, s.Inputs.Unread)
		}
	}
}

// TestOfflineIsSaidOnlyWhenNothingCameBack is watchpost D-239: an update is
// said Offline only when no NOAA file it asked for came back and GIRO was
// not due; with any file read and the rest named unread, it is Partial.
func TestOfflineIsSaidOnlyWhenNothingCameBack(t *testing.T) {
	l, n, c := giroLibrary(t)
	update(t, l)
	update(t, l) // the burst: GIRO is not due again this hour
	c.Add(11 * time.Minute)
	n.down[indexURL], n.down[drapURL], n.down[scalesURL] = true, true, true
	if s := update(t, l); s.Reason != ionomaps.Offline {
		t.Errorf("nothing came back, GIRO not due: said %v; want Offline", s.Reason)
	}
	l, n, c = giroLibrary(t)
	update(t, l)
	update(t, l)
	c.Add(11 * time.Minute)
	n.status[indexURL] = 404 // answered, not read: D-RAP's and the scales' asks follow, and come back
	if s := update(t, l); s.Reason != ionomaps.Partial || !slices.Equal(s.Inputs.Unread, []string{"GloTEC"}) {
		t.Errorf("GloTEC not found, the rest read: said %v, unread %v; want Partial, GloTEC alone named", s.Reason, s.Inputs.Unread)
	}
}
