package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheExportMatchesItsRecordedChecksums is plan task G3.2 (watchpost
// D-114): the committed export is exactly what the one-time script wrote, as
// SHA256SUMS records; a byte changed by hand is caught.
func TestTheExportMatchesItsRecordedChecksums(t *testing.T) {
	sums, err := os.ReadFile(filepath.Join("export", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) < 4 {
		t.Fatalf("SHA256SUMS records %d files; the export is two tables and two goldens", len(lines))
	}
	for _, line := range lines {
		want, name, ok := strings.Cut(line, "  ")
		if !ok {
			t.Fatalf("SHA256SUMS line %q", line)
		}
		body, err := os.ReadFile(filepath.Join("export", name))
		if err != nil {
			t.Fatal(err)
		}
		if got := sum(body); got != want {
			t.Errorf("%s: sha256 %s, recorded %s: the export was changed after the script wrote it", name, got, want)
		}
		if sum(append(body, 'x')) == want {
			t.Fatal("a changed export would pass: the check cannot fail")
		}
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestTheConvertedTablesRoundTrip is plan task G3.2: a table read from the
// export and written in the compact form reads back value for value, its
// shape and order kept.
func TestTheConvertedTablesRoundTrip(t *testing.T) {
	for _, name := range []string{"foF2_CCIR.txt", "M3000F2.txt"} {
		body, err := os.ReadFile(filepath.Join("export", name))
		if err != nil {
			t.Fatal(err)
		}
		tab, err := parseExport(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tab.levels != 2 || tab.months != 12 || tab.terms != 11 || tab.harmonics != 900 || len(tab.values) != 2*12*11*900 {
			t.Fatalf("%s: shape %d %d %d %d with %d values", name, tab.levels, tab.months, tab.terms, tab.harmonics, len(tab.values))
		}
		back, err := readCompact(writeCompact(tab))
		if err != nil {
			t.Fatal(err)
		}
		if len(back) != len(tab.values) {
			t.Fatalf("%s: %d values back of %d", name, len(back), len(tab.values))
		}
		for i := range back {
			if back[i] != tab.values[i] {
				t.Fatalf("%s: value %d reads back as %v, written %v", name, i, back[i], tab.values[i])
			}
		}
	}
}

// TestAShortOrOddExportIsRefused: an export missing a value, or with one that
// is no number, is an error, never a table filled with zeros.
func TestAShortOrOddExportIsRefused(t *testing.T) {
	head := "# shape: 1 1 1 3 (solar level, month, Fourier term, harmonic)\n"
	for _, c := range []struct{ what, body string }{
		{"a value missing", head + "1 2\n"},
		{"a value that is no number", head + "1 two 3\n"},
		{"no shape", "1 2 3\n"},
		{"a line too many", head + "1 2 3\n4 5 6\n"},
	} {
		if _, err := parseExport(strings.NewReader(c.body)); err == nil {
			t.Errorf("%s was accepted", c.what)
		}
	}
	if tab, err := parseExport(strings.NewReader(head + "1 2.5 -3\n")); err != nil || len(tab.values) != 3 || tab.values[1] != 2.5 {
		t.Errorf("a whole export was refused: %v %+v", err, tab)
	}
}
