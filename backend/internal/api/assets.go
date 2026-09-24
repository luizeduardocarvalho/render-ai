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
	a, err := s.repo.CreateAsset(pid, req.Name, req.Description, req.Color)
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
	a, err := s.repo.UpdateAsset(pid, aid, req.Name, req.Description, req.Color)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")
	if err := s.repo.DeleteAsset(pid, aid); err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// uploadAssetReference takes the photo either as a multipart "file" field
// or, for a direct upload (see uploads.go), as JSON {"uploadId"}.
func (s *Server) uploadAssetReference(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")

	var imageID string
	if isJSONRequest(r) {
		var body struct {
			UploadID string `json:"uploadId"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		// Check the asset before claiming, so a bad aid doesn't consume the
		// upload and leave an orphaned blob.
		project, err := s.repo.GetProject(pid)
		if err != nil {
			return mapStoreErr(err, "project %s not found", pid)
		}
		if findAssetIn(project, aid) == nil {
			return notFoundErr("asset %s not found", aid)
		}
		id, _, _, err := s.claimUpload(body.UploadID, "reference image")
		if err != nil {
			return err
		}
		imageID = id
	} else {
		data, _, err := readMultipartFile(r, "file")
		if err != nil {
			return err
		}
		_, format, decErr := decodeImageConfig(data)
		if decErr != nil {
			return badRequest("invalid reference image: %v", decErr)
		}
		imageID, err = s.blobs.PutBlob(data, contentTypeForFormat(format))
		if err != nil {
			return badGateway("storing reference image: %v", err)
		}
	}

	a, err := s.repo.SetAssetReference(pid, aid, imageID)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}
