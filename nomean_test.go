package ionomaps_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// TestNoMeanAsksNoGrid is RL-2 and watchpost D-239: with no F10.7 mean to
// build a field from, no grid is fetched to be thrown away; the snapshot
// with no field names what it lacks, and says Offline only when nothing
// came back.
func TestNoMeanAsksNoGrid(t *testing.T) {
	for _, c := range []struct {
		name   string
		set    func(n *noaa)
		reason ionomaps.Reason
	}{
		{"the solar file unreadable", func(n *noaa) { n.solar = []byte("<html>moved</html>") }, ionomaps.Partial},
		{"the solar file unanswered", func(n *noaa) { n.down[solarURL] = true }, ionomaps.Offline},
	} {
		l, n, _ := library(t)
		n.list(t, start.Add(-30*time.Minute), 7)
		c.set(n)
		s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
		if !errors.Is(err, ionomaps.ErrNoField) || s.Reason != c.reason {
			t.Errorf("%s: reason %v, error %v; want %v and no field", c.name, s.Reason, err, c.reason)
		}
		if asked := n.take(); !slices.Equal(asked, []string{solarURL}) {
			t.Errorf("%s: asked %v; want the solar file alone", c.name, asked)
		}
		for _, in := range []string{"solar", "GloTEC", "D-RAP", "scales"} {
			if !slices.Contains(s.Inputs.Unread, in) {
				t.Errorf("%s: unread %v does not name %s", c.name, s.Inputs.Unread, in)
			}
		}
	}
}

// TestTheMeanIsKeptAcrossARestart is watchpost D-249: the F10.7 mean held
// goes into the state, and a library restored from it builds a field while
// the solar file cannot be read, the mean's age said and the file named
// unread, for up to 7 days; past them, no field.
func TestTheMeanIsKeptAcrossARestart(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	first, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state := l.State()
	n.down[solarURL] = true
	c.Add(3 * 24 * time.Hour)
	n.list(t, c.Now().Add(-20*time.Minute), 7)
	again := restarted(t, n, c, state)
	s, err := settled(again, context.Background(), ionomaps.UpdateOptions{})
	if err != nil || s.Inputs.F107Mean != first.Inputs.F107Mean || s.Inputs.F107Age < 3*24*time.Hour || !slices.Contains(s.Inputs.Unread, "solar") || s.Reason == ionomaps.Fresh {
		t.Fatalf("three days on, restored: mean %v (was %v), age %v, unread %v, reason %v, error %v", s.Inputs.F107Mean, first.Inputs.F107Mean, s.Inputs.F107Age, s.Inputs.Unread, s.Reason, err)
	}
	c.Add(4*24*time.Hour + time.Hour)
	if _, err := settled(again, context.Background(), ionomaps.UpdateOptions{}); !errors.Is(err, ionomaps.ErrNoField) {
		t.Errorf("a mean over 7 days old built a field: %v", err)
	}
}
