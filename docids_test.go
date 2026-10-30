package ionomaps_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// docIDs are the IDs no exported doc comment carries (watchpost D-281,
// D-288): any word of capitals and digits ending in a dash and a number -
// R-7.1, L-2.3, FR-15, D-53, A-29, RC-15, P-6, INST-5 - a red-team ID such as
// R4C-1, a capital and a number - G5, O1, N3, M5b, G10.4, L3.11 - two to
// four capitals, a dash and a capital - PS-G, CQ-T1, PF-F2, G-M3, L-F4 - a
// numbered task, batch or UAT point, a mutant, another project's name, and
// a go doc pointer into this module's internal packages. It takes every
// family this release's own documents use; published work is allowed by
// name.
var docIDs = regexp.MustCompile(`\b[A-Z][A-Z0-9]{0,4}-\d+(?:\.\d+)*[a-z]?\b|\bR\d[A-Z]-\d+\b|\b[A-Z]\d+(?:\.\d+)*[a-z]?\b|\b[A-Z]{1,4}-[A-Z]\d*\b|\b(?:task|batch|UAT) \d+(?:\.\d+)?\b|\bm\d{3}\b|\bwatchpost\b|go doc \./internal`)

// publishedWork are the names docIDs would take for IDs and are the
// published work's own: the solar flux index, the ionosphere's layers, the
// sunspot number, the geomagnetic field model, the ITU's radio sector, an
// ionospheric characteristic, a hash, a text encoding.
var publishedWork = map[string]bool{"F10.7": true, "F107": true, "F1": true, "F2": true, "R12": true, "IGRF-14": true, "ITU-R": true, "M3000": true, "SHA-256": true, "UTF-8": true}

// TestNoExportedDocCarriesAnID is watchpost D-288: what go doc shows a
// library user says what the code does and cites the published work, with
// no ID of any kind; the IDs stay in unexported comments, the build log and
// the rulings. The pattern is checked against what it must and must not
// catch first, so a pattern that matches nothing fails here.
func TestNoExportedDocCarriesAnID(t *testing.T) {
	for _, id := range []string{"R-7.1", "L-2.3", "FR-15", "G10.4", "D-53", "A-29", "RC-15", "R2C-6", "R4C-1", "L-F4", "P-6", "INST-5", "G5", "W10", "L3.11", "task 14.19", "G-M3", "O1", "O2", "M5b", "N3", "CQ-T1", "PF-F2", "PS-G", "batch 68", "UAT 112", "m544", "watchpost"} {
		if hit := docIDs.FindString("see " + id + " here"); hit == "" || publishedWork[hit] {
			t.Fatalf("the pattern misses %q", id)
		}
	}
	for _, work := range []string{"IGRF-14", "ITU-R P.533-14", "R12", "D-RAP", "M(3000)F2", "F10.7", "M3000", "SHA-256", "F107", "foF2", "MUF(3000)", "UTF-8"} {
		for _, hit := range docIDs.FindAllString(work, -1) {
			if !publishedWork[hit] {
				t.Fatalf("the pattern takes published work %q for an ID (%q)", work, hit)
			}
		}
	}
	for _, pkg := range []string{".", "./ionomapstest"} {
		out, err := exec.Command("go", "doc", "-all", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go doc -all %s: %v\n%s", pkg, err, out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			for _, hit := range docIDs.FindAllString(line, -1) {
				if !publishedWork[hit] {
					t.Errorf("go doc -all %s shows %q: %s", pkg, hit, strings.TrimSpace(line))
				}
			}
		}
	}
}
