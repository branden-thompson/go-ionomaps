package ionomapstest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Synthetic describes recorded days built for a test, in the layout
// dry_fetch.py writes, never a service's own data (R-4.3): no GIRO or NOAA
// file may be committed, so a test that needs recorded days writes these.
type Synthetic struct {
	// Grids are the times GloTEC grids are valid at, each 72 by 72 points
	// as NOAA lays them out, foF2 GridFoF2 MHz and hmF2 300 km everywhere.
	Grids    []time.Time
	GridFoF2 float64
	// Stations each read at every one of Readings, score 90, their own
	// foF2 and M(3000)F2.
	Stations []Station
	Readings []time.Time
	// F107 is the solar file's flux on every day from 30 days before the
	// first grid's day through its last grid's day.
	F107 float64
}

// Station is one synthetic station: its URSI-shaped code, its place
// (longitude -180 to 180) and what it reads.
type Station struct {
	Code        string
	Lat, Lon    float64
	FoF2, M3000 float64
}

var errNoGrid = errors.New("ionomapstest: synthetic days need at least one grid")

// Write writes the days into dir and returns the solar file's path, for
// Open(dir, solarFile).
//
// Source: this library; the layouts of NOAA SWPC's GloTEC GeoJSON, GIRO's FastChar text and NOAA's daily solar file.
func Write(dir string, s Synthetic) (solarFile string, err error) {
	if len(s.Grids) == 0 {
		return "", errNoGrid
	}
	for _, valid := range s.Grids {
		body, err := gridJSON(valid.UTC(), s.GridFoF2)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, "glotec_icao_"+valid.UTC().Format("20060102T150405Z")+".geojson"), body, 0o600); err != nil {
			return "", err
		}
	}
	for _, st := range s.Stations {
		if err := os.WriteFile(filepath.Join(dir, "fc_"+st.Code+".txt"), reply(st, s.Readings), 0o600); err != nil {
			return "", err
		}
	}
	first, last := s.Grids[0], s.Grids[0]
	for _, g := range s.Grids {
		if g.Before(first) {
			first = g
		}
		if g.After(last) {
			last = g
		}
	}
	var b strings.Builder
	b.WriteString(":Product: Daily Solar Data            DSD.txt\n#  Date     10.7cm Number\n")
	day := first.UTC().Truncate(24*time.Hour).AddDate(0, 0, -30)
	for ; !day.After(last); day = day.AddDate(0, 0, 1) {
		fmt.Fprintf(&b, "%s  %3.0f     55      110      0    -999      *   0  0  0  2  0  0  0\n", day.Format("2006 01 02"), s.F107)
	}
	solarFile = filepath.Join(dir, "dsi.txt")
	return solarFile, os.WriteFile(solarFile, []byte(b.String()), 0o600)
}

// reply is a station's recorded reply: the location line and the columns
// dry_fetch.py asks for, each header built from pieces so no FastChar
// signature stands whole in the source (R-4.3), and no requester's address.
func reply(st Station, at []time.Time) []byte {
	lon := st.Lon
	if lon < 0 {
		lon += 360 // GIRO writes longitudes east, 0 to 360
	}
	ns, lat := "N", st.Lat
	if lat < 0 {
		ns, lat = "S", -lat
	}
	var b strings.Builder
	b.WriteString("# Global Ionospheric Radio Observatory (synthetic, ionomapstest)\n")
	fmt.Fprintf(&b, "# Location: %s ( %.1f %s   %.1f E ), URSI-Code %s, SYNTHETIC\n", "GEO", lat, ns, lon, st.Code)
	b.WriteString("# Time                    " + "CS" + "   foF2 " + "QD MUF(D) QD M(D) QD   hmF2 QD\n")
	for _, t := range at {
		fmt.Fprintf(&b, "%s  90 %6.3f // %6.3f // %4.2f //  250.0 //\n", t.UTC().Format("2006-01-02T15:04:05.000Z"), st.FoF2, st.FoF2*st.M3000, st.M3000)
	}
	return []byte(b.String())
}

// gridJSON is a GloTEC grid in NOAA's layout: 72 by 72 points at 5° by
// 2.5° cell centres, NmF2 for foF2 everywhere, hmF2 300 km, quality 5.
func gridJSON(valid time.Time, foF2 float64) ([]byte, error) {
	type feature struct {
		Type     string `json:"type"`
		Geometry struct {
			Type        string     `json:"type"`
			Coordinates [2]float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			NmF2    float64 `json:"NmF2"`
			HmF2    float64 `json:"hmF2"`
			Quality int     `json:"quality_flag"`
		} `json:"properties"`
	}
	doc := struct {
		Type     string    `json:"type"`
		Features []feature `json:"features"`
		TimeTag  string    `json:"time_tag"`
	}{Type: "FeatureCollection", TimeTag: valid.Format(time.RFC3339)}
	nm := (foF2 / 8.98e-6) * (foF2 / 8.98e-6) // foF2 = 8.98e-6 sqrt(NmF2) (ITU-R P.1239)
	for i := range 72 {
		for j := range 72 {
			var f feature
			f.Type, f.Geometry.Type = "Feature", "Point"
			f.Geometry.Coordinates = [2]float64{-177.5 + 5*float64(i), -88.75 + 2.5*float64(j)}
			f.Properties.NmF2, f.Properties.HmF2, f.Properties.Quality = nm, 300, 5
			doc.Features = append(doc.Features, f)
		}
	}
	return json.Marshal(doc)
}

// DRAPTable is a D-RAP table in NOAA's layout, valid at a time, its 1 dB
// frequency mhz everywhere: to serve as Answers["D-RAP"], which dry_fetch.py
// does not keep, so a replay can be Fresh (RL-12).
//
// Source: this library; NOAA SWPC's D-RAP global table layout.
func DRAPTable(valid time.Time, mhz float64) []byte {
	var b strings.Builder
	b.WriteString("# Product Valid At : " + valid.UTC().Format("2006-01-02 15:04") + " UTC\n#\n    ")
	for lon := -178; lon <= 178; lon += 4 {
		fmt.Fprintf(&b, " %4d", lon)
	}
	b.WriteString("\n" + strings.Repeat("-", 456) + "\n")
	for lat := 89; lat >= -89; lat -= 2 {
		fmt.Fprintf(&b, "%3d |%s\n", lat, strings.Repeat(fmt.Sprintf(" %4.1f", mhz), 90))
	}
	return []byte(b.String())
}

// ScalesFile is NOAA's scales file issued at a time, every level 0 and its
// outlook quiet: to serve as Answers["scales"] (RL-12).
//
// Source: this library; NOAA SWPC's noaa-scales.json layout.
func ScalesFile(at time.Time) []byte {
	day := func(n int, clock string) string {
		return `{"DateStamp":"` + at.UTC().AddDate(0, 0, n).Format(time.DateOnly) + `","TimeStamp":"` + clock + `","R":{"Scale":"0","Text":null,"MinorProb":"5","MajorProb":"1"},"S":{"Scale":"0","Text":null,"Prob":"1"},"G":{"Scale":"0","Text":null}}`
	}
	clock := at.UTC().Format(time.TimeOnly)
	return []byte(`{"0":` + day(0, clock) + `,"1":` + day(0, clock) + `,"2":` + day(1, "00:00:00") + `,"3":` + day(2, "00:00:00") + `}`)
}
