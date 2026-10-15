package ionomaps

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/limits"
	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// A mid-latitude path of about 1260 km whose middle is at 0°E, so noon UTC
// is its local noon and midnight UTC its local midnight.
var (
	westEnd = LatLon{Lat: 45, Lon: -8}
	eastEnd = LatLon{Lat: 45, Lon: 8}
)

// TestDaytimeAbsorptionClosesTheLowBands is R-2.4 (G6.2b, watchpost D-186 to
// D-189): on a mid-latitude path at local noon the reference circuit falls
// short of +13 dB on 80 m, which is absorbed, named as the lower limit at
// the LUF; at local midnight it is open; 20 m is open at both. The LUF lies
// between the highest absorbed band's top and the next open band's, where
// the S/N is +13 dB.
func TestDaytimeAbsorptionClosesTheLowBands(t *testing.T) {
	day := flatSnapshot(noonUTC, 7, 21)
	night := flatSnapshot(noonUTC.Add(12*time.Hour), 7, 21)
	d, err := day.Path(westEnd, []LatLon{eastEnd}, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	n, err := night.Path(westEnd, []LatLon{eastEnd}, noonUTC.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if st := statusOf(d[0].Bands, Band80m); st != Absorbed {
		t.Errorf("80 m at local noon is %v; want absorbed", st)
	}
	if st := statusOf(n[0].Bands, Band80m); st != Open {
		t.Errorf("80 m at local midnight is %v; want open", st)
	}
	if statusOf(d[0].Bands, Band20m) != Open || statusOf(n[0].Bands, Band20m) == Absorbed {
		t.Errorf("20 m: %v at noon, %v at midnight; open at noon, never absorbed", statusOf(d[0].Bands, Band20m), statusOf(n[0].Bands, Band20m))
	}
	var luf float64
	highest, next := 0.0, math.Inf(1)
	for _, b := range d[0].Bands {
		switch {
		case b.Status == Absorbed:
			if b.Limit != LimitAbsorption {
				t.Errorf("%v absorbed names limit %v", b.Band, b.Limit)
			}
			luf, highest = float64(b.LimitMHz), b.Band.HighMHz()
		case b.Status == Open && b.Band.HighMHz() > highest && b.Band.HighMHz() < next:
			next = b.Band.HighMHz()
		}
	}
	if !(luf >= highest && luf <= next) {
		t.Errorf("the LUF %.2f MHz is not between the highest absorbed band's top %.2f and the next open band's %.2f", luf, highest, next)
	}
	c, ok := day.newCircuit(hourReader(day.Hours[0]), westEnd, eastEnd, newSunTrack(noonUTC), nil)
	if snr, okS := c.snr(luf, true); !ok || !okS || math.Abs(snr-limits.RequiredSNR) > 0.2 {
		t.Errorf("the S/N at the LUF is %v (%v, %v); want +13 dB", snr, ok, okS)
	}
}

// TestTheNoiseEnvironmentMovesTheLowerLimit is D-187: a noisier
// environment needs more signal, so its LUF is higher; the environment is
// the snapshot's, set by the update's options.
func TestTheNoiseEnvironmentMovesTheLowerLimit(t *testing.T) {
	luf := func(env Noise) float64 {
		s := flatSnapshot(noonUTC, 7, 21)
		s.Noise = env
		p, _ := s.Path(westEnd, []LatLon{eastEnd}, noonUTC)
		for _, b := range p[0].Bands {
			if b.Status == Absorbed {
				return float64(b.LimitMHz)
			}
		}
		return 0
	}
	city, res, quiet := luf(NoiseCity), luf(NoiseResidential), luf(NoiseQuietRural)
	if !(city > res && res > quiet) {
		t.Errorf("LUF: city %.2f, residential %.2f, quiet rural %.2f MHz; noisier is higher", city, res, quiet)
	}
}

// TestTheTypicalDayKeepsTheLowerLimit is R-2.8 with R-2.4: a band is
// typically open in an hour only between both limits, so 80 m on the
// mid-latitude path is not typically open at local noon, and is at local
// midnight.
func TestTheTypicalDayKeepsTheLowerLimit(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	p, _ := s.Path(westEnd, []LatLon{eastEnd}, noonUTC)
	b80 := p[0].Bands[1]
	day := noonUTC.Truncate(24 * time.Hour)
	at := func(h int) bool {
		for _, sp := range b80.OpenHours {
			if t := day.Add(time.Duration(h) * time.Hour); !t.Before(sp.From) && t.Before(sp.To) {
				return true
			}
		}
		return false
	}
	if at(12) || !at(0) {
		t.Errorf("80 m typically open at noon %v, at midnight %v; want closed by day, open by night (%v)", at(12), at(0), b80.OpenHours)
	}
}

// TestTheLongPathHasALowerLimit is §5.3 (D-191): past 7000 km the circuit
// is made from the typical day beside the snapshot, and on a 10000 km path
// at 0 dBi every band under the MUF is closed, named as the circuit's or the
// absorption's limit; without the day no long circuit is made.
func TestTheLongPathHasALowerLimit(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	p, err := s.Path(LatLon{Lat: 40, Lon: -100}, []LatLon{{Lat: 40, Lon: 0}}, noonUTC)
	if err != nil {
		t.Fatal(err)
	}
	closed := 0
	for _, b := range p[0].Bands {
		switch b.Status {
		case Weak, Absorbed:
			closed++
			if b.Limit != LimitCircuit && b.Limit != LimitAbsorption {
				t.Errorf("%v closed, named %v", b.Band, b.Limit)
			}
		case Open:
			t.Errorf("%v open on a 10000 km path at 0 dBi", b.Band)
		}
	}
	if closed == 0 {
		t.Error("no band was closed by the lower limit past 7000 km")
	}
	if _, ok := s.newCircuit(hourReader(s.Hours[0]), LatLon{Lat: 40, Lon: -100}, LatLon{Lat: 40, Lon: 0}, newSunTrack(noonUTC), nil); ok {
		t.Error("a long circuit was made with no typical day")
	}
}

// TestTheLongPathIsP533s is §5.3 and §5.4 in the circuit: past 9000 km the
// field is eq. (39)'s from the path's fM, this hour's fL, the mean
// gyrofrequency and fM's slant range; from 7000 to 9000 km it is eq. (42)'s
// blend with §5.2's; fM is the lower of the two control points' K fBM; fL
// is the night LUF at local midnight along the path and above it at noon.
func TestTheLongPathIsP533s(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	day := s.typicalDay(noonUTC)
	from, to := LatLon{Lat: 20, Lon: -60}, LatLon{Lat: 20, Lon: 40}
	c, ok := s.newCircuit(hourReader(s.Hours[0]), from, to, newSunTrack(noonUTC), &day)
	if !ok || c.km <= shortToKm {
		t.Fatalf("a %v km circuit (%v); the test needs one past 9000 km", c.km, ok)
	}
	if got, _ := c.field(14, true); math.Abs(got-limits.LongField(14, c.long.fM, c.long.fL, c.long.fH, c.long.slant, c.km)) > 1e-9 {
		t.Errorf("past 9000 km the field %v is not eq. (39)'s", got)
	}
	if bare, _ := c.field(14, false); math.Abs(bare-limits.LongField(14, c.long.fM, c.long.fLN, c.long.fH, c.long.slant, c.km)) > 1e-9 {
		t.Errorf("without the absorption the field %v is not eq. (39)'s at the night LUF", bare)
	}
	nM, dM, deltaM := limits.LongHops(c.km)
	if c.long.slant != limits.SlantRange(nM, dM, deltaM) {
		t.Errorf("the slant range %v is not fM's hops'", c.long.slant)
	}
	midLat, midLon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, 0.5)
	bearing := bearingFrom(LatLon{midLat, midLon}, to)
	fM, fH := math.Inf(1), 0.0
	for _, fr := range []float64{dM / (2 * c.km), 1 - dM/(2*c.km)} {
		lat, lon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, fr)
		fo, m3, _ := hourReader(s.Hours[0])(LatLon{lat, lon})
		gh := magcoords.Gyro(lat, lon, noonUTC)
		fH += gh / 2
		noonH := int(math.Mod(math.Round(12-lon/15)+48, 24))
		least, noon := math.Inf(1), 0.0
		for h := range day {
			foH, m3H, _ := day[h].read(LatLon{lat, lon})
			v := limits.LongBasicMUF(foH, m3H, gh, dM)
			least = math.Min(least, v)
			if h == noonH {
				noon = v
			}
		}
		fbm := limits.LongBasicMUF(fo, m3, gh, dM)
		fM = math.Min(fM, limits.KFactor(fbm, noon, least, bearing)*fbm)
	}
	if math.Abs(c.long.fM-fM) > 1e-9 || math.Abs(c.long.fH-fH) > 1e-12 {
		t.Errorf("fM %v and fH %v; the two control points give %v and %v", c.long.fM, c.long.fH, fM, fH)
	}
	nL := math.Ceil(c.km / limits.FLHopKm)
	deltaL := limits.ElevationAngle(c.km/nL, limits.LongMirrorKm)
	var fl [24]float64
	for h := range day {
		sum := 0.0
		for _, fr := range limits.PenetrationFractions(c.km, int(nL), deltaL) {
			lat, lon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, fr)
			_, chi, _ := day[h].sun.sunAt(lat, lon)
			sum += math.Sqrt(math.Max(math.Cos(chi*math.Pi/180), 0))
		}
		fl[h] = limits.LongLUF(c.r12, sum, limits.IncidenceAngle(deltaL, limits.PenetrationKm), c.long.slant, fH, limits.WinterAnomaly(midLat, time.October))
	}
	if want := limits.LUFDecay(fl, c.long.fLN)[12]; math.Abs(c.long.fL-want) > 1e-9 {
		t.Errorf("fL at noon %v; eq. (33) through the day, decayed, gives %v", c.long.fL, want)
	}
	midnight := noonUTC.Add(-12 * time.Hour) // the path's middle is at 10° W: local midnight about 00:40
	night, _ := s.newCircuit(hourReader(s.Hours[0]), from, to, newSunTrack(midnight.Add(time.Hour)), &day)
	if math.Abs(night.long.fL-night.long.fLN) > 1e-9 || !(c.long.fL > c.long.fLN) {
		t.Errorf("fL %v at night (night LUF %v), %v at noon; the night LUF by night, above it by day", night.long.fL, night.long.fLN, c.long.fL)
	}
	mid, _ := s.newCircuit(hourReader(s.Hours[0]), LatLon{Lat: 20, Lon: -40}, LatLon{Lat: 20, Lon: 40}, newSunTrack(noonUTC), &day)
	if !(mid.km > longFromKm && mid.km < shortToKm) {
		t.Fatalf("a %v km circuit; the blend needs 7000 to 9000", mid.km)
	}
	es := limits.SumFields(mid.modes(14, true))
	el := limits.LongField(14, mid.long.fM, mid.long.fL, mid.long.fH, mid.long.slant, mid.km)
	if got, _ := mid.field(14, true); math.Abs(got-limits.BlendField(mid.km, es, el)) > 1e-9 {
		t.Errorf("at %v km the field %v is not eq. (42)'s blend of %v and %v", mid.km, got, es, el)
	}
}

