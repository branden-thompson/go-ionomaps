package solar

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// daily is a file in NOAA's daily solar data layout, built here: its header,
// then one line a day from start with the given fluxes, -999 for a day
// without one.
func daily(start time.Time, flux ...int) []byte {
	var b strings.Builder
	b.WriteString(":Product: Daily Solar Data            DSD.txt\n#\n#  Date     10.7cm Number  Hemis. Regions Field  Flux   C  M  X  S  1  2  3\n#---\n")
	for i, f := range flux {
		d := start.AddDate(0, 0, i)
		fmt.Fprintf(&b, "%d %02d %02d  %3d     83      570      2    -999      *   2  0  0  1  1  0  0\n", d.Year(), int(d.Month()), d.Day(), f)
	}
	return []byte(b.String())
}

// TestTheF107RuleIsTheThirtyDayMean is R-9.5 (watchpost D-104): the mean of
// the observed fluxes over the 30 days before the day - not the day itself,
// nor a day before them - a day without one skipped and the days counted.
func TestTheF107RuleIsTheThirtyDayMean(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	flux := make([]int, 35)
	for i := range flux {
		flux[i] = 100 + i // 2026-09-01 is 100, and on
	}
	flux[20] = -999 // 2026-09-21, missing
	days, err := Parse(daily(start, flux...))
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC) // the 30 days before it: 2026-09-03 to 2026-10-02
	mean, n, ok := Mean(days, day)
	want, count := 0.0, 0
	for i := 2; i <= 31; i++ {
		if i != 20 {
			want += float64(100 + i)
			count++
		}
	}
	want /= float64(count)
	if !ok || n != count || math.Abs(mean-want) > 1e-9 {
		t.Errorf("mean %v over %d days (%v); want %v over %d", mean, n, ok, want, count)
	}
	if _, _, ok := Mean(days, start.AddDate(1, 0, 0)); ok {
		t.Error("a mean was taken for a day a year past the file's last")
	}
}

// TestAMissingSolarFileUsesTheLastMeanWithItsAge is R-9.5: when the file
// cannot be fetched or read, the mean held before is kept, with the time it
// was taken, so its age can be said; a good file replaces it.
func TestAMissingSolarFileUsesTheLastMeanWithItsAge(t *testing.T) {
	start := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC) // 2026-09-30 to 2026-10-02: within the 30 days before 2026-10-03
	t1 := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	if age := (Held{}).Age(t1); age != 0 {
		t.Errorf("no mean held has an age of %v", age)
	}
	held := Hold(Held{}, daily(start, 120, 130, 140), t1)
	if !held.OK || held.Mean != 130 || !held.Taken.Equal(t1) || held.Days != 3 {
		t.Fatalf("held %+v", held)
	}
	t2 := t1.Add(26 * time.Hour)
	for _, broken := range [][]byte{nil, []byte("<html>gone</html>"), daily(start)} {
		if again := Hold(held, broken, t2); again != held || again.Age(t2) != 26*time.Hour {
			t.Errorf("a missing or broken file replaced the held mean: %+v", again)
		}
	}
	if fresh := Hold(held, daily(start, 90, 90), t2); fresh.Mean != 90 || !fresh.Taken.Equal(t2) {
		t.Errorf("a good file did not replace the held mean: %+v", fresh)
	}
}

// FuzzSolarIndicesParser is R-3.1, R-3.3: no input panics; every day read
// has a real date and a flux within its physical bounds.
func FuzzSolarIndicesParser(f *testing.F) {
	f.Add(daily(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), 110, -999, 95))
	f.Add([]byte("2026 13 40  999\n2026 02 30  100\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		days, err := Parse(data)
		if err != nil {
			return
		}
		for _, d := range days {
			if d.Date.IsZero() || !(d.F107 >= minFlux && d.F107 <= maxFlux) {
				t.Fatalf("a day read as %+v", d)
			}
		}
	})
}

// TestAFileIsReadWhole is RI-3: a day named twice counts once in the mean;
// a line the scanner cannot take, or a file larger than NOAA's is, makes
// the file unreadable rather than a mean of the days before it.
func TestAFileIsReadWhole(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var b strings.Builder
	for i := 30; i >= 1; i-- {
		fmt.Fprintf(&b, "%s  70  0\n", day.AddDate(0, 0, -i).Format("2006 01 02"))
	}
	for range 900 {
		fmt.Fprintf(&b, "%s 500  0\n", day.AddDate(0, 0, -1).Format("2006 01 02"))
	}
	days, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if mean, n, _ := Mean(days, day); n != 30 || mean != 70 {
		t.Errorf("a day named 900 times: mean %v over %d days; want 70 over 30", mean, n)
	}
	long := day.AddDate(0, 0, -1).Format("2006 01 02") + " 450 0\n" + strings.Repeat("x", 70_000) + "\n"
	if _, err := Parse([]byte(long)); err == nil {
		t.Error("a 70 KB line was read past")
	}
	if _, err := Parse(make([]byte, maxBytes+1)); err == nil {
		t.Error("a file larger than NOAA's was read")
	}
}

// TestAMeanKeptIsChecked is watchpost D-249 and R-3.3: a mean kept across a
// restart comes back within a flux's bounds, its days and the clock's skew,
// a moment ahead taken as now; anything else is refused.
func TestAMeanKeptIsChecked(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if h, err := Resume(140, 30, now.Add(-time.Hour), now); err != nil || !h.OK || h.Mean != 140 || h.Age(now) != time.Hour {
		t.Errorf("a good mean: %+v, %v", h, err)
	}
	if h, err := Resume(140, 30, now.Add(time.Minute), now); err != nil || !h.Taken.Equal(now) {
		t.Errorf("a mean a minute ahead: %+v, %v; want taken as now", h, err)
	}
	if h, err := Resume(0, 0, time.Time{}, now); err != nil || h.OK {
		t.Errorf("nothing kept: %+v, %v; want no mean and no error", h, err)
	}
	for name, c := range map[string]struct {
		mean  float64
		days  int
		taken time.Time
	}{
		"past a flux's bounds": {900, 30, now},
		"no days":              {140, 0, now},
		"past 30 days":         {140, 31, now},
		"taken past the skew":  {140, 30, now.Add(time.Hour)},
	} {
		if _, err := Resume(c.mean, c.days, c.taken, now); err == nil {
			t.Errorf("%s: kept", name)
		}
	}
}
