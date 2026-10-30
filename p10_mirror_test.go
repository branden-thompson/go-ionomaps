package ionomaps_test

// p10_mirror_test.go — D-51, after go-tuiMaps v0.2.0 D-118: the P10 exemptions the HUM LEAD
// ratified live in the harness's ledger, which the repository does not
// track, so "P10 clean" is reproduced from 06_docs/p10-ledger.md, a mirror
// generated from that ledger. The ledger is the harness's: this file reads
// it and never writes it.

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var updateP10Mirror = flag.Bool("update-p10-mirror", false, "write 06_docs/p10-ledger.md from the local P10 ledger")

const (
	p10Ledger = ".a2dh-p10-exemptions.yml"
	p10Mirror = "06_docs/p10-ledger.md"
)

// p10Row is one ratified exemption.
type p10Row struct {
	File, Symbol, Rule, Ratified, Reason string
}

// readP10Ledger reads the ledger's rows: a list of maps whose reasons are
// single-quoted and folded over lines, which is all the harness writes.
func readP10Ledger(body string) []p10Row {
	var rows []p10Row
	var open *string // a quoted value still being read
	set := func(r *p10Row, key, value string) {
		at := map[string]*string{"file": &r.File, "symbol": &r.Symbol, "rule_id": &r.Rule, "ratified": &r.Ratified, "reason": &r.Reason}[key]
		if at == nil {
			return
		}
		if strings.HasPrefix(value, "'") {
			*at = value[1:]
			if closed(*at) {
				*at = unquote(strings.TrimSuffix(*at, "'"))
				return
			}
			open = at
			return
		}
		*at = value
	}
	for _, line := range strings.Split(body, "\n") {
		if open != nil {
			*open += " " + strings.TrimSpace(line)
			if closed(*open) {
				*open = unquote(strings.TrimSuffix(*open, "'"))
				open = nil
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			rows = append(rows, p10Row{})
			trimmed = strings.TrimPrefix(trimmed, "- ")
		}
		key, value, ok := strings.Cut(trimmed, ": ")
		if !ok || len(rows) == 0 {
			continue
		}
		set(&rows[len(rows)-1], key, value)
	}
	return rows
}

// closed reports whether a single-quoted value has reached its closing
// quote: an odd run of quotes at its end (” is a quote inside it).
func closed(v string) bool {
	n := len(v) - len(strings.TrimRight(v, "'"))
	return n%2 == 1
}

func unquote(v string) string { return strings.ReplaceAll(v, "''", "'") }

// p10MirrorOf is the mirror's text for the ledger's rows.
func p10MirrorOf(rows []p10Row) string {
	var b strings.Builder
	b.WriteString("<!-- GENERATED from the local P10 ledger by `go test -run TestTheP10MirrorMatchesTheLedger -update-p10-mirror .` - do not edit by hand. -->\n\n")
	b.WriteString("# P10 exemption ledger - the tracked mirror\n\n")
	b.WriteString("**Every row here was RATIFIED by the HUM LEAD, never self-issued** (D-51). The ledger the P10 check reads\n")
	b.WriteString("is the harness's and is not tracked, so this mirror is how a clone reproduces \"P10 clean\": each row is an\n")
	b.WriteString("exemption the HUM LEAD approved, with the reason as ratified. A row is added by ratifying it with the HUM\n")
	b.WriteString("LEAD, writing it to the ledger and regenerating this file; `TestTheP10MirrorMatchesTheLedger` fails while\n")
	b.WriteString("the two differ, and `TestEveryP10MirrorRowNamesCodeThatExists` while a row names code that is gone.\n\n")
	b.WriteString("**" + strconv.Itoa(len(rows)) + " rows.**\n\n")
	b.WriteString("| File | Symbol | Rule | Ratified | Reason |\n|---|---|---|---|---|\n")
	for _, r := range rows {
		b.WriteString("| `" + r.File + "` | `" + r.Symbol + "` | " + r.Rule + " | " + r.Ratified + " | " + strings.ReplaceAll(r.Reason, "|", "\\|") + " |\n")
	}
	return b.String()
}

// TestTheP10MirrorMatchesTheLedger (D-51): the tracked mirror is what the
// local ledger says, row for row. Where the harness is not on this machine
// - a hosted run - there is nothing to compare, and the test says NOT RUN;
// where the harness is but its ledger is not, the test fails, never skips
// (watchpost RC-13): there it could check.
func TestTheP10MirrorMatchesTheLedger(t *testing.T) {
	body, err := os.ReadFile(p10Ledger)
	_, harness := exec.LookPath("a2dh")
	if os.IsNotExist(err) && harness != nil {
		t.Skip("NOT RUN: no P10 harness here (a hosted run has none); the mirror is held by TestEveryP10MirrorRowNamesCodeThatExists")
	}
	if err != nil {
		t.Fatal(err)
	}
	rows := readP10Ledger(string(body))
	if len(rows) == 0 {
		t.Fatal("the ledger read as no rows; the reader has stopped matching how the harness writes it")
	}
	for _, r := range rows {
		if r.File == "" || r.Symbol == "" || r.Rule == "" || r.Reason == "" || !strings.Contains(r.Reason, "RATIFIED") {
			t.Errorf("a ledger row read incompletely, or carries no ratification: %+v", r)
		}
	}
	want := p10MirrorOf(rows)
	if *updateP10Mirror {
		if err := os.WriteFile(p10Mirror, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(p10Mirror)
	if err != nil {
		t.Fatalf("no mirror: %v; write it with -update-p10-mirror", err)
	}
	if string(got) != want {
		t.Errorf("%s differs from the ledger; regenerate it with go test -run TestTheP10MirrorMatchesTheLedger -update-p10-mirror .", p10Mirror)
	}
}

// TestEveryP10MirrorRowNamesCodeThatExists (D-51): every row of the
// mirror names a file or package in the tree, and a symbol the file still
// holds - a ratification of nothing guards nothing.
func TestEveryP10MirrorRowNamesCodeThatExists(t *testing.T) {
	raw, err := os.ReadFile(p10Mirror)
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile("(?m)^\\| `([^`]+)` \\| `([^`]+)` \\| ").FindAllStringSubmatch(string(raw), -1)
	if len(rows) == 0 {
		t.Fatal("the mirror has no rows the pattern matches")
	}
	for _, m := range rows {
		file, symbol := m[1], m[2]
		info, err := os.Stat(filepath.FromSlash(file))
		if err != nil {
			t.Errorf("a ratified row names `%s` · `%s`, and no such file or package exists", file, symbol)
			continue
		}
		if symbol == "package" || info.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.FromSlash(file))
		if err != nil {
			t.Fatal(err)
		}
		name := symbol
		if _, after, ok := strings.Cut(symbol, "."); ok {
			name = after
		}
		declared := `(?m)^(func (\([^)]*\) )?|type |var |const |\t)` + regexp.QuoteMeta(name) + `\b`
		if !regexp.MustCompile(declared).Match(body) { // declared there, not merely named in a comment (RC-13)
			t.Errorf("a ratified row names `%s` · `%s`, and the file no longer holds it", file, symbol)
		}
	}
}
