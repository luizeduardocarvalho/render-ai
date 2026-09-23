// Package api implements the HTTP surface described in API_CONTRACT.md:
// projects, style, anchor, assets, views, masks, inventory, render, and the
// image blob endpoint.
package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/inventory"
	"render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// allowedOrigin is the only frontend origin CORS is opened to, per the
// contract ("CORS is open to http://localhost:5173").
const allowedOrigin = "http://localhost:5173"

// Server holds everything the HTTP handlers need: the in-memory store, the
// image renderer, the text model for inventory/preservation, and config.
type Server struct {
	store      *store.Store
	renderer   render.Renderer
	textModel  *inventory.TextModel
	cfg        *config.Config
	promptPath string
	roleCache  *roleCache
}

// NewServer builds a Server. textModel may be nil in environments without
// Vertex AI credentials; inventory-generation and preservation-check
// endpoints will then return a clear error instead of panicking.
func NewServer(st *store.Store, renderer render.Renderer, textModel *inventory.TextModel, cfg *config.Config, promptPath string) *Server {
	initAuth(cfg.Auth.ClerkSecretKey)
	return &Server{
		store:      st,
		renderer:   renderer,
		textModel:  textModel,
		cfg:        cfg,
		promptPath: promptPath,
		roleCache:  newRoleCache(),
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

	mux.HandleFunc("POST /api/projects", s.handle(s.requireAdmin(s.createProject)))
	mux.HandleFunc("GET /api/projects/{pid}", s.handle(s.requireAdmin(s.getProject)))
	mux.HandleFunc("PUT /api/projects/{pid}/style", s.handle(s.requireAdmin(s.updateStyle)))
	mux.HandleFunc("POST /api/projects/{pid}/anchor", s.handle(s.requireAdmin(s.setAnchor)))

	mux.HandleFunc("POST /api/projects/{pid}/assets", s.handle(s.requireAdmin(s.createAsset)))
	mux.HandleFunc("PUT /api/projects/{pid}/assets/{aid}", s.handle(s.requireAdmin(s.updateAsset)))
	mux.HandleFunc("DELETE /api/projects/{pid}/assets/{aid}", s.handle(s.requireAdmin(s.deleteAsset)))
	mux.HandleFunc("POST /api/projects/{pid}/assets/{aid}/reference", s.handle(s.requireAdmin(s.uploadAssetReference)))

	mux.HandleFunc("POST /api/projects/{pid}/views", s.handle(s.requireAdmin(s.createView)))
	mux.HandleFunc("GET /api/projects/{pid}/views/{vid}", s.handle(s.requireAdmin(s.getView)))
	mux.HandleFunc("DELETE /api/projects/{pid}/views/{vid}", s.handle(s.requireAdmin(s.deleteView)))
	mux.HandleFunc("PUT /api/projects/{pid}/views/{vid}/inventory", s.handle(s.requireAdmin(s.putInventory)))
	mux.HandleFunc("POST /api/projects/{pid}/views/{vid}/inventory/generate", s.handle(s.requireAdmin(s.generateInventory)))

	mux.HandleFunc("POST /api/projects/{pid}/views/{vid}/masks", s.handle(s.requireAdmin(s.createMask)))
	mux.HandleFunc("PUT /api/projects/{pid}/views/{vid}/masks/{mid}", s.handle(s.requireAdmin(s.updateMask)))
	mux.HandleFunc("PUT /api/projects/{pid}/views/{vid}/masks/{mid}/bitmap", s.handle(s.requireAdmin(s.uploadMaskBitmap)))
	mux.HandleFunc("DELETE /api/projects/{pid}/views/{vid}/masks/{mid}", s.handle(s.requireAdmin(s.deleteMask)))

	mux.HandleFunc("POST /api/projects/{pid}/views/{vid}/render", s.handle(s.requireAdmin(s.renderView)))

	// Any signed-in user, not admin-gated - see the doc comment above.
	mux.HandleFunc("GET /api/me", s.handle(s.requireAuth(s.getMe)))

	// Left unauthenticated - see the doc comment above.
	mux.HandleFunc("GET /api/images/{id}", s.handle(s.getImage))

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
