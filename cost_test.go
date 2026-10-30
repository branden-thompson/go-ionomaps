package ionomaps_test

// cost_test.go — plan task G10.3 (G-G1, watchpost D-98, D-112, D-119,
// D-345, D-347): the cost of an update on the reference machine, measured
// as a release step and kept in a record the release check holds to this
// code.

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
)

// recordCost measures G-G1 on this machine and writes its record.
var recordCost = flag.Bool("record-cost", false, "measure G-G1 on this machine and write "+costRecordFile)

// costRecordFile is G-G1's record: scripts/gate --cost writes it and
// scripts/gate --release reads it.
const costRecordFile = "06_docs/02_features/go-ionomaps/07-readiness/cost-record.md"

// referenceMachine is the machine G-G1's targets were set on (watchpost
// D-98).
const referenceMachine = "Apple M5 Pro"

// costRuns is how many times each figure is measured; the record keeps the
// median.
const costRuns = 5

// The record's rows, each a figure and its target (watchpost D-347): a cold
// open of the hour now at 2° in CPU seconds; an hour ahead, built when
// asked, and a refresh in CPU milliseconds; the live heap with 24 hours
// ahead held, in MB. The 24 hours ahead together and peak RSS are recorded,
// not targeted.
const (
	rowTree    = "Code tree"
	rowMachine = "Machine"
	rowGo      = "Go"
	rowWhen    = "Measured"
	rowOpen    = "Cold open, the hour now at 2°, CPU s"
	rowAhead   = "An hour ahead, CPU ms"
	rowDay     = "24 hours ahead, CPU ms"
	rowRefresh = "Refresh, CPU ms"
	rowHeap    = "Live heap, 24 hours ahead held, MB"
	rowRSS     = "Peak RSS, MB"
)

// costTargets are the figures' bounds, by row.
var costTargets = map[string]float64{rowOpen: 0.5, rowAhead: 50, rowRefresh: 100, rowHeap: 15}

// cpuNow is the process's CPU time so far, user and system.
func cpuNow(t *testing.T) time.Duration {
	t.Helper()
	var u syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
		t.Fatal(err)
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}

// peakRSS is the process's largest resident set so far, in MB: the kernel
// counts bytes on darwin and kilobytes on linux.
func peakRSS(t *testing.T) float64 {
	t.Helper()
	var u syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" {
		return float64(u.Maxrss) / (1 << 20)
	}
	return float64(u.Maxrss) / (1 << 10)
}

// opened is what one cold open measured: the open's CPU time, the median
// hour ahead's, the 24 hours', the live heap held, and the library to
// refresh.
type opened struct {
	open, hour, day time.Duration
	heap            float64
	l               *ionomaps.Library
	n               *noaa
	c               *clock
}

// coldOpen is one cold open from the test service and the hours ahead after
// it: a new library, NOAA's update and GIRO's burst, then each of the 24
// hours ahead built, as a host asks for them.
func coldOpen(t *testing.T) opened {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	from := cpuNow(t)
	l, n, c := giroLibrary(t)
	update(t, l)
	s := update(t, l)
	o := opened{open: cpuNow(t) - from, l: l, n: n, c: c}
	if len(s.NearestKm) == 0 || s.Reason != ionomaps.Fresh {
		t.Fatalf("the cold open assimilated no station (%v): it measured less than an open", s.Reason)
	}
	held := make([]ionomaps.Snapshot, 0, 24)
	hours := make([]float64, 0, 24)
	for k := 1; k <= 24; k++ {
		from = cpuNow(t)
		a, err := s.Ahead(k)
		spent := cpuNow(t) - from
		if err != nil {
			t.Fatal(err)
		}
		o.day += spent
		hours = append(hours, float64(spent))
		held = append(held, a)
	}
	o.hour = time.Duration(median(hours))
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(held)
	o.heap = float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / (1 << 20)
	return o
}

