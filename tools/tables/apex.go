package main

import "math"

// Earth's figure for geodetic coordinates (WGS84), and the mean radius the
// apex coordinates are defined with (Richmond 1995), km.
const (
	wgs84A     = 6378.137
	wgs84F     = 1 / 298.257223563
	meanRadius = 6371.009
)

// geocentric is a geodetic latitude, longitude (degrees) and height above
// the ellipsoid (km) as a point in Earth-centred Cartesian coordinates, km.
func geocentric(lat, lon, h float64) [3]float64 {
	e2 := wgs84F * (2 - wgs84F)
	phi, lam := lat*math.Pi/180, lon*math.Pi/180
	n := wgs84A / math.Sqrt(1-e2*math.Sin(phi)*math.Sin(phi))
	return [3]float64{(n + h) * math.Cos(phi) * math.Cos(lam), (n + h) * math.Cos(phi) * math.Sin(lam), (n*(1-e2) + h) * math.Sin(phi)}
}

// fieldXYZ is the main field at a Cartesian point, as a Cartesian vector.
func (m model) fieldXYZ(p [3]float64) [3]float64 {
	r := math.Sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2])
	theta, phi := math.Acos(p[2]/r), math.Atan2(p[1], p[0])
	br, bt, bp := m.field(r, theta, phi)
	st, ct, sp, cp := math.Sin(theta), math.Cos(theta), math.Sin(phi), math.Cos(phi)
	return [3]float64{
		br*st*cp + bt*ct*cp - bp*sp,
		br*st*sp + bt*ct*sp + bp*cp,
		br*ct - bt*st,
	}
}

// geodeticHeight is a Cartesian point's height above the ellipsoid, km
// (Bowring's method, iterated: exact to well under a metre).
func geodeticHeight(p [3]float64) float64 {
	e2 := wgs84F * (2 - wgs84F)
	rho := math.Hypot(p[0], p[1])
	lat := math.Atan2(p[2], rho*(1-e2))
	h := 0.0
	for range 6 {
		n := wgs84A / math.Sqrt(1-e2*math.Sin(lat)*math.Sin(lat))
		h = rho/math.Cos(lat) - n
		lat = math.Atan2(p[2], rho*(1-e2*n/(n+h)))
	}
	return h
}

// norm is a vector's length.
func norm(v [3]float64) float64 { return math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]) }

// dipoleFrom is how far out a trace is finished on the dipole, km: past it
// the field is the dipole's, along whose lines r = L cos²λ, so the apex is
// r/cos²λ and the dipole longitude does not change.
const dipoleFrom = 10 * meanRadius

// apex traces the field line from a point to its apex, the point on it
// farthest from Earth's centre: the apex's height above the ellipsoid, km,
// and its centred-dipole longitude, degrees; false where the trace does not
// end. Past dipoleFrom the apex is thousands of kilometres up, where the
// ellipsoid's few kilometres are nothing, so its height is from the mean
// radius.
func (m model) apex(start [3]float64) (float64, float64, bool) {
	dir := func(p [3]float64, sign float64) [3]float64 {
		b := m.fieldXYZ(p)
		n := norm(b)
		return [3]float64{sign * b[0] / n, sign * b[1] / n, sign * b[2] / n}
	}
	// Go the way that rises: along the field where it leaves Earth.
	r0 := norm(start)
	b := m.fieldXYZ(start)
	sign := 1.0
	if (b[0]*start[0]+b[1]*start[1]+b[2]*start[2])/r0 < 0 {
		sign = -1
	}
	p := start
	rk4 := func(p [3]float64, h float64) [3]float64 {
		k1 := dir(p, sign)
		k2 := dir(add(p, scale(k1, h/2)), sign)
		k3 := dir(add(p, scale(k2, h/2)), sign)
		k4 := dir(add(p, scale(k3, h)), sign)
		return add(p, scale(add(add(k1, scale(k2, 2)), add(scale(k3, 2), k4)), h/6))
	}
	rising := func(q [3]float64) bool { d := dir(q, sign); return d[0]*q[0]+d[1]*q[1]+d[2]*q[2] > 0 }
	for steps := 0; steps < 200000; steps++ {
		r := norm(p)
		if r > dipoleFrom {
			lat := m.dipoleLatitude(p)
			c := math.Cos(lat * math.Pi / 180)
			return r/(c*c) - meanRadius, m.dipoleLongitude(p), true
		}
		// A step a five-hundredth of the radius: the apex itself is found
		// by halving along the field line, so the step sets only the cost.
		h := 0.002 * r
		next := rk4(p, h)
		if !rising(next) {
			// The apex lies within this step: halve along the field line itself.
			lo, hi := 0.0, h
			for range 50 {
				mid := (lo + hi) / 2
				if rising(rk4(p, mid)) {
					lo = mid
				} else {
					hi = mid
				}
			}
			top := rk4(p, lo)
			return geodeticHeight(top), m.dipoleLongitude(top), true
		}
		p = next
	}
	return 0, 0, false
}

