// Command tables converts the one-time export of NRL's refits (watchpost
// D-114, tools/export) into the compact tables the library embeds, and
// copies PyIRI's golden values beside the climatology's tests. It reads only
// the plain-text export: no netCDF, no Python, standard library only.
//
//	cd tools/tables && go run . -root ../..
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// magic opens a compact table: the format's name and version.
const magic = "IONOTAB1"

// table is one exported table: its shape and its values in order (solar
// level, month, Fourier term, harmonic).
type table struct {
	levels, months, terms, harmonics int
	values                           []float64
}

// maxValues bounds a table: far past the refits' 237,600.
const maxValues = 4 << 20

// maxExportLines is the most lines an export is read to: one a solar level,
// month and Fourier term, and its comments, far under it.
const maxExportLines = 1 << 16

var errShape = errors.New("tables: the export has no shape line, or its values do not fill it")

// parseExport reads an export: comment lines first, one of them its shape,
// then one line of harmonics for each solar level, month and Fourier term.
func parseExport(r io.Reader) (table, error) {
	var t table
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for ; line < maxExportLines && sc.Scan(); line++ {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "# shape:") {
			if _, err := fmt.Sscanf(line, "# shape: %d %d %d %d", &t.levels, &t.months, &t.terms, &t.harmonics); err != nil {
				return table{}, errShape
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if t.harmonics <= 0 || len(fields) != t.harmonics {
			return table{}, errShape
		}
		for _, f := range fields {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return table{}, fmt.Errorf("tables: a value that is no finite number: %w", errShape)
			}
			t.values = append(t.values, v)
		}
		if len(t.values) > maxValues {
			return table{}, errShape
		}
	}
	if line == maxExportLines && sc.Scan() {
		return table{}, errShape // longer than an export's lines can be
	}
	if err := sc.Err(); err != nil {
		return table{}, err
	}
	if t.levels*t.months*t.terms*t.harmonics != len(t.values) || len(t.values) == 0 {
		return table{}, errShape
	}
	return t, nil
}

// writeCompact is a table in the compact form: the magic, the shape as four
// little-endian uint32s, then every value as a little-endian float64.
func writeCompact(t table) []byte {
	var b bytes.Buffer
	b.WriteString(magic)
	for _, n := range []int{t.levels, t.months, t.terms, t.harmonics} {
		_ = binary.Write(&b, binary.LittleEndian, uint32(n)) // a bytes.Buffer write never fails
	}
	_ = binary.Write(&b, binary.LittleEndian, t.values)
	return b.Bytes()
}

// readCompact reads the compact form back: its values, checked against the
// shape it states.
func readCompact(b []byte) ([]float64, error) {
	if len(b) < len(magic)+16 || string(b[:len(magic)]) != magic {
		return nil, errShape
	}
	n := 1
	for i := range 4 {
		n *= int(binary.LittleEndian.Uint32(b[len(magic)+4*i:]))
	}
	body := b[len(magic)+16:]
	if n <= 0 || len(body) != 8*n {
		return nil, errShape
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(body[8*i:]))
	}
	return out, nil
}

func main() {
	root := flag.String("root", "../..", "the repository's root")
	gyroOnly := flag.Bool("gyro", false, "write the field tables alone: the gyrofrequency at 300 km, the field at 100 km")
	flag.Parse()
	if *gyroOnly {
		writeGyroTable(*root)
		return
	}
	for _, c := range []struct{ from, to string }{{"foF2_CCIR.txt", "fof2_ccir.bin"}, {"M3000F2.txt", "m3000f2.bin"}} {
		f, err := os.Open(filepath.Join("export", c.from))
		if err != nil {
			fail(err)
		}
		t, err := parseExport(f)
		_ = f.Close() // read only
		if err != nil {
			fail(fmt.Errorf("%s: %w", c.from, err))
		}
		out := filepath.Join(*root, "internal", "climatology", "tables", c.to)
		if err := os.WriteFile(out, writeCompact(t), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("%s -> %s (%d values)\n", c.from, out, len(t.values))
	}
	model, err := loadIGRF(filepath.Join("igrf", "igrf14coeffs.txt"))
	if err != nil {
		fail(err)
	}
	years := []int{2025, 2026, 2027, 2028, 2029, 2030}
	coords, err := generateCoords(model, years)
	if err != nil {
		fail(err)
	}
	out := filepath.Join(*root, "internal", "magcoords", "coords.bin")
	if err := os.WriteFile(out, writeCoords(years, coords), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("igrf14coeffs.txt -> %s (%v)\n", out, years)
	writeGyroTable(*root)
	sample, err := os.ReadFile(filepath.Join("apex", "apex-sample.txt"))
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(*root, "internal", "magcoords", "testdata", "apex-sample.txt"), sample, 0o644); err != nil {
		fail(err)
	}
	for _, c := range []struct{ from, to string }{{"golden.txt", "pyiri-golden.txt"}, {"golden-day.txt", "pyiri-golden-day.txt"}} {
		golden, err := os.ReadFile(filepath.Join("export", c.from))
		if err != nil {
			fail(err)
		}
		out = filepath.Join(*root, "internal", "climatology", "testdata", c.to)
		if err := os.WriteFile(out, golden, 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("%s -> %s\n", c.from, out)
	}
}

// writeGyroTable writes the field tables from IGRF-14's coefficients, the
// coordinate table's years: the gyrofrequency at 300 km, and the
// gyrofrequency and dip at 100 km.
func writeGyroTable(root string) {
	model, err := loadIGRF(filepath.Join("igrf", "igrf14coeffs.txt"))
	if err != nil {
		fail(err)
	}
	years := []int{2025, 2026, 2027, 2028, 2029, 2030}
	out := filepath.Join(root, "internal", "magcoords", "gyro.bin")
	if err := os.WriteFile(out, writeGyro(years, generateGyro(model, years)), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("igrf14coeffs.txt -> %s (%v)\n", out, years)
	out = filepath.Join(root, "internal", "magcoords", "field100.bin")
	fh, dip := generateField100(model, years)
	if err := os.WriteFile(out, writeField100(years, fh, dip), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("igrf14coeffs.txt -> %s (%v)\n", out, years)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
