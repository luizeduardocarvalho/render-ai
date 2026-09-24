package api

import (
	"net/http"

	"render-ai/backend/internal/store"
)

// assetRequest is the JSON body for creating/updating a library asset -
// shared by the user-wide asset-library routes and the project-scoped
// back-compat routes below.
type assetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// withLibraryAssets fills p.Assets from ownerID's asset library (see
// API_CONTRACT.md's asset library section), migrating any legacy assets
// still embedded directly in the project doc into that library first -
// lazily, idempotently (see Repository.ImportAssets) and keeping their ids,
// so existing mask bindings keep resolving - then clearing them from the
// project doc so this only runs once per project.
//
// Every handler that returns a Project, and the render worker
// (assembleRenderRequest), goes through this instead of calling
// s.repo.GetProject/UpdateStyle/etc. directly, so the migration and the
// "assets is always the owner's library" contract are implemented in one
// place. It takes (p, err) so it composes directly with a Repository call
// that already returns (*store.Project, error), e.g.
// s.withLibraryAssets(s.repo.GetProject(pid)).
func (s *Server) withLibraryAssets(p *store.Project, err error) (*store.Project, error) {
	if err != nil {
		return nil, err
	}
	if len(p.Assets) > 0 {
		if err := s.repo.ImportAssets(p.OwnerID, p.Assets); err != nil {
			return nil, internalErr("migrating project assets: %v", err)
		}
		if err := s.repo.ClearProjectAssets(p.ID); err != nil {
			return nil, internalErr("clearing migrated project assets: %v", err)
		}
	}
	library, err := s.repo.ListAssets(p.OwnerID)
	if err != nil {
		return nil, internalErr("loading asset library: %v", err)
	}
	p.Assets = library
	return p, nil
}

// --- User-wide asset library (requireAuth; scoped to the caller) -----------

// listLibraryAssets is GET /api/assets.
func (s *Server) listLibraryAssets(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	assets, err := s.repo.ListAssets(ownerID)
	if err != nil {
		return internalErr("listing assets: %v", err)
	}
	writeJSON(w, http.StatusOK, assets)
	return nil
}

// createLibraryAsset is POST /api/assets.
func (s *Server) createLibraryAsset(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	var req assetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return badRequest("name is required")
	}
	a, err := s.repo.CreateAsset(ownerID, req.Name, req.Description, req.Color)
	if err != nil {
		return internalErr("creating asset: %v", err)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

// updateLibraryAsset is PUT /api/assets/{aid}.
func (s *Server) updateLibraryAsset(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	aid := r.PathValue("aid")
	var req assetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	a, err := s.repo.UpdateAsset(ownerID, aid, req.Name, req.Description, req.Color)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

// deleteLibraryAsset is DELETE /api/assets/{aid}. It does not scan the
// caller's projects to unassign the deleted asset from masks: render already
// skips a mask whose assetId no longer resolves to a library asset (see
// qualifyingMasks in render.go), and per API_CONTRACT.md the frontend treats
// a dangling assetId as "unassigned" - so a stale mask binding is harmless
// and left to go stale, rather than paying for a cross-project write on
// every delete.
func (s *Server) deleteLibraryAsset(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	aid := r.PathValue("aid")
	if err := s.repo.DeleteAsset(ownerID, aid); err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// uploadLibraryAssetReference is POST /api/assets/{aid}/reference.
func (s *Server) uploadLibraryAssetReference(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	aid := r.PathValue("aid")
	return s.setAssetReferenceFromRequest(w, r, ownerID, aid)
}

// getAssetReferenceURL is GET /api/assets/{aid}/reference-url - a signed URL
// for the library screen, which has no project id to authenticate a
// project-scoped images/{id}/url call through.
func (s *Server) getAssetReferenceURL(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	aid := r.PathValue("aid")
	a, err := s.repo.GetAsset(ownerID, aid)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	if !a.HasReferenceImage {
		return notFoundErr("asset %s has no reference image", aid)
	}
	url, err := s.blobs.SignedURL(a.ReferenceImageID, signedURLTTL)
	if err != nil {
		return internalErr("signing reference image url: %v", err)
	}
	writeJSON(w, http.StatusOK, signedImageURLResponse{URL: url})
	return nil
}

// --- Project-scoped back-compat routes --------------------------------------
//
// These predate the user-wide library (see API_CONTRACT.md's asset library
// section) and keep working for older call sites: {pid} is already
// ownership-checked by requireOwner, so these resolve straight to the
// project's owner and operate on that owner's library - the same data the
// routes above expose.

// createAsset is POST /api/projects/{pid}/assets.
func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) error {
	pid := r.PathValue("pid")
	ownerID, err := s.repo.ProjectOwner(pid)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	var req assetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return badRequest("name is required")
	}
	a, err := s.repo.CreateAsset(ownerID, req.Name, req.Description, req.Color)
	if err != nil {
		return internalErr("creating asset: %v", err)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

// updateAsset is PUT /api/projects/{pid}/assets/{aid}.
func (s *Server) updateAsset(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")
	ownerID, err := s.repo.ProjectOwner(pid)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	var req assetRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	a, err := s.repo.UpdateAsset(ownerID, aid, req.Name, req.Description, req.Color)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

// deleteAsset is DELETE /api/projects/{pid}/assets/{aid}.
func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")
	ownerID, err := s.repo.ProjectOwner(pid)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	if err := s.repo.DeleteAsset(ownerID, aid); err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// uploadAssetReference is POST /api/projects/{pid}/assets/{aid}/reference:
// the photo either as a multipart "file" field or, for a direct upload (see
// uploads.go), as JSON {"uploadId"}.
func (s *Server) uploadAssetReference(w http.ResponseWriter, r *http.Request) error {
	pid, aid := r.PathValue("pid"), r.PathValue("aid")
	ownerID, err := s.repo.ProjectOwner(pid)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	return s.setAssetReferenceFromRequest(w, r, ownerID, aid)
}

// setAssetReferenceFromRequest is the shared body of uploadAssetReference and
// uploadLibraryAssetReference: read the photo (multipart or a claimed direct
// upload) and attach it to ownerID's asset aid.
func (s *Server) setAssetReferenceFromRequest(w http.ResponseWriter, r *http.Request, ownerID, aid string) error {
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
		if _, err := s.repo.GetAsset(ownerID, aid); err != nil {
			return mapStoreErr(err, "asset %s not found", aid)
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
		id, putErr := s.blobs.PutBlob(data, contentTypeForFormat(format))
		if putErr != nil {
			return badGateway("storing reference image: %v", putErr)
		}
		imageID = id
	}

	a, err := s.repo.SetAssetReference(ownerID, aid, imageID)
	if err != nil {
		return mapStoreErr(err, "asset %s not found", aid)
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}
