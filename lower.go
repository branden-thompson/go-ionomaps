package ionomaps

import (
	"math"

	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// The lower limit's bounds (R-2.4): P.533-14 §5.2 alone to 7000 km, §5.3
// alone past 9000 km, the two blended between (D-191); E modes and the E
// layer's screening reach 4000 km, an E mode's lowest-order hop 2000 km, and
// screening's foE is read 1000 km from each end of a path past 2000 km.
const (
	longFromKm = 7000.0
	shortToKm  = 9000.0
	eModesKm   = 4000.0
	eHopKm     = 2000.0
	screenAtKm = 1000.0
	eModes     = 3
	f2Modes    = 6
	lufSteps   = 12
)

// Noise is the man-made noise the reference circuit hears (watchpost D-187):
// ITU-R P.372's category, Residential by default; galactic noise is added
// above foF2, atmospheric noise is not modelled.
type Noise uint8

// The categories.
const (
	NoiseResidential Noise = iota // the default
	NoiseCity
	NoiseRural
	NoiseQuietRural
)

// noiseEnvironments are each category's P.372 environment, by Noise.
var noiseEnvironments = [...]limits.Environment{NoiseResidential: limits.Residential, NoiseCity: limits.City,
	NoiseRural: limits.Rural, NoiseQuietRural: limits.QuietRural}

// environment is the category's P.372 environment; the default,
// residential, for a value that is no category.
func (n Noise) environment() limits.Environment {
	if int(n) >= len(noiseEnvironments) {
		return limits.Residential
	}
	return noiseEnvironments[n]
}

// circuit is the reference circuit on one path in one hour (R-2.4): the
// path, its middle's control point, the foE the E layer screens with, foF2
// at the receiver (galactic noise is heard above it), the sunspot number,
// the sun, and the noise.
type circuit struct {
	from, to  LatLon
	km        float64
	mid       limits.Point
	screenFoE float64
	rxFoF2    float64
	r12, flux float64
	sun       *sunTrack
	env       limits.Environment
	lhE, lhF2 float64 // the auroral loss for E and F2 modes, Table 2 at Table 1d's points
	long      longPath
}

// longMemo is a typical day's §5.3 work by path - each control point's
// basic MUF at local noon and its least, and the LUF through the day - done
// once and read by each hour (P-3).
type longMemo map[[2]LatLon]*longDay

// longDay is one path's §5.3 work over a typical day.
type longDay struct {
	noon, least [2]float64
	fl          [24]float64
}

// longPath is §5.3's view of a path past 7000 km (D-191): its operational
// MUF, its LUF this hour and at night, the mean gyrofrequency at its
// control points, and the slant range of fM's hops.
type longPath struct {
	fM, fL, fLN, fH, slant float64
}

// newCircuit is the circuit from one place to another in the hour a reader
// and a sun give, the typical day beside it for a path past 7000 km (§5.3's
// noon and daily least, and its LUF through the day); false for a path of
// no length, no value at its middle or receiver, or a long path with no
// day or no value at its control points.
func (s Snapshot) newCircuit(read reader, from, to LatLon, sun *sunTrack, day *[24]typicalHour) (circuit, bool) {
	km := limits.Distance(from.Lat, from.Lon, to.Lat, to.Lon)
	if !(km > 0) {
		return circuit{}, false
	}
	mid, okM := s.controlPoint(read, from, to, 0.5, sun)
	rx, _, okR := read(to)
	if !okM || !okR {
		return circuit{}, false
	}
	c := circuit{from: from, to: to, km: km, mid: mid, screenFoE: mid.FoE, rxFoF2: rx,
		flux: float64(s.Inputs.F107Mean), r12: limits.R12FromFlux(float64(s.Inputs.F107Mean)), sun: sun, env: s.Noise.environment()}
	if km > eHopKm { // the higher foE of the points 1000 km from each end (§4)
		a, okA := s.controlPoint(read, from, to, screenAtKm/km, sun)
		b, okB := s.controlPoint(read, from, to, 1-screenAtKm/km, sun)
		if okA && okB {
			c.screenFoE = math.Max(a.FoE, b.FoE)
		}
	}
	c.lhE, c.lhF2 = c.auroral(true), c.auroral(false)
	if km > longFromKm {
		long, ok := c.longPath(read, day)
		if !ok {
			return circuit{}, false
		}
		c.long = long
	}
	return c, true
}

// longPath works out §5.3 for the circuit (D-191): fM's hops and control
// points d_M/2 from each end, each point's basic MUF now, at its local noon
// and its least through the typical day, K by the bearing at the path's
// middle, fM the lower of the two; and the LUF by eq. (33) through the day
// at fL's penetration points (hops of at most 3000 km from 300 km), the
// night LUF and the decay into it, this hour's taken.
func (c circuit) longPath(read reader, day *[24]typicalHour) (longPath, bool) {
	if day == nil {
		return longPath{}, false
	}
	nM, dM, deltaM := limits.LongHops(c.km)
	l := longPath{slant: limits.SlantRange(nM, dM, deltaM), fLN: limits.NightLUF(c.km), fM: math.Inf(1)}
	midLat, midLon := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, 0.5)
	bearing := bearingFrom(LatLon{midLat, midLon}, c.to)
	ends := [...]float64{dM / (2 * c.km), 1 - dM/(2*c.km)}
	var fbm [2]float64
	for i, fr := range ends {
		lat, lon := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, fr)
		fo, m3, ok := read(LatLon{Lat: lat, Lon: lon})
		if !ok {
			return longPath{}, false
		}
		fh := magcoords.Gyro(lat, lon, c.sun.at)
		l.fH += fh / 2
		fbm[i] = limits.LongBasicMUF(fo, m3, fh, dM)
	}
	ld := c.longDay(day, ends, dM, l)
	for i := range ends {
		l.fM = math.Min(l.fM, limits.KFactor(fbm[i], ld.noon[i], ld.least[i], bearing)*fbm[i])
	}
	l.fL = ld.fl[c.sun.at.UTC().Hour()]
	usable := l.fM > 0 && !math.IsInf(l.fM, 0) && l.fL > 0
	return l, usable
}