// TestTheNearVerticalAreaHasALowerLimit is R-2.4 for the area: at local
// noon the area's 160 m is absorbed, at midnight open.
func TestTheNearVerticalAreaHasALowerLimit(t *testing.T) {
	day, night := flatSnapshot(noonUTC, 7, 21), flatSnapshot(noonUTC.Add(12*time.Hour), 7, 21)
	a, _ := day.Bands([]LatLon{{Lat: 45, Lon: 0}}, 400, noonUTC)
	b, _ := night.Bands([]LatLon{{Lat: 45, Lon: 0}}, 400, noonUTC.Add(12*time.Hour))
	if statusOf(a[0].Bands, Band160m) != Absorbed || statusOf(b[0].Bands, Band160m) != Open {
		t.Errorf("the area's 160 m: %v at noon, %v at midnight; want absorbed, open", statusOf(a[0].Bands, Band160m), statusOf(b[0].Bands, Band160m))
	}
}

// TestALongPathTooWeakForTheCircuitIsWeak is D-70: at 5000 km at local
// midnight, absorption all but gone, the reference circuit still cannot
// reach +13 dB, so every band under the MUF is Weak, named as the circuit's
// limit - not Absorbed.
func TestALongPathTooWeakForTheCircuitIsWeak(t *testing.T) {
	night := noonUTC.Add(12 * time.Hour)
	s := flatSnapshot(night, 7, 21)
	from, to := destination(LatLon{Lat: 45, Lon: 0}, 270, 2500), destination(LatLon{Lat: 45, Lon: 0}, 90, 2500)
	p, err := s.Path(from, []LatLon{to}, night)
	if err != nil {
		t.Fatal(err)
	}
	weak := 0
	for _, b := range p[0].Bands {
		switch b.Status {
		case Weak:
			weak++
			if b.Limit != LimitCircuit {
				t.Errorf("%v weak, named %v", b.Band, b.Limit)
			}
		case Absorbed, Open:
			t.Errorf("%v is %v at 5000 km at midnight; the circuit cannot reach +13 dB even without absorption", b.Band, b.Status)
		}
	}
	if weak == 0 {
		t.Error("no band was weak")
	}
}

