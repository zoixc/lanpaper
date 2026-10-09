// SPDX-License-Identifier: MIT

package recovery

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lanpaper/storage"
)

type Operation struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}
type RepairPlan struct {
	Root       string      `json:"root"`
	Operations []Operation `json:"operations"`
	Manual     []Issue     `json:"manual"`
}

func PlanRepair(root string) (RepairPlan, error) {
	report, err := Audit(root)
	if err != nil {
		return RepairPlan{}, err
	}
	plan := RepairPlan{Root: root, Operations: []Operation{}, Manual: []Issue{}}
	for _, i := range report.Issues {
		switch i.Code {
		case "orphan-file", "stale-temporary":
			plan.Operations = append(plan.Operations, Operation{"quarantine", i.Path, i.Code})
		case "unsafe-permissions":
			plan.Operations = append(plan.Operations, Operation{"chmod", i.Path, "0600"})
		case "size-drift":
			plan.Operations = append(plan.Operations, Operation{"update-size", i.Path, i.Detail})
		case "missing-history", "missing-item":
			plan.Operations = append(plan.Operations, Operation{"remove-reference", i.Path, i.Code})
		default:
			plan.Manual = append(plan.Manual, i)
		}
	}
	return plan, nil
}

func ApplyRepair(plan RepairPlan) (err error) {
	dataDir := filepath.Join(plan.Root, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	lockPath := filepath.Join(dataDir, ".repair.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return errors.New("another repair is active or a stale data/.repair.lock exists")
		}
		return err
	}
	defer func() { _ = lock.Close(); _ = os.Remove(lockPath) }()
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	qdir := filepath.Join(dataDir, "quarantine", stamp)
	journalPath := filepath.Join(dataDir, "repair-"+stamp+".jsonl")
	journal, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer journal.Close()
	writer := bufio.NewWriter(journal)
	record := func(op Operation, status string, e error) error {
		entry := struct {
			Time      string    `json:"time"`
			Operation Operation `json:"operation"`
			Status    string    `json:"status"`
			Error     string    `json:"error,omitempty"`
		}{time.Now().UTC().Format(time.RFC3339Nano), op, status, ""}
		if e != nil {
			entry.Error = e.Error()
		}
		body, _ := json.Marshal(entry)
		if _, x := writer.Write(append(body, '\n')); x != nil {
			return x
		}
		if x := writer.Flush(); x != nil {
			return x
		}
		return journal.Sync()
	}
	metaChanged := false
	records, err := readRecords(plan.Root)
	if err != nil {
		return err
	}
	for _, op := range plan.Operations {
		if err := record(op, "started", nil); err != nil {
			return err
		}
		var opErr error
		abs := filepath.Join(plan.Root, filepath.FromSlash(op.Path))
		switch op.Action {
		case "quarantine":
			rel := filepath.FromSlash(op.Path)
			dst := filepath.Join(qdir, rel)
			if opErr = os.MkdirAll(filepath.Dir(dst), 0o700); opErr == nil {
				opErr = os.Rename(abs, dst)
			}
		case "chmod":
			opErr = os.Chmod(abs, 0o600)
		case "update-size":
			var info os.FileInfo
			info, opErr = os.Stat(abs)
			if opErr == nil {
				metaChanged = updateRecord(records, op.Path, info.Size(), false) || metaChanged
			}
		case "remove-reference":
			metaChanged = updateRecord(records, op.Path, 0, true) || metaChanged
		default:
			opErr = fmt.Errorf("unknown repair action %q", op.Action)
		}
		if opErr != nil {
			_ = record(op, "failed", opErr)
			return opErr
		}
		if err := record(op, "completed", nil); err != nil {
			return err
		}
	}
	if metaChanged {
		metaPath := filepath.Join(dataDir, "wallpapers.json")
		body, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return err
		}
		backup := filepath.Join(qdir, "data", "wallpapers.json.before-repair")
		if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
			return err
		}
		if original, readErr := os.ReadFile(metaPath); readErr == nil {
			if err := os.WriteFile(backup, original, 0o600); err != nil {
				return err
			}
		}
		tmp := metaPath + ".repair.tmp"
		if err := os.WriteFile(tmp, body, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, metaPath); err != nil {
			return err
		}
	}
	return nil
}

func readRecords(root string) (map[string]*storage.Wallpaper, error) {
	body, err := os.ReadFile(filepath.Join(root, "data", "wallpapers.json"))
	if os.IsNotExist(err) {
		return map[string]*storage.Wallpaper{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records map[string]*storage.Wallpaper
	err = json.Unmarshal(body, &records)
	return records, err
}
func updateRecord(records map[string]*storage.Wallpaper, path string, size int64, remove bool) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 3 {
		return false
	}
	for key, w := range records {
		if w == nil {
			continue
		}
		name := w.LinkName
		if name == "" {
			name = key
		}
		switch parts[1] {
		case "media":
			if parts[2] == name+"."+w.MIMEType && !remove {
				w.SizeBytes = size
				return true
			}
		case "previews":
			if parts[2] == name+".webp" && remove {
				w.Preview = ""
				return true
			}
		case "history":
			if len(parts) == 4 && parts[2] == name {
				for n, h := range w.History {
					if fmt.Sprintf("%d.%s", h.Version, h.Ext) == parts[3] {
						if remove {
							w.History = append(w.History[:n], w.History[n+1:]...)
						} else {
							w.History[n].SizeBytes = size
						}
						return true
					}
				}
			}
		case "items":
			if len(parts) == 4 && parts[2] == name {
				for n, it := range w.Items {
					if strconv.Itoa(it.ID)+"."+it.Ext == parts[3] {
						if remove {
							w.Items = append(w.Items[:n], w.Items[n+1:]...)
						} else {
							w.Items[n].SizeBytes = size
						}
						return true
					}
				}
			}
		}
	}
	return false
}
