package limits

import "math"

// earthKm is the mean Earth radius the path geometry is measured on, km.
const earthKm = 6371.0

// Distance is the great-circle distance between two places, km; no number
// for a latitude off the globe.
//
// Source: the haversine formula on a sphere of the mean Earth radius.
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	if !(math.Abs(lat1) <= 90 && math.Abs(lat2) <= 90) {
		return math.NaN()
	}
	r := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*r/2), 2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Along is the place a part frac of the way along the great circle from one
// place to another, degrees; no number for a part outside 0 to 1, or for
// places opposite each other, which no one great circle joins.
//
// Source: spherical interpolation along the great circle.
func Along(lat1, lon1, lat2, lon2, frac float64) (lat, lon float64) {
	if !(frac >= 0 && frac <= 1) {
		return math.NaN(), math.NaN()
	}
	r := math.Pi / 180
	d := Distance(lat1, lon1, lat2, lon2) / earthKm
	if d < 1e-12 {
		return lat1, lon1
	}
	if !(math.Pi-d > 1e-9) {
		return math.NaN(), math.NaN()
	}
	a, b := math.Sin((1-frac)*d)/math.Sin(d), math.Sin(frac*d)/math.Sin(d)
	x := a*math.Cos(lat1*r)*math.Cos(lon1*r) + b*math.Cos(lat2*r)*math.Cos(lon2*r)
	y := a*math.Cos(lat1*r)*math.Sin(lon1*r) + b*math.Cos(lat2*r)*math.Sin(lon2*r)
	z := a*math.Sin(lat1*r) + b*math.Sin(lat2*r)
	return math.Atan2(z, math.Hypot(x, y)) / r, math.Atan2(y, x) / r
}

// Point is what the basic MUF reads at a control point: foF2, M(3000)F2 and
// foE there, MHz (M(3000)F2 a ratio), and the gyrofrequency at 300 km, MHz.
type Point struct{ FoF2, M3000, FoE, FH float64 }

// PathMUF is a path's basic MUF, MHz, from its length and its control points
// at the middle and d₀/2 from each end (d₀ the lowest-order mode's hop): a
// path within dmax at the middle is one hop, its MUF the middle's; a path
// longer takes the lower dmax MUF of the two end points. No number for a
// length past half the globe's circumference, which no great circle has.
//
// Source: ITU-R P.533-14 §3.5.1 and Table 1a) (02-analysis/evidence/p533-equations.md).
func PathMUF(km float64, middle, nearEnd, farEnd Point) float64 {
	dmax := Dmax(middle.FoF2, middle.M3000, middle.FoE)
	if !(km > 0) || dmax != dmax {
		return math.NaN()
	}
	if !(km <= math.Pi*earthKm) {
		return math.NaN()
	}
	if km <= dmax {
		return BasicMUF(km, middle.FoF2, middle.M3000, middle.FoE, middle.FH) // one hop
	}
	near := BasicMUF(dmax, nearEnd.FoF2, nearEnd.M3000, nearEnd.FoE, nearEnd.FH)
	far := BasicMUF(dmax, farEnd.FoF2, farEnd.M3000, farEnd.FoE, farEnd.FH)
	return math.Min(near, far)
}

// Hops is how many hops the lowest-order mode takes over a path of km at a
// control point's dmax: d₀ is km over it.
//
// Source: ITU-R P.533-14 §3.5.1.
func Hops(km, dmax float64) float64 {
	if !(km > 0) || !(dmax > 0) {
		return math.NaN()
	}
	return math.Max(1, math.Ceil(km/dmax))
}
