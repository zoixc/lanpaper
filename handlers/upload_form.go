// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"lanpaper/config"
	"lanpaper/middleware"
	"lanpaper/storage"
)

// Limits for the text parts of an upload form. A real form needs a few hundred
// bytes; these bounds only stop a client from spending memory on parts.
const (
	maxUploadFieldBytes = 64 << 10
	maxUploadFields     = 32
)

var (
	errTooManyFormParts = errors.New("too many form parts")
	errFormFieldTooBig  = errors.New("form field too large")
	// errPublishReplace: a publish key asked to replace a link's live media.
	errPublishReplace = errors.New("publish key may not replace existing media")
)

// publishReplaceDenied reports whether this request is a publish-key request
// that would replace the live media of an existing link. Admin requests are
// never refused here. The check is advisory before the link lock is taken: the
// same rule is enforced again under the lock in Upload.
func publishReplaceDenied(r *http.Request, name, modeRaw string) bool {
	if middleware.PublisherFingerprint(r) == "" || !isValidLinkName(name) {
		return false
	}
	mode, ok := uploadMode(modeRaw)
	if !ok || mode != uploadModeReplace {
		return false
	}
	prev, exists := storage.Global.Get(name)
	return exists && prev.HasImage
}

// uploadForm is the parsed multipart body of POST /api/upload. Text fields are
// in memory; the file part is spooled to a temporary file, so a request that is
// refused early never writes its media to disk.
//
// Values come from the URL query first and then from the body, the same order
// http.Request.FormValue uses, so existing clients keep working unchanged.
type uploadForm struct {
	query    url.Values
	fields   map[string]string
	file     *os.File
	fileName string
}

// value returns the first value for key: the query string, then the body.
func (f *uploadForm) value(key string) string {
	if v := f.query[key]; len(v) > 0 {
		return v[0]
	}
	return f.fields[key]
}

// close releases the spooled file, if any.
func (f *uploadForm) close() {
	if f.file != nil {
		name := f.file.Name()
		f.file.Close()
		os.Remove(name)
		f.file = nil
	}
}

// readUploadForm streams the multipart body of r. beforeFile runs once the text
// parts that precede the file are known and before a single byte of the file is
// written; returning an error stops the request there. That is what lets a
// publish key be refused without accepting its media. Fields that come after
// the file are still read, and the caller checks them as before.
//
// The body is already limited by http.MaxBytesReader, so a body that is too
// large surfaces as *http.MaxBytesError.
func readUploadForm(r *http.Request, maxBytes int64, beforeFile func(*uploadForm) error) (*uploadForm, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	form := &uploadForm{query: r.URL.Query(), fields: make(map[string]string)}
	fileSeen := false
	parts := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			form.close()
			return nil, err
		}
		parts++
		if parts > maxUploadFields {
			form.close()
			return nil, errTooManyFormParts
		}
		name := part.FormName()
		isFile := part.FileName() != ""

		if isFile && name == "file" && !fileSeen {
			fileSeen = true
			// On refusal the part is deliberately not closed: part.Close would
			// read and discard the rest of the file. Returning ends the request
			// and the server drops the connection instead.
			if beforeFile != nil {
				if err := beforeFile(form); err != nil {
					form.close()
					return nil, err
				}
			}
			if err := spoolFilePart(form, part, part.FileName(), maxBytes); err != nil {
				form.close()
				return nil, err
			}
			part.Close()
			continue
		}
		if isFile {
			// A second file, or a file under another name: FormFile and
			// FormValue never looked at these, so they are drained and dropped.
			_, _ = io.Copy(io.Discard, io.LimitReader(part, maxBytes+1))
			part.Close()
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, maxUploadFieldBytes+1))
		part.Close()
		if err != nil {
			form.close()
			return nil, err
		}
		if len(value) > maxUploadFieldBytes {
			form.close()
			return nil, errFormFieldTooBig
		}
		// First value wins, as with FormValue.
		if _, seen := form.fields[name]; !seen {
			form.fields[name] = string(value)
		}
	}
	return form, nil
}

// spoolFilePart writes the file part to a temporary file in the media directory
// (the directory the file is published from) and rewinds it for reading.
func spoolFilePart(form *uploadForm, part io.Reader, filename string, maxBytes int64) error {
	if err := os.MkdirAll(config.MediaDir, config.DataDirPerm); err != nil {
		return fmt.Errorf("media directory: %w", err)
	}
	f, err := os.CreateTemp(config.MediaDir, ".upload-*")
	if err != nil {
		return err
	}
	form.file = f
	form.fileName = filename
	n, err := io.Copy(f, io.LimitReader(part, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return errMediaTooLarge
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return nil
}

// uploadFormError maps a form-reading error to the response the client gets.
func uploadFormError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge), errors.Is(err, errMediaTooLarge):
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, errPublishReplace):
		http.Error(w, "Publish keys cannot replace existing media; use mode=append", http.StatusForbidden)
	case errors.Is(err, errTooManyFormParts), errors.Is(err, errFormFieldTooBig):
		http.Error(w, "Invalid multipart form", http.StatusBadRequest)
	default:
		http.Error(w, "Invalid multipart form", http.StatusBadRequest)
	}
}