// TestTheCircuitCountsP533sModes is §5.2.1 and §4: the circuit counts up to
// three E modes (to 4000 km) and six F2 modes, an F2 mode the E layer screens
// at the frequency left out, and a mode whose ray would leave below the
// horizon left out too; screening reads the higher foE of the points 1000 km
// from each end.
func TestTheCircuitCountsP533sModes(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	from, to := LatLon{40, -100}, LatLon{40, -64.8} // about 3000 km, sunrise along it
	c, ok := s.newCircuit(hourReader(s.Hours[0]), from, to, newSunTrack(noonUTC), nil)
	if !ok {
		t.Fatal("no circuit")
	}
	a, _ := s.controlPoint(hourReader(s.Hours[0]), from, to, 1000/c.km, newSunTrack(noonUTC))
	b, _ := s.controlPoint(hourReader(s.Hours[0]), from, to, 1-1000/c.km, newSunTrack(noonUTC))
	if a.FoE == b.FoE || c.screenFoE != math.Max(a.FoE, b.FoE) {
		t.Errorf("screening's foE %v; the points 1000 km from each end give %v and %v, the higher wanted (and different, for the test to measure)", c.screenFoE, a.FoE, b.FoE)
	}
	f := 3.8
	unscreened := 0
	dmax := limits.Dmax(c.mid.FoF2, c.mid.M3000, c.mid.FoE)
	n0 := int(limits.Hops(c.km, dmax))
	for n := n0; n < n0+f2Modes; n++ {
		d := c.km / float64(n)
		delta := limits.ElevationAngle(d, limits.MirrorHeight(c.mid.FoF2, c.mid.FoE, c.mid.M3000, c.r12, f, d))
		if limits.ScreeningFrequency(c.screenFoE, delta) < f && delta >= 0 {
			unscreened++
		}
	}
	if got := len(c.modes(f, true)); got != eModes+unscreened || unscreened == f2Modes {
		t.Errorf("at %v MHz over %.0f km %d modes counted; three E modes and the %d F2 modes the E layer does not screen (of six)", f, c.km, got, unscreened)
	}
	high := flatSnapshot(noonUTC, 12, 43.2) // a high, low-lying F2 layer: its two-hop mode over 6900 km would leave below the horizon
	long, okL := high.newCircuit(hourReader(high.Hours[0]), destination(LatLon{45, -40}, 270, 3450), destination(LatLon{45, -40}, 90, 3450), newSunTrack(noonUTC), nil)
	if !okL {
		t.Fatal("no circuit at 6900 km")
	}
	below := 0
	dl := limits.Dmax(long.mid.FoF2, long.mid.M3000, long.mid.FoE)
	nl := int(limits.Hops(long.km, dl))
	for n := nl; n < nl+f2Modes; n++ {
		d := long.km / float64(n)
		if limits.ElevationAngle(d, limits.MirrorHeight(long.mid.FoF2, long.mid.FoE, long.mid.M3000, long.r12, 7.2, d)) < 0 {
			below++
		}
	}
	if got := len(long.modes(7.2, true)); below == 0 || got != f2Modes-below {
		t.Errorf("6900 km at 7.2 MHz: %d modes counted, %d of six F2 modes below the horizon; those left out, and at least one, for the test to measure", got, below)
	}
}

