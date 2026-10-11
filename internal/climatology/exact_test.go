package climatology

import (
	"math"
	"testing"
	"time"
)

// The evaluation as first ported, kept verbatim as the reference the
// optimised one must equal bit for bit (the quality pass, 2026-10-11).
func legendreRef(z float64) [lmax + 1][lmax + 1]float64 {
	var p [lmax + 1][lmax + 1]float64
	s := math.Sqrt(math.Max(0, 1-z*z))
	p[0][0] = 1
	for m := 1; m <= lmax; m++ {
		p[m][m] = -float64(2*m-1) * s * p[m-1][m-1]
	}
	for m := 0; m < lmax; m++ {
		p[m+1][m] = z * float64(2*m+1) * p[m][m]
	}
	for m := 0; m <= lmax; m++ {
		for l := m + 2; l <= lmax; l++ {
			p[l][m] = (float64(2*l-1)*z*p[l-1][m] - float64(l+m-1)*p[l-2][m]) / float64(l-m)
		}
	}
	return p
}

func harmonicsRef(theta, phi float64) [Harmonics]float64 {
	var out [Harmonics]float64
	z := math.Cos(theta)
	p := legendreRef(z)
	var cm, sm [lmax + 1]float64
	c1, s1 := math.Cos(phi), math.Sin(phi)
	cm[0] = 1
	for m := 1; m <= lmax; m++ {
		cm[m], sm[m] = cm[m-1]*c1-sm[m-1]*s1, sm[m-1]*c1+cm[m-1]*s1
	}
	for l := 0; l <= lmax; l++ {
		base := l * (l + 1)
		out[base] = p[l][0] * norms[l][0]
		for m := 1; m <= l; m++ {
			pn := p[l][m] * norms[l][m]
			out[base+m] = pn * cm[m]
			out[base-m] = pn * sm[m]
		}
	}
	return out
}

func atRef(h Hour, qdlat, mlt float64) (foF2, m3000 float64) {
	sh := harmonicsRef((90-qdlat)*math.Pi/180, mlt*15*math.Pi/180)
	for k, y := range sh {
		foF2 += h.fo[k] * y
		m3000 += h.m3[k] * y
	}
	return foF2, m3000
}

// TestTheEvaluationIsExactlyAsPorted holds every optimisation of the
// evaluation to the port's own arithmetic: the same bits at every place,
// not a tolerance - the PyIRI goldens were measured against these bits.
func TestTheEvaluationIsExactlyAsPorted(t *testing.T) {
	h := NewHour(Refits(), time.Date(2026, 10, 11, 14, 30, 0, 0, time.UTC), 137)
	for lat := -90.0; lat <= 90; lat += 2.5 {
		for mlt := 0.0; mlt < 24; mlt += 0.37 {
			fo, m3 := h.At(lat, mlt)
			rfo, rm3 := atRef(h, lat, mlt)
			if fo != rfo || m3 != rm3 {
				t.Fatalf("at %v, %v: %v, %v; the port gives %v, %v", lat, mlt, fo, m3, rfo, rm3)
			}
		}
	}
	again := NewHour(Refits(), time.Date(2026, 10, 11, 14, 30, 0, 0, time.UTC), 137)
	if again != h {
		t.Error("the same hour folded twice differs: the tables are not read the same way twice")
	}
}

// TestATableOfAnotherShapeIsRefused: both embedded tables decode; one with
// another magic, another shape or a length that is not whole decodes to
// nothing.
func TestATableOfAnotherShapeIsRefused(t *testing.T) {
	for _, table := range [][]byte{fof2Table, m3000Table} {
		if slice(table, 1, Min) == nil {
			t.Fatal("an embedded table is refused")
		}
		for name, plant := range map[string]func(b []byte) []byte{
			"magic":     func(b []byte) []byte { b[0] = 'X'; return b },
			"levels":    func(b []byte) []byte { b[len(tableMagic)]++; return b },
			"months":    func(b []byte) []byte { b[len(tableMagic)+4]++; return b },
			"terms":     func(b []byte) []byte { b[len(tableMagic)+8]++; return b },
			"harmonics": func(b []byte) []byte { b[len(tableMagic)+12]++; return b },
			"length":    func(b []byte) []byte { return b[:len(b)-8] },
		} {
			if slice(plant(append([]byte(nil), table...)), 1, Min) != nil {
				t.Errorf("a table with its %s planted wrong was decoded", name)
			}
		}
	}
}
