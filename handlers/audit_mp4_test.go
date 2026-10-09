// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"testing"
)

// An ftyp tag at offset 4 is not enough to be a video: the box must declare a
// video brand as major or compatible brand.
func TestInspectMediaFileRequiresVideoBrand(t *testing.T) {
	// Accepted: a video brand as compatible brand, and QuickTime's "qt  ".
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"video brand only as compatible", mp4WithBrand("xxxx", "isom")},
		{"QuickTime qt brand", mp4WithBrand("qt  ")},
	} {
		if ext, err := inspectMediaFile(bytes.NewReader(tt.data), ".download-1", int64(len(tt.data)), 1<<20); err != nil || ext != "mp4" {
			t.Errorf("%s: inspectMediaFile() = (%q, %v), want mp4", tt.name, ext, err)
		}
	}

	// Refused: an ftyp box that declares no video brand at all.
	unknown := mp4WithBrand("xxxx", "yyyy", "zzzz")
	if ext, err := inspectMediaFile(bytes.NewReader(unknown), "clip.mp4", int64(len(unknown)), 1<<20); err == nil {
		t.Errorf("ftyp without a video brand accepted as %q", ext)
	}

	// Refused: a box whose declared size is too small to hold a brand.
	short := []byte{0, 0, 0, 8, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 0, 0, 0, 0}
	if _, err := inspectMediaFile(bytes.NewReader(short), "clip.mp4", int64(len(short)), 1<<20); err == nil {
		t.Error("ftyp box with size 8 accepted")
	}
}