// TestABandTheMUFCutsIsJudgedAtTheMUF is R-2.4: a band whose top is above
// the upper limit is judged by the lower limit at the upper limit, so the
// part above the MUF never makes it absorbed or weak: 15 m, 21.0 to 21.45
// MHz, under a 21.04 MHz MUF, is open.
func TestABandTheMUFCutsIsJudgedAtTheMUF(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	p, _ := s.Path(LatLon{40, -100}, []LatLon{{40, -64.8}}, noonUTC)
	if muf := mufOf(p[0].Bands); !(muf > 21 && muf < 21.45) {
		t.Fatalf("the MUF is %.2f; the test needs it inside 15 m", muf)
	}
	if st := statusOf(p[0].Bands, Band15m); st != Open {
		t.Errorf("15 m, cut by the MUF, is %v; judged at the MUF it is open", st)
	}
}

// TestGalacticNoiseIsHeardAboveFoF2 is P.372 in the circuit: the receiver
// hears galactic noise only above foF2 there.
func TestGalacticNoiseIsHeardAboveFoF2(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	c, _ := s.newCircuit(hourReader(s.Hours[0]), westEnd, eastEnd, newSunTrack(noonUTC), nil)
	if c.noise(5) != limits.NoiseFactor(limits.Residential, 5, false) || c.noise(10) != limits.NoiseFactor(limits.Residential, 10, true) {
		t.Errorf("noise at 5 and 10 MHz under foF2 7: %v, %v", c.noise(5), c.noise(10))
	}
}

