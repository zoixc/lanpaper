// SPDX-License-Identifier: MIT

package handlers

import "errors"

type uploadPhase uint8

const (
	uploadValidated uploadPhase = iota
	uploadStaged
	uploadPublished
	uploadCommitted
	uploadFinalized
	uploadRolledBack
)

// uploadTransaction makes the upload boundary ordering explicit. Staged paths
// remain owned by existing request cleanup until publish succeeds; every
// published file is then rolled back in reverse order unless metadata commits.
type uploadTransaction struct {
	phase uploadPhase
	files []publishedFile
}

func (t *uploadTransaction) staged() error {
	if t.phase != uploadValidated {
		return errors.New("upload transaction staged out of order")
	}
	t.phase = uploadStaged
	return nil
}

func (t *uploadTransaction) publish(stage, dst string, maxBytes int64) (publishedFile, error) {
	if t.phase != uploadStaged && t.phase != uploadPublished {
		return publishedFile{}, errors.New("upload transaction publish out of order")
	}
	file, err := publishUploadFile(stage, dst, maxBytes)
	if err != nil {
		return file, err
	}
	t.files = append(t.files, file)
	t.phase = uploadPublished
	return file, nil
}

func (t *uploadTransaction) commit() error {
	if t.phase != uploadPublished {
		return errors.New("upload transaction commit out of order")
	}
	t.phase = uploadCommitted
	return nil
}

func (t *uploadTransaction) finalize() error {
	if t.phase != uploadCommitted {
		return errors.New("upload transaction finalize out of order")
	}
	for _, file := range t.files {
		file.finish()
	}
	t.phase = uploadFinalized
	return nil
}

func (t *uploadTransaction) rollback() {
	if t.phase == uploadCommitted || t.phase == uploadFinalized || t.phase == uploadRolledBack {
		return
	}
	for i := len(t.files) - 1; i >= 0; i-- {
		t.files[i].rollback()
	}
	t.phase = uploadRolledBack
}

var publishUploadFile = publishStaged
