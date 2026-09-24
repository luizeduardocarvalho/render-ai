package api

import "net/http"

// getImage streams a stored blob by ID. It is the read path used by the
// in-memory store (local dev), where SignedURL returns a same-origin
// /api/images/{id} path. In the deployed backend, images load directly from
// GCS via signed URLs, so this endpoint is effectively dev-only - but it stays
// wired so both storage backends share one contract.
func (s *Server) getImage(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	blob, ok := s.blobs.GetBlob(id)
	if !ok {
		return notFoundErr("image %s not found", id)
	}
	w.Header().Set("Content-Type", blob.ContentType)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(blob.Data)
	return err
}

// signedImageURLResponse is the JSON body returned by GET /api/images/{id}/url.
type signedImageURLResponse struct {
	URL string `json:"url"`
}

// getImageURL returns a URL the browser can load the blob from directly. For
// GCS it is a short-lived V4 signed URL; for the in-memory store it is the
// same-origin /api/images/{id} path. This route is authenticated and
// ownership-gated (see the router), so minting the URL is where per-user
// access is enforced - the resulting URL is then a time-boxed capability.
func (s *Server) getImageURL(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	url, err := s.blobs.SignedURL(id, signedURLTTL)
	if err != nil {
		return internalErr("signing image url: %v", err)
	}
	writeJSON(w, http.StatusOK, signedImageURLResponse{URL: url})
	return nil
}