// longDay is the path's §5.3 work over the typical day, from the day's memo
// or done and kept there: each control point's basic MUF at its local noon
// and its least through the day, and the LUF of eq. (33) in each hour at
// fL's penetration points (hops of at most 3000 km from 300 km), the night
// LUF and the decay into it applied.
func (c circuit) longDay(day *[24]typicalHour, ends [2]float64, dM float64, l longPath) *longDay {
	key := [2]LatLon{c.from, c.to}
	memo := day[0].long
	if d, ok := memo[key]; ok {
		return d
	}
	d := &longDay{}
	for i, fr := range ends {
		lat, lon := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, fr)
		p, fh := LatLon{Lat: lat, Lon: lon}, magcoords.Gyro(lat, lon, c.sun.at)
		noonHour := int(math.Mod(math.Round(12-lon/15)+48, 24))
		d.noon[i], d.least[i] = math.NaN(), math.Inf(1)
		for h := range day {
			foH, m3H, okH := day[h].read(p)
			if !okH {
				continue
			}
			v := limits.LongBasicMUF(foH, m3H, fh, dM)
			d.least[i] = math.Min(d.least[i], v)
			if h == noonHour {
				d.noon[i] = v
			}
		}
	}
	midLat, _ := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, 0.5)
	nL := math.Ceil(c.km / limits.FLHopKm)
	deltaL := limits.ElevationAngle(c.km/nL, limits.LongMirrorKm)
	i90 := limits.IncidenceAngle(deltaL, limits.PenetrationKm)
	aw := limits.WinterAnomaly(midLat, c.sun.at.Month())
	fracs := limits.PenetrationFractions(c.km, int(nL), deltaL)
	pts := make([]LatLon, len(fracs))
	for i, fr := range fracs {
		pts[i].Lat, pts[i].Lon = limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, fr)
	}
	for h := range day {
		sum := 0.0
		for _, p := range pts {
			sum += math.Sqrt(math.Max(day[h].sun.cosZenith(p.Lat, p.Lon), 0)) // none where the sun is down (§5.3.2)
		}
		d.fl[h] = limits.LongLUF(c.r12, sum, i90, l.slant, l.fH, aw)
	}
	d.fl = limits.LUFDecay(d.fl, l.fLN)
	if memo != nil {
		memo[key] = d
	}
	return d
}

