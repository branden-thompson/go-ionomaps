package ionomaps_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

const drapURL = "https://services.swpc.noaa.gov/text/drap_global_frequencies.txt"

// drapTable is a D-RAP table valid at a time, one frequency everywhere,
// in the layout NOAA's server writes, built here (R-4.3).
func drapTable(valid time.Time, mhz float64) []byte {
	var b strings.Builder
	b.WriteString("# Product Valid At : " + valid.UTC().Format("2006-01-02 15:04") + " UTC\n#\n    ")
	for lon := -178; lon <= 178; lon += 4 {
		fmt.Fprintf(&b, " %4d", lon)
	}
	b.WriteString("\n" + strings.Repeat("-", 456) + "\n")
	for lat := 89; lat >= -89; lat -= 2 {
		fmt.Fprintf(&b, "%3d |%s\n", lat, strings.Repeat(fmt.Sprintf(" %4.1f", mhz), 90))
	}
	return []byte(b.String())
}

// TestDRAPIsFetchedAndCarried is R-2.5 and R-5.7: each update asks D-RAP
// after NOAA's grid and solar file, with the validators of the table held;
// the snapshot carries its 1 dB frequency a cell and its valid time, and the
// bands under it are disturbed; a table over an hour old, or none, leaves
// no disturbance, said by a zero valid time.
func TestDRAPIsFetchedAndCarried(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.drap = drapTable(start.Add(-3*time.Minute), 3)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.drapAsks) == 0 || !s.Inputs.DRAPValid.Equal(start.Add(-3*time.Minute)) || s.Disturbance.Values[100] != 3 {
		t.Fatalf("D-RAP asked %d times; valid %v, a cell %v MHz; want asked, its time, 3", len(n.drapAsks), s.Inputs.DRAPValid, s.Disturbance.Values[100])
	}
	a, _ := s.Bands([]ionomaps.LatLon{{Lat: 40, Lon: -100}}, 400, s.Computed)
	if a[0].Bands[0].Status != ionomaps.Disturbed || a[0].Bands[1].Status != ionomaps.Open {
		t.Errorf("D-RAP at 3 MHz: 160 m %v, 80 m %v; want disturbed and open", a[0].Bands[0].Status, a[0].Bands[1].Status)
	}
	s.Disturbance.Values[100] = 99
	if again, _ := l.Update(context.Background(), ionomaps.UpdateOptions{}); again.Disturbance.Values[100] != 3 {
		t.Error("a change to one snapshot's disturbance reached the next")
	}
	n.drapAsks = nil
	c.Add(10 * time.Minute)
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	if len(n.drapAsks) != 1 || n.drapValidators[len(n.drapValidators)-1].ETag != `"drap-1"` {
		t.Errorf("the next update asked D-RAP %d times, with %+v; want once, with the table's validators", len(n.drapAsks), n.drapValidators)
	}
	n.drap = nil // no answer from now on
	c.Add(61 * time.Minute)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{})
	if !s.Inputs.DRAPValid.IsZero() || s.Disturbance.Cols != 0 {
		t.Errorf("D-RAP's table over an hour old is still used: valid %v", s.Inputs.DRAPValid)
	}
}

// TestAnUnreadableDRAPIsNoRefusal is R-3.1: a table that cannot be read
// leaves the one held and backs nothing off - the next ask waits its ten
// minutes, not a refusal's 60 s.
func TestAnUnreadableDRAPIsNoRefusal(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.drap = drapTable(start.Add(-3*time.Minute), 3)
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	c.Add(10 * time.Minute)
	n.drap = []byte("# not a table\n")
	s, _ := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if !s.Inputs.DRAPValid.Equal(start.Add(-3 * time.Minute)) {
		t.Errorf("an unreadable table dropped the one held: valid %v", s.Inputs.DRAPValid)
	}
	n.take()
	c.Add(61 * time.Second)
	if s, _ := settled(l, context.Background(), ionomaps.UpdateOptions{}); s.Early != ionomaps.TooSoon || len(n.take()) != 0 {
		t.Errorf("61 s after an unreadable table the update is %v; it backed off as if refused", s.Early)
	}
}

// TestARefusedDRAPBacksOff is D-39 for D-RAP: a 429 stops the update and
// starts NOAA's back-off, as the grid's does.
func TestARefusedDRAPBacksOff(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.status[drapURL] = 429
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	n.take()
	n.drapAsks = nil
	c.Add(30 * time.Second)
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	if a := n.take(); len(a) != 0 || len(n.drapAsks) != 0 {
		t.Errorf("30 s after D-RAP's refusal NOAA was asked %v and D-RAP %d times; the back-off is 60 s", a, len(n.drapAsks))
	}
	c.Add(time.Minute)
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	if len(n.drapAsks) != 1 {
		t.Errorf("after the back-off D-RAP was asked %d times; want once", len(n.drapAsks))
	}
}
