// Package api implements the HTTP surface described in API_CONTRACT.md:
// projects, style, anchor, assets, views, masks, inventory, render, and the
// image blob endpoint.
package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/inventory"
	"render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// allowedOrigin is the only frontend origin CORS is opened to, per the
// contract ("CORS is open to http://localhost:5173").
const allowedOrigin = "http://localhost:5173"

// Server holds everything the HTTP handlers need: the structured-data
// repository, the blob store for image bytes, the image renderer, the text
// model for inventory/preservation, and config.
type Server struct {
	repo       store.Repository
	blobs      store.BlobStore
	renderer   render.Renderer
	textModel  *inventory.TextModel
	cfg        *config.Config
	promptPath string
	roleCache  *roleCache
	// serveBlobsLocally is true when the blob store is the in-memory one, whose
	// SignedURL points back at GET /api/images/{id}. That public byte-serving
	// route is then registered. With GCS, images load directly from signed GCS
	// URLs, so the public route is omitted (it would otherwise stream private
	// bytes to anyone holding the blob UUID).
	serveBlobsLocally bool
}

// signedURLTTL is how long the signed URLs handed to the browser stay valid.
// Long enough to keep a working session's images loading without refetching,
// short enough that a leaked URL soon stops working.
const signedURLTTL = time.Hour

// NewServer builds a Server. textModel may be nil in environments without
// Vertex AI credentials; inventory-generation and preservation-check
// endpoints will then return a clear error instead of panicking.
func NewServer(repo store.Repository, blobs store.BlobStore, renderer render.Renderer, textModel *inventory.TextModel, cfg *config.Config, promptPath string) *Server {
	initAuth(cfg.Auth.ClerkSecretKey)
	_, localBlobs := blobs.(*store.MemoryStore)
	return &Server{
		repo:              repo,
		blobs:             blobs,
		renderer:          renderer,
		textModel:         textModel,
		cfg:               cfg,
		promptPath:        promptPath,
		roleCache:         newRoleCache(),
		serveBlobsLocally: localBlobs,
	}
}

// Router builds the full HTTP handler, including CORS.
//
// The app is deployed at a public URL with open Clerk sign-up, so every
// data/render route below is admin-gated with requireAdmin (see roles.go):
// a signed-in user is let through only if their Clerk publicMetadata role is
// exactly "admin", read server-side from the verified Clerk user - never
// from anything the client sends. This protects renderView in particular,
// which spends real Vertex AI credits.
//
// GET /api/me is the one exception among authenticated routes: it requires
// only a verified session (requireAuth), not the admin role, so the
// frontend can learn its own role without eating a 403.
//
// GET /api/images/{id} is deliberately left fully public: it's fetched via
// <img src>, which can't send an Authorization header, so it stays readable
// only by knowing its unguessable UUID.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Not project-scoped: create takes no pid, list is filtered to the caller's
	// own projects inside the handler.
	mux.HandleFunc("POST /api/projects", s.handle(s.requireAdmin(s.createProject)))
	mux.HandleFunc("GET /api/projects", s.handle(s.requireAdmin(s.listProjects)))

	// owned wraps a project-scoped handler with requireAdmin + the per-user
	// ownership gate, so every {pid} route below is reachable only by the
	// project's owner.
	owned := func(fn handlerFunc) http.HandlerFunc {
		return s.handle(s.requireAdmin(s.requireOwner(fn)))
	}

	mux.HandleFunc("GET /api/projects/{pid}", owned(s.getProject))
	mux.HandleFunc("DELETE /api/projects/{pid}", owned(s.deleteProject))
	mux.HandleFunc("PUT /api/projects/{pid}/style", owned(s.updateStyle))
	mux.HandleFunc("POST /api/projects/{pid}/anchor", owned(s.setAnchor))

	mux.HandleFunc("POST /api/projects/{pid}/assets", owned(s.createAsset))
	mux.HandleFunc("PUT /api/projects/{pid}/assets/{aid}", owned(s.updateAsset))
	mux.HandleFunc("DELETE /api/projects/{pid}/assets/{aid}", owned(s.deleteAsset))
	mux.HandleFunc("POST /api/projects/{pid}/assets/{aid}/reference", owned(s.uploadAssetReference))

	mux.HandleFunc("POST /api/projects/{pid}/views", owned(s.createView))
	mux.HandleFunc("GET /api/projects/{pid}/views/{vid}", owned(s.getView))
	mux.HandleFunc("DELETE /api/projects/{pid}/views/{vid}", owned(s.deleteView))
	mux.HandleFunc("PUT /api/projects/{pid}/views/{vid}/inventory", owned(s.putInventory))
	mux.HandleFunc("POST /api/projects/{pid}/views/{vid}/inventory/generate", owned(s.generateInventory))

	mux.HandleFunc("POST /api/projects/{pid}/views/{vid}/masks", owned(s.createMask))
	mux.HandleFunc("PUT /api/projects/{pid}/views/{vid}/masks/{mid}", owned(s.updateMask))
	mux.HandleFunc("PUT /api/projects/{pid}/views/{vid}/masks/{mid}/bitmap", owned(s.uploadMaskBitmap))
	mux.HandleFunc("DELETE /api/projects/{pid}/views/{vid}/masks/{mid}", owned(s.deleteMask))

	mux.HandleFunc("POST /api/projects/{pid}/views/{vid}/render", owned(s.renderView))

	// Mint a signed URL for an image blob. Project-scoped so the ownership gate
	// above enforces per-user access; the {id} is the blob's own id.
	mux.HandleFunc("GET /api/projects/{pid}/images/{id}/url", owned(s.getImageURL))

	// Any signed-in user, not admin-gated - see the doc comment above.
	mux.HandleFunc("GET /api/me", s.handle(s.requireAuth(s.getMe)))

	// The public byte-serving route is only registered for the in-memory blob
	// store (local dev), whose signed URLs point back here. With GCS, images
	// load directly from signed GCS URLs and this route is intentionally absent
	// so private bytes are never served by blob UUID alone.
	if s.serveBlobsLocally {
		mux.HandleFunc("GET /api/images/{id}", s.handle(s.getImage))
	}

	return withCORS(mux)
}

// handle adapts an error-returning handler into a plain http.HandlerFunc,
// centralizing the {"error": "..."} response shape required by the contract.
func (s *Server) handle(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			writeErr(w, err)
		}
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: writing JSON response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, err error) {
	status, msg := statusAndMessage(err)
	log.Printf("api: request error (%d): %v", status, err)
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes the request body into v. An empty body is treated as "no
// fields set" (v is left at its zero value) rather than an error, so
// endpoints with an all-optional body (e.g. POST masks) can omit it.
func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if err == io.EOF {
			return nil
		}
		return badRequest("invalid JSON body: %v", err)
	}
	return nil
}

// readMultipartFile parses the request as multipart/form-data and returns
// the bytes and declared content type of the named file field.
func readMultipartFile(r *http.Request, field string) ([]byte, string, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, "", badRequest("invalid multipart form: %v", err)
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, "", badRequest("missing multipart field %q: %v", field, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", internalErr("reading uploaded file: %v", err)
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}
