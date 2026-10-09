// SPDX-License-Identifier: MIT

package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lanpaper/storage"
)

func repairFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"data/media", "data/previews", "data/history/wall", "data/items/wall"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	records := map[string]*storage.Wallpaper{"wall": {LinkName: "wall", HasImage: true, MIMEType: "png", SizeBytes: 1, Preview: "/api/preview/wall", History: []storage.HistoryEntry{{Version: 1, Ext: "jpg", SizeBytes: 2}}, Items: []storage.PlaylistItem{{ID: 1, Ext: "webp", SizeBytes: 2}}}}
	body, _ := json.Marshal(records)
	_ = os.WriteFile(filepath.Join(root, "data/wallpapers.json"), body, 0o600)
	_ = os.WriteFile(filepath.Join(root, "data/media/wall.png"), []byte("live"), 0o666)
	_ = os.WriteFile(filepath.Join(root, "data/media/orphan.png"), []byte("orphan"), 0o600)
	return root
}

func TestRepairDryRunDoesNotMutate(t *testing.T) {
	root := repairFixture(t)
	plan, err := PlanRepair(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) == 0 {
		t.Fatal("empty plan")
	}
	if _, err := os.Stat(filepath.Join(root, "data/media/orphan.png")); err != nil {
		t.Fatal("planning mutated storage")
	}
}

func TestApplyRepairQuarantinesAndRepairsMetadata(t *testing.T) {
	root := repairFixture(t)
	plan, err := PlanRepair(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyRepair(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "data/media/orphan.png")); !os.IsNotExist(err) {
		t.Fatalf("orphan remains: %v", err)
	}
	report, err := Audit(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range report.Issues {
		if i.Code != "missing-preview" {
			t.Errorf("unexpected remaining issue: %#v", i)
		}
	}
	records, err := readRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	w := records["wall"]
	if w.SizeBytes != 4 || len(w.History) != 0 || len(w.Items) != 0 || w.Preview == "" {
		t.Fatalf("metadata not repaired: %#v", w)
	}
	journals, _ := filepath.Glob(filepath.Join(root, "data/repair-*.jsonl"))
	if len(journals) != 1 {
		t.Fatalf("journals=%v", journals)
	}
}

func TestRepairExclusiveLock(t *testing.T) {
	root := repairFixture(t)
	lock := filepath.Join(root, "data/.repair.lock")
	if err := os.WriteFile(lock, []byte("busy"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, _ := PlanRepair(root)
	if err := ApplyRepair(plan); err == nil {
		t.Fatal("repair ignored exclusive lock")
	}
}