// TestTheAreasPathIsCentredOnIt is A-11: the near-vertical area's
// representative path runs half the radius west and half east of the
// origin, its middle the origin.
func TestTheAreasPathIsCentredOnIt(t *testing.T) {
	o := LatLon{Lat: 45, Lon: 10}
	w, e := areaEnds(o, 400)
	dw, de := limits.Distance(w.Lat, w.Lon, o.Lat, o.Lon), limits.Distance(o.Lat, o.Lon, e.Lat, e.Lon)
	mlat, mlon := limits.Along(w.Lat, w.Lon, e.Lat, e.Lon, 0.5)
	if math.Abs(dw-200) > 1e-6 || math.Abs(de-200) > 1e-6 || limits.Distance(mlat, mlon, o.Lat, o.Lon) > 1e-6 {
		t.Errorf("the area's ends are %.3f and %.3f km from it, its middle %.6f km off it; want 200, 200, 0", dw, de, limits.Distance(mlat, mlon, o.Lat, o.Lon))
	}
}

// TestAModesAbsorptionIsEq20 is eq. (20) in the circuit: a mode's Li is the
// hops times (1 + 0.0067 R12) times the mean of its penetration points'
// terms, over (f + fL)^2 and cos i at 110 km, fL the mean of fH sin I at
// 100 km at those points - each read where the mode crosses 90 km.
func TestAModesAbsorptionIsEq20(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	c, _ := s.newCircuit(hourReader(s.Hours[0]), westEnd, eastEnd, newSunTrack(noonUTC), nil)
	n, f := 2, 3.8
	delta := limits.ElevationAngle(c.km/float64(n), 250)
	cosI := math.Cos(limits.IncidenceAngle(delta, limits.ELayerKm) * math.Pi / 180)
	var terms, fl float64
	fr := limits.PenetrationFractions(c.km, n, delta)
	for _, at := range fr {
		lat, lon := limits.Along(westEnd.Lat, westEnd.Lon, eastEnd.Lat, eastEnd.Lon, at)
		fh, dip := magcoords.Field100(lat, lon, noonUTC)
		decl, chi, since := c.sun.sunAt(lat, lon)
		terms += limits.AbsorptionTerm(limits.Penetration{Lat: lat, Dip: dip, Chi: chi, ChiNoon: math.Abs(lat - decl), FoE: limits.FoE(lat, decl, chi, c.flux, since)}, time.October, f*cosI)
		fl += fh * math.Abs(math.Sin(dip*math.Pi/180))
	}
	want := limits.AbsorptionLoss(n, c.r12, f, fl/float64(len(fr)), cosI, terms/float64(len(fr)))
	if got := c.absorption(n, delta, f, cosI); math.Abs(got-want) > 1e-9 || !(fl > 0) {
		t.Errorf("a two-hop mode's Li at %v MHz: %v dB; eq. (20) over its penetration points gives %v", f, got, want)
	}
}

