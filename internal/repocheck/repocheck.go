// Package repocheck holds the repository's own checks (plan tasks G0.2,
// G0.3): what reads as third-party data, and what the NOTICE must name.
package repocheck

import (
	"regexp"
	"strings"

	"github.com/branden-thompson/go-ionomaps/internal/terms"
)

var (
	// location is a FastChar reply's station header: its geographic position.
	location = regexp.MustCompile(`GEO\s*\(\s*-?[\d.]+\s*[NS]\s+[\d.]+\s*E\s*\)`)
	// columns is a FastChar column header: a confidence score and the URSI
	// qualifier columns beside the characteristics.
	columns = regexp.MustCompile(`(?m)\bCS\b[^\n]*\bfoF2\s+QD\b`)
	// row is one sounding: a time, a confidence score and a frequency.
	row = regexp.MustCompile(`(?m)^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z?\s+\d{1,3}\s+\d+\.\d+\s`)
)

// minRows is how many sounding rows read as a reply rather than an example.
const minRows = 3

// GIROReply reports whether content reads as a GIRO FastChar reply, by what
// it holds rather than its name, and why (R-4.3).
//
// Source: this library; the reply's shape as GIRO's DIDBase FastChar service
// serves it.
func GIROReply(content []byte) (string, bool) {
	if len(content) == 0 {
		return "", false // an empty file holds nothing
	}
	switch {
	case location.Match(content):
		return "a station's location header", true
	case columns.Match(content):
		return "a column header with confidence and qualifier columns", true
	case len(row.FindAllIndex(content, minRows)) >= minRows:
		return "rows of soundings", true
	}
	return "", false
}

// MissingFromNotice names each source and each committed dataset the NOTICE
// does not name (R-4.2, R-4.3).
//
// Source: this library.
func MissingFromNotice(notice string, sources []terms.Source, datasets []terms.Dataset) []string {
	if len(sources) == 0 && len(datasets) == 0 {
		return []string{"the list of sources itself"} // a check over nothing would pass whatever the NOTICE says
	}
	var missing []string
	for _, s := range sources {
		if s.Name == "" || !strings.Contains(notice, s.Name) {
			missing = append(missing, "source "+s.Name)
		}
	}
	for _, d := range datasets {
		if d.Path == "" || !strings.Contains(notice, d.Path) {
			missing = append(missing, "dataset "+d.Path)
		}
	}
	return missing
}
