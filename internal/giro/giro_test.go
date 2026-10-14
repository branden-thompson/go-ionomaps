package giro

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// from and to are the window the fixtures were asked for; now is after it.
var (
	from = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	to   = time.Date(2026, 9, 29, 23, 59, 59, 0, time.UTC)
	now  = time.Date(2026, 9, 30, 6, 0, 0, 0, time.UTC)
)

// header is a FastChar reply's header in GIRO's layout, the requester's
// address a documentation one (RFC 5737); never GIRO's own data (R-4.3). It
// is built from pieces, as the repository's own planted reply is, so that no
// signature of a reply stands whole in the source.
const header = "# Global Ionospheric Radio Observatory\n" +
	"# Location: " + "GEO" + " ( 19.6 S   146.9 E ), URSI-Code XX00X, SOMEWHERE\n" +
	"# Requested by unknown guest from IP:192.0.2.1\n" +
	"# Time                    " + "CS" + "   foF2 " + "QD M(D) QD\n"

// reply is the header and rows.
func reply(rows ...string) []byte { return []byte(header + strings.Join(rows, "\n") + "\n") }

// TestARowIsRead is G2.1: a row's time, confidence score, foF2, MUF(3000)
// and M(3000)F2 are read, a value GIRO marks missing is missing, and the
// station's place comes from the location line, its longitude west of 180°E
// made negative.
func TestARowIsRead(t *testing.T) {
	r, err := ParseReply(reply(
		"2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //",
		"2026-09-29T00:40:00.000Z 100 11.209 //  --- __",
		"2026-09-29T01:00:00.000Z 999  9.500 /F 3.16 //",
	), "XX00X", from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Station.Code != "XX00X" || r.Station.Lat != -19.6 || math.Abs(r.Station.Lon-146.9) > 1e-9 {
		t.Errorf("the station is at %v, %v; want 19.6°S 146.9°E", r.Station.Lat, r.Station.Lon)
	}
	if len(r.Readings) != 3 {
		t.Fatalf("%d readings; want 3", len(r.Readings))
	}
	a, b, c := r.Readings[0], r.Readings[1], r.Readings[2]
	if !a.At.Equal(from.Add(5*time.Minute)) || a.Score != 95 || a.FoF2 != 10.888 || a.M3000 != 3.17 || a.MUF3000 != float32(10.888)*float32(3.17) || !a.HasM300 {
		t.Errorf("the first reading is %+v", a)
	}
	if b.HasM300 || b.MUF3000 != 0 || b.FoF2 != 11.209 {
		t.Errorf("an M(3000)F2 GIRO marks missing is read as %+v", b)
	}
	if c.Score != 999 || c.FoF2 != 9.5 {
		t.Errorf("a manually scaled reading with a descriptor is read as %+v", c)
	}
	west, _ := ParseReply([]byte(strings.Replace(string(reply()), "146.9 E", "276.4 E", 1)), "XX00X", from, to, now)
	if math.Abs(west.Station.Lon-(-83.6)) > 1e-9 {
		t.Errorf("276.4°E is read as %v; want -83.6", west.Station.Lon)
	}
}

// TestReplyHeadersNeverLeaveTheParser is R-3.4 (IS-2): the header, which
// carries the requester's address, is dropped before the rows are read;
// nothing the parser returns holds any of its text.
func TestReplyHeadersNeverLeaveTheParser(t *testing.T) {
	r, err := ParseReply(reply("2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //"), "XX00X", from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(strings.Fields(strings.ReplaceAll(dump(r), "XX00X", "")), " "); strings.Contains(got, "192.0.2.1") || strings.Contains(got, "SOMEWHERE") || strings.Contains(got, "GIRO") {
		t.Errorf("the parsed reply holds the header's text: %s", got)
	}
}

// dump is everything a reply holds, as text.
func dump(r Reply) string {
	var b strings.Builder
	b.WriteString(r.Station.Code)
	for _, x := range r.Readings {
		b.WriteString(x.At.String())
	}
	return b.String()
}

// TestOutOfRangeReadingsAreRejectedAndCounted is R-3.3 (IS-3): a value
// outside its physical bounds, a time outside the window asked or in the
// future, a score that is no score, or a row of another shape is refused and
// counted, never read.
func TestOutOfRangeReadingsAreRejectedAndCounted(t *testing.T) {
	r, err := ParseReply(reply(
		"2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //", // kept
		"2026-09-29T00:10:00.000Z  95 40.000 // 3.17 //", // foF2 past any ionosphere
		"2026-09-29T00:15:00.000Z  95 10.888 // 9.00 //", // M(3000)F2 past its bounds
		"2026-09-28T23:55:00.000Z  95 10.888 // 3.17 //", // before the window
		"2026-10-01T00:00:00.000Z  95 10.888 // 3.17 //", // in the future
		"2026-09-29T00:20:00.000Z 150 10.888 // 3.17 //", // no score
		"2026-09-29T00:25:00.000Z  95 NaN // 3.17 //",    // no number
		"2026-09-29T00:30:00.000Z  95 10.888 //",         // another shape
		"2026-09-29T00:35:00.000Z  95    --- __  --- __", // no foF2: missing, not malformed
	), "XX00X", from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Readings) != 1 || r.Rejected != 7 || r.Missing != 1 {
		t.Errorf("%d kept, %d rejected and %d missing; want 1, 7 and 1", len(r.Readings), r.Rejected, r.Missing)
	}
}

// TestLowConfidenceReadingsAreDropped is R-9.4 and D-61 (watchpost D-181):
// a reading scored under 70 is dropped and counted apart from the rejected,
// which are malformed; one scored 70 or more or 999 (manual) is kept, and
// one whose score is unknown (-1) is kept and marked unscored.
func TestLowConfidenceReadingsAreDropped(t *testing.T) {
	r, err := ParseReply(reply(
		"2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //",
		"2026-09-29T00:10:00.000Z  40 10.888 // 3.17 //",
		"2026-09-29T00:15:00.000Z   0 10.888 // 3.17 //",
		"2026-09-29T00:20:00.000Z  -1 10.888 // 3.17 //",
		"2026-09-29T00:25:00.000Z  70 10.888 // 3.17 //",
		"2026-09-29T00:30:00.000Z  69 10.888 // 3.17 //",
	), "XX00X", from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Readings) != 3 || r.LowConfidence != 3 || r.Rejected != 0 {
		t.Fatalf("%d kept, %d low, %d rejected; want 3, 3 and 0", len(r.Readings), r.LowConfidence, r.Rejected)
	}
	for _, x := range r.Readings {
		if x.Unscored != (x.Score == -1) {
			t.Errorf("a reading scored %d is marked unscored %v", x.Score, x.Unscored)
		}
	}
}

// TestErrorsNeverQuoteInput is R-3.4: a reply that is not FastChar's, or for
// another station, is refused with words of the parser's own.
func TestErrorsNeverQuoteInput(t *testing.T) {
	for name, body := range map[string][]byte{
		"not FastChar":    []byte("<html>secret-token-123</html>"),
		"another station": []byte(strings.Replace(header, "XX00X", "YY11Y", 1) + "secret-token-123\n"),
		"no location":     []byte("# nothing here secret-token-123\n"),
		"too large":       reply(strings.Repeat("2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //\n", maxBytes/len("2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //")+1)), // well formed, but larger than any reply
	} {
		_, err := ParseReply(body, "XX00X", from, to, now)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), "secret-token-123") || strings.Contains(err.Error(), "YY11Y") {
			t.Errorf("%s: the error quotes the input: %v", name, err)
		}
	}
	if _, err := ParseReply(reply(), "xx00x", from, to, now); !errors.Is(err, ErrBadCode) {
		t.Errorf("a station code that is not five capitals or digits: %v", err)
	}
}

