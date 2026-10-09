// SPDX-License-Identifier: MIT

package recovery

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lanpaper/config"
	"lanpaper/storage"
)

type Issue struct {
	Code       string `json:"code"`
	Path       string `json:"path"`
	Link       string `json:"link,omitempty"`
	Detail     string `json:"detail"`
	Repairable bool   `json:"repairable"`
}
type Report struct {
	Root   string  `json:"root"`
	Issues []Issue `json:"issues"`
}

func (r Report) Healthy() bool { return len(r.Issues) == 0 }

func safeName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, `/\\`)
}

func Audit(root string) (Report, error) {
	report := Report{Root: root, Issues: []Issue{}}
	metaPath := filepath.Join(root, "data", "wallpapers.json")
	body, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		body = []byte("{}")
	} else if err != nil {
		return report, err
	}
	var records map[string]*storage.Wallpaper
	if err := json.Unmarshal(body, &records); err != nil {
		return report, fmt.Errorf("parse %s: %w", metaPath, err)
	}
	expected := map[string]bool{}
	addMissing := func(path, code, link string, size int64) {
		rel, _ := filepath.Rel(root, path)
		expected[filepath.Clean(path)] = true
		info, e := os.Lstat(path)
		if e != nil {
			if os.IsNotExist(e) {
				report.Issues = append(report.Issues, Issue{code, filepath.ToSlash(rel), link, "referenced file is missing", code != "missing-media"})
			}
			return
		}
		if !info.Mode().IsRegular() {
			report.Issues = append(report.Issues, Issue{"invalid-type", filepath.ToSlash(rel), link, "expected a regular file", false})
			return
		}
		if size > 0 && info.Size() != size {
			report.Issues = append(report.Issues, Issue{"size-drift", filepath.ToSlash(rel), link, fmt.Sprintf("metadata=%d disk=%d", size, info.Size()), true})
		}
		if info.Mode().Perm()&0o022 != 0 {
			report.Issues = append(report.Issues, Issue{"unsafe-permissions", filepath.ToSlash(rel), link, info.Mode().Perm().String(), true})
		}
	}
	for key, wp := range records {
		if wp == nil {
			report.Issues = append(report.Issues, Issue{"invalid-record", "data/wallpapers.json", key, "null wallpaper record", false})
			continue
		}
		name := wp.LinkName
		if name == "" {
			name = key
		}
		if !safeName(name) {
			report.Issues = append(report.Issues, Issue{"invalid-record", "data/wallpapers.json", name, "unsafe link name", false})
			continue
		}
		if wp.HasImage {
			if !config.AllowedMediaExts["."+wp.MIMEType] {
				report.Issues = append(report.Issues, Issue{"invalid-record", "data/wallpapers.json", name, "unsafe or unsupported media extension", false})
				continue
			}
			addMissing(filepath.Join(root, "data", "media", name+"."+wp.MIMEType), "missing-media", name, wp.SizeBytes)
			if !config.IsVideoExt(wp.MIMEType) {
				addMissing(filepath.Join(root, "data", "previews", name+".webp"), "missing-preview", name, 0)
			}
		}
		for _, h := range wp.History {
			if h.Version == 0 || !config.AllowedMediaExts["."+h.Ext] {
				report.Issues = append(report.Issues, Issue{"invalid-record", "data/wallpapers.json", name, "invalid history reference", false})
				continue
			}
			addMissing(filepath.Join(root, "data", "history", name, fmt.Sprintf("%d.%s", h.Version, h.Ext)), "missing-history", name, h.SizeBytes)
		}
		for _, it := range wp.Items {
			if it.ID <= 0 || !config.AllowedMediaExts["."+it.Ext] {
				report.Issues = append(report.Issues, Issue{"invalid-record", "data/wallpapers.json", name, "invalid playlist reference", false})
				continue
			}
			addMissing(filepath.Join(root, "data", "items", name, fmt.Sprintf("%d.%s", it.ID, it.Ext)), "missing-item", name, it.SizeBytes)
		}
	}
	for _, dir := range []string{"media", "previews", "history", "items"} {
		base := filepath.Join(root, "data", dir)
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			clean := filepath.Clean(path)
			rel, _ := filepath.Rel(root, clean)
			n := d.Name()
			if strings.HasPrefix(n, ".upload-") || strings.HasPrefix(n, ".tmp-") || strings.HasPrefix(n, ".sessions-") || strings.HasPrefix(n, ".wallpapers-") {
				report.Issues = append(report.Issues, Issue{"stale-temporary", filepath.ToSlash(rel), "", "uncommitted temporary file", true})
				return nil
			}
			if !expected[clean] {
				report.Issues = append(report.Issues, Issue{"orphan-file", filepath.ToSlash(rel), "", "file is not referenced by metadata", true})
			}
			return nil
		})
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Path == report.Issues[j].Path {
			return report.Issues[i].Code < report.Issues[j].Code
		}
		return report.Issues[i].Path < report.Issues[j].Path
	})
	return report, nil
}
