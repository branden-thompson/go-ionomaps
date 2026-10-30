package ionomaps

import (
	"fmt"
	"time"

	"github.com/branden-thompson/go-ionomaps/internal/assimilate"
)

// constant is one figure the library holds that was measured on recorded
// days rather than derived: its value as the code holds it, where it was
// measured, the citation, and the last day of the data it was measured on
// (R-9.8, plan task G10.5).
type constant struct {
	name, value, basis, source string
	measuredTo                 time.Time
}

// The days the constants were measured on: PLAN's week, 2026-09-29 to
// 10-05, and the fresh days fetched in REVIEW, 2026-10-12 to 10-17.
var (
	planWeekEnd  = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	freshDaysEnd = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
)

// constantMaxAge is how old a constant's data may be when the library is
// released: past it, a fresh week is fetched and the figure measured again
// with dry_fetch.py and week.py (R-9.8). A release step checks it, never
// the everyday gate (scripts/gate --release).
const constantMaxAge = 365 * 24 * time.Hour

// constants are every measured figure in one table, built from the values
// the code uses, so the table and the code cannot disagree (R-9.8). The
// snapshot carries the typical offset's and the typical error's bases for
// a host to say.
var constants = []constant{
	{"the foF2 kernel", fmt.Sprintf("%g km, noise %g", assimilate.FoF2Kernel.LengthKm, assimilate.FoF2Kernel.Noise),
		"tuned against GIRO's stations over the week of 2026-09-29 to 10-05", "week.py section \"kernel tuned\" (A-29; D-61, readings of unknown score taken)", planWeekEnd},
	{"the M(3000)F2 kernel", fmt.Sprintf("%g km, noise %g", assimilate.M3000Kernel.LengthKm, assimilate.M3000Kernel.Noise),
		"tuned against GIRO's stations over the week of 2026-09-29 to 10-05", "week.py section \"kernel tuned\" (A-29; D-61)", planWeekEnd},
	{"the typical offset's seed and spread", fmt.Sprintf("%g MHz, SD %g", seedOffsetMHz, seedSpreadMHz), offsetBasis, offsetSource, planWeekEnd},
	{"the hours ahead's typical error", fmt.Sprint(typicalError().Bands), typicalError().Basis, typicalError().Source, planWeekEnd},
	{"the plausibility bounds", fmt.Sprintf("foF2 %g MHz or a fold of %g either way, M(3000)F2 %g", assimilate.FoF2Bound, assimilate.FoF2Fold, assimilate.M3000Bound),
		"above every residual and every fold of GIRO's readings over PLAN's week and the fresh days, quiet days with no storm among them", "gm3 -residuals over both (watchpost D-248, D-279; A-31)", freshDaysEnd},
}