// TestStationCodesAreChecked is G2.4 (R-3.3): a code is five capitals or
// digits, nothing else.
func TestStationCodesAreChecked(t *testing.T) {
	for code, ok := range map[string]bool{"TV51R": true, "AL945": true, "tv51r": false, "TV51": false, "TV51R1": false, "TV5&R": false, "": false} {
		if CodeOK(code) != ok {
			t.Errorf("%q: accepted %v; want %v", code, !ok, ok)
		}
	}
}

// FuzzGIROParser is R-3.3 and R-3.4: whatever arrives, the parser does not
// panic, and nothing it returns is NaN, Inf or out of its bounds.
func FuzzGIROParser(f *testing.F) {
	f.Add(reply("2026-09-29T00:05:00.000Z  95 10.888 // 3.17 //"))
	f.Add(reply("2026-09-29T00:40:00.000Z 100 11.209 //    --- __  --- __  265.9 //"))
	f.Add([]byte("# Location: " + "GEO" + " ( 91.0 N   400.0 E ), URSI-Code XX00X\n")) // a place off the globe
	f.Fuzz(func(t *testing.T, body []byte) {
		r, err := ParseReply(body, "XX00X", from, to, now)
		if err != nil {
			return
		}
		if !(r.Station.Lat >= -90 && r.Station.Lat <= 90 && r.Station.Lon >= -180 && r.Station.Lon <= 180) {
			t.Fatalf("a station off the globe: %+v", r.Station)
		}
		for _, x := range r.Readings {
			bad := func(v float32) bool { return math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) }
			if bad(x.FoF2) || bad(x.MUF3000) || bad(x.M3000) || x.FoF2 < minFoF2 || x.FoF2 > maxFoF2 || x.At.Before(from) || x.At.After(to) {
				t.Fatalf("a reading out of bounds: %+v", x)
			}
		}
	})
}

