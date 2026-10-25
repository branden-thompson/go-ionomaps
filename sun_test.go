package ionomaps

import (
	"math"
	"testing"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/magcoords"
)

// TestTheSunTrackReadsEachPlace is foE's sun (P.1239-4 §4, R-2.7): the
// declination, the zenith angle and, at night, the hours since sunset that
// a sun track gives a place agree with the sun worked out afresh, the hours
// since sunset stepped back a minute at a time, to within the track's ten
// minutes, and none in day; a polar night is 24 hours since sunset.
func TestTheSunTrackReadsEachPlace(t *testing.T) {
	at := noonUTC.Add(17 * time.Minute)
	track := newSunTrack(at)
	r := math.Pi / 180
	zenith := func(lat, lon float64, when time.Time) float64 {
		slon, slat := magcoords.Subsolar(when)
		c := math.Sin(lat*r)*math.Sin(slat*r) + math.Cos(lat*r)*math.Cos(slat*r)*math.Cos((lon-slon)*r)
		return math.Acos(math.Max(-1, math.Min(1, c))) / r
	}
	_, slat := magcoords.Subsolar(at)
	nights := 0
	for lat := -60.0; lat <= 60; lat += 30 {
		for lon := -180.0; lon < 180; lon += 20 {
			decl, chi, since := track.sunAt(lat, lon)
			if math.Abs(decl-slat) > 1e-9 || math.Abs(chi-zenith(lat, lon, at)) > 1e-9 {
				t.Fatalf("(%v, %v): declination %v and zenith %v; the sun gives %v and %v", lat, lon, decl, chi, slat, zenith(lat, lon, at))
			}
			want := 0.0
			for m := 0; chi >= 90 && m <= 24*60; m++ { // a day back, a minute at a time (P10-02)
				if zenith(lat, lon, at.Add(-time.Duration(m)*time.Minute)) < 90 {
					want = float64(m) / 60
					break
				}
			}
			if chi < 90 && since != 0 {
				t.Errorf("(%v, %v) is in day, the sun %v° from overhead, yet %v hours past sunset", lat, lon, chi, since)
			}
			if chi >= 90 {
				nights++
			}
			if since < want || since > want+1.0/6+1e-9 {
				t.Errorf("(%v, %v): %v hours since sunset; the sun set %v hours before", lat, lon, since, want)
			}
		}
	}
	if nights < 10 {
		t.Fatalf("only %d of the places were in night; the check needs the night side", nights)
	}
	if _, _, since := track.sunAt(89, 0); since != 24 {
		t.Errorf("near the north pole in October the sun has not risen for a day; %v hours since sunset", since)
	}
}

// TestTheSunsStepsPassedOverChangeNothing is watchpost RP-5: sunAt passes
// over the steps the sun cannot have risen in, and gives every place, at
// every moment, the hours since sunset that a step at a time gives - the
// polar edges, where a day can be shorter than an hour, among them.
func TestTheSunsStepsPassedOverChangeNothing(t *testing.T) {
	r := math.Pi / 180
	checked := 0
	for _, at := range []time.Time{noonUTC, noonUTC.AddDate(0, 2, 3).Add(5 * time.Hour), noonUTC.AddDate(0, 4, 0).Add(-7 * time.Hour), noonUTC.AddDate(0, 8, 11).Add(11 * time.Hour)} {
		track := newSunTrack(at)
		for lat := -89.5; lat <= 89.5; lat += 0.5 {
			for lon := -180.0; lon < 180; lon += 7 {
				_, chi, since := track.sunAt(lat, lon)
				if chi < 90 {
					continue
				}
				sl, cl := math.Sincos(lat * r)
				want := 24.0
				for i := 1; i <= sunSteps; i++ { // a step at a time (P10-02)
					if sl*track.sin[i]+cl*track.cos[i]*math.Cos((lon-track.lon[i])*r) > 0 {
						want = float64(i) / 6
						break
					}
				}
				checked++
				if since != want {
					t.Fatalf("%v at (%v, %v): %v hours since sunset; a step at a time gives %v", at, lat, lon, since, want)
				}
			}
		}
	}
	if checked < 10000 {
		t.Fatalf("only %d night places checked", checked)
	}
}
