package api

import "net/http"

func (s *Server) createView(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")

	data, _, err := readMultipartFile(r, "file")
	if err != nil {
		return err
	}
	name := r.FormValue("name")
	if name == "" {
		name = "Untitled view"
	}

	cfg, format, decErr := decodeImageConfig(data)
	if decErr != nil {
		return badRequest("invalid screenshot image: %v", decErr)
	}

	imageID := s.store.PutBlob(data, contentTypeForFormat(format))
	v, err := s.store.CreateView(pid, name, imageID, cfg.Width, cfg.Height)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) getView(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")
	v, err := s.store.GetView(pid, vid)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) deleteView(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")
	if err := s.store.DeleteView(pid, vid); err != nil {
		return mapStoreErr(err, "view %s not found", vid)
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
	v, err := s.store.SetInventory(pid, vid, req.Inventory)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) generateInventory(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")

	v, err := s.store.GetView(pid, vid)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	if !v.HasScreenshot {
		return badRequest("view %s has no screenshot", vid)
	}
	if s.textModel == nil {
		return internalErr("text model is not configured (missing Vertex AI credentials)")
	}
	blob, ok := s.store.GetBlob(v.ScreenshotImageID)
	if !ok {
		return internalErr("screenshot blob missing for view %s", vid)
	}

	text, _, _, err := s.textModel.GenerateInventory(r.Context(), blob.Data)
	if err != nil {
		return badGateway("generating inventory: %v", err)
	}
	if _, err := s.store.SetInventory(pid, vid, text); err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, map[string]string{"inventory": text})
	return nil
}
