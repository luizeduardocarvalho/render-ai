package api

import "net/http"

// createView takes the screenshot either as a multipart "file" field or, for
// a direct upload (see uploads.go), as JSON {"name", "uploadId"}.
func (s *Server) createView(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")

	var name, imageID string
	var width, height int
	if isJSONRequest(r) {
		var body struct {
			Name     string `json:"name"`
			UploadID string `json:"uploadId"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		// Check the project before claiming, so a bad pid doesn't consume
		// the upload.
		if _, err := s.repo.ProjectOwner(pid); err != nil {
			return mapStoreErr(err, "project %s not found", pid)
		}
		id, wd, ht, err := s.claimUpload(body.UploadID, "screenshot")
		if err != nil {
			return err
		}
		name, imageID, width, height = body.Name, id, wd, ht
	} else {
		data, _, err := readMultipartFile(r, "file")
		if err != nil {
			return err
		}
		name = r.FormValue("name")

		cfg, format, decErr := decodeImageConfig(data)
		if decErr != nil {
			return badRequest("invalid screenshot image: %v", decErr)
		}
		imageID, err = s.blobs.PutBlob(data, contentTypeForFormat(format))
		if err != nil {
			return badGateway("storing screenshot: %v", err)
		}
		width, height = cfg.Width, cfg.Height
	}
	if name == "" {
		name = "Untitled view"
	}

	v, err := s.repo.CreateView(pid, name, imageID, width, height)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) getView(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")
	v, err := s.repo.GetView(pid, vid)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) deleteView(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")
	blobIDs, err := s.repo.DeleteView(pid, vid)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	// Best-effort blob cleanup: the metadata is already gone, so a failed blob
	// delete only leaves an orphan, never a dangling reference.
	for _, id := range blobIDs {
		s.blobs.DeleteBlob(id)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type inventoryRequest struct {
	Inventory string `json:"inventory"`
}

func (s *Server) putInventory(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")
	var req inventoryRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	v, err := s.repo.SetInventory(pid, vid, req.Inventory)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) generateInventory(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")

	// Not charged (it's a cheap text-model call), but still gated on having
	// some balance left - see API_CONTRACT.md's credits section. Skipped
	// entirely when auth is disabled, like every credit check in this
	// package.
	if s.cfg.Auth.ClerkSecretKey != "" {
		ownerID, err := s.repo.ProjectOwner(pid)
		if err != nil {
			return mapStoreErr(err, "project %s not found", pid)
		}
		units, err := s.repo.GetCredits(ownerID)
		if err != nil {
			return internalErr("checking credits: %v", err)
		}
		if units <= 0 {
			return insufficientCreditsErr()
		}
	}

	v, err := s.repo.GetView(pid, vid)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	if !v.HasScreenshot {
		return badRequest("view %s has no screenshot", vid)
	}
	if s.textModel == nil {
		return internalErr("text model is not configured (missing Vertex AI credentials)")
	}
	blob, ok := s.blobs.GetBlob(v.ScreenshotImageID)
	if !ok {
		return internalErr("screenshot blob missing for view %s", vid)
	}

	text, _, _, err := s.textModel.GenerateInventory(r.Context(), blob.Data, r.URL.Query().Get("lang"))
	if err != nil {
		return badGateway("generating inventory: %v", err)
	}
	if _, err := s.repo.SetInventory(pid, vid, text); err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, map[string]string{"inventory": text})
	return nil
}
