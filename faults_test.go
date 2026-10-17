package ionomaps_test

import (
	"context"
	"slices"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// TestEveryFaultIsReported is G10.2 (G-M4): each input - GloTEC's grid, the
// solar file, D-RAP's table, NOAA's scales, GIRO's readings - stopped,
// stale, truncated and reformatted, one at a time, the others as served;
// every case is said in the snapshot or the error - never a field that
// looks fresh and whole.
func TestEveryFaultIsReported(t *testing.T) {
	half := func(b []byte) []byte { return b[:len(b)/2] }
	refused := func(name string) func(ionomaps.Snapshot, error) bool {
		return func(s ionomaps.Snapshot, _ error) bool { return slices.Contains(s.Inputs.Refused, name) }
	}
	noGloTEC := func(s ionomaps.Snapshot, _ error) bool {
		return s.Background.FoF2 == ionomaps.Climatology && s.Inputs.GloTECValid.IsZero()
	}
	noField := func(_ ionomaps.Snapshot, err error) bool { return err != nil }
	noDRAP := func(s ionomaps.Snapshot, _ error) bool {
		return s.Inputs.DRAPValid.IsZero() && len(s.Disturbance.Values) == 0
	}
	noScales := func(s ionomaps.Snapshot, _ error) bool {
		return s.Scales.Valid.IsZero() && s.Scales.R == ionomaps.ScaleUnset
	}
	noStations := func(s ionomaps.Snapshot, _ error) bool { return s.Inputs.Stations == 0 }
	for _, c := range []struct {
		input, fault string
		set          func(n *noaa, c *clock)
		said         func(ionomaps.Snapshot, error) bool
	}{
		{"GloTEC", "stopped", func(n *noaa, _ *clock) { n.down[indexURL] = true }, noGloTEC},
		{"GloTEC", "stale", func(n *noaa, c *clock) { n.list(t, c.Now().Add(-2*time.Hour), 7) }, noGloTEC},
		{"GloTEC", "truncated", func(n *noaa, c *clock) {
			name := n.list(t, c.Now().Add(-30*time.Minute), 7)
			n.grids[name] = half(n.grids[name])
		}, refused("GloTEC")},
		{"GloTEC", "reformatted", func(n *noaa, c *clock) {
			name := n.list(t, c.Now().Add(-30*time.Minute), 7)
			n.grids[name] = []byte(`{"type":"Feature"}`)
		}, refused("GloTEC")},
		{"solar", "stopped", func(n *noaa, _ *clock) { n.down[solarURL] = true }, noField},
		{"solar", "stale", func(n *noaa, c *clock) { n.solar = solarFile(c.Now().AddDate(0, 0, -60), 30, 140) }, noField},
		{"solar", "truncated", func(n *noaa, _ *clock) { n.solar = half(n.solar)[:20] }, noField},
		{"solar", "reformatted", func(n *noaa, _ *clock) { n.solar = []byte("<html>moved</html>") }, noField},
		{"D-RAP", "stopped", func(n *noaa, _ *clock) { n.drap = nil }, noDRAP},
		{"D-RAP", "stale", func(n *noaa, c *clock) { n.drap = drapTable(c.Now().Add(-2*time.Hour), 3) }, noDRAP},
		{"D-RAP", "truncated", func(n *noaa, c *clock) { n.drap = half(drapTable(c.Now().Add(-3*time.Minute), 3)) }, refused("D-RAP")},
		{"D-RAP", "reformatted", func(n *noaa, _ *clock) { n.drap = []byte("# a page that moved\n") }, refused("D-RAP")},
		{"scales", "stopped", func(n *noaa, _ *clock) { n.scales = nil }, noScales},
		{"scales", "stale", func(n *noaa, c *clock) { n.scales = scalesFile(c.Now().Add(-2*time.Hour), `"1"`, `"0"`, `"3"`) }, noScales},
		{"scales", "truncated", func(n *noaa, c *clock) { n.scales = half(scalesFile(c.Now().Add(-2*time.Minute), `"1"`, `"0"`, `"3"`)) }, refused("scales")},
		{"scales", "reformatted", func(n *noaa, _ *clock) { n.scales = []byte(`["moved"]`) }, refused("scales")},
	} {
		l, n, clk := library(t)
		if c.input != "GloTEC" {
			n.list(t, start.Add(-30*time.Minute), 7)
		}
		c.set(n, clk)
		s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
		if !c.said(s, err) {
			t.Errorf("%s %s: not said - reason %v, refused %v, background %v, error %v", c.input, c.fault, s.Reason, s.Inputs.Refused, s.Background.FoF2, err)
		}
		if err == nil && s.Reason == ionomaps.Fresh && c.input == "GloTEC" && c.fault == "stopped" {
			t.Errorf("GloTEC stopped: said fresh")
		}
	}
	for _, c := range []struct {
		fault string
		set   func(n *noaa, c *clock)
		said  func(ionomaps.Snapshot, error) bool
	}{
		{"stopped", func(n *noaa, _ *clock) { n.giro.refuse = 1 }, func(s ionomaps.Snapshot, _ error) bool {
			return s.Reason == ionomaps.GIROPaused && s.Inputs.Stations == 0
		}},
		{"truncated", func(n *noaa, _ *clock) { n.giro.truncate = true }, func(s ionomaps.Snapshot, _ error) bool { return s.Inputs.Rejected > 0 || s.Inputs.Stations == 0 }},
		{"reformatted", func(n *noaa, _ *clock) { n.giro.reformat = true }, noStations},
	} {
		l, n, clk := giroLibrary(t)
		c.set(n, clk)
		update(t, l)
		if s := update(t, l); !c.said(s, nil) {
			t.Errorf("GIRO %s: not said - reason %v, %d stations, %d rejected", c.fault, s.Reason, s.Inputs.Stations, s.Inputs.Rejected)
		}
	}
	l, n, clk := giroLibrary(t) // stale: the readings held age past the hour while GIRO goes quiet
	update(t, l)
	if s := update(t, l); s.Inputs.Stations == 0 {
		t.Fatal("control: the burst assimilated no station")
	}
	n.giro.speak = false
	clk.Add(70 * time.Minute)
	update(t, l)
	if s := update(t, l); s.Inputs.Stations != 0 || !s.Inputs.OldestReading.IsZero() {
		t.Errorf("GIRO stale: %d stations assimilated, the oldest at %v; readings over an hour old are not (D-64)", s.Inputs.Stations, s.Inputs.OldestReading)
	}
}
