package ionomaps

import (
	"os"
	"testing"
	"time"
)

// TestEveryConstantCarriesItsBasis is R-9.8 (G10.5): every measured figure
// sits in the one table with its value, where it was measured, its citation
// and the last day of its data; the snapshot carries the bases a host says.
func TestEveryConstantCarriesItsBasis(t *testing.T) {
	if len(constants) < 5 {
		t.Fatalf("%d constants in the table; the kernels, the offset, the typical error and the bounds are five", len(constants))
	}
	for _, c := range constants {
		if c.name == "" || c.value == "" || c.basis == "" || c.source == "" || c.measuredTo.IsZero() {
			t.Errorf("%q: a constant carries its value, basis, source and the day its data ends", c.name)
		}
	}
	if b := typicalError().Basis; b != constants[3].basis {
		t.Errorf("the snapshot's typical error says %q; the table %q", b, constants[3].basis)
	}
	o := seedOffset()
	if b := o.offset(0, false, false).Basis; b != constants[2].basis {
		t.Errorf("the snapshot's offset says %q; the table %q", b, constants[2].basis)
	}
}

// TestNoConstantIsOverAYearOld is R-9.8's release step: a release fails when
// a constant's data is more than a year old. It runs only under
// scripts/gate --release (IONOMAPS_RELEASE=1), never the everyday gate.
func TestNoConstantIsOverAYearOld(t *testing.T) {
	if os.Getenv("IONOMAPS_RELEASE") != "1" {
		t.Skip("a release step: scripts/gate --release runs it")
	}
	for _, c := range constants {
		if age := time.Since(c.measuredTo); age > constantMaxAge {
			t.Errorf("%s was measured on data ending %s, %d days ago: fetch a fresh week and measure it again (dry_fetch.py, week.py) before releasing", c.name, c.measuredTo.Format(time.DateOnly), int(age.Hours()/24))
		}
	}
}
