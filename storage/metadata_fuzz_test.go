// SPDX-License-Identifier: MIT

package storage

import (
	"encoding/json"
	"testing"
)

func FuzzMetadataDecodeAndNormalize(f *testing.F) {
	for _, seed := range []string{`{}`, `{"wall":{"linkName":"wall","mimeType":"png","accessLevel":"token"}}`, `{"x":{"history":[{"version":1,"ext":"jpg"}],"items":[{"id":1,"ext":"png"}]}}`, `null`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256<<10 {
			t.Skip()
		}
		var records map[string]*Wallpaper
		if json.Unmarshal(data, &records) != nil {
			return
		}
		if len(records) > 2000 {
			t.Skip()
		}
		for _, wp := range records {
			if wp == nil {
				continue
			}
			_ = NormalizeAccessLevel(wp.AccessLevel)
			_ = NormalizeRotate(func() RotateConfig {
				if wp.Rotate != nil {
					return *wp.Rotate
				}
				return RotateConfig{}
			}(), len(wp.Items) > 0)
			_ = HistoryBytes(wp.History)
		}
	})
}
