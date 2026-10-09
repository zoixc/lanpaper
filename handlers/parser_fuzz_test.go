// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

func FuzzRouteSelectors(f *testing.F) {
	for _, seed := range []string{"/api/link/wall/history/1", "/api/link/%2e%2e/history/0", "/rollback", "//history/999999999999999999999"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, path string) {
		if len(path) > 4096 {
			t.Skip()
		}
		_, _, _ = historyVersionFromPath(path)
		_, _ = linkNameFromSubPath(path, "/rollback")
	})
}

func FuzzMultipartFields(f *testing.F) {
	f.Add("wall", "replace", []byte("not an image"))
	f.Add("../escape", "append", []byte{0, 1, 2})
	f.Fuzz(func(t *testing.T, name, mode string, data []byte) {
		if len(name)+len(mode) > 4096 || len(data) > 64<<10 {
			t.Skip()
		}
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("linkName", name)
		_ = mw.WriteField("mode", mode)
		part, err := mw.CreateFormFile("file", "f.bin")
		if err != nil {
			return
		}
		_, _ = part.Write(data)
		_ = mw.Close()
		r := httptest.NewRequest("POST", "/api/upload", &body)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		form, err := readUploadForm(r, 128<<10, nil)
		if form != nil {
			form.cleanup()
		}
		_ = err
	})
}
