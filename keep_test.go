package ionomaps_test

import (
	"context"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// TestTheDaysClimatologyHoursAreKept is R-8.1 (watchpost D-222, P-4): an
// hour ahead is worked out once, whichever snapshot of the day asks it - the
// same snapshot again, or a later one whose hour ahead falls in the same
// UTC hour - and it is that hour's climatology, from its start.
func TestTheDaysClimatologyHoursAreKept(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	first, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := first.Ahead(3)
	if err != nil {
		t.Fatal(err)
	}
	if want := first.Now.At.Add(3 * time.Hour).Truncate(time.Hour); !a.Now.At.Equal(want) {
		t.Errorf("three hours ahead is %v; the start of its UTC hour, %v", a.Now.At, want)
	}
	if _, err := first.Ahead(3); err != nil {
		t.Fatal(err)
	}
	c.Add(10 * time.Minute)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	later, err := l.Update(context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if later.Now.At.Add(3*time.Hour).Truncate(time.Hour) != a.Now.At {
		t.Fatalf("control: the later snapshot's third hour ahead is another hour (%v)", later.Now.At)
	}
	if _, err := later.Ahead(3); err != nil {
		t.Fatal(err)
	}
	if cells, _ := ionomaps.Built(l); cells != len(a.Now.FoF2.Values) {
		t.Errorf("one hour ahead asked three times over two snapshots worked out %d cells; its %d, once", cells, len(a.Now.FoF2.Values))
	}
}

// TestAnUnchangedBackgroundIsBuiltOn is R-8.1 (watchpost D-222, P-4): the
// host's options changed at the snapshot's moment - the correction turned on
// - rebuild the snapshot on the background it was made on, not a new one.
func TestAnUnchangedBackgroundIsBuiltOn(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	if _, err := settled(l, context.Background(), ionomaps.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	_, before := ionomaps.Built(l)
	c.Add(time.Minute)
	s, err := l.Update(context.Background(), ionomaps.UpdateOptions{CorrectNoReadings: true})
	if err != nil || s.Reason != ionomaps.TooSoon {
		t.Fatalf("control: the correction turned on is not answered too soon (%v, %v)", s.Reason, err)
	}
	if _, after := ionomaps.Built(l); after != before {
		t.Errorf("the correction turned on worked out %d new backgrounds; none, the moment and grid unchanged", after-before)
	}
}
