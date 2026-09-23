package api

import "net/http"

func (s *Server) getImage(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	blob, ok := s.store.GetBlob(id)
	if !ok {
		return notFoundErr("image %s not found", id)
	}
	w.Header().Set("Content-Type", blob.ContentType)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(blob.Data)
	return err
}
