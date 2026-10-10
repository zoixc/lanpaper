// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"net/http"

	"lanpaper/storage"
)

// UploadService owns the metadata dependency for the complete upload
// transaction. Network/form resolution and image processing remain package
// functions, while publication and metadata commit flow through this service.
type UploadService struct {
	Store   *storage.Store
	Fetcher *RemoteFetcher
}

// UploadError identifies a transaction boundary while preserving the original
// error for errors.Is/errors.As and the existing HTTP response mapping.
type UploadError struct {
	Stage string
	Err   error
}

func (e *UploadError) Error() string { return fmt.Sprintf("upload %s: %v", e.Stage, e.Err) }
func (e *UploadError) Unwrap() error { return e.Err }

func NewUploadService(store *storage.Store) *UploadService {
	if store == nil {
		store = storage.Global
	}
	return &UploadService{Store: store, Fetcher: NewRemoteFetcher()}
}

// Upload is the compatibility HTTP entry point used by direct handler tests.
// Resolve storage.Global at call time because legacy tests replace that pointer.
func Upload(w http.ResponseWriter, r *http.Request) { NewUploadService(storage.Global).Upload(w, r) }
