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
	"render-ai/backend/internal/jobs"
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

	repo, blobs, err := buildStorage(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}
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

	queue, inlineQueue, err := buildQueue(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("initializing render job queue: %w", err)
	}

	srv := api.NewServer(repo, blobs, renderer, textModel, cfg, promptPath, queue)
	if inlineQueue != nil {
		// The inline queue's handler is a bound method on srv, so it can only
		// be wired up after srv exists - see jobs.Inline's doc comment.
		inlineQueue.SetHandler(srv.RunRenderVariation)
	}

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

// buildStorage constructs the structured-data repository and blob store from
// config. The "memory" backend (default) returns a single in-process store
// acting as both - zero external dependencies, lost on restart, for local dev.
// The "firestore" backend wires Cloud Firestore for metadata and a GCS bucket
// for image blobs.
func buildStorage(ctx context.Context, cfg *config.Config) (store.Repository, store.BlobStore, error) {
	switch cfg.Storage.Backend {
	case "memory":
		ms := store.NewMemory()
		log.Println("server: storage backend=memory (in-process; projects are lost on restart)")
		return ms, ms, nil
	case "firestore":
		if cfg.Storage.BlobBucket == "" {
			return nil, nil, fmt.Errorf("firestore storage requires a blob bucket (BLOB_BUCKET / storage.blobBucket)")
		}
		// Storage.Project is separate from the (possibly cross-project) Vertex
		// project; empty means auto-detect the ambient Cloud Run project.
		repo, err := store.NewFirestore(ctx, cfg.Storage.Project, cfg.Storage.FirestoreDatabase)
		if err != nil {
			return nil, nil, fmt.Errorf("firestore repository: %w", err)
		}
		blobs, err := store.NewGCS(ctx, cfg.Storage.BlobBucket, cfg.Storage.SignerServiceAccount)
		if err != nil {
			return nil, nil, fmt.Errorf("gcs blob store: %w", err)
		}
		log.Printf("server: storage backend=firestore project=%s database=%s bucket=%s",
			orDefault(cfg.Storage.Project, "(detected)"), orDefault(cfg.Storage.FirestoreDatabase, "(default)"), cfg.Storage.BlobBucket)
		return repo, blobs, nil
	default:
		return nil, nil, fmt.Errorf("unknown storage backend %q (want \"memory\" or \"firestore\")", cfg.Storage.Backend)
	}
}

// buildQueue constructs the render-job queue from cfg.Jobs.Queue ("inline"
// or "cloudtasks" - config.Load already validated the value and, for
// cloudtasks, that the required fields are present). The returned
// *jobs.Inline is non-nil only in inline mode, so the caller can wire its
// handler up once the Server (which owns RunRenderVariation) exists.
func buildQueue(ctx context.Context, cfg *config.Config) (jobs.Queue, *jobs.Inline, error) {
	switch cfg.Jobs.Queue {
	case "cloudtasks":
		ct, err := jobs.NewCloudTasks(ctx, cfg.Jobs.TasksQueue, cfg.Jobs.WorkerURL, cfg.Jobs.InvokerServiceAccount, cfg.Jobs.WorkerAudience, cfg.Server.RenderTimeoutSec)
		if err != nil {
			return nil, nil, fmt.Errorf("cloud tasks queue: %w", err)
		}
		log.Printf("server: jobs.queue=cloudtasks tasksQueue=%s workerUrl=%s", cfg.Jobs.TasksQueue, cfg.Jobs.WorkerURL)
		return ct, nil, nil
	default: // "inline", already validated by config.Load
		log.Println("server: jobs.queue=inline (render-job variations run in-process)")
		// Cloud Run sets K_SERVICE. There, inline renders run after the 202 is
		// sent, when Cloud Run throttles the instance's CPU - they crawl or die.
		// This means the image shipped before `terraform apply` set RENDER_QUEUE.
		if os.Getenv("K_SERVICE") != "" {
			log.Println("server: WARNING jobs.queue=inline on Cloud Run - set RENDER_QUEUE=cloudtasks (run terraform apply); renders will be unreliable")
		}
		inlineQueue := jobs.NewInline()
		return inlineQueue, inlineQueue, nil
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
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
