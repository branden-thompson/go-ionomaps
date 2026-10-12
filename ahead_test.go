package ionomaps

import "testing"

// TestAnHourAheadIsMarkedTypical is R-1.3 (D-109): an hour ahead says it is
// typical, never a forecast; the hour now does not.
func TestAnHourAheadIsMarkedTypical(t *testing.T) {
	s := flatSnapshot(noonUTC, 7, 21)
	h, err := s.Ahead(1)
	if err != nil || !h.Typical || s.Hours[0].Typical {
		t.Errorf("an hour ahead typical %v (%v); now typical %v", h.Typical, err, s.Hours[0].Typical)
	}
}
