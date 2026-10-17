package ionomaps_test

import (
	"context"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// asksSoFar is every ask the fake sources have had: NOAA's, D-RAP's, the
// scales' and GIRO's.
func asksSoFar(n *noaa) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.asked) + len(n.drapAsks) + n.scalesAsks + len(n.giro.asks)
}

// TestTheAnswersFetchNothing is R-3.5: the answers read the snapshot held -
// a counting fetcher sees no ask from At, Bands, Path or Reach.
func TestTheAnswersFetchNothing(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before := asksSoFar(n)
	if before == 0 {
		t.Fatal("control: the update asked nothing, so the count measures nothing")
	}
	here, there := ionomaps.LatLon{Lat: 40, Lon: -100}, ionomaps.LatLon{Lat: 51, Lon: 0}
	now := c.Now()
	if _, err := s.At([]ionomaps.LatLon{here, there}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Bands([]ionomaps.LatLon{here}, 400, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Path(here, []ionomaps.LatLon{there}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reach(here, 14.2, now); err != nil {
		t.Fatal(err)
	}
	if after := asksSoFar(n); after != before {
		t.Errorf("the answers asked %d times", after-before)
	}
}

// TestAHeldSnapshotIsUnchangedByTheNextUpdate is P-6 (R-5.4): a snapshot
// the host holds keeps its fields when the next update brings another grid.
func TestAHeldSnapshotIsUnchangedByTheNextUpdate(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	held, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	const cell = 4000
	was, computed := held.Hours[0].FoF2.Values[cell], held.Computed
	c.Add(10 * time.Minute)
	n.list(t, c.Now().Add(-2*time.Minute), 11)
	next, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if next.Hours[0].FoF2.Values[cell] == was {
		t.Fatalf("control: the next grid did not change the cell (%v), so this proves nothing", was)
	}
	if held.Hours[0].FoF2.Values[cell] != was || !held.Computed.Equal(computed) {
		t.Errorf("the held snapshot changed: foF2 %v (was %v), computed %v", held.Hours[0].FoF2.Values[cell], was, held.Computed)
	}
}

// TestAStaleInputIsSaidStale is R-3.1: a GloTEC grid past its hour is not
// drawn as current - the snapshot names the climatology for foF2 and holds
// no grid time - and the solar mean says how old it is.
func TestAStaleInputIsSaidStale(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil || s.Background.FoF2 != ionomaps.GloTEC {
		t.Fatalf("control: the first snapshot's foF2 is %v (%v); GloTEC's", s.Background.FoF2, err)
	}
	c.Add(2 * time.Hour) // NOAA's index lists no newer grid
	s, err = settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Background.FoF2 != ionomaps.Climatology || !s.Inputs.GloTECValid.IsZero() {
		t.Errorf("a grid two and a half hours old: foF2 from %v, valid %v; the climatology, named, and no grid time", s.Background.FoF2, s.Inputs.GloTECValid)
	}
	if s.Inputs.F107Age <= 0 {
		t.Errorf("the solar mean's age is %v; said", s.Inputs.F107Age)
	}
}
