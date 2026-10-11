package glotec

import (
	"bytes"
	"time"
)

// maxIndexBytes bounds the directory index: about eight times the measured
// 505 KB (watchpost W0.0).
const maxIndexBytes = 4 << 20

// A grid's name in the index is its valid time, to the second, between
// these: glotec_icao_YYYYMMDDTHHMMSSZ.geojson.
const (
	namePrefix = "glotec_icao_"
	nameSuffix = ".geojson"
	stampForm  = "20060102T150405Z"
)

// Newest is the newest grid the directory index names, asked at now: its
// name and valid time. A name past the clock, or whose time does not read,
// is passed over; false when the index names no grid.
//
// Source: NOAA SWPC GloTEC's directory index, as served (watchpost D-111,
// D-141).
func Newest(index []byte, now time.Time) (name string, valid time.Time, ok bool) {
	if len(index) > maxIndexBytes {
		return "", time.Time{}, false
	}
	latest := now.Add(clockSkew)
	rest := index
	for range len(index)/len(namePrefix) + 1 { // each pass takes at least a prefix off
		at := bytes.Index(rest, []byte(namePrefix))
		if at < 0 {
			break
		}
		rest = rest[at+len(namePrefix):]
		stamp, named := gridStamp(rest)
		if !named {
			continue
		}
		rest = rest[len(stampForm)+len(nameSuffix):]
		t, err := time.Parse(stampForm, string(stamp))
		usable := err == nil && !t.After(latest) && t.After(valid)
		if usable {
			name, valid = namePrefix+string(stamp)+nameSuffix, t
		}
	}
	if name == "" {
		return "", time.Time{}, false
	}
	return name, valid, true
}

// gridStamp is the stamp at the start of b when a grid's name goes on
// there: eight digits, T, six digits, Z, then the suffix.
func gridStamp(b []byte) ([]byte, bool) {
	if len(b) < len(stampForm)+len(nameSuffix) || string(b[len(stampForm):len(stampForm)+len(nameSuffix)]) != nameSuffix {
		return nil, false
	}
	for i, f := range []byte(stampForm) {
		digit := '0' <= b[i] && b[i] <= '9'
		if f == 'T' || f == 'Z' {
			digit = b[i] == f
		}
		if !digit {
			return nil, false
		}
	}
	return b[:len(stampForm)], true
}