// bearingFrom is the initial great-circle bearing from one place to
// another, degrees from north.
func bearingFrom(a, b LatLon) float64 {
	r := math.Pi / 180
	y := math.Sin((b.Lon-a.Lon)*r) * math.Cos(b.Lat*r)
	x := math.Cos(a.Lat*r)*math.Sin(b.Lat*r) - math.Sin(a.Lat*r)*math.Cos(b.Lat*r)*math.Cos((b.Lon-a.Lon)*r)
	return math.Mod(math.Atan2(y, x)/r+360, 360)
}

// auroral is the mean auroral loss Lh at a mode's control points (Table 1d,
// §5.2.2), at the mid-path local time: the middle within 2000 km; the middle
// and the points 1000 km from each end within 4000 km for E modes and within
// dmax for F2 modes; past dmax, those and the points d0/2 from each end.
func (c circuit) auroral(eMode bool) float64 {
	fr := []float64{0.5}
	if c.km > eHopKm {
		fr = append(fr, screenAtKm/c.km, 1-screenAtKm/c.km)
	}
	dmax := limits.Dmax(c.mid.FoF2, c.mid.M3000, c.mid.FoE)
	if !eMode && c.km > dmax {
		half := 1 / (2 * limits.Hops(c.km, dmax)) // d0/2 of the path, as a part of it
		fr = append(fr, half, 1-half)
	}
	_, midLon := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, 0.5)
	at := c.sun.at.UTC()
	local := float64(at.Hour()) + float64(at.Minute())/60 + midLon/15
	sum := 0.0
	for _, f := range fr {
		lat, lon := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, f)
		sum += limits.AuroralLoss(c.km, lat, limits.GeomagneticLatitude(lat, lon), at.Month(), local)
	}
	return sum / float64(len(fr))
}

// snr is the circuit's median S/N at a frequency, dB in 2.5 kHz (§5.2 to
// §7): up to three E modes (to 4000 km) and six F2 modes from the lowest
// order, an F2 mode screened by the E layer dropped, as is a mode whose ray
// would leave below the horizon; each mode's absorption at its penetration
// points, or none when absorbed is false. False with no mode.
func (c circuit) snr(f float64, absorbed bool) (float64, bool) {
	e, ok := c.field(f, absorbed)
	if !ok {
		return 0, false
	}
	return limits.SNR(limits.PowerFromField(f, e), c.noise(f)), true
}

// field is the circuit's median field strength at a frequency, dB(1 µV/m):
// §5.2's modes summed to 7000 km, §5.3's composite past 9000 km, blended
// between (eq. 42); "without the absorption", §5.3's LUF is its night floor.
// False with no mode or no field.
func (c circuit) field(f float64, absorbed bool) (float64, bool) {
	var es, el float64
	if c.km <= shortToKm {
		fields := c.modes(f, absorbed)
		if len(fields) == 0 {
			return 0, false
		}
		es = limits.SumFields(fields)
	}
	if c.km <= longFromKm {
		return es, true
	}
	fl := c.long.fL
	if !absorbed {
		fl = c.long.fLN
	}
	el = limits.LongField(f, c.long.fM, fl, c.long.fH, c.long.slant, c.km)
	if el != el {
		return 0, false
	}
	if c.km > shortToKm {
		return el, true
	}
	return limits.BlendField(c.km, es, el), true
}

// noise is the noise factor the circuit's receiver hears at a frequency:
// the environment's man-made noise, and galactic noise where the frequency
// is above foF2 at the receiver (P.372-18).
func (c circuit) noise(f float64) float64 { return limits.NoiseFactor(c.env, f, f > c.rxFoF2) }

