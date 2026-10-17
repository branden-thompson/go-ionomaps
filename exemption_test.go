package ionomaps_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// rootRow is the root package's P10 density row in the ledger's mirror.
var rootRow = regexp.MustCompile("(?m)^\\| `\\.` \\| `package` \\| P10-05-INVARIANT-DENSITY \\|")

// ratifiedFigure is the density a row's reason was ratified at: "measured
// 0.69".
var ratifiedFigure = regexp.MustCompile(`measured (\d+\.\d+) over (\d+) functions`)

// TestTheRootDensityHoldsItsRatifiedFigure is D-74 (BUILD red team C-8):
// the root package's density row was renewed at the figure it measured, so
// the gate fails when the package falls below it - a row ratified over one
// package cannot quietly cover a thinner one - and fails closed when the row
// or its figure cannot be read. The counter is P10-05's rule, checked here
// against a planted file.
func TestTheRootDensityHoldsItsRatifiedFigure(t *testing.T) {
	mirror, err := os.ReadFile("06_docs/p10-ledger.md")
	if err != nil {
		t.Fatal(err)
	}
	row := rootRow.FindIndex(mirror)
	if row == nil {
		t.Fatal("the mirror has no root density row: D-74 renewed it, so its loss is a change to put to the HUM LEAD")
	}
	line, _, _ := strings.Cut(string(mirror[row[0]:]), "\n")
	m := ratifiedFigure.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("the root row states no measured figure: %q", line)
	}
	floor, _ := strconv.ParseFloat(m[1], 64)
	checks, funcs := rootDensity(t)
	if got := float64(checks) / float64(funcs); got < floor-0.005 {
		t.Errorf("the root package is at %.2f (%d checks over %d functions), under the %s it was ratified at: add real checks, or put the row to the HUM LEAD again", got, checks, funcs, m[1])
	}
	planted := "package x\n\nfunc f(n int, s []int) int {\n\tif n < 0 {\n\t\treturn 0\n\t}\n\tif len(s) == 0 {\n\t\treturn 1\n\t}\n\tif g(n) {\n\t\treturn 2\n\t}\n\treturn 3\n}\n\nfunc g(int) bool { return true }\n"
	if c, n := densityOf(t, planted); c != 2 || n != 2 {
		t.Errorf("the planted file counts %d checks over %d functions; want 2 over 2 (a guard that calls g is no check)", c, n)
	}
}
