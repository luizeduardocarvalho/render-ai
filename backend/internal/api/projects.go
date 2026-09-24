package api

import (
	"net/http"

	"render-ai/backend/internal/store"
)

type createProjectRequest struct {
	Name string `json:"name"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) error {
	var req createProjectRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return badRequest("name is required")
	}
	ownerID, _ := userIDFromContext(r.Context())
	p := s.repo.CreateProject(ownerID, req.Name)
	writeJSON(w, http.StatusOK, p)
	return nil
}

// listProjects returns the caller's own projects as lightweight summaries for
// the project picker.
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	summaries, err := s.repo.ListProjects(ownerID)
	if err != nil {
		return internalErr("listing projects: %v", err)
	}
	writeJSON(w, http.StatusOK, summaries)
	return nil
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	p, err := s.repo.GetProject(pid)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (s *Server) updateStyle(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	var style store.StyleSettings
	if err := readJSON(r, &style); err != nil {
		return err
	}
	p, err := s.repo.UpdateStyle(pid, style)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

type setAnchorRequest struct {
	RenderID *string `json:"renderId"`
}

func (s *Server) setAnchor(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	var req setAnchorRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	p, err := s.repo.SetAnchor(pid, req.RenderID)
	if err != nil {
		return mapStoreErr(err, "project or render not found")
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}