// refresh is one update ten minutes on, with a newer GloTEC grid listed:
// GIRO not yet due, its last readings assimilated again.
func refresh(t *testing.T, l *ionomaps.Library, n *noaa, c *clock, k int) time.Duration {
	t.Helper()
	c.Add(10 * time.Minute)
	n.list(t, start.Add(time.Duration(k)*10*time.Minute-20*time.Minute), 7)
	from := cpuNow(t)
	s, err := l.Update(context.Background(), ionomaps.UpdateOptions{})
	spent := cpuNow(t) - from
	if err != nil || s.Reason != ionomaps.Fresh {
		t.Fatalf("the refresh: %v, %v", s.Reason, err)
	}
	return spent
}

// median is the middle of the figures.
func median(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[len(s)/2]
}

// machineName is this machine's processor, as the kernel names it.
func machineName() string {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "unknown (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
	}
	return strings.TrimSpace(string(out))
}

// TestTheCostIsRecorded is G10.3's measure (watchpost D-345): G-G1's
// figures, each the median of five runs, written with the machine, the Go
// version and the code they were measured on. It runs only under
// scripts/gate --cost, which names the code (IONOMAPS_TREE), on a machine
// otherwise at rest; a target missed fails it, and the record says so.
func TestTheCostIsRecorded(t *testing.T) {
	if !*recordCost {
		t.Skip("a release step: scripts/gate --cost runs it")
	}
	tree := os.Getenv("IONOMAPS_TREE")
	if tree == "" {
		t.Fatal("IONOMAPS_TREE names the code measured; scripts/gate --cost sets it")
	}
	var opens, hours, days, refreshes, heaps []float64
	for range costRuns {
		o := coldOpen(t)
		opens = append(opens, o.open.Seconds())
		hours = append(hours, float64(o.hour.Microseconds())/1000)
		days = append(days, float64(o.day.Microseconds())/1000)
		heaps = append(heaps, o.heap)
		refreshes = append(refreshes, float64(refresh(t, o.l, o.n, o.c, 1).Microseconds())/1000)
	}
	figures := map[string]float64{rowOpen: median(opens), rowAhead: median(hours), rowDay: median(days), rowRefresh: median(refreshes), rowHeap: median(heaps), rowRSS: peakRSS(t)}
	rows := [][2]string{
		{rowTree, "`" + tree + "`"},
		{rowMachine, machineName()},
		{rowGo, runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH},
		{rowWhen, time.Now().UTC().Format(time.RFC3339)},
	}
	for _, r := range []string{rowOpen, rowAhead, rowDay, rowRefresh, rowHeap, rowRSS} {
		v := strconv.FormatFloat(figures[r], 'f', 3, 64)
		if bound, ok := costTargets[r]; ok {
			v += " (target " + strconv.FormatFloat(bound, 'f', -1, 64) + ")"
		}
		rows = append(rows, [2]string{r, v})
	}
	var b strings.Builder
	b.WriteString("# G-G1's record: the cost of an update on the reference machine\n\n")
	b.WriteString("Written by `scripts/gate --cost` (plan task G10.3, watchpost D-345), each figure the median of " + strconv.Itoa(costRuns) + " runs from\n")
	b.WriteString("the library's test service: a cold open is a new library, NOAA's update and GIRO's burst assimilated, the\n")
	b.WriteString("hour now; each of the 24 hours ahead is then built, as a host asks for them; a refresh is an update ten\n")
	b.WriteString("minutes on with a newer grid (watchpost D-347). `scripts/gate --release` refuses this record when it is\n")
	b.WriteString("missing, names other code or another machine, or misses a target.\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	for _, r := range rows {
		b.WriteString("| " + r[0] + " | " + r[1] + " |\n")
	}
	if err := os.WriteFile(costRecordFile, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", b.String())
	for _, problem := range costRecordProblems(b.String(), tree) {
		t.Error(problem)
	}
}

// costRecordProblems is every reason the release refuses a record for the
// code named tree: no record, another tree, another machine, a target's
// figure missing, unreadable or past its bound.
func costRecordProblems(record, tree string) []string {
	fields := map[string]string{}
	for _, line := range strings.Split(record, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) == 2 {
			fields[strings.TrimSpace(cells[0])] = strings.TrimSpace(cells[1])
		}
	}
	var problems []string
	if got := strings.Trim(fields[rowTree], "`"); got != tree {
		problems = append(problems, fmt.Sprintf("the record names code %q; this is %q: run scripts/gate --cost on the release's code", got, tree))
	}
	if !strings.Contains(fields[rowMachine], referenceMachine) {
		problems = append(problems, fmt.Sprintf("the record was measured on %q, not the reference machine, an %s", fields[rowMachine], referenceMachine))
	}
	for _, r := range []string{rowOpen, rowAhead, rowRefresh, rowHeap} {
		figure, _, _ := strings.Cut(fields[r], " ")
		v, err := strconv.ParseFloat(figure, 64)
		nothing := err != nil || math.IsNaN(v)
		if nothing {
			problems = append(problems, fmt.Sprintf("%s: %q is no figure", r, fields[r]))
			continue
		}
		if v > costTargets[r] {
			problems = append(problems, fmt.Sprintf("%s is %.3f, past its target %v (G-G1)", r, v, costTargets[r]))
		}
	}
	return problems
}

