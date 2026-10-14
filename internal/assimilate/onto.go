package assimilate

import (
	"errors"
	"math"

	"github.com/branden-thompson/go-ionomaps/internal/grid"
)

// Reading is a station's reading at its place: foF2 and, when it carries
// one, M(3000)F2; whether its score is unknown (D-61).
type Reading struct {
	Lat, Lon    float64
	FoF2, M3000 float64
	HasM        bool
	Unscored    bool
}

// Result is the background with the readings assimilated: foF2 and
// M(3000)F2 a cell, how many readings were used for each, foF2's mean
// residual over its background at the readings used, and per cell the
// distance to the nearest reading used and whether it is unscored - none
// when nothing was used.
type Result struct {
	FoF2, M3000     []float32
	Used, UsedM     int
	MeanResidual    float64
	NearestKm       []float32
	NearestUnscored []bool
}

var (
	errBackground = errors.New("assimilate: a background is not on the library's grid")
	errTooMany    = errors.New("assimilate: more readings than any update holds")
)

// maxReadings bounds the readings assimilated at once (R-3.3): five times a
// GIRO burst of 40.
const maxReadings = 200

// Onto assimilates the readings onto the backgrounds (R-9.2): foF2's
// residuals over its background (GloTEC, else the climatology), M(3000)F2's
// over the climatology, each fitted with its kernel and added cell by cell;
// a characteristic with fewer than three readings is left as it was.
//
// Source: as NewFit; watchpost D-101, D-116; go-ionomaps R-3.2, D-61.
func Onto(foBack, mBack []float32, readings []Reading) (Result, error) {
	sized := len(foBack) == grid.Cells && len(mBack) == grid.Cells
	if !sized {
		return Result{}, errBackground
	}
	tooMany := len(readings) > maxReadings
	if tooMany {
		return Result{}, errTooMany // the fit is a cubic solve and a distance a cell a reading
	}
	r := Result{FoF2: append([]float32(nil), foBack...), M3000: append([]float32(nil), mBack...)}
	var foObs, mObs []Obs
	var used []Reading
	for _, x := range readings {
		b, ok := grid.Sample(foBack, x.Lat, x.Lon)
		if !ok || !(x.FoF2 > 0) {
			continue
		}
		foObs, used = append(foObs, Obs{Lat: x.Lat, Lon: x.Lon, Residual: x.FoF2 - b}), append(used, x)
		bm, okM := grid.Sample(mBack, x.Lat, x.Lon)
		if x.HasM && okM && x.M3000 > 0 {
			mObs = append(mObs, Obs{Lat: x.Lat, Lon: x.Lon, Residual: x.M3000 - bm})
		}
	}
	foFit, err := NewFit(foObs, FoF2Kernel)
	if err != nil {
		return r, nil // too few, or none: the background as it was
	}
	mFit, errM := NewFit(mObs, M3000Kernel)
	r.Used = len(foObs)
	for _, o := range foObs {
		r.MeanResidual += o.Residual / float64(r.Used)
	}
	if errM == nil {
		r.UsedM = len(mObs)
	}
	r.NearestKm, r.NearestUnscored = make([]float32, grid.Cells), make([]bool, grid.Cells)
	for i := range r.FoF2 {
		lat, lon := grid.Centre(i)
		r.FoF2[i] = float32(math.Max(float64(foBack[i])+foFit.At(lat, lon), minFoF2))
		if errM == nil {
			r.M3000[i] = float32(math.Max(float64(mBack[i])+mFit.At(lat, lon), minM3000))
		}
		r.NearestKm[i], r.NearestUnscored[i] = nearest(used, lat, lon)
	}
	return r, nil
}

// Shifted is foF2 moved by a number of MHz a cell, each held to the floor
// (R-9.6): a correction pulling far below a thin background never makes a
// value of no meaning. False, and nothing made, for a field not on the
// library's grid or a shift that is no finite number.
//
// Source: watchpost D-105.
func Shifted(foF2 []float32, by float64) ([]float32, bool) {
	sized := len(foF2) == grid.Cells
	if !sized {
		return nil, false
	}
	finite := !math.IsNaN(by) && !math.IsInf(by, 0)
	if !finite {
		return nil, false
	}
	out := make([]float32, len(foF2))
	for i, f := range foF2 {
		out[i] = float32(math.Max(float64(f)+by, minFoF2))
	}
	return out, true
}

// The floors a cell is held to: the parser's lowest foF2 and M(3000)F2, so
// a fit pulling far below a background never makes a value of no meaning.
const (
	minFoF2  = 0.5
	minM3000 = 1.5
)

// nearest is the distance to the nearest reading used, km, and whether it
// is unscored.
func nearest(used []Reading, lat, lon float64) (float32, bool) {
	best, unscored := math.Inf(1), false
	for _, x := range used {
		if d := distance(Obs{Lat: x.Lat, Lon: x.Lon}, lat, lon); d < best {
			best, unscored = d, x.Unscored
		}
	}
	return float32(best), unscored
}
