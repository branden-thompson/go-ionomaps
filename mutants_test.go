package ionomaps_test

// mutants_test.go — the mutants anyone can run again (watchpost RL-5, D-245):
// each a change its test must catch (06_docs/mutants/mutants.json). This file checks, on
// every run, that each mutant still finds its line and names a test that
// exists; mutants_kill_test.go, behind the `mutants` tag, applies each one to
// a copy of the tree and checks its test fails (scripts/gate's full lane).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mutantFile is the mutant table.
const mutantFile = "06_docs/mutants/mutants.json"

// mutant is one row of the table: a change to one file that the named test,
// run in the package, must catch.
type mutant struct {
	ID      string `json:"id"`
	Row     string `json:"row"`
	What    string `json:"what"`
	File    string `json:"file"`
	Old     string `json:"old"`
	New     string `json:"new"`
	Package string `json:"package"`
	Test    string `json:"test"`
}

// loadMutants is the table, or the test fails.
func loadMutants(t *testing.T) []mutant {
	t.Helper()
	b, err := os.ReadFile(mutantFile)
	if err != nil {
		t.Fatal(err)
	}
	var ms []mutant
	if err := json.Unmarshal(b, &ms); err != nil {
		t.Fatalf("%s: %v", mutantFile, err)
	}
	if len(ms) == 0 {
		t.Fatalf("%s holds no mutant", mutantFile)
	}
	return ms
}

// testNamed reports whether a test function of that name is declared in the
// package's directory.
func testNamed(t *testing.T, dir, name string) bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	decl := regexp.MustCompile(`(?m)^func ` + regexp.QuoteMeta(name) + `\(t \*testing\.T\)`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if decl.Match(b) {
			return true
		}
	}
	return false
}

// TestEveryMutantFindsItsLine is the cheap half: each mutant's text is
// in its file exactly once, its change changes something, its id is its own,
// and the test it names is declared in its package. A mutant that no longer
// finds its line checks nothing, and passes quietly where it is not checked.
func TestEveryMutantFindsItsLine(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range loadMutants(t) {
		if m.ID == "" || seen[m.ID] {
			t.Errorf("mutant %q: an id of its own is needed", m.ID)
		}
		seen[m.ID] = true
		if m.Row == "" || m.What == "" || m.Old == m.New {
			t.Errorf("%s: a mutant names its row, says what it breaks and changes something", m.ID)
		}
		b, err := os.ReadFile(m.File)
		if err != nil {
			t.Errorf("%s: %v", m.ID, err)
			continue
		}
		if n := strings.Count(string(b), m.Old); n != 1 {
			t.Errorf("%s: its text is in %s %d times; once is needed - re-point it to the line it breaks", m.ID, m.File, n)
		}
		if !strings.HasPrefix(m.Package, ".") || !testNamed(t, m.Package, m.Test) {
			t.Errorf("%s: no test %s in package %s", m.ID, m.Test, m.Package)
		}
	}
}
