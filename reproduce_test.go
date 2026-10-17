package ionomaps_test

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// updateReproduced writes the reproduced field's golden figures afresh.
var updateReproduced = flag.Bool("update-reproduced", false, "write testdata/reproduced.txt from this machine's field")

// reproducedFile holds the field's figures from synthetic inputs, as one
// machine made them; every runner is held to them (G10.1).
var reproducedFile = filepath.Join("testdata", "reproduced.txt")

// reproducedCells are the cells the golden figures are kept for: a spread
// over the globe, the poles, the antimeridian and the stations' own region.
var reproducedCells = []int{0, 179, 4500, 6363, 8100, 8190, 9000, 11111, 13000, 16019, 16199}

// reproduced is the field from synthetic inputs at a fixed clock: the solar
// file and no GloTEC, so both fields are the climatology's - the Legendre,
// Fourier and magnetic-coordinate arithmetic an architecture could round
// differently.
func reproduced(t *testing.T) ionomaps.Snapshot {
	t.Helper()
	l, n, _ := library(t)
	n.status[indexURL] = 404
	s, err := settled(l, context.Background(), ionomaps.UpdateOptions{})
	if err != nil || s.Background.FoF2 != ionomaps.Climatology {
		t.Fatalf("control: background %v (%v); the climatology's wanted", s.Background.FoF2, err)
	}
	return s
}

// TestAFieldIsReproducedFromItsInputs is G10.1 (G-M2, D-97): the same
// inputs make the same field - bit for bit on one machine - and the field
// is the golden figures every runner, amd64 and arm64, is held to within
// 0.001 MHz.
func TestAFieldIsReproducedFromItsInputs(t *testing.T) {
	a, b := reproduced(t), reproduced(t)
	for i := range a.Now.FoF2.Values {
		if a.Now.FoF2.Values[i] != b.Now.FoF2.Values[i] || a.Now.MUF3000.Values[i] != b.Now.MUF3000.Values[i] {
			t.Fatalf("cell %d differs between two runs on one machine", i)
		}
	}
	if *updateReproduced {
		var out strings.Builder
		out.WriteString("# cell foF2 MUF(3000), MHz: the field from TestAFieldIsReproducedFromItsInputs' synthetic inputs\n")
		for _, i := range reproducedCells {
			fmt.Fprintf(&out, "%d %.6f %.6f\n", i, a.Now.FoF2.Values[i], a.Now.MUF3000.Values[i])
		}
		if err := os.WriteFile(reproducedFile, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(reproducedFile)
	if err != nil {
		t.Fatalf("%v: run go test -run TestAFieldIsReproducedFromItsInputs -update-reproduced once and commit it", err)
	}
	defer func() { _ = f.Close() }()
	read := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		i, _ := strconv.Atoi(fields[0])
		fo, _ := strconv.ParseFloat(fields[1], 64)
		muf, _ := strconv.ParseFloat(fields[2], 64)
		read++
		if d := math.Max(math.Abs(float64(a.Now.FoF2.Values[i])-fo), math.Abs(float64(a.Now.MUF3000.Values[i])-muf)); d > 0.001 {
			t.Errorf("cell %d: foF2 %.6f, MUF %.6f; the golden %.6f, %.6f (%.4f MHz apart, past 0.001)", i, a.Now.FoF2.Values[i], a.Now.MUF3000.Values[i], fo, muf, d)
		}
	}
	if read != len(reproducedCells) {
		t.Errorf("the golden holds %d cells; want %d", read, len(reproducedCells))
	}
}
