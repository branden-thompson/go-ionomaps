package giro

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

// TestTimeLeftReadsTheDeadline: the time a context's deadline leaves, and none
// read from a context without one.
func TestTimeLeftReadsTheDeadline(t *testing.T) {
	if _, set := TimeLeft(context.Background()); set {
		t.Error("a context with no deadline read as having one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	if left, set := TimeLeft(ctx); !set || left <= 59*time.Minute || left > time.Hour {
		t.Errorf("an hour's deadline read as %v, %v", left, set)
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
