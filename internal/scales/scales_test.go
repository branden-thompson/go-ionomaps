package scales

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// now is the host's clock in these tests.
var now = time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC)

// day is one day of the file, its scales' values written as the file
// writes them: a quoted number, or null.
func day(date, clock, r, minor, major, s, prob, g string) string {
	return `{"DateStamp":"` + date + `","TimeStamp":"` + clock + `",` +
		`"R":{"Scale":` + r + `,"Text":null,"MinorProb":` + minor + `,"MajorProb":` + major + `},` +
		`"S":{"Scale":` + s + `,"Text":null,"Prob":` + prob + `},` +
		`"G":{"Scale":` + g + `,"Text":null}}`
}

// file is a scales file shaped as NOAA's: now ("0"), the day before
// ("-1"), and the outlook for today and the next two days ("1" to "3").
func file(now string) string {
	return `{"0":` + now +
		`,"1":` + day("2026-10-16", "19:50:00", "null", `"20"`, `"5"`, "null", `"5"`, `"0"`) +
		`,"2":` + day("2026-10-17", "00:00:00", "null", `"25"`, `"5"`, "null", `"10"`, `"1"`) +
		`,"3":` + day("2026-10-18", "00:00:00", "null", `"20"`, `"5"`, "null", `"5"`, "null") +
		`,"-1":` + day("2026-10-15", "19:50:00", `"0"`, "null", "null", `"0"`, "null", `"0"`) + `}`
}

// TestTheLevelsAndOutlookAreRead is R-2.10 (watchpost D-88): R, S and G
// now, a level NOAA leaves null unset, never 0; the outlook's three days
// with their probabilities and G level; the file's time.
func TestTheLevelsAndOutlookAreRead(t *testing.T) {
	l, err := Read([]byte(file(day("2026-10-16", "19:50:00", `"2"`, "null", "null", `"0"`, "null", "null"))), now)
	if err != nil {
		t.Fatal(err)
	}
	if l.R != 2 || l.S != 0 || l.G != None || !l.Valid.Equal(time.Date(2026, 10, 16, 19, 50, 0, 0, time.UTC)) {
		t.Errorf("now: R%d S%d G%d at %v; want R2 S0 G unset at 19:50", l.R, l.S, l.G, l.Valid)
	}
	if len(l.Days) != 3 {
		t.Fatalf("%d outlook days; three", len(l.Days))
	}
	want := []Day{
		{Day: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), RMinorPct: 20, RMajorPct: 5, SPct: 5, G: 0},
		{Day: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), RMinorPct: 25, RMajorPct: 5, SPct: 10, G: 1},
		{Day: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC), RMinorPct: 20, RMajorPct: 5, SPct: 5, G: None},
	}
	for i, w := range want {
		if l.Days[i] != w {
			t.Errorf("day %d: %+v; want %+v", i+1, l.Days[i], w)
		}
	}
}

// TestABadFileIsRefused is R-3.1 and R-3.3: a level outside 0 to 5, a
// probability outside 0 to 100, no current day, no time, a time in the
// future or a file larger than any NOAA writes is an error, never a level;
// no error quotes the file (R-3.4).
func TestABadFileIsRefused(t *testing.T) {
	good := day("2026-10-16", "19:50:00", `"0"`, "null", "null", `"0"`, "null", `"0"`)
	for name, body := range map[string]string{
		"level 6":         file(day("2026-10-16", "19:50:00", `"6"`, "null", "null", `"0"`, "null", `"0"`)),
		"level -1":        file(day("2026-10-16", "19:50:00", `"0"`, "null", "null", `"-1"`, "null", `"0"`)),
		"level a word":    file(day("2026-10-16", "19:50:00", `"0"`, "null", "null", `"0"`, "null", `"G1"`)),
		"probability 101": strings.Replace(file(good), `"25"`, `"101"`, 1),
		"no current day":  `{"1":` + good + `}`,
		"no time":         file(day("2026-10-16", "", `"0"`, "null", "null", `"0"`, "null", `"0"`)),
		"in the future":   file(day("2026-10-17", "19:50:00", `"0"`, "null", "null", `"0"`, "null", `"0"`)),
		"not JSON":        "<html>secret</html>",
		"too large":       file(good) + strings.Repeat(" ", maxBytes),
	} {
		_, err := Read([]byte(body), now)
		if err == nil {
			t.Errorf("%s: read as levels", name)
			continue
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "G1") {
			t.Errorf("%s: the error quotes the file: %v", name, err)
		}
	}
	if _, err := Read([]byte(file(good)), now); err != nil {
		t.Errorf("the good file: %v", err)
	}
	if _, err := Read([]byte(file(good)+strings.Repeat(" ", maxBytes)), now); !errors.Is(err, errTooLarge) {
		t.Errorf("a file too large: %v", err)
	}
}
