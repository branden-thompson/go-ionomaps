package ionomaps_test

// repo_test.go — plan tasks G0.2 to G0.4: what the repository may hold, what
// its NOTICE must say, and that every exported function cites its source.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/branden-thompson/go-ionomaps/internal/repocheck"
	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

// repoFiles is every file git knows of, tracked or new and not ignored.
func repoFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		t.Fatalf("listing the tree: %v", err)
	}
	var files []string
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if info, err := os.Lstat(name); err == nil && info.Mode().IsRegular() {
			files = append(files, name)
		}
	}
	return files
}

// TestNoThirdPartyDataIsCommitted is R-4.3 (plan task G0.2): no file in the
// tree holds a GIRO reply, by its content, whatever its name; a reply planted
// in the scan's own input is caught.
func TestNoThirdPartyDataIsCommitted(t *testing.T) {
	for _, name := range repoFiles(t) {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if why, found := repocheck.GIROReply(body); found {
			t.Errorf("%s holds what reads as a GIRO reply (%s); no GIRO data is ever committed (R-4.3)", name, why)
		}
	}
	for _, part := range plantedParts() { // each signature alone is enough
		if _, found := repocheck.GIROReply([]byte(part)); !found {
			t.Errorf("a planted part of a FastChar reply was not caught: %q", part)
		}
	}
	if _, found := repocheck.GIROReply([]byte("2026-10-05T12:00:00Z 90 7.425 is one example row, not a reply\n")); found {
		t.Error("one example row was taken for a reply")
	}
}

// plantedParts are a FastChar reply's three signatures, each alone: its
// location header, its column header, and its rows.
func plantedParts() []string {
	reply := strings.SplitN(plantedReply(), "\n", 3)
	return []string{reply[0] + "\n", reply[1] + "\n", reply[2]}
}

// plantedReply is the shape of a FastChar reply, built here so that this file
// is not one: a location header, the column header with its confidence and
// qualifier columns, and rows of soundings.
func plantedReply() string {
	return "# Location: " + "GEO" + " ( 38.0N 255.0E)\n#Time " + "CS" + " foF2 " + "QD foF1 QD mufD QD MD QD\n" +
		"2026-10-05T12:00:00.000Z  90 7.425 //  5.10 // 22.10 // 2.976 //\n" +
		"2026-10-05T12:15:00.000Z  85 7.500 //  5.12 // 22.31 // 2.975 //\n" +
		"2026-10-05T12:30:00.000Z  95 7.610 //  5.14 // 22.80 // 2.995 //\n"
}

// TestTheNoticeNamesEverySource is R-4.2 (plan task G0.3): every source the
// library reads, and every third-party dataset committed, has its entry in
// the NOTICE; a source missing from it is caught.
func TestTheNoticeNamesEverySource(t *testing.T) {
	notice, err := os.ReadFile("NOTICE")
	if err != nil {
		t.Fatal(err)
	}
	if missing := repocheck.MissingFromNotice(string(notice), terms.All(), terms.Datasets()); len(missing) != 0 {
		t.Errorf("the NOTICE does not name: %s", strings.Join(missing, ", "))
	}
	planted := append(terms.All(), terms.Source{Name: "A Source Nobody Credited"})
	if missing := repocheck.MissingFromNotice(string(notice), planted, nil); len(missing) != 1 {
		t.Errorf("a planted source missing from the NOTICE gave %v: the check cannot fail", missing)
	}
}

// TestEveryExportedFunctionCitesItsSource is R-7.1 (plan task G0.4): each
// exported function and method of this module, outside tests, has a "Source:"
// line in its doc comment, naming the published work it implements or saying
// it implements none; one planted without it is caught. A nested module - the
// plan's evidence, which is no part of the library - is not this module.
func TestEveryExportedFunctionCitesItsSource(t *testing.T) {
	files := repoFiles(t)
	var nested []string
	for _, name := range files {
		if filepath.Base(name) == "go.mod" && name != "go.mod" {
			nested = append(nested, filepath.Dir(name)+"/")
		}
	}
	for _, name := range files {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "_a2dh/") {
			continue
		}
		if slices.ContainsFunc(nested, func(dir string) bool { return strings.HasPrefix(filepath.ToSlash(name), filepath.ToSlash(dir)) }) {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, fn := range uncited(t, filepath.ToSlash(name), body) {
			t.Errorf("%s: %s has no \"Source:\" line in its doc comment (R-7.1)", name, fn)
		}
	}
	planted := "package p\n\n// Planted does a thing.\nfunc Planted() {}\n\n// Cited does a thing.\n//\n// Source: this library.\nfunc Cited() {}\n"
	if got := uncited(t, "planted.go", []byte(planted)); len(got) != 1 || got[0] != "Planted" {
		t.Errorf("a planted uncited function gave %v: the check cannot fail", got)
	}
}

// uncited names the exported functions and methods in a file with no
// "Source:" line in their doc comments.
func uncited(t *testing.T, name string, body []byte) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, body, parser.ParseComments)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || !fn.Name.IsExported() {
			continue
		}
		if fn.Doc == nil || !strings.Contains(fn.Doc.Text(), "Source:") {
			out = append(out, fn.Name.Name)
		}
	}
	return out
}

// TestTheNoticeCarriesPyIRIsLicenceInFull is watchpost F-215 (R-4.2): the
// climatology ports PyIRI's method and ships NRL's refits from it, so the
// NOTICE carries PyIRI's MIT licence whole - its copyright, the permission
// notice MIT asks to accompany every copy, and the warranty's disclaimer.
func TestTheNoticeCarriesPyIRIsLicenceInFull(t *testing.T) {
	raw, err := os.ReadFile("NOTICE")
	if err != nil {
		t.Fatal(err)
	}
	notice := strings.Join(strings.Fields(string(raw)), " ")
	for _, want := range []string{
		"Copyright (c) 2023 victoriyaforsythe",
		"Permission is hereby granted, free of charge, to any person obtaining a copy of this software",
		"The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.",
		`THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND`,
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("the NOTICE does not carry PyIRI's licence: missing %q", want)
		}
	}
}
