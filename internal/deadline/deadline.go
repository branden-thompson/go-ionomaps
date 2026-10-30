// Package deadline is what NOAA's round and GIRO's burst read of the host's
// update before each ask (watchpost D-291, D-295, D-298, D-301): the time
// its context leaves, the room an ask has in it, and whether an ask's error
// is a timeout by its own account. It is written once for both services.
package deadline

import (
	"context"
	"errors"
	"time"
)

// Margin is the time every ask leaves the host's update after it, so an ask
// given all the room there is still ends before the update's deadline
// (watchpost D-301).
var Margin = time.Second

// Left is the time the context's deadline leaves, and whether it has one;
// with none set, there is no limit to read.
//
// Source: watchpost D-291, D-298.
func Left(ctx context.Context) (left time.Duration, set bool) {
	at, set := ctx.Deadline()
	if !set {
		return 0, false // no deadline: the caller's own limits alone
	}
	return time.Until(at), true
}

// Room is how long an ask that wants need may take, and whether that is all
// of need: need while the update leaves it and Margin after; past that,
// what is left less Margin; zero when that would be under floor, no ask to
// start. An ask that must have all of need passes floor equal to need. With
// no deadline set, every ask has all it wants.
//
// Source: watchpost D-295, D-301.
func Room(ctx context.Context, need, floor time.Duration) (limit time.Duration, full bool) {
	left, set := Left(ctx)
	if !set {
		return need, true // no deadline: the ask has all it wants
	}
	left -= Margin
	if left >= need {
		return need, true
	}
	if left < floor {
		return 0, false // not even the least the ask may have fits
	}
	return left, false
}

// IsTimeout is whether an error is a timeout by its own account - the
// host's client giving up before the library's own limit does - told by the
// Timeout method the standard library's network errors carry, never by
// importing them (the library opens no connection of its own).
//
// Source: watchpost R4L-3.
func IsTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}