// TestTheAuroralZoneAddsItsLoss is §5.2.2's Lh (D-189): a high-latitude
// path carries Table 2's loss at its control points and its S/N falls by
// it; a low-latitude path carries none; taking the absorption out (D-70)
// takes Lh out with Li, as both are the D region's.
func TestTheAuroralZoneAddsItsLoss(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	read, sun := hourReader(s.Hours[0]), newSunTrack(noonUTC)
	north, ok := s.newCircuit(read, LatLon{64.8, -147.7}, LatLon{69.6, 18.9}, sun, nil) // Fairbanks to Tromsø, over the pole
	if !ok {
		t.Fatal("no circuit")
	}
	if !(north.lhF2 > 0) || !(north.lhE >= 0) {
		t.Fatalf("the polar path's Lh: E %v, F2 %v; Table 2 gives it a loss", north.lhE, north.lhF2)
	}
	bare := north
	bare.lhE, bare.lhF2 = 0, 0
	with, _ := north.snr(14.35, true)
	without, _ := bare.snr(14.35, true)
	if !(with < without) {
		t.Errorf("Lh did not lower the polar path's S/N: %v with it, %v without", with, without)
	}
	if a, _ := north.snr(14.35, false); a != func() float64 { v, _ := bare.snr(14.35, false); return v }() {
		t.Error("the S/N without the absorption kept Lh")
	}
	south, _ := s.newCircuit(read, LatLon{10, -20}, LatLon{10, 10}, sun, nil)
	if south.lhE != 0 || south.lhF2 != 0 {
		t.Errorf("a path at 10° N carries Lh %v, %v; Table 2 starts at 42.5° geomagnetic", south.lhE, south.lhF2)
	}
}

// TestLhIsReadAtTable1dsPoints is §5.2.2 and Table 1d: past dmax an F2 mode's
// Lh is the mean at the middle, the points 1000 km from each end and those
// d0/2 from each end, at the mid-path local time.
func TestLhIsReadAtTable1dsPoints(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	from, to := LatLon{64.8, -147.7}, LatLon{69.6, 18.9}
	c, _ := s.newCircuit(hourReader(s.Hours[0]), from, to, newSunTrack(noonUTC), nil)
	dmax := limits.Dmax(c.mid.FoF2, c.mid.M3000, c.mid.FoE)
	if !(c.km > dmax) {
		t.Fatalf("the path, %.0f km, is within dmax %.0f: the test needs it past", c.km, dmax)
	}
	half := 1 / (2 * limits.Hops(c.km, dmax))
	_, midLon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, 0.5)
	local := 12 + midLon/15
	var sum float64
	fr := []float64{0.5, 1000 / c.km, 1 - 1000/c.km, half, 1 - half}
	for _, f := range fr {
		lat, lon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, f)
		sum += limits.AuroralLoss(c.km, lat, limits.GeomagneticLatitude(lat, lon), time.October, local)
	}
	if want := sum / 5; math.Abs(c.lhF2-want) > 1e-12 {
		t.Errorf("the F2 modes' Lh is %v; Table 2 at Table 1d's five points gives %v", c.lhF2, want)
	}
	var e float64
	for _, f := range fr[:3] {
		lat, lon := limits.Along(from.Lat, from.Lon, to.Lat, to.Lon, f)
		e += limits.AuroralLoss(c.km, lat, limits.GeomagneticLatitude(lat, lon), time.October, local)
	}
	if want := e / 3; math.Abs(c.lhE-want) > 1e-12 {
		t.Errorf("the E modes' Lh is %v; at the three points %v", c.lhE, want)
	}
}

// TestAnEModeTakesItsOwnLh is Table 1d: E and F2 modes read Lh at different
// points once a path is past dmax and still within 4000 km, and an E mode's
// field carries the E modes' Lh.
func TestAnEModeTakesItsOwnLh(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 28) // M(3000)F2 4: dmax under 4000 km
	from := LatLon{40, -60}
	to := destination(from, 0, 3950) // north from 40° N into the auroral zone
	c, ok := s.newCircuit(hourReader(s.Hours[0]), from, to, newSunTrack(noonUTC), nil)
	if !ok || c.lhE == c.lhF2 {
		t.Fatalf("E and F2 Lh %v, %v (%v): the test needs them different", c.lhE, c.lhF2, ok)
	}
	f, n := 3.8, 2
	delta := limits.ElevationAngle(c.km/float64(n), limits.ELayerKm)
	i110 := limits.IncidenceAngle(delta, limits.ELayerKm) * math.Pi / 180
	fb := c.mid.FoE / math.Cos(i110)
	got := c.mode(nil, n, limits.ELayerKm, true, true, f, fb)
	want := limits.ModeField(f, limits.SlantRange(n, c.km/float64(n), delta), c.absorption(n, delta, f, math.Cos(i110)), limits.AboveMUFLoss(true, f, fb), n, c.lhE)
	if len(got) != 1 || math.Abs(got[0]-want) > 1e-9 {
		t.Errorf("a two-hop E mode's field %v; with the E modes' Lh %v", got, want)
	}
}
