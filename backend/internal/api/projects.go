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
	p, err := s.withLibraryAssets(s.repo.CreateProject(ownerID, req.Name), nil)
	if err != nil {
		return err
	}
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
	p, err := s.withLibraryAssets(s.repo.GetProject(pid))
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

// deleteProject soft-deletes the project named by {pid}: views, renders and
// their blobs are left in place (see store.Repository.DeleteProject), so this
// is a 30-day-recoverable delete, not a purge. An already-deleted or missing
// project returns 404, same as every other project-scoped route.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	if err := s.repo.DeleteProject(pid); err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) updateStyle(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	var style store.StyleSettings
	if err := readJSON(r, &style); err != nil {
		return err
	}
	if !style.InteriorLights.Valid() {
		return badRequest("interiorLights must be one of off, 3000k, 4000k, 6000k")
	}
	p, err := s.withLibraryAssets(s.repo.UpdateStyle(pid, style))
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
	p, err := s.withLibraryAssets(s.repo.SetAnchor(pid, req.RenderID))
	if err != nil {
		return mapStoreErr(err, "project or render not found")
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}