// TestAFutureReadingIsRefused is R-3.3: a reading past the host's clock is
// refused, even inside the window asked.
func TestAFutureReadingIsRefused(t *testing.T) {
	r, err := ParseReply(reply("2026-09-30T07:00:00.000Z  95 10.888 // 3.17 //"), "XX00X", from, now.Add(24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Readings) != 0 || r.Rejected != 1 {
		t.Errorf("a reading an hour past the clock: %d kept, %d rejected", len(r.Readings), r.Rejected)
	}
}

// TestAChangedFormatIsRefused is R-3.1: a row whose qualifier columns have
// moved is refused, never read across; a window that ends before it starts
// or starts after the clock is refused.
func TestAChangedFormatIsRefused(t *testing.T) {
	r, err := ParseReply(reply("2026-09-29T00:05:00.000Z  95 10.888 /// 3.17 ///"), "XX00X", from, to, now)
	if err != nil || len(r.Readings) != 0 || r.Rejected != 1 {
		t.Errorf("a row with its qualifiers moved: %d kept, %d rejected, %v", len(r.Readings), r.Rejected, err)
	}
	if _, err := ParseReply(reply(), "XX00X", to, from, now); err == nil {
		t.Error("a window that ends before it starts was taken")
	}
	if _, err := ParseReply(reply(), "XX00X", now.Add(time.Hour), now.Add(2*time.Hour), now); err == nil {
		t.Error("a window that starts after the clock was taken")
	}
}

// TestTheRowsAreCapped is R-3.3: a reply gives at most maxRows readings;
// the rest are counted as rejected, never kept.
func TestTheRowsAreCapped(t *testing.T) {
	start := from
	var rows []string
	for i := range maxRows + 5 {
		rows = append(rows, start.Add(time.Duration(i)*time.Minute).Format("2006-01-02T15:04:05.000Z")+"  95 10.888 // 3.17 //")
	}
	end := start.Add(time.Duration(maxRows+5) * time.Minute)
	r, err := ParseReply(reply(rows...), "XX00X", start, end, end.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Readings) != maxRows || r.Rejected != 5 {
		t.Errorf("%d kept and %d rejected; want %d and 5", len(r.Readings), r.Rejected, maxRows)
	}
}

// TestTheAskNamesOnlyWhatIsUsed is R-5.2 and R-3.3 (D-62): the ask names
// foF2 and M(D) alone, at 3000 km, the window in FastChar's form, and a
// code reaches the address only through url.Values, never as it came.
func TestTheAskNamesOnlyWhatIsUsed(t *testing.T) {
	u, err := AskURL("AL945", from, to)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ursiCode=AL945", "charName=foF2%2CM%28D%29", "DMUF=3000", "fromDate=2026%2F09%2F29+00%3A00%3A00", "toDate=2026%2F09%2F29+23%3A59%3A59"} {
		if !strings.Contains(u, want) {
			t.Errorf("the ask %q lacks %q", u, want)
		}
	}
	for _, bad := range []string{"AL945&x=1", "al945", ""} {
		if _, err := AskURL(bad, from, to); err == nil {
			t.Errorf("the code %q was put in an address", bad)
		}
	}
	if _, err := AskURL("AL945", to, from); err == nil {
		t.Error("a window that ends before it starts was asked")
	}
}
