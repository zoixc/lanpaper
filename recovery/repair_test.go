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

// An installation that still has only wallpapers.json is repaired in the
// database. The JSON file is imported, never rewritten.
func TestRepairOfLegacyInstallLeavesJSONUntouched(t *testing.T) {
	root := repairFixture(t)
	jsonPath := filepath.Join(root, "data/wallpapers.json")
	before, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanRepair(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyRepair(plan); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("repair rewrote wallpapers.json")
	}
	if !storage.MetadataDatabaseExists(root) {
		t.Fatal("repair did not leave a database behind")
	}
	backups, _ := filepath.Glob(filepath.Join(root, "data/quarantine/*/data/wallpapers.db.before-repair"))
	if len(backups) != 1 {
		t.Fatalf("database backup before repair: %v", backups)
	}
}

// A running server holds the data directory lock, so repair refuses to touch
// the metadata while it runs.
func TestRepairRefusesWhileServerHoldsDataDir(t *testing.T) {
	root := repairFixture(t)
	unlock, err := storage.LockDataDir(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	plan, err := PlanRepair(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyRepair(plan); err == nil {
		t.Fatal("repair ran while the data directory was locked by a server")
	}
}
