package terms

import (
	"net/url"
	"slices"
	"testing"
)

// TestEveryAddressIsOnAListedHost is R-4.1: every address the library asks
// is on a host All names, so the hosts a host's client allows are the ones
// the library uses.
func TestEveryAddressIsOnAListedHost(t *testing.T) {
	var hosts []string
	for _, s := range All() {
		hosts = append(hosts, s.Hosts...)
	}
	for _, a := range []string{GloTECIndex, SolarFile, DRAPTable, ScalesFile, FastChar} {
		u, err := url.Parse(a)
		if err != nil || u.Scheme != "https" || !slices.Contains(hosts, u.Hostname()) {
			t.Errorf("%s: not an https address on a listed host (%v)", a, err)
		}
	}
}
