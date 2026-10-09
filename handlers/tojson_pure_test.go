// SPDX-License-Identifier: MIT

package handlers

import (
	"reflect"
	"testing"

	"lanpaper/storage"
)

// toResponse runs on records that concurrent readers share, so it must leave
// them exactly as they were, including a non-normalised access level.
func TestToResponseDoesNotWriteToRecord(t *testing.T) {
	wp := &storage.Wallpaper{
		LinkName:       "x",
		AccessLevel:    "  PUBLIC  ",
		HasImage:       true,
		MIMEType:       "jpg",
		CurrentVersion: 2,
		History:        []storage.HistoryEntry{{Version: 1, Ext: "jpg"}},
	}
	before := *wp
	before.History = append([]storage.HistoryEntry(nil), wp.History...)

	resp := toResponse(wp)

	if !reflect.DeepEqual(*wp, before) {
		t.Fatalf("toResponse modified the record:\nbefore %+v\nafter  %+v", before, *wp)
	}
	if resp.AccessLevel == "" {
		t.Fatal("response lost the normalised access level")
	}
}
