// Package terms is the static list of the sources the library reads, their
// hosts, terms and citations (R-4.1), never text parsed from replies; and of
// the third-party datasets committed to the repository (R-4.3).
package terms

// Source is one source: its name as the NOTICE names it, the hosts the
// library asks it on at run time (none for a source used only at build
// time), its terms and the citation it asks for, and the most one reply from
// it may be - which a host's client is to take no more than (watchpost
// FR-4.4) - zero for a source not asked at run time.
type Source struct {
	Name     string
	Hosts    []string
	Terms    string
	Cite     string
	MaxBytes int
}

// The bounds on one reply, by source: GIRO's, about ten times a station's
// week of five-minute rows (170 KB); NOAA's, about three times the
// measured 2.5 MB GloTEC grid, its largest file (watchpost D-141). The
// library's own checks read these.
const (
	GIROReplyBytes = 2 << 20
	NOAAReplyBytes = 8 << 20
)

// Dataset is a third-party dataset committed to the repository: its path and
// the source it comes from (R-4.3 lists those allowed).
type Dataset struct {
	Path   string
	Source string
}

// All is every source the library reads or is built from.
//
// Source: this library; each entry's terms and citation as the NOTICE gives
// them.
func All() []Source {
	return []Source{
		{Name: "GIRO", Hosts: []string{"lgdc.uml.edu"}, MaxBytes: GIROReplyBytes,
			Terms: "CC BY-NC-SA 4.0; offered only for educational and non-commercial research purposes",
			Cite:  "Reinisch, B. W., and I. A. Galkin, Global ionospheric radio observatory (GIRO), Earth, Planets and Space, 63, 377-381, doi:10.5047/eps.2011.03.001, 2011"},
		{Name: "NOAA Space Weather Prediction Center", Hosts: []string{"services.swpc.noaa.gov"}, MaxBytes: NOAAReplyBytes,
			Terms: "public domain; no endorsement by NOAA or the NWS is implied"},
		{Name: "PyIRI", Terms: "MIT licence, Copyright (c) 2023 victoriyaforsythe",
			Cite: "Forsythe, V. V., et al. (2024), PyIRI: whole-globe approach to the International Reference Ionosphere modeling implemented in Python, Space Weather, 22, doi:10.1029/2023SW003739"},
		{Name: "IGRF-14", Terms: "freely available from IAGA",
			Cite: "IAGA Working Group V-MOD, the International Geomagnetic Reference Field, 14th generation (IGRF-14), coefficients as published by IAGA in igrf14coeffs.txt"},
		{Name: "ITU-R P.533", Terms: "the published recommendation; no text reproduced; Tables 2, 3 and 5 ported, as the NOTICE lists them"},
		{Name: "ITUHFProp", Terms: "ITU-R Study Group 3's reference implementation of ITU-R P.533: \"may be used by implementers in their implementation of the Recommendation ... free from any copyright assertions\", as is, no warranty",
			Cite: "ITU-R Study Group 3, ITURHFProp and the P533 library, github.com/ITU-R-Study-Group-3/ITU-R-HF"},
		{Name: "P.1239", Terms: "the published recommendation; no text or tables reproduced"},
		{Name: "P.372", Terms: "the published recommendation; no text reproduced; Table 1's man-made noise constants ported"},
		{Name: "P.371", Terms: "the published recommendation; no text or tables reproduced"},
	}
}

// Datasets is every third-party dataset committed (R-4.3), each with its
// NOTICE entry: NRL's refits from PyIRI as exported once (watchpost D-114)
// and as converted for the library; PyIRI's own values at fixed inputs, the
// climatology's test oracle; IGRF-14's coefficients, their golden values,
// the coordinate table traced from them and the field tables; ITUHFProp's
// own absorption factors, their test oracle; and the Apex sample the
// coordinates are tested against.
//
// Source: this library.
func Datasets() []Dataset {
	return []Dataset{
		{Path: "tools/tables/export/foF2_CCIR.txt", Source: "PyIRI"},
		{Path: "tools/tables/export/M3000F2.txt", Source: "PyIRI"},
		{Path: "tools/tables/export/golden.txt", Source: "PyIRI"},
		{Path: "internal/climatology/tables/fof2_ccir.bin", Source: "PyIRI"},
		{Path: "internal/climatology/tables/m3000f2.bin", Source: "PyIRI"},
		{Path: "internal/climatology/testdata/pyiri-golden.txt", Source: "PyIRI"},
		{Path: "tools/tables/export/golden-day.txt", Source: "PyIRI"},
		{Path: "internal/climatology/testdata/pyiri-golden-day.txt", Source: "PyIRI"},
		{Path: "tools/tables/igrf/igrf14coeffs.txt", Source: "IGRF-14"},
		{Path: "tools/tables/igrf/golden-igrf.txt", Source: "IGRF-14"},
		{Path: "internal/magcoords/coords.bin", Source: "IGRF-14"},
		{Path: "internal/magcoords/gyro.bin", Source: "IGRF-14"},
		{Path: "internal/magcoords/field100.bin", Source: "IGRF-14"},
		{Path: "internal/limits/testdata/itu-absorption-golden.txt", Source: "ITUHFProp"},
		{Path: "tools/tables/apex/apex-sample.txt", Source: "PyIRI"},
		{Path: "internal/stations/seed.txt", Source: "GIRO"}, // station codes and positions, never readings (D-51)
		{Path: "internal/magcoords/testdata/apex-sample.txt", Source: "PyIRI"},
	}
}
