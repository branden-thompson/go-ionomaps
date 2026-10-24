package ionomaps_test

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// rootRow is the root package's P10 density row in the ledger's mirror.
var densityRow = regexp.MustCompile("(?m)^\\| `([^`]+)` \\| `package` \\| P10-05-INVARIANT-DENSITY \\|(.*)$")

// ratifiedFigure is the density a row's reason was ratified at: "measured
// 0.69".
var ratifiedFigure = regexp.MustCompile(`measured (\d+\.\d+) over (\d+) functions`)

// belowFloor is whether a package's density is under its floor, the floor
// written to two places as a2dh prints the figure.
func belowFloor(checks, funcs int, floor float64) bool {
	return float64(checks)/float64(funcs) < floor-0.005
}

// TestEveryDensityRowHoldsItsFloor is D-74 (BUILD red team C-8) and
// watchpost D-264 (D-91): each density row was ratified at the figure it
// measured, so the gate fails when its package falls below it - a row
// ratified over one package cannot quietly cover a thinner one - and fails
// closed when the root row or its figure cannot be read. A row stating no
// figure is not held to one. The counter is P10-05's rule, checked here
// against a planted file.
func TestEveryDensityRowHoldsItsFloor(t *testing.T) {
	mirror, err := os.ReadFile("06_docs/p10-ledger.md")
	if err != nil {
		t.Fatal(err)
	}
	held, root := 0, false
	for _, row := range densityRow.FindAllStringSubmatch(string(mirror), -1) {
		dir, reason := row[1], row[2]
		root = root || dir == "."
		m := ratifiedFigure.FindStringSubmatch(reason)
		if m == nil {
			continue
		}
		floor, _ := strconv.ParseFloat(m[1], 64)
		checks, funcs := packageDensity(t, dir)
		if funcs == 0 {
			t.Errorf("%s: its row states a floor and the package has no function: the row outlived its code", dir)
			continue
		}
		held++
		if belowFloor(checks, funcs, floor) {
			t.Errorf("%s is at %.2f (%d checks over %d functions), under the %s it was ratified at: add real checks, or put the row to the HUM LEAD again", dir, float64(checks)/float64(funcs), checks, funcs, m[1])
		}
	}
	if !belowFloor(69, 100, 0.70) || belowFloor(70, 100, 0.70) || belowFloor(78, 112, 0.70) {
		t.Error("the planted figures: 0.69 is not under a 0.70 floor, or 0.70 and 0.696 (78 over 112) are")
	}
	if !root || held == 0 {
		t.Fatalf("the mirror's root density row found %v, %d rows held to a floor: D-74 renewed the root's, so its loss is a change to put to the HUM LEAD", root, held)
	}
	planted := "package x\n\nfunc f(n int, s []int) int {\n\tif n < 0 {\n\t\treturn 0\n\t}\n\tif len(s) == 0 {\n\t\treturn 1\n\t}\n\tif g(n) {\n\t\treturn 2\n\t}\n\treturn 3\n}\n\nfunc g(int) bool { return true }\n"
	if c, n := densityOf(t, planted); c != 2 || n != 2 {
		t.Errorf("the planted file counts %d checks over %d functions; want 2 over 2 (a guard that calls g is no check)", c, n)
	}
}
