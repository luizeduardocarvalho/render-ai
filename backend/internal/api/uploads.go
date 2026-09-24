package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"render-ai/backend/internal/store"
)

// Direct uploads: the browser asks POST /api/projects/{pid}/uploads for a
// signed URL, PUTs the image straight to the bucket, then hands the returned
// uploadId to the endpoint that uses the image (create view, asset
// reference). The bytes never pass through Firebase Hosting, whose 60s
// cutoff an 8 MB screenshot on an ordinary upload link can exceed. See
// store.DirectUploads.

// uploadURLTTL is how long the browser has to start its PUT. GCS only checks
// expiry when a request begins, so a slow upload that started in time still
// completes.
const uploadURLTTL = 15 * time.Minute

// maxUploadBytes caps a direct upload. The multipart path's 32 MiB form limit
// was the old effective ceiling; 50 MiB leaves room for large 4K exports.
const maxUploadBytes = 50 << 20

// uploadContentTypes are the image types the decoders are registered for.
// Anything else would upload fine and then fail validation, so reject it
// before the browser spends time uploading.
var uploadContentTypes = map[string]bool{"image/png": true, "image/jpeg": true}

type uploadRequestBody struct {
	ContentType string `json:"contentType"`
}

type uploadResponse struct {
	UploadID string            `json:"uploadId"`
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
}

// createUpload is POST /api/projects/{pid}/uploads. It is project-scoped only
// so the ownership gate applies; the upload itself isn't tied to the project
// until it is used. 501 when the blob store can't take direct uploads (the
// in-memory store in local dev) - the frontend then uploads multipart.
func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) error {
	uploads, ok := s.blobs.(store.DirectUploads)
	if !ok {
		return &httpError{status: http.StatusNotImplemented, message: "direct uploads are not supported by this blob store"}
	}
	var body uploadRequestBody
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if !uploadContentTypes[body.ContentType] {
		return badRequest(`contentType must be "image/png" or "image/jpeg"`)
	}
	id := uuid.NewString()
	signed, err := uploads.UploadURL(id, body.ContentType, uploadURLTTL)
	if err != nil {
		return internalErr("creating upload url: %v", err)
	}
	writeJSON(w, http.StatusOK, uploadResponse{
		UploadID: id,
		URL:      signed.URL,
		Method:   http.MethodPut,
		Headers:  signed.Headers,
	})
	return nil
}

// claimUpload turns a finished direct upload into a blob: it reads the staged
// object, checks it is a real PNG/JPEG, stores it with PutBlob under a fresh
// blob ID and deletes the staged copy. what names the image in error
// messages ("screenshot", "reference image").
func (s *Server) claimUpload(uploadID, what string) (blobID string, width, height int, err error) {
	uploads, ok := s.blobs.(store.DirectUploads)
	if !ok {
		return "", 0, 0, badRequest("uploadId is not supported by this blob store; send the file as multipart")
	}
	if _, err := uuid.Parse(uploadID); err != nil {
		return "", 0, 0, badRequest("invalid uploadId")
	}
	blob, err := uploads.ReadUpload(uploadID, maxUploadBytes)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return "", 0, 0, badRequest("upload %s not found (not uploaded yet, or already used)", uploadID)
	case errors.Is(err, store.ErrUploadTooLarge):
		uploads.DeleteUpload(uploadID)
		return "", 0, 0, badRequest("%s is larger than %d MB", what, maxUploadBytes>>20)
	case err != nil:
		return "", 0, 0, badGateway("reading uploaded %s: %v", what, err)
	}

	cfg, format, decErr := decodeImageConfig(blob.Data)
	if decErr != nil {
		uploads.DeleteUpload(uploadID)
		return "", 0, 0, badRequest("invalid %s image: %v", what, decErr)
	}
	blobID, err = s.blobs.PutBlob(blob.Data, contentTypeForFormat(format))
	if err != nil {
		// Keep the staged upload so the client can retry with the same uploadId.
		return "", 0, 0, badGateway("storing %s: %v", what, err)
	}
	uploads.DeleteUpload(uploadID)
	return blobID, cfg.Width, cfg.Height, nil
}

// isJSONRequest reports whether the request body is JSON (a direct-upload
// reference) rather than multipart form data (the file itself).
func isJSONRequest(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}