func add(a, b [3]float64) [3]float64           { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func scale(a [3]float64, k float64) [3]float64 { return [3]float64{a[0] * k, a[1] * k, a[2] * k} }

// quasiDipole is a geodetic point's quasi-dipole latitude and longitude,
// degrees (Richmond 1995): the latitude from the apex's height and the
// point's own, both geodetic, over the mean radius, its sign the hemisphere
// the field line starts in; the longitude the apex's in centred-dipole
// coordinates.
func (m model) quasiDipole(lat, lon, h float64) (qdlat, qdlon float64, ok bool) {
	start := geocentric(lat, lon, h)
	ha, alon, ok := m.apex(start)
	if !ok {
		return 0, 0, false
	}
	c := (meanRadius + h) / (meanRadius + ha)
	qdlat = math.Acos(math.Sqrt(math.Min(1, c))) * 180 / math.Pi
	b := m.fieldXYZ(start)
	if b[0]*start[0]+b[1]*start[1]+b[2]*start[2] > 0 {
		qdlat = -qdlat // the field leaves Earth here: the southern magnetic hemisphere
	}
	return qdlat, alon, true
}

// dipoleLatitude is a point's latitude in centred-dipole coordinates,
// degrees.
func (m model) dipoleLatitude(p [3]float64) float64 {
	_, _, zd := m.dipoleFrame(p)
	return math.Asin(zd/norm(p)) * 180 / math.Pi
}

// dipoleLongitude is a point's longitude in centred-dipole coordinates: the
// dipole axis from IGRF's first-degree coefficients, longitude measured from
// the meridian through the axis and the geographic pole.
func (m model) dipoleLongitude(p [3]float64) float64 {
	xd, yd, _ := m.dipoleFrame(p)
	lon := math.Atan2(yd, xd) * 180 / math.Pi
	if lon < 0 {
		lon += 360
	}
	return lon
}

// dipoleFrame is a point in the centred-dipole frame: its z along the north
// dipole axis, its x in the meridian through the axis and the geographic
// pole.
func (m model) dipoleFrame(p [3]float64) (xd, yd, zd float64) {
	g, h := m.g, m.h
	g10, g11, h11 := g[1][0], g[1][1], h[1][1]
	b0 := math.Sqrt(g10*g10 + g11*g11 + h11*h11)
	// The north dipole axis points along -(g11, h11, g10).
	theta0 := math.Acos(-g10 / b0)
	phi0 := math.Atan2(-h11, -g11)
	// Rotate the point into the dipole frame: about z by phi0, then about y by theta0.
	x := p[0]*math.Cos(phi0) + p[1]*math.Sin(phi0)
	y := -p[0]*math.Sin(phi0) + p[1]*math.Cos(phi0)
	z := p[2]
	return x*math.Cos(theta0) - z*math.Sin(theta0), y, x*math.Sin(theta0) + z*math.Cos(theta0)
}
