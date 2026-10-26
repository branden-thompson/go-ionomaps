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
// R4C-1, a numbered task or plan step - G5, G10.4, W10.3, L3.11, task 14.19,
// G-M3 - another project's name, and a go doc pointer into this module's
// internal packages. The pattern is general, so a new family fails rather
// than passes; published work is allowed by name.
var docIDs = regexp.MustCompile(`\b[A-Z][A-Z0-9]{0,4}-\d+(?:\.\d+)*[a-z]?\b|\bR\d[A-Z]-\d+\b|\b[A-Z]\d+(?:\.\d+)+\b|\b[GWLMP]\d+\b|\bG-M\d+\b|\bL-F\d+\b|\btask \d+(?:\.\d+)?\b|\bwatchpost\b|go doc \./internal`)

// publishedWork are the names docIDs would take for IDs and are the
// published work's own: the solar flux index, the geomagnetic field model,
// an ionospheric characteristic, a hash.
var publishedWork = map[string]bool{"F10.7": true, "IGRF-14": true, "M3000": true, "SHA-256": true}

// TestNoExportedDocCarriesAnID is watchpost D-288: what go doc shows a
// library user says what the code does and cites the published work, with
// no ID of any kind; the IDs stay in unexported comments, the build log and
// the rulings. The pattern is checked against what it must and must not
// catch first, so a pattern that matches nothing fails here.
func TestNoExportedDocCarriesAnID(t *testing.T) {
	for _, id := range []string{"R-7.1", "L-2.3", "FR-15", "G10.4", "D-53", "A-29", "RC-15", "R2C-6", "R4C-1", "L-F4", "P-6", "INST-5", "G5", "W10", "L3.11", "task 14.19", "G-M3", "watchpost"} {
		if hit := docIDs.FindString("see " + id + " here"); hit == "" || publishedWork[hit] {
			t.Fatalf("the pattern misses %q", id)
		}
	}
	for _, work := range []string{"IGRF-14", "ITU-R P.533-14", "R12", "D-RAP", "M(3000)F2", "F10.7", "M3000", "SHA-256"} {
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
