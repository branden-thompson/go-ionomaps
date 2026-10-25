package ionomaps_test

import (
	"context"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/background"
)

// TestHoursAheadAreTheClimatology is R-1.3 (D-60): hour k ahead, built when
// asked, is the climatology's for that hour at the snapshot's F10.7 mean -
// foF2 and MUF(3000), its product with M(3000)F2 - and no field from now.
func TestHoursAheadAreTheClimatology(t *testing.T) {
	l, n, _ := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := l.Update(context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Ahead(3)
	if err != nil {
		t.Fatal(err)
	}
	h := a.Now
	if !h.At.Equal(s.Now.At.Add(3 * time.Hour)) {
		t.Errorf("hour 3 ahead is at %v; want %v", h.At, s.Now.At.Add(3*time.Hour))
	}
	want := background.Build(h.At, float64(s.Inputs.F107Mean), nil)
	for _, i := range []int{0, 4500, 16199} {
		fo, muf := float64(h.FoF2.Values[i]), float64(h.MUF3000.Values[i])
		if math.Abs(fo-float64(want.FoF2[i])) > 1e-4 || math.Abs(muf-float64(want.FoF2[i]*want.M3000[i])) > 1e-3 {
			t.Errorf("cell %d: foF2 %.4f, MUF %.4f; the climatology gives %.4f, %.4f", i, fo, muf, want.FoF2[i], want.FoF2[i]*want.M3000[i])
		}
		if fo == 7 {
			t.Errorf("cell %d of an hour ahead is GloTEC's foF2, now's", i)
		}
	}
	for _, k := range []int{0, 25, -1} {
		if _, err := s.Ahead(k); err == nil {
			t.Errorf("hour %d ahead was built", k)
		}
	}
}

// TestTheTypicalErrorCarriesItsBasis is R-1.3 (watchpost D-134, AX-3): the
// snapshot carries the hours ahead's typical error by distance to the
// nearest station, and where it was measured - in words a listener can be
// told, the citation apart.
func TestTheTypicalErrorCarriesItsBasis(t *testing.T) {
	l, n, _ := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := l.Update(context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e := s.TypicalError
	if len(e.Bands) != 4 || e.Bands[0] != (ionomaps.BandError{UpToKm: 500, FoF2MHz: 0.77, MUF3000MHz: 2.91}) || e.Bands[3] != (ionomaps.BandError{UpToKm: 0, FoF2MHz: 1.37, MUF3000MHz: 4.73}) || !strings.Contains(e.Source, "D-181") || !listenable(e.Basis) { // D-61: re-measured with the unknown scores taken
		t.Errorf("the typical error is %+v", e)
	}
}

// listenable is whether words can be said to a listener as they are: no
// ruling, file or setting a developer cites.
func listenable(words string) bool {
	return words != "" && !strings.ContainsAny(words, "()_=") && !strings.Contains(words, "D-") && !strings.Contains(words, ".py") && !strings.Contains(words, "watchpost")
}

// TestTwentyFourHoursAheadFitTheHeap is watchpost D-251 under D-112: a host
// holding a snapshot and every hour ahead it gives - 24, each answered at a
// place, as watchpost's hour steps keep them - holds under 15 MB of the
// library's live heap, counted from before its first update (the test's
// own fake service left out): each hour its own tables (P-6), the
// climatology's cells shared inside the library and made only for the rows
// read (R2P-1, R2P-2).
func TestTwentyFourHoursAheadFitTheHeap(t *testing.T) {
	l, _, _ := giroLibrary(t)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	update(t, l)
	s := update(t, l)
	if len(s.NearestKm) == 0 {
		t.Fatal("the snapshot assimilated no station: no table to own")
	}
	held := make([]ionomaps.Snapshot, 0, 24)
	for k := 1; k <= 24; k++ {
		a, err := s.Ahead(k)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Bands([]ionomaps.LatLon{{Lat: 42.4, Lon: -71.1}}, 300); err != nil {
			t.Fatal(err)
		}
		held = append(held, a)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	added := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("the library, its snapshot and 24 hours ahead, each answered: %.1f MB of live heap", float64(added)/(1<<20))
	if added > 15<<20 {
		t.Errorf("the library, its snapshot and 24 hours ahead, each answered, hold %.1f MB of live heap; D-112's proxy is 15", float64(added)/(1<<20))
	}
	for i := range held {
		if &held[i].NearestKm[0] == &s.NearestKm[0] {
			t.Fatalf("hour %d ahead shares its nearest-station table with the snapshot: each owns its own (P-6)", i+1)
		}
	}
	runtime.KeepAlive(held)
	runtime.KeepAlive(s)
}