// modes are the field strengths of the modes the circuit counts at a
// frequency (§5.2.1), absorption included when asked.
func (c circuit) modes(f float64, absorbed bool) []float64 {
	var fields []float64
	if c.km <= eModesKm {
		n0 := int(math.Max(1, math.Ceil(c.km/eHopKm)))
		for n := n0; n < n0+eModes; n++ {
			fields = c.mode(fields, n, limits.ELayerKm, true, absorbed, f, c.mid.FoE/math.Cos(limits.IncidenceAngle(limits.ElevationAngle(c.km/float64(n), limits.ELayerKm), limits.ELayerKm)*math.Pi/180)) // eq. (1)
		}
	}
	dmax := limits.Dmax(c.mid.FoF2, c.mid.M3000, c.mid.FoE)
	n0 := int(limits.Hops(c.km, dmax))
	for n := n0; n < n0+f2Modes && n0 > 0; n++ {
		d := c.km / float64(n)
		hr := limits.MirrorHeight(c.mid.FoF2, c.mid.FoE, c.mid.M3000, c.r12, f, d)
		screened := c.km <= eModesKm && limits.ScreeningFrequency(c.screenFoE, limits.ElevationAngle(d, hr)) >= f
		if !screened {
			fields = c.mode(fields, n, hr, false, absorbed, f, limits.BasicMUF(d, c.mid.FoF2, c.mid.M3000, c.mid.FoE, c.mid.FH))
		}
	}
	return fields
}

// mode adds an n-hop mode's field strength from a mirror hr km high: its
// elevation, slant range, absorption (when asked), above-the-MUF loss over
// its basic MUF fb, and ground loss; a mode below the horizon, or with any
// part no number, adds nothing.
func (c circuit) mode(fields []float64, n int, hr float64, eMode, absorbed bool, f, fb float64) []float64 {
	d := c.km / float64(n)
	delta := limits.ElevationAngle(d, hr)
	if !(delta >= 0) {
		return fields
	}
	i110 := limits.IncidenceAngle(delta, limits.ELayerKm) * math.Pi / 180
	li, lh := 0.0, 0.0
	if absorbed { // the absorption: the day's (Li) and the auroral zone's (Lh)
		li, lh = c.absorption(n, delta, f, math.Cos(i110)), c.lhF2
		if eMode {
			lh = c.lhE
		}
	}
	ew := limits.ModeField(f, limits.SlantRange(n, d, delta), li, limits.AboveMUFLoss(eMode, f, fb), n, lh)
	if ew != ew {
		return fields
	}
	return append(fields, ew)
}

// absorption is an n-hop mode's Li (eq. 20): each penetration point's term
// - its latitude, the dip and gyrofrequency at 100 km, the sun there now and
// at local noon, foE there - averaged, with fL averaged over the points.
func (c circuit) absorption(n int, delta, f, cosI110 float64) float64 {
	fv := f * cosI110
	var terms, fl float64
	fr := limits.PenetrationFractions(c.km, n, delta)
	for _, at := range fr {
		lat, lon := limits.Along(c.from.Lat, c.from.Lon, c.to.Lat, c.to.Lon, at)
		fh, dip := magcoords.Field100(lat, lon, c.sun.at)
		decl, chi, since := c.sun.sunAt(lat, lon)
		foE := limits.FoE(lat, decl, chi, c.flux, since)
		terms += limits.AbsorptionTerm(limits.Penetration{Lat: lat, Dip: dip, Chi: chi, ChiNoon: math.Abs(lat - decl), FoE: foE}, c.sun.at.Month(), fv)
		fl += fh * math.Abs(math.Sin(dip*math.Pi/180))
	}
	k := float64(len(fr))
	return limits.AbsorptionLoss(n, c.r12, f, fl/k, cosI110, terms/k)
}

