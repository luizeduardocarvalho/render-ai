package store

import (
	"errors"
	"time"
)

// DirectUploads is implemented by blob stores the browser can upload image
// bytes to directly (GCSStore, via a signed PUT URL), so a large file never
// passes through Firebase Hosting or this backend - Hosting cuts requests to
// Cloud Run off at 60s, which a multi-megabyte screenshot on an ordinary
// connection can exceed. The in-memory store does not implement it; the API
// then answers 501 and the frontend falls back to a multipart upload.
//
// An upload lands in a staging area, not as a blob: the API reads it back,
// validates it, copies it into a fresh blob with PutBlob and deletes the
// staged object. So an upload ID can only ever become one blob, and a client
// can never write to (or overwrite) a real blob ID.
type DirectUploads interface {
	// UploadURL returns a URL the browser PUTs the file to, valid for ttl,
	// and the headers it must send with it (they are part of the signature).
	UploadURL(uploadID, contentType string, ttl time.Duration) (SignedUpload, error)
	// ReadUpload returns a staged upload's bytes. ErrNotFound if nothing was
	// uploaded under uploadID; ErrUploadTooLarge if it exceeds maxBytes (it is
	// not read in that case).
	ReadUpload(uploadID string, maxBytes int64) (Blob, error)
	// DeleteUpload removes a staged upload. Missing uploads are not an error.
	DeleteUpload(uploadID string)
}

// SignedUpload is where and how the browser sends a direct upload.
type SignedUpload struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// ErrUploadTooLarge is returned by ReadUpload for an upload over its limit.
var ErrUploadTooLarge = errors.New("upload too large")
