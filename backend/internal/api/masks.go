package api

import (
	"encoding/json"
	"net/http"
)

type createMaskRequest struct {
	AssetID *string `json:"assetId"`
}

func (s *Server) createMask(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")
	var req createMaskRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	m, err := s.store.CreateMask(pid, vid, req.AssetID)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}

// updateMask applies partial updates. assetId and hidden are both optional
// in the request body, and assetId may be explicitly null to clear it - so
// the body is decoded as raw JSON first to tell "field absent" apart from
// "field present with a null/zero value".
func (s *Server) updateMask(w http.ResponseWriter, r *http.Request) error {
	pid, vid, mid := r.PathValue("pid"), r.PathValue("vid"), r.PathValue("mid")

	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		return err
	}

	var assetID *string
	assetIDSet := false
	if v, ok := raw["assetId"]; ok {
		assetIDSet = true
		if err := json.Unmarshal(v, &assetID); err != nil {
			return badRequest("invalid assetId: %v", err)
		}
	}

	var hidden *bool
	if v, ok := raw["hidden"]; ok {
		if err := json.Unmarshal(v, &hidden); err != nil {
			return badRequest("invalid hidden: %v", err)
		}
	}

	m, err := s.store.UpdateMask(pid, vid, mid, assetID, assetIDSet, hidden)
	if err != nil {
		return mapStoreErr(err, "mask %s not found", mid)
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}

func (s *Server) uploadMaskBitmap(w http.ResponseWriter, r *http.Request) error {
	pid, vid, mid := r.PathValue("pid"), r.PathValue("vid"), r.PathValue("mid")

	v, err := s.store.GetView(pid, vid)
	if err != nil {
		return mapStoreErr(err, "view %s not found", vid)
	}

	data, _, err := readMultipartFile(r, "file")
	if err != nil {
		return err
	}
	cfg, _, decErr := decodeImageConfig(data)
	if decErr != nil {
		return badRequest("invalid mask bitmap image: %v", decErr)
	}
	if cfg.Width != v.Width || cfg.Height != v.Height {
		return badRequest("mask bitmap size %dx%d does not match screenshot size %dx%d",
			cfg.Width, cfg.Height, v.Width, v.Height)
	}

	// The mask's bitmap blob is stored under the mask's own ID, so
	// GET /api/images/{maskId} fetches it - Mask has no separate image-id
	// field in the contract.
	s.store.PutBlobAt(mid, data, "image/png")
	m, err := s.store.SetMaskBitmap(pid, vid, mid)
	if err != nil {
		return mapStoreErr(err, "mask %s not found", mid)
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}

func (s *Server) deleteMask(w http.ResponseWriter, r *http.Request) error {
	pid, vid, mid := r.PathValue("pid"), r.PathValue("vid"), r.PathValue("mid")
	if err := s.store.DeleteMask(pid, vid, mid); err != nil {
		return mapStoreErr(err, "mask %s not found", mid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