// lowerStatus is a band's status under the lower limit (R-2.4, D-70), at
// its highest frequency under the upper limit - its top, or the limit where
// the limit cuts through it, so a part above the MUF is the upper limit's:
// Open where the circuit reaches +13 dB, Absorbed where it would without
// the absorption, Weak where it would not even then.
func (c circuit) lowerStatus(b Band, upper float64) Status {
	return c.lowerAt(math.Min(b.HighMHz(), upper))
}

// lowerAt is the lower limit's status at a frequency: Open where the
// circuit reaches +13 dB, Absorbed where it would without the absorption,
// Weak where it would not even then (R-2.4, D-70).
func (c circuit) lowerAt(f float64) Status {
	snr, ok := c.snr(f, true)
	if !ok || snr >= limits.RequiredSNR {
		return Open
	}
	if bare, okB := c.snr(f, false); okB && bare >= limits.RequiredSNR {
		return Absorbed
	}
	return Weak
}

// closedBelow is whether the lower limit closes a band (R-2.4, D-70).
func (c circuit) closedBelow(b Band, upper float64) bool { return c.lowerStatus(b, upper) != Open }

// absorb marks each band open under its upper limit that the circuit cannot
// carry, Absorbed or Weak (D-70), at the LUF: where the S/N reaches +13 dB,
// between the highest such band's top and the next open band's top, or the
// upper limit (R-2.4).
func absorb(bands []BandStatus, c circuit, upper float64) []BandStatus {
	closed := make([]Status, len(bands))
	lo, hi := 0.0, upper
	for i, b := range bands {
		if b.Status != Open {
			continue
		}
		closed[i] = c.lowerStatus(b.Band, upper)
		if closed[i] != Open {
			lo, hi = b.Band.HighMHz(), upper
		} else if top := b.Band.HighMHz(); top > lo && top < hi && lo > 0 {
			hi = top
		}
	}
	if lo == 0 {
		return bands
	}
	luf := float32(c.luf(lo, math.Max(hi, lo)))
	for i, st := range closed {
		switch st {
		case Absorbed:
			bands[i] = BandStatus{Band: bands[i].Band, Status: Absorbed, Limit: LimitAbsorption, LimitMHz: luf}
		case Weak:
			bands[i] = BandStatus{Band: bands[i].Band, Status: Weak, Limit: LimitCircuit, LimitMHz: luf}
		}
	}
	return bands
}

// luf is the frequency between lo (failing) and hi where the circuit's S/N
// reaches +13 dB, by halving; hi where it does not reach it there.
func (c circuit) luf(lo, hi float64) float64 {
	if snr, ok := c.snr(hi, true); !ok || snr < limits.RequiredSNR {
		return hi
	}
	for range lufSteps {
		mid := (lo + hi) / 2
		if snr, ok := c.snr(mid, true); ok && snr >= limits.RequiredSNR {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

// areaEnds are the near-vertical area's representative path: half the
// radius west and east of the origin along the great circle through it
// (A-11).
func areaEnds(p LatLon, radiusKm float64) (LatLon, LatLon) {
	return destination(p, 270, radiusKm/2), destination(p, 90, radiusKm/2)
}

// destination is the place km along a bearing (degrees from north) from p,
// on the mean Earth sphere.
func destination(p LatLon, bearing, km float64) LatLon {
	r := math.Pi / 180
	d := km / 6371.0
	lat1, lon1, b := p.Lat*r, p.Lon*r, bearing*r
	lat2 := math.Asin(math.Sin(lat1)*math.Cos(d) + math.Cos(lat1)*math.Sin(d)*math.Cos(b))
	lon2 := lon1 + math.Atan2(math.Sin(b)*math.Sin(d)*math.Cos(lat1), math.Cos(d)-math.Sin(lat1)*math.Sin(lat2))
	return LatLon{Lat: lat2 / r, Lon: math.Mod(lon2/r+540, 360) - 180}
}

// circuitAt is the circuit in a typical hour of a typical day.
func (s Snapshot) circuitAt(t *typicalHour, day *[24]typicalHour, from, to LatLon) (circuit, bool) {
	return s.newCircuit(t.read, from, to, t.sun, day)
}
