package ionomaps_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// TestAFileThatCannotBeReadIsNamed is L-F5: a NOAA file fetched but not
// readable - a changed format - is named in the snapshot, the one held used
// in its place, never passed over in silence; the next update that reads
// it names it no more.
func TestAFileThatCannotBeReadIsNamed(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.drap, n.scales = []byte("not D-RAP's table"), []byte("{not the scales")
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(s.Inputs.Unread, "D-RAP") || !slices.Contains(s.Inputs.Unread, "scales") || slices.Contains(s.Inputs.Unread, "GloTEC") {
		t.Errorf("refused %v; want D-RAP and the scales named, GloTEC not", s.Inputs.Unread)
	}
	n.drap, n.scales = nil, nil
	n.mu.Lock()
	name := "glotec_icao_" + c.Now().Add(-15*time.Minute).UTC().Format("20060102T150405Z") + ".geojson"
	n.names, n.grids[name] = append(n.names, name), []byte("{not a grid")
	n.mu.Unlock()
	c.Add(10 * time.Minute)
	s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{})
	if !slices.Equal(s.Inputs.Unread, []string{"GloTEC"}) || s.Background.FoF2 != ionomaps.GloTEC {
		t.Errorf("refused %v, background %v; want GloTEC named and the grid held used", s.Inputs.Unread, s.Background.FoF2)
	}
	n.list(t, c.Now().Add(-5*time.Minute), 7)
	c.Add(10 * time.Minute)
	if s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{}); len(s.Inputs.Unread) != 0 {
		t.Errorf("after a readable grid, refused %v", s.Inputs.Unread)
	}
}

// TestTheCellsGloTECCouldNotGiveAreCounted is L-F5: where GloTEC gives no
// value the climatology fills the cell, and the snapshot counts the cells
// filled, so a host can say the field is partly the climatology's.
func TestTheCellsGloTECCouldNotGiveAreCounted(t *testing.T) {
	l, n, _ := library(t)
	name := n.list(t, start.Add(-30*time.Minute), 7)
	var doc map[string]any
	if err := json.Unmarshal(n.grids[name], &doc); err != nil {
		t.Fatal(err)
	}
	for _, f := range doc["features"].([]any) {
		feat := f.(map[string]any)
		at := feat["geometry"].(map[string]any)["coordinates"].([]any)
		if at[1].(float64) > 60 {
			feat["properties"].(map[string]any)["NmF2"] = 0.0 // no value north of 60°N
		}
	}
	n.grids[name], _ = json.Marshal(doc)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Background.FoF2 != ionomaps.GloTEC || s.Inputs.GloTECFilled == 0 || s.Inputs.GloTECFilled > 180*90/2 {
		t.Errorf("background %v, %d cells filled; want GloTEC with the cells north of about 60°N filled", s.Background.FoF2, s.Inputs.GloTECFilled)
	}
}

// TestGIROPausedIsSaid is L-F6 and watchpost's architecture ("GIRO
// refuses"): a burst GIRO refuses is said GIROPaused, and so is each update
// while the pause holds; after it, the reason is fresh again.
func TestGIROPausedIsSaid(t *testing.T) {
	l, n, c := giroLibrary(t)
	n.giro.refuse = 3
	update(t, l)
	if s := update(t, l); s.Reason != ionomaps.GIROPaused {
		t.Errorf("the refused burst's reason is %v", s.Reason)
	}
	c.Add(40 * time.Second)
	if s := update(t, l); s.Reason != ionomaps.GIROPaused && s.Reason != ionomaps.TooSoon {
		t.Errorf("within the pause the reason is %v", s.Reason)
	}
	c.Add(10 * time.Minute)
	if s := update(t, l); s.Reason != ionomaps.Fresh {
		t.Errorf("after the pause the reason is %v", s.Reason)
	}

	l, n, c = giroLibrary(t)
	n.giro.refuse, n.giro.retry = 3, "900" // GIRO asks for fifteen minutes
	update(t, l)
	update(t, l)
	c.Add(11 * time.Minute)
	if s := update(t, l); s.Reason != ionomaps.GIROPaused {
		t.Errorf("NOAA's update eleven minutes into a fifteen-minute pause: reason %v", s.Reason)
	}
}

// TestRejectedIsTheLastBurstsCount is L-F6: the rows refused are the last
// burst's, beside the readings it gave - not a count since the library was
// made.
func TestRejectedIsTheLastBurstsCount(t *testing.T) {
	l, n, c := giroLibrary(t)
	n.giro.score = map[string]string{"AL945": "150"} // a score that is no score is refused and counted (R-3.3)
	update(t, l)
	first := update(t, l)
	if first.Inputs.Rejected == 0 {
		t.Fatal("control: the low score was not counted")
	}
	c.Add(61 * time.Minute)
	update(t, l)
	second := update(t, l)
	if second.Inputs.Rejected != first.Inputs.Rejected {
		t.Errorf("the second burst's rejected %d; the first's %d, the same rows refused", second.Inputs.Rejected, first.Inputs.Rejected)
	}
}
