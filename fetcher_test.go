package ionomaps_test

// fetcher_test.go — plan task G1.2 (R-5.1, R-4.1, watchpost D-113): the
// library opens no connection of its own, every source carries its terms,
// and every address the library could ask is on a host it exports.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	ionomaps "github.com/branden-thompson/go-ionomaps"
	"github.com/branden-thompson/go-ionomaps/internal/giro"
	"github.com/branden-thompson/go-ionomaps/internal/glotec"
)

// noFetch is a fetcher that must never be asked.
type noFetch struct{ asked int }

func (f *noFetch) Fetch(context.Context, string, ionomaps.Validators) (ionomaps.Response, error) {
	f.asked++
	return ionomaps.Response{}, nil
}

// libraryFiles is this module's own Go source outside tests, each parsed.
func libraryFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	files := map[string]*ast.File{}
	for _, name := range repoFiles(t) {
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "_a2dh/") || strings.HasPrefix(name, "06_docs/") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, body, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if full, err := parser.ParseFile(token.NewFileSet(), name, body, 0); err == nil {
			f = full
		}
		files[name] = f
	}
	return files
}

// forbidden are the packages a library that opens no connection never needs.
var forbidden = []string{"net", "net/http", "crypto/tls", "os/exec"}

// TestTheLibraryOpensNoConnectionOfItsOwn is R-5.1: outside tests, the module
// imports none of net, net/http, crypto/tls or os/exec (net/url alone is
// allowed); New takes the host's fetcher and refuses none, and asks nothing
// of it on its own.
func TestTheLibraryOpensNoConnectionOfItsOwn(t *testing.T) {
	for name, f := range libraryFiles(t) {
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if slices.Contains(forbidden, path) {
				t.Errorf("%s imports %s: the host supplies the network (R-5.1)", name, path)
			}
		}
	}
	if _, err := ionomaps.New(nil, ionomaps.Options{}); err == nil {
		t.Error("New took no fetcher: the library has no network of its own to fall back on")
	}
	f := &noFetch{}
	l, err := ionomaps.New(f, ionomaps.Options{})
	if err != nil || l == nil {
		t.Fatalf("New with a fetcher: %v", err)
	}
	if f.asked != 0 {
		t.Errorf("New asked the fetcher %d times; nothing is fetched until an update", f.asked)
	}
}

// TestEverySourceCarriesItsTerms is R-4.1 (G-M1): every source has a name and
// its terms; one read at run time names its hosts, bare host names; the host
// can read them all from the library.
func TestEverySourceCarriesItsTerms(t *testing.T) {
	l, err := ionomaps.New(&noFetch{}, ionomaps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sources := l.Sources()
	runtime := 0
	for _, s := range sources {
		if s.Name == "" || s.Terms == "" {
			t.Errorf("source %+v lacks its name or terms", s)
		}
		for _, h := range s.Hosts {
			runtime++
			if h == "" || h != strings.ToLower(h) || strings.ContainsAny(h, ":/ ") {
				t.Errorf("source %s: host %q is not a bare host name", s.Name, h)
			}
		}
	}
	if runtime < 2 {
		t.Errorf("%d run-time hosts; GIRO and NOAA are read at run time", runtime)
	}
	sources[0].Name = "changed by the host"
	if l.Sources()[0].Name == "changed by the host" {
		t.Error("a host's change to the list reached the library's own")
	}
}

// TestEveryRequestGoesToAnExportedHost is watchpost D-113: every web address
// written in the library's code is on a host the library exports, so a host
// that allows those hosts alone allows every request; one planted on another
// host is caught.
func TestEveryRequestGoesToAnExportedHost(t *testing.T) {
	l, err := ionomaps.New(&noFetch{}, ionomaps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	for _, s := range l.Sources() {
		hosts = append(hosts, s.Hosts...)
	}
	for name, f := range libraryFiles(t) {
		for _, u := range addresses(f) {
			if !onHost(u, hosts) {
				t.Errorf("%s asks %s, on a host the library does not export (D-113)", name, u)
			}
		}
	}
	planted, err := parser.ParseFile(token.NewFileSet(), "planted.go", "package p\n\nconst u = \"https://elsewhere.example/data\"\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	if u := addresses(planted); len(u) != 1 || onHost(u[0], hosts) {
		t.Errorf("a planted address on another host was not caught: %v", u)
	}
}

// addresses are a file's string constants that are web addresses.
func addresses(f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if v, err := strconv.Unquote(lit.Value); err == nil && (strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://")) {
			out = append(out, v)
		}
		return true
	})
	return out
}

// onHost reports whether an address is on one of the hosts.
func onHost(address string, hosts []string) bool {
	u, err := url.Parse(address)
	return err == nil && slices.Contains(hosts, u.Hostname())
}

// TestEveryRunTimeSourceCarriesItsByteBound is watchpost FR-4.4 and W0.2
// (D-201): every source asked at run time says the most one reply from it
// may be, so a host's client takes nothing larger from that host; GIRO's is
// its own, smaller than NOAA's grid; and the library's own checks are those
// same bounds - one carrier each.
func TestEveryRunTimeSourceCarriesItsByteBound(t *testing.T) {
	l, err := ionomaps.New(&noFetch{}, ionomaps.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bound := map[string]int{}
	for _, s := range l.Sources() {
		if len(s.Hosts) > 0 && s.MaxBytes <= 0 {
			t.Errorf("%s is asked at run time with no byte bound", s.Name)
		}
		for _, h := range s.Hosts {
			bound[h] = s.MaxBytes
		}
	}
	if giro, noaa := bound["lgdc.uml.edu"], bound["services.swpc.noaa.gov"]; giro <= 0 || giro >= noaa {
		t.Errorf("GIRO's bound %d, NOAA's %d: GIRO's replies are its stations' rows, NOAA's largest a grid", giro, noaa)
	}
	if _, err := glotec.Decode(make([]byte, bound["services.swpc.noaa.gov"]+1), time.Now()); err == nil {
		t.Error("GloTEC decodes a grid past NOAA's bound")
	}
	now := time.Now()
	if _, err := giro.ParseReply(make([]byte, bound["lgdc.uml.edu"]+1), "AL945", now.Add(-time.Hour), now, now); err == nil {
		t.Error("GIRO's parser takes a reply past GIRO's bound")
	}
}
