package ionomaps_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// docIDs are the IDs no exported doc comment carries (watchpost D-281,
// D-288): requirement, plan, ruling and red-team IDs - R-7.1, L-2.3, FR-15,
// G10.4, D-53, A-29, RC-15, L-F4 - another project's name, and a go doc
// pointer into this module's internal packages. Published work keeps its
// own numbering: IGRF-14, ITU-R P.533-14, R12.
var docIDs = regexp.MustCompile(`\b(?:(?:D|A|R|G|W|L|FR|NFR|HR|C|F|H|I|IS|AX|RK|PS|N|LN|CN|DN|PN|IN|R[BCDHILP]|R[23][BCDHILP]|P10)-\d+(?:\.\d+)*[a-z]?|L-F\d+|[GW]\d+\.\d+)\b|\bwatchpost\b|go doc \./internal`)

// TestNoExportedDocCarriesAnID is watchpost D-288: what go doc shows a
// library user says what the code does and cites the published work, with
// no ID of any kind; the IDs stay in unexported comments, the build log and
// the rulings. The pattern is checked against what it must and must not
// catch first, so a pattern that matches nothing fails here.
func TestNoExportedDocCarriesAnID(t *testing.T) {
	for _, id := range []string{"R-7.1", "L-2.3", "FR-15", "G10.4", "D-53", "A-29", "RC-15", "R2C-6", "L-F4", "watchpost"} {
		if !docIDs.MatchString("see " + id + " here") {
			t.Fatalf("the pattern misses %q", id)
		}
	}
	for _, work := range []string{"IGRF-14", "ITU-R P.533-14", "R12", "D-RAP", "M(3000)F2"} {
		if docIDs.MatchString(work) {
			t.Fatalf("the pattern catches published work %q", work)
		}
	}
	for _, pkg := range []string{".", "./ionomapstest"} {
		out, err := exec.Command("go", "doc", "-all", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go doc -all %s: %v\n%s", pkg, err, out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if hit := docIDs.FindString(line); hit != "" {
				t.Errorf("go doc -all %s shows %q: %s", pkg, hit, strings.TrimSpace(line))
			}
		}
	}
}