// TestTheCostRecordHoldsForThisCode is G10.3's release check (watchpost
// D-119, D-345): the record names the code released (IONOMAPS_TREE, set by
// scripts/gate --release), the reference machine, and every target met.
func TestTheCostRecordHoldsForThisCode(t *testing.T) {
	if os.Getenv("IONOMAPS_RELEASE") != "1" {
		t.Skip("a release step: scripts/gate --release runs it")
	}
	body, err := os.ReadFile(costRecordFile)
	if err != nil {
		t.Fatalf("%v: run scripts/gate --cost on the reference machine and commit the record", err)
	}
	for _, problem := range costRecordProblems(string(body), os.Getenv("IONOMAPS_TREE")) {
		t.Error(problem)
	}
}

// TestACostRecordIsRefused (G10.3): the release check refuses a record of
// other code, another machine, a missed target or a figure that is none,
// and passes one that holds.
func TestACostRecordIsRefused(t *testing.T) {
	good := "| Field | Value |\n|---|---|\n| " + rowTree + " | `abc123` |\n| " + rowMachine + " | Apple M5 Pro |\n| " +
		rowOpen + " | 0.300 (target 0.5) |\n| " + rowAhead + " | 40.000 (target 50) |\n| " + rowRefresh + " | 6.000 (target 100) |\n| " + rowHeap + " | 9.000 (target 15) |\n"
	if p := costRecordProblems(good, "abc123"); len(p) != 0 {
		t.Fatalf("a record that holds is refused: %v", p)
	}
	for _, c := range []struct{ name, record, tree, want string }{
		{"no record", "", "abc123", "names code"},
		{"other code", good, "def456", "names code"},
		{"another machine", strings.Replace(good, "Apple M5 Pro", "Apple M1", 1), "abc123", "reference machine"},
		{"a slow open", strings.Replace(good, "0.300", "0.501", 1), "abc123", "past its target"},
		{"a slow hour ahead", strings.Replace(good, "40.000", "50.100", 1), "abc123", "past its target"},
		{"a slow refresh", strings.Replace(good, "6.000", "100.100", 1), "abc123", "past its target"},
		{"a large heap", strings.Replace(good, "9.000", "15.200", 1), "abc123", "past its target"},
		{"no figure", strings.Replace(good, "9.000", "NaN", 1), "abc123", "no figure"},
	} {
		p := costRecordProblems(c.record, c.tree)
		if !strings.Contains(strings.Join(p, "; "), c.want) {
			t.Errorf("%s: refused for %v; want a reason saying %q", c.name, p, c.want)
		}
	}
}
