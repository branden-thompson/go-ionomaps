// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package tmbench

import (
	"context"
	"fmt"
	"testing"

	tuimaps "github.com/branden-thompson/go-tuimaps"
)

// BenchmarkEmptyPan is the same pan frame with no overlay: the frame's own cost.
func BenchmarkEmptyPan(b *testing.B) {
	for _, s := range sizes {
		b.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(b *testing.B) {
			m, err := tuimaps.New(tuimaps.WithSize(s[0], s[1]))
			if err != nil {
				b.Fatal(err)
			}
			defer m.Close()
			m.ColourDepth(tuimaps.Truecolor)
			_ = m.FitWorld()
			_, _ = m.Settle(context.Background())
			size := tuimaps.Size{Cols: s[0], Rows: s[1]}
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				step := 1
				if i%2 == 1 {
					step = -1
				}
				if err := m.PanCells(step, 0); err != nil {
					b.Fatal(err)
				}
				if _, err := m.Render(size, valid); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
