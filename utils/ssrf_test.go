package utils

import "testing"

// TestPrivateRanges verifies that PrivateRanges initialises without error
// and contains entries for the expected RFC blocks.
func TestPrivateRanges(t *testing.T) {
	ranges := PrivateRanges()
	if len(ranges) == 0 {
		t.Fatal("PrivateRanges() returned empty slice")
	}
	// Spot-check: 10.0.0.1 must be covered.
	ip := []byte{10, 0, 0, 1}
	for _, r := range ranges {
		if r.Contains(ip) {
			return
		}
	}
	t.Error("PrivateRanges() does not contain 10.0.0.0/8")
}
