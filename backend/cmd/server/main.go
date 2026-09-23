// Command server runs the render-ai backend: an in-memory HTTP API in front
// of the Vertex AI Gemini image models. See API_CONTRACT.md for the full
// HTTP contract.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"render-ai/backend/internal/api"
	"render-ai/backend/internal/config"
	"render-ai/backend/internal/inventory"
	"render-ai/backend/internal/render"
	"render-ai/backend/internal/store"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return fmt.Errorf("locating backend root: %w", err)
	}

	// Load backend/.env (if present) so CLERK_SECRET_KEY, GOOGLE_CLOUD_*, etc.
	// can live in a local file during development. godotenv does not overwrite
	// variables already set, so a real exported env var still wins.
	if err := godotenv.Load(filepath.Join(root, ".env")); err == nil {
		log.Printf("server: loaded environment from %s", filepath.Join(root, ".env"))
	}

	cfg, err := config.Load(filepath.Join(root, "config", "config.yaml"))
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	st := store.New()
	promptPath := filepath.Join(root, "prompts", "render.tmpl")

	var renderer render.Renderer
	var textModel *inventory.TextModel

	if cfg.Vertex.Project == "" {
		log.Println("server: GOOGLE_CLOUD_PROJECT / vertex.project not set - render and text-model endpoints will return a clear error instead of calling Vertex AI")
	} else {
		ctx := context.Background()
		// Image models run only on the "global" endpoint.
		imageClient, err := genai.NewClient(ctx, &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Project:  cfg.Vertex.Project,
			Location: cfg.Vertex.Location,
		})
		if err != nil {
			return fmt.Errorf("creating vertex ai image client: %w", err)
		}
		renderer = render.NewVertexRendererFromClient(imageClient)

		// The text model runs on a regional endpoint, which is usually a
		// different location than the image models. Reuse the image client when
		// the locations happen to match, otherwise build a second one.
		textClient := imageClient
		if cfg.Vertex.TextLocation != cfg.Vertex.Location {
			textClient, err = genai.NewClient(ctx, &genai.ClientConfig{
				Backend:  genai.BackendVertexAI,
				Project:  cfg.Vertex.Project,
				Location: cfg.Vertex.TextLocation,
			})
			if err != nil {
				return fmt.Errorf("creating vertex ai text client: %w", err)
			}
		}
		textModel = inventory.NewTextModel(textClient, cfg.Models.Text)
		log.Printf("server: vertex project=%s image-location=%s text-location=%s image-model=%s text-model=%s",
			cfg.Vertex.Project, cfg.Vertex.Location, cfg.Vertex.TextLocation, cfg.Models.ProImage, cfg.Models.Text)
	}

	srv := api.NewServer(st, renderer, textModel, cfg, promptPath)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	timeout := time.Duration(cfg.Server.RenderTimeoutSec+30) * time.Second
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(),
		ReadTimeout:       timeout,
		WriteTimeout:      timeout,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("server: listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// repoRoot returns the backend module directory, so config/prompts load
// correctly regardless of the working directory the binary is run from. It
// prefers the current working directory (if it contains go.mod) and falls
// back to this source file's location (two directories up from
// cmd/server/main.go), which works under `go run`, `go build`, and tests.
func repoRoot() (string, error) {
	if wd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("could not determine source file location")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile))), nil
}
