package deadline

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// saysTimeout is an error that says, by its own account, whether it is a
// timeout.
type saysTimeout struct{ timeout bool }

func (saysTimeout) Error() string   { return "the client gave up" }
func (s saysTimeout) Timeout() bool { return s.timeout }

// TestLeftReadsTheDeadline: the time a context's deadline leaves, and none
// read from a context without one.
func TestLeftReadsTheDeadline(t *testing.T) {
	if _, set := Left(context.Background()); set {
		t.Error("a context with no deadline read as having one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	if left, set := Left(ctx); !set || left <= 59*time.Minute || left > time.Hour {
		t.Errorf("an hour's deadline read as %v, %v", left, set)
	}
}

// TestRoomKeepsTheMargin is watchpost D-295 and D-301: an ask has all it
// wants while the update leaves that and the margin after, the time left
// less the margin past that, and none under its floor; with no deadline,
// all it wants.
func TestRoomKeepsTheMargin(t *testing.T) {
	need, floor := 15*time.Second, 2*time.Second
	if limit, full := Room(context.Background(), need, floor); limit != need || !full {
		t.Errorf("no deadline: %v, %v; want all of the ask", limit, full)
	}
	for _, c := range []struct {
		left      time.Duration
		wantFull  bool
		wantAbove time.Duration // the limit is above this, and not over need
	}{
		{need + Margin + time.Second, true, need - time.Nanosecond},
		{need + Margin - 50*time.Millisecond, false, need - Margin - time.Second}, // just under a full ask's room: shortened
		{floor + Margin + time.Second, false, floor},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), c.left)
		limit, full := Room(ctx, need, floor)
		cancel()
		if full != c.wantFull || limit <= c.wantAbove || limit > need {
			t.Errorf("%v left: %v, full %v; want full %v, the limit above %v", c.left, limit, full, c.wantFull, c.wantAbove)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), floor+Margin-50*time.Millisecond)
	defer cancel()
	if limit, _ := Room(ctx, need, floor); limit != 0 {
		t.Errorf("under the floor and the margin: %v; want no ask", limit)
	}
	exact, cancelExact := context.WithTimeout(context.Background(), need+Margin+50*time.Millisecond)
	defer cancelExact()
	if limit, full := Room(exact, need, need); !full || limit != need {
		t.Errorf("an ask that must have all of need, with it and the margin left: %v, %v", limit, full)
	}
}

// TestATimeoutIsToldByItsOwnAccount: an error carrying Timeout() true,
// wrapped or not, is a timeout; one saying false, or none, is not.
func TestATimeoutIsToldByItsOwnAccount(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{saysTimeout{true}, true},
		{fmt.Errorf("asking: %w", saysTimeout{true}), true},
		{saysTimeout{false}, false},
		{errors.New("refused"), false},
		{nil, false},
	} {
		if got := IsTimeout(c.err); got != c.want {
			t.Errorf("IsTimeout(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
