package api

import "net/http"

type assetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	var req assetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return badRequest("name is required")
	}
	a, err := s.store.CreateAsset(pid, req.Name, req.Description, req.Color)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

func (s *Server) updateAsset(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")
	var req assetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	a, err := s.store.UpdateAsset(pid, aid, req.Name, req.Description, req.Color)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")
	if err := s.store.DeleteAsset(pid, aid); err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) uploadAssetReference(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")

	data, _, err := readMultipartFile(r, "file")
	if err != nil {
		return err
	}
	_, format, decErr := decodeImageConfig(data)
	if decErr != nil {
		return badRequest("invalid reference image: %v", decErr)
	}

	imageID := s.store.PutBlob(data, contentTypeForFormat(format))
	a, err := s.store.SetAssetReference(pid, aid, imageID)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}
