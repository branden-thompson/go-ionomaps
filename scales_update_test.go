package ionomaps_test

import (
	"context"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

const scalesURL = "https://services.swpc.noaa.gov/products/noaa-scales.json"

// scalesFile is a scales file in NOAA's layout, built here (R-4.3): now at
// a time with R, S and G as given (a level "null" for none), and an
// outlook of three days, G 2 on the second.
func scalesFile(at time.Time, r, s, g string) []byte {
	day := func(date, clock, r, minor, major, s, prob, g string) string {
		return `{"DateStamp":"` + date + `","TimeStamp":"` + clock + `","R":{"Scale":` + r + `,"Text":null,"MinorProb":` + minor + `,"MajorProb":` + major +
			`},"S":{"Scale":` + s + `,"Text":null,"Prob":` + prob + `},"G":{"Scale":` + g + `,"Text":null}}`
	}
	d := func(n int) string { return at.AddDate(0, 0, n).Format(time.DateOnly) }
	return []byte(`{"0":` + day(d(0), at.Format(time.TimeOnly), r, "null", "null", s, "null", g) +
		`,"1":` + day(d(0), at.Format(time.TimeOnly), "null", `"30"`, `"10"`, "null", `"5"`, `"1"`) +
		`,"2":` + day(d(1), "00:00:00", "null", `"30"`, `"10"`, "null", `"5"`, `"2"`) +
		`,"3":` + day(d(2), "00:00:00", "null", `"20"`, `"5"`, "null", `"5"`, `"0"`) + `}`)
}

// TestTheScalesAreCarriedInTheSnapshot is R-2.10 (watchpost D-88, G2.5):
// each update asks NOAA's scales with D-RAP, with the validators of the
// file held; the snapshot carries R, S and G now, the outlook and the
// file's time, in slices of its own.
func TestTheScalesAreCarriedInTheSnapshot(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	n.scales = scalesFile(start.Add(-2*time.Minute), `"1"`, `"0"`, `"3"`)
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sc := s.Scales
	if n.scalesAsks == 0 || sc.R != 1 || sc.S != 0 || sc.G != 3 || !sc.Valid.Equal(start.Add(-2*time.Minute).Truncate(time.Second)) {
		t.Fatalf("asked %d times; scales %+v", n.scalesAsks, sc)
	}
	if len(sc.Outlook) != 3 || sc.Outlook[1].G != 2 || sc.Outlook[0].RMinorPct != 30 || sc.Outlook[0].RMajorPct != 10 || sc.Outlook[0].SPct != 5 {
		t.Errorf("the outlook: %+v", sc.Outlook)
	}
	s.Scales.Outlook[1].G = 5
	if again, _ := l.Update(context.Background(), ionomaps.UpdateOptions{}); again.Scales.Outlook[1].G != 2 {
		t.Error("a change to one snapshot's outlook reached the next")
	}
	n.scalesAsks = 0
	c.Add(10 * time.Minute)
	settled(l, context.Background(), ionomaps.UpdateOptions{})
	if n.scalesAsks != 1 || n.scalesValidators[len(n.scalesValidators)-1].ETag != `"scales-1"` {
		t.Errorf("the next update asked the scales %d times, with %+v; want once, with the file's validators", n.scalesAsks, n.scalesValidators)
	}
}

// TestMissingScalesAreUnsetNotZero is R-2.10 (watchpost D-88): with no
// scales file, a file that cannot be read, or one over an hour old, every
// level is unset and the valid time zero; a level NOAA leaves null is unset
// while the others are read - never level 0.
func TestMissingScalesAreUnsetNotZero(t *testing.T) {
	l, n, c := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, _ := settled(l, context.Background(), ionomaps.UpdateOptions{})
	unset := func(sc ionomaps.Scales) bool {
		return sc.R == ionomaps.ScaleUnset && sc.S == ionomaps.ScaleUnset && sc.G == ionomaps.ScaleUnset && sc.Valid.IsZero() && len(sc.Outlook) == 0
	}
	if !unset(s.Scales) {
		t.Errorf("no scales file, yet the snapshot carries %+v", s.Scales)
	}
	c.Add(10 * time.Minute)
	n.scales = []byte(`{"0":{"DateStamp":"x"}}`)
	if s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{}); !unset(s.Scales) {
		t.Errorf("a file that cannot be read, yet the snapshot carries %+v", s.Scales)
	}
	c.Add(10 * time.Minute)
	n.scales = scalesFile(c.Now().Add(-time.Minute), `"0"`, `"0"`, "null")
	if s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{}); s.Scales.R != 0 || s.Scales.G != ionomaps.ScaleUnset {
		t.Errorf("G null in NOAA's file: R %d, G %d; want R 0 and G unset", s.Scales.R, s.Scales.G)
	}
	n.scales = nil
	c.Add(61 * time.Minute)
	n.list(t, c.Now().Add(-30*time.Minute), 7)
	if s, _ = settled(l, context.Background(), ionomaps.UpdateOptions{}); !unset(s.Scales) {
		t.Errorf("a scales file over an hour old is still carried: %+v", s.Scales)
	}
}
