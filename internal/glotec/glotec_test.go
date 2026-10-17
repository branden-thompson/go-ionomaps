package glotec

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func (c Cells) has(i int) bool { return i >= 0 && i < len(c) && c[i] }

func (c Cells) count() int {
	n := 0
	for _, v := range c {
		if v {
			n++
		}
	}
	return n
}

// now is the clock the tests read at: after every synthetic grid's time.
var now = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

// synthetic is a GloTEC-shaped grid built here, never NOAA's own data
// (R-4.3): 72 by 72 points at 5° by 2.5° cell centres, each with NmF2 from
// the rule nm, hmF2 300 km and quality 5.
func synthetic(t testing.TB, nm func(lon, lat float64) float64) []byte {
	t.Helper()
	type props struct {
		TEC     float64 `json:"tec"`
		HmF2    float64 `json:"hmF2"`
		NmF2    float64 `json:"NmF2"`
		Quality int     `json:"quality_flag"`
	}
	type feature struct {
		Type     string `json:"type"`
		Geometry struct {
			Type        string     `json:"type"`
			Coordinates [2]float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties props `json:"properties"`
	}
	doc := struct {
		Type     string    `json:"type"`
		Features []feature `json:"features"`
		TimeTag  string    `json:"time_tag"`
	}{Type: "FeatureCollection", TimeTag: "2026-10-09T11:55:00Z"}
	for i := range 72 {
		for j := range 72 {
			var f feature
			f.Type, f.Geometry.Type = "Feature", "Point"
			lon, lat := -177.5+5*float64(i), -88.75+2.5*float64(j)
			f.Geometry.Coordinates = [2]float64{lon, lat}
			f.Properties = props{TEC: 10, HmF2: 300, NmF2: nm(lon, lat), Quality: 5}
			doc.Features = append(doc.Features, f)
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// flat is NmF2 for a foF2 of 10 MHz everywhere.
func flat(float64, float64) float64 { return math.Pow(10/8.98e-6, 2) }

// TestFoF2FromNmF2 is the plasma frequency (ITU-R P.1239): foF2 in MHz is
// 8.98e-6 times the square root of NmF2 in electrons per cubic metre.
func TestFoF2FromNmF2(t *testing.T) {
	for _, c := range []struct{ nm, mhz float64 }{{1.24e12, 10.0}, {2.162e12, 13.2}, {3.1e10, 1.58}} {
		if got := FoF2(c.nm); math.Abs(got-c.mhz) > 0.01 {
			t.Errorf("NmF2 %g gives %.3f MHz; want %.2f", c.nm, got, c.mhz)
		}
	}
}

// TestAGridIsReadIntoItsCells is G2.2: the 72 by 72 points become cells rows
// from the north, each west to east, with their foF2, hmF2 and quality, and
// the grid's own valid time.
func TestAGridIsReadIntoItsCells(t *testing.T) {
	g, err := Read(synthetic(t, func(lon, lat float64) float64 {
		return math.Pow((5+lat/30+lon/180)/8.98e-6, 2)
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	if g.Cols != 72 || g.Rows != 72 || g.West != -180 || g.East != 180 || g.South != -90 || g.North != 90 {
		t.Fatalf("grid %d by %d over %v..%v, %v..%v", g.Cols, g.Rows, g.West, g.East, g.South, g.North)
	}
	if !g.Valid.Equal(time.Date(2026, 10, 9, 11, 55, 0, 0, time.UTC)) {
		t.Errorf("valid %v", g.Valid)
	}
	first, last := g.FoF2[0], g.FoF2[len(g.FoF2)-1] // the north-west cell, then the south-east
	if math.Abs(float64(first)-(5+88.75/30-177.5/180)) > 0.001 || math.Abs(float64(last)-(5-88.75/30+177.5/180)) > 0.001 {
		t.Errorf("north-west %.3f and south-east %.3f MHz: rows are not from the north, west to east", first, last)
	}
	if g.HmF2[0] != 300 || g.Quality[0] != 5 || g.Rejected != 0 || g.NoData.count() != 0 {
		t.Errorf("hmF2 %v, quality %v, %d rejected, %d without data", g.HmF2[0], g.Quality[0], g.Rejected, g.NoData.count())
	}
}

// TestOutOfRangeCellsAreRejectedAndCounted is R-3.3: a cell whose foF2 or
// hmF2 is past its physical bounds is marked without data, counted, and its
// value never read; every value that leaves is finite.
func TestOutOfRangeCellsAreRejectedAndCounted(t *testing.T) {
	g, err := Read(synthetic(t, func(lon, lat float64) float64 {
		switch {
		case lon == -177.5 && lat == 88.75:
			return 1e16 // foF2 of 898 MHz
		case lon == 177.5 && lat == -88.75:
			return 0
		}
		return flat(lon, lat)
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	if g.Rejected != 2 || !g.NoData.has(0) || !g.NoData.has(len(g.FoF2)-1) || g.NoData.has(1) {
		t.Errorf("%d rejected; the two out-of-range cells marked %v, %v", g.Rejected, g.NoData.has(0), g.NoData.has(len(g.FoF2)-1))
	}
	for i, v := range g.FoF2 {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("cell %d is %v", i, v)
		}
	}
}

// TestAChangedFormatIsAnError is R-3.1: a grid that is not the 72 by 72
// layout, has no time, is from the future, or holds too many points is an
// error, never a silently wrong field; and no error quotes the input.
func TestAChangedFormatIsAnError(t *testing.T) {
	good := string(synthetic(t, flat))
	for _, c := range []struct{ what, doc string }{
		{"not JSON", "<html>secret-token-123</html>"},
		{"no time", strings.Replace(good, `"time_tag":"2026-10-09T11:55:00Z"`, `"time_tag":""`, 1)},
		{"a time from the future", strings.Replace(good, "2026-10-09T11:55:00Z", "2026-10-10T11:55:00Z", 1)},
		{"a point moved off the grid", strings.Replace(good, "[-177.5,-88.75]", "[-177.4,-88.75]", 1)},
		{"a point missing", dropPoint(t, good)},
		{"a point repeated in another's place", repeatPoint(t, good)},
	} {
		_, err := Read([]byte(c.doc), now)
		if err == nil {
			t.Errorf("%s was accepted", c.what)
			continue
		}
		if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "html") {
			t.Errorf("%s: the error quotes the input: %v", c.what, err)
		}
	}
}

// dropPoint is a grid with its first point taken out.
func dropPoint(t *testing.T, doc string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	features := m["features"].([]any)
	m["features"] = features[1:]
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// repeatPoint is a grid whose first point stands where its second does, so
// one cell is given twice and another never.
func repeatPoint(t *testing.T, doc string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	features := m["features"].([]any)
	second := features[1].(map[string]any)["geometry"].(map[string]any)["coordinates"]
	features[0].(map[string]any)["geometry"].(map[string]any)["coordinates"] = second
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// FuzzGloTECDecode is R-3.1, R-3.3: no input panics; an accepted grid is
// the 72 by 72 layout with every value finite and in its bounds.
func FuzzGloTECDecode(f *testing.F) {
	f.Add(synthetic(f, flat))
	f.Add([]byte(`{"type":"FeatureCollection","features":[],"time_tag":"2026-10-09T11:55:00Z"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := Read(data, now)
		if err != nil {
			return
		}
		if g.Cols*g.Rows != len(g.FoF2) || len(g.FoF2) != gridCells {
			t.Fatalf("%d by %d with %d values", g.Cols, g.Rows, len(g.FoF2))
		}
		for i, v := range g.FoF2 {
			if g.NoData.has(i) {
				continue
			}
			if !(float64(v) >= minFoF2 && float64(v) <= maxFoF2) {
				t.Fatalf("cell %d: foF2 %v outside its bounds", i, v)
			}
		}
	})
}

// BenchmarkGloTECDecode is R-8.2: a full grid, typed decode.
func BenchmarkGloTECDecode(b *testing.B) {
	data := synthetic(b, flat)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Read(data, now); err != nil {
			b.Fatal(err)
		}
	}
}

// listing is a directory index in the layout NOAA's server writes, built
// here: one line a name.
func listing(names ...string) []byte {
	var b strings.Builder
	b.WriteString("<html><body><pre><a href=\"?C=N;O=D\">Name</a>\n<a href=\"/products/glotec/\">Parent Directory</a>\n")
	for _, n := range names {
		b.WriteString("<a href=\"" + n + "\">" + n + "</a> 2026-10-09 11:20  2.4M\n")
	}
	b.WriteString("</pre></body></html>\n")
	return []byte(b.String())
}

// TestTheNewestGridIsReadFromTheIndex is watchpost D-111 and D-141: the
// newest grid is the latest time in the index's names, wherever it is
// listed; a name past the clock, or not a grid's, is passed over; an index
// with no grid names none.
func TestTheNewestGridIsReadFromTheIndex(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	idx := listing("glotec_icao_20261009T110500Z.geojson", "glotec_icao_20261009T113500Z.geojson",
		"glotec_icao_20261009T112500Z.geojson", "glotec_icao_20261009T130500Z.geojson", // an hour past the clock
		"glotec_icao_20261309T113500Z.geojson", "glotec_icao_20261009T1145Z.geojson", "other_20261009T115500Z.geojson")
	name, valid, ok := Newest(idx, now)
	if !ok || name != "glotec_icao_20261009T113500Z.geojson" || !valid.Equal(time.Date(2026, 10, 9, 11, 35, 0, 0, time.UTC)) {
		t.Errorf("newest %q at %v (%v)", name, valid, ok)
	}
	for _, empty := range [][]byte{nil, listing(), []byte("not an index"), listing("glotec_icao_20261009T130500Z.geojson")} {
		if name, _, ok := Newest(empty, now); ok || name != "" {
			t.Errorf("%q named %q", empty, name)
		}
	}
	if _, _, ok := Newest(append(listing("glotec_icao_20261009T113500Z.geojson"), make([]byte, maxIndexBytes)...), now); ok {
		t.Error("an index past any GloTEC index's size was read")
	}
}

// newestRef is Newest by the name's pattern, the reading Newest must equal.
func newestRef(index []byte, now time.Time) (name string, valid time.Time, ok bool) {
	if len(index) > maxIndexBytes {
		return "", time.Time{}, false
	}
	for _, m := range regexp.MustCompile(`glotec_icao_(\d{8}T\d{6}Z)\.geojson`).FindAllSubmatch(index, -1) {
		t, err := time.Parse("20060102T150405Z", string(m[1]))
		if err == nil && !t.After(now.Add(clockSkew)) && t.After(valid) {
			name, valid = string(m[0]), t
		}
	}
	return name, valid, name != ""
}

// FuzzGloTECIndex: no index panics the reading; it reads as the name's
// pattern does; a name read is a grid's, at a time no later than the clock
// allows.
func FuzzGloTECIndex(f *testing.F) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	f.Add(listing("glotec_icao_20261009T113500Z.geojson"))
	f.Add([]byte("glotec_icao_99999999T999999Z.geojson"))
	f.Add([]byte("glotec_icao_glotec_icao_20261009T113500Z.geojson glotec_icao_20261009T113500Z.geojso glotec_icao_2026100９T113500Z.geojson"))
	f.Add(indexOfSize(now, 4<<10))
	f.Fuzz(func(t *testing.T, idx []byte) {
		name, valid, ok := Newest(idx, now)
		if rn, rv, rok := newestRef(idx, now); name != rn || !valid.Equal(rv) || ok != rok {
			t.Fatalf("read %q at %v (%v); the pattern reads %q at %v (%v)", name, valid, ok, rn, rv, rok)
		}
		if !ok {
			return
		}
		if !strings.HasPrefix(name, "glotec_icao_") || !strings.HasSuffix(name, ".geojson") || valid.After(now.Add(clockSkew)) {
			t.Errorf("read %q at %v", name, valid)
		}
	})
}

// indexOfSize is a listing of grids ten minutes apart, ending before now,
// about n bytes long: the measured index is 505 KB (watchpost W0.0).
func indexOfSize(now time.Time, n int) []byte {
	var names []string
	for at := now.Add(-10 * time.Minute); len(listing(names...)) < n; at = at.Add(-10 * time.Minute) {
		names = append(names, "glotec_icao_"+at.UTC().Format("20060102T150405Z")+".geojson")
	}
	return listing(names...)
}

func BenchmarkGloTECNewest(b *testing.B) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	idx := indexOfSize(now, 505<<10)
	b.SetBytes(int64(len(idx)))
	for b.Loop() {
		if _, _, ok := Newest(idx, now); !ok {
			b.Fatal("no grid named")
		}
	}
}

// TestAGridOfTooManyPointsIsRefusedAsItIsRead is I-5: a body within the
// byte bound holding far more points than GloTEC's 5184 is refused once the
// count passes them, never read whole into memory first - a hostile 8 MB of
// empty points would otherwise cost about a hundred megabytes.
func TestAGridOfTooManyPointsIsRefusedAsItIsRead(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"time_tag":"2026-10-09T11:30:00Z","features":[{}`)
	for b.Len() < maxBytes-16 {
		b.WriteString(",{}")
	}
	b.WriteString("]}")
	body := []byte(b.String())
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := Read(body, time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	runtime.ReadMemStats(&after)
	if !errors.Is(err, errLayout) {
		t.Errorf("a grid of %d bytes of empty points: %v; want the layout refused", len(body), err)
	}
	if used := after.TotalAlloc - before.TotalAlloc; used > 4<<20 {
		t.Errorf("refusing it took %d MB", used>>20)
	}
}
