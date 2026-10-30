package assimilate

import (
	"math"
	"math/rand/v2"
	"testing"
)

// stations are six stations' residuals, MHz, a fixed set for the oracle.
var stations = []Obs{{45.1, -83.6, 0.8}, {37.1, -6.7, -0.4}, {50.1, 4.6, 0.3}, {-33.3, 26.5, 1.1}, {13.6, 144.9, -0.9}, {42.6, -71.5, 0.5}}

// queries are where the oracle was read: at a station, near one, far from
// all, and at another station.
var queries = [][2]float64{{45.1, -83.6}, {40.0, -80.0}, {0.0, 0.0}, {-60.0, -120.0}, {50.1, 4.6}}

// TestTheFitIsWeeksMethod is R-9.2 (the hybrid, D-101): the fit at each
// query is what PLAN's week was scored with - week.py's gp_loo, its formula
// worked in numpy on these six stations (02-analysis/evidence/week.py,
// lines 193 to 212): the mean residual, an exponential kernel in
// great-circle distance scaled by the residuals' variance, the noise a part
// of it - for foF2's kernel and for M(3000)F2's.
func TestTheFitIsWeeksMethod(t *testing.T) {
	for _, c := range []struct {
		k    Kernel
		want []float64
	}{
		{FoF2Kernel, []float64{0.614799754721, 0.516337775748, 0.244566287453, 0.237589480303, 0.215766139697}},
		{M3000Kernel, []float64{0.702453831718, 0.568130984404, 0.245600699745, 0.238592671525, 0.247103730899}},
	} {
		f, err := NewFit(stations, c.k)
		if err != nil {
			t.Fatal(err)
		}
		for i, q := range queries {
			if got := f.At(q[0], q[1]); math.Abs(got-c.want[i]) > 1e-8 { // at a station acos(≈1) is ill-conditioned: Go and numpy part by 4e-9 there
				t.Errorf("kernel %+v at %v: %.12f; week.py's method gives %.12f", c.k, q, got, c.want[i])
			}
		}
	}
}

// TestTheKernelIsValidOnTheSphere is R-9.2 (Gneiting 2013): on random sets
// of stations - near-duplicates among them - the covariance is positive
// definite, so every fit solves.
func TestTheKernelIsValidOnTheSphere(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for trial := range 200 {
		n := 3 + r.IntN(38)
		obs := make([]Obs, n)
		for i := range obs {
			obs[i] = Obs{Lat: math.Asin(2*r.Float64()-1) * 180 / math.Pi, Lon: 360*r.Float64() - 180, Residual: r.NormFloat64()}
		}
		obs[n-1] = Obs{Lat: obs[0].Lat + 1e-6, Lon: obs[0].Lon, Residual: obs[0].Residual + 0.1} // two at one place
		if _, err := NewFit(obs, FoF2Kernel); err != nil {
			t.Fatalf("trial %d, %d stations: %v", trial, n, err)
		}
	}
}

// TestAResidualAtAStationIsRecovered is R-9.2: at a station the fit lies
// between the mean residual and the station's own, nearer its own; far from
// every station it is near the mean, the stations' pull a tenth of one's
// own at most.
func TestAResidualAtAStationIsRecovered(t *testing.T) {
	f, _ := NewFit(stations, FoF2Kernel)
	mean := 0.2333333333333333
	at := f.At(45.1, -83.6)
	if !(at > mean && at < 0.8) || at-mean < (0.8-mean)/2 {
		t.Errorf("at the station whose residual is 0.8, the fit is %v; between the mean %v and 0.8, nearer 0.8", at, mean)
	}
	if far := f.At(-89, 0); math.Abs(far-mean) > 0.1*(at-mean) { // over 6,000 km from every station
		t.Errorf("far from every station the fit is %v; the mean is %v, and a station's own pull %v", far, mean, at-mean)
	}
}

// TestTooFewStationsMakeNoFit is week.py's floor: fewer than three
// residuals, or one that is no number, make no fit.
func TestTooFewStationsMakeNoFit(t *testing.T) {
	if _, err := NewFit(stations[:2], FoF2Kernel); err == nil {
		t.Error("two stations made a fit")
	}
	bad := append([]Obs(nil), stations...)
	bad[2].Residual = math.NaN()
	if _, err := NewFit(bad, FoF2Kernel); err == nil {
		t.Error("a residual that is no number made a fit")
	}
}

// TestABadKernelOrPlaceAddsNothing is R-3.3: a kernel of no length is
// refused; a fit never made, or asked at a place that is no number, adds
// nothing to the background; a system of the wrong size does not solve.
func TestABadKernelOrPlaceAddsNothing(t *testing.T) {
	if _, err := NewFit(stations, Kernel{LengthKm: 0, Noise: 0.5}); err == nil {
		t.Error("a kernel of no length made a fit")
	}
	if _, err := NewFit(stations, Kernel{LengthKm: 2000, Noise: -1}); err == nil {
		t.Error("a kernel of negative noise made a fit")
	}
	if v := (Fit{}).At(10, 10); v != 0 {
		t.Errorf("a fit never made adds %v", v)
	}
	f, _ := NewFit(stations, FoF2Kernel)
	if v := f.At(math.NaN(), 10); v != 0 {
		t.Errorf("a place that is no number gets %v", v)
	}
	if _, ok := solve([]float64{1, 0, 0, 1}, []float64{1}, 2); ok {
		t.Error("a system of the wrong size solved")
	}
	if _, ok := solve(nil, nil, 0); ok {
		t.Error("an empty system solved")
	}
	if _, ok := solve([]float64{1, 1.2, 1.2, 1}, []float64{1, 1}, 2); ok {
		t.Error("a matrix that is not positive definite solved")
	}
}
