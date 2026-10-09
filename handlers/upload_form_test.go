// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"lanpaper/config"
)

// multipartBody builds a body from ordered text fields and one file part.
// fieldsBeforeFile are written first; fieldsAfterFile come after the file.
func multipartBody(t *testing.T, before, after [][2]string, file []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	write := func(kv [2]string) {
		if err := mw.WriteField(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, kv := range before {
		write(kv)
	}
	if file != nil {
		part, err := mw.CreateFormFile("file", "picture.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(file); err != nil {
			t.Fatal(err)
		}
	}
	for _, kv := range after {
		write(kv)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// countingReader records how many bytes were actually pulled from the body.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// The point of the streaming parser: a request refused before the file part
// must not have its media read or written.
func TestReadUploadFormRefusesBeforeFileBytesAreRead(t *testing.T) {
	setupTempDirs(t)
	file := bytes.Repeat([]byte("x"), 1<<20)
	body, contentType := multipartBody(t, [][2]string{{"linkName", "guarded"}, {"mode", "replace"}}, nil, file)

	src := &countingReader{r: body}
	req := httptest.NewRequest(http.MethodPost, "/api/upload", src)
	req.Header.Set("Content-Type", contentType)

	_, err := readUploadForm(req, 2<<20, func(f *uploadForm) error {
		return errPublishReplace
	})
	if !errors.Is(err, errPublishReplace) {
		t.Fatalf("err = %v, want errPublishReplace", err)
	}
	// Only the text parts and the file's part header may have been consumed.
	if src.n >= len(file) {
		t.Fatalf("read %d bytes of the body before refusing; the file was consumed", src.n)
	}
	entries, _ := os.ReadDir(config.MediaDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			t.Fatalf("refused upload left a spool file behind: %s", e.Name())
		}
	}
}

// Fields sent after the file must still reach the handler: the early check is
// an extra gate, not a replacement for reading the whole form.
func TestReadUploadFormKeepsFieldsAfterTheFile(t *testing.T) {
	setupTempDirs(t)
	body, contentType := multipartBody(t,
		[][2]string{{"linkName", "late"}},
		[][2]string{{"category", "nature"}, {"accessLevel", "local"}},
		[]byte("payload"))

	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	form, err := readUploadForm(req, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer form.close()
	if form.value("category") != "nature" || form.value("accessLevel") != "local" {
		t.Fatalf("fields after the file were lost: %+v", form.fields)
	}
	data, _ := io.ReadAll(form.file)
	if string(data) != "payload" || form.fileName != "picture.png" {
		t.Fatalf("file = %q (%s)", data, form.fileName)
	}
}

// As with FormValue, a query-string value takes precedence over the body.
func TestUploadFormValueQueryWinsOverBody(t *testing.T) {
	setupTempDirs(t)
	body, contentType := multipartBody(t, [][2]string{{"linkName", "from-body"}}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload?linkName=from-query", body)
	req.Header.Set("Content-Type", contentType)
	form, err := readUploadForm(req, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := form.value("linkName"); got != "from-query" {
		t.Fatalf("value = %q, want the query value", got)
	}
}

func TestReadUploadFormBoundsTextParts(t *testing.T) {
	setupTempDirs(t)

	var many [][2]string
	for i := 0; i <= maxUploadFields; i++ {
		many = append(many, [2]string{"f", "v"})
	}
	body, contentType := multipartBody(t, many, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	if _, err := readUploadForm(req, 1<<20, nil); !errors.Is(err, errTooManyFormParts) {
		t.Fatalf("too many parts: err = %v", err)
	}

	huge := strings.Repeat("a", maxUploadFieldBytes+1)
	body, contentType = multipartBody(t, [][2]string{{"category", huge}}, nil, nil)
	req = httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	if _, err := readUploadForm(req, 1<<20, nil); !errors.Is(err, errFormFieldTooBig) {
		t.Fatalf("oversized field: err = %v", err)
	}
}

// setupTempDirs runs the test from a fresh temporary directory so spool files
// from these tests stay isolated.
func setupTempDirs(t *testing.T) {
	t.Helper()
	// config.MediaDir is a relative constant, so run from a temporary directory.
	t.Chdir(t.TempDir())
}
