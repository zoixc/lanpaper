// SPDX-License-Identifier: MIT

package storage

import "testing"

// Renaming a link must not lose its traffic, and it must not hand that traffic
// to a different link that later takes the old name.
func TestRenameStatsMovesCounters(t *testing.T) {
	ResetStats()
	t.Cleanup(ResetStats)

	RecordHit("old", 100)
	RecordHit("old", 50)

	RenameStats("old", "new")
	if _, ok := StatsFor("old"); ok {
		t.Fatal("counters stayed under the old name")
	}
	got, ok := StatsFor("new")
	if !ok || got.Hits != 2 || got.Bytes != 150 || got.LastHit == 0 {
		t.Fatalf("after rename: %+v ok=%v", got, ok)
	}
	if tracked := trackedLinks.Load(); tracked != 1 {
		t.Fatalf("tracked links = %d, want 1", tracked)
	}
}

// A counter can outlive its link (a stale name), and a rename onto it must
// merge the two totals instead of dropping one of them.
func TestRenameStatsMergesWithoutLosingEitherSide(t *testing.T) {
	ResetStats()
	t.Cleanup(ResetStats)

	RecordHit("new", 3)
	RecordHit("old", 7)

	RenameStats("old", "new")
	got, ok := StatsFor("new")
	if !ok || got.Hits != 2 || got.Bytes != 10 {
		t.Fatalf("after merge: %+v ok=%v", got, ok)
	}
	if _, ok := StatsFor("old"); ok {
		t.Fatal("the old name kept counters")
	}
	if tracked := trackedLinks.Load(); tracked != 1 {
		t.Fatalf("tracked links = %d, want 1", tracked)
	}
}

func TestRenameStatsIgnoresNoOps(t *testing.T) {
	ResetStats()
	t.Cleanup(ResetStats)

	RecordHit("kept", 5)
	for _, tc := range []struct{ from, to string }{
		{"", "kept"},
		{"kept", "kept"},
		{"kept", ""},
		{"missing", "target"},
	} {
		RenameStats(tc.from, tc.to)
	}
	if got, ok := StatsFor("kept"); !ok || got.Hits != 1 || got.Bytes != 5 {
		t.Fatalf("a no-op rename changed the counters: %+v ok=%v", got, ok)
	}
	if _, ok := StatsFor("target"); ok {
		t.Fatal("renaming a link without counters created them")
	}
}
