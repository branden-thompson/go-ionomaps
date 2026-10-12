package ionomaps_test

import (
	"context"
	"math"
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
	h, err := s.Ahead(3)
	if err != nil {
		t.Fatal(err)
	}
	if !h.At.Equal(s.Hours[0].At.Add(3 * time.Hour)) {
		t.Errorf("hour 3 ahead is at %v; want %v", h.At, s.Hours[0].At.Add(3*time.Hour))
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

// TestTheTypicalErrorCarriesItsBasis is R-1.3 (watchpost D-134): the
// snapshot carries the hours ahead's typical error by distance to the
// nearest station, and where it was measured.
func TestTheTypicalErrorCarriesItsBasis(t *testing.T) {
	l, n, _ := library(t)
	n.list(t, start.Add(-30*time.Minute), 7)
	s, err := l.Update(context.Background(), ionomaps.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e := s.TypicalError
	if len(e.Bands) != 4 || e.Bands[0] != (ionomaps.BandError{UpToKm: 500, FoF2MHz: 0.77, MUF3000MHz: 2.91}) || e.Bands[3].UpToKm != 0 || e.Basis == "" {
		t.Errorf("the typical error is %+v", e)
	}
}
