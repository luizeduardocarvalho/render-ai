// Package config loads the server's YAML configuration at startup.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level shape of config/config.yaml.
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Vertex       VertexConfig       `yaml:"vertex"`
	Models       ModelsConfig       `yaml:"models"`
	Pricing      PricingConfig      `yaml:"pricing"`
	Preservation PreservationConfig `yaml:"preservation"`
	Auth         AuthConfig         `yaml:"auth"`
	Storage      StorageConfig      `yaml:"storage"`
	Jobs         JobsConfig         `yaml:"jobs"`
}

// JobsConfig selects and configures how render-job variations are
// dispatched to the worker (POST /internal/render-tasks) - see
// internal/jobs and internal/api/render.go.
//
// Queue is "inline" (default - runs the worker func in a goroutine,
// in-process; used with memory storage and local dev) or "cloudtasks"
// (creates one Cloud Tasks HTTP task per variation, calling this same
// service's own run.app URL directly so Firebase Hosting's 60s cutoff never
// applies). The remaining fields are only
// meaningful (and required) in "cloudtasks" mode.
type JobsConfig struct {
	Queue string `yaml:"queue"`
	// TasksQueue is the full Cloud Tasks queue resource name:
	// projects/P/locations/L/queues/Q.
	TasksQueue string `yaml:"tasksQueue"`
	// WorkerURL is this service's own base URL (its run.app URL, never the
	// Firebase Hosting domain), e.g. https://render-ai-api-xxx.a.run.app.
	// Cloud Tasks POSTs to WorkerURL + "/internal/render-tasks".
	WorkerURL string `yaml:"workerUrl"`
	// WorkerAudience is the OIDC audience the worker route validates
	// incoming tokens against. Empty defaults to WorkerURL.
	WorkerAudience string `yaml:"workerAudience"`
	// InvokerServiceAccount is the service account Cloud Tasks mints the
	// worker's OIDC token as; the worker route requires the verified
	// token's email claim to match this exactly.
	InvokerServiceAccount string `yaml:"invokerServiceAccount"`
}

// StorageConfig selects and configures where projects and image blobs are
// persisted.
//
// Backend is "memory" (the default - in-process, lost on restart, for local
// dev) or "firestore" (Cloud Firestore for structured data + a GCS bucket for
// blobs, for the deployed backend). The firestore backend reuses the GCP
// project from vertex.project.
type StorageConfig struct {
	Backend string `yaml:"backend"`
	// Project is the GCP project that owns the Firestore database and blob
	// bucket. Empty means auto-detect from the runtime credentials / metadata
	// server (the ambient Cloud Run project). This is deliberately separate from
	// vertex.project, which may point at a different (cross-project) project for
	// Vertex AI.
	Project string `yaml:"project"`
	// BlobBucket is the GCS bucket name that holds image blobs (firestore
	// backend only). Required when Backend is "firestore".
	BlobBucket string `yaml:"blobBucket"`
	// FirestoreDatabase is the Firestore database id. Empty means the project's
	// default database ("(default)").
	FirestoreDatabase string `yaml:"firestoreDatabase"`
	// SignerServiceAccount is the service-account email used to sign GCS URLs
	// via the IAM credentials API. Empty means auto-detect from the runtime
	// service account (the normal case on Cloud Run).
	SignerServiceAccount string `yaml:"signerServiceAccount"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port             int `yaml:"port"`
	RenderTimeoutSec int `yaml:"renderTimeoutSec"`
}

// VertexConfig holds Vertex AI connection settings.
//
// Location is used for the image models, which run only on the "global"
// endpoint. TextLocation is used for the text model (inventory + preservation
// check), which runs on a regional endpoint (for example "us-central1") - the
// text models are not served from "global". When TextLocation is empty it
// falls back to Location.
type VertexConfig struct {
	Project      string `yaml:"project"`
	Location     string `yaml:"location"`
	TextLocation string `yaml:"textLocation"`
}

// ModelsConfig holds the resolved model ids sent to Vertex AI.
type ModelsConfig struct {
	ProImage   string `yaml:"proImage"`
	FlashImage string `yaml:"flashImage"`
	Text       string `yaml:"text"`
}

// PricingConfig holds cost-estimation prices for the metrics panel and the
// GET /api/pricing preview. Each model gets its own token rates, since the
// two image models and the text model are priced independently by Vertex.
type PricingConfig struct {
	ProImage   ImageModelPricing `yaml:"proImage"`
	FlashImage ImageModelPricing `yaml:"flashImage"`
	Text       TextPricing       `yaml:"text"`
	// UsdToBrl is the USD->BRL rate used to show a BRL estimate alongside the
	// USD one. Fill with a current rate; it's a display convenience only, not
	// itself billed.
	UsdToBrl float64 `yaml:"usdToBrl"`
}

// ImageModelPricing holds an image model's per-image prices (by resolution)
// plus its own input/output token rates. The output-image tokens themselves
// are never billed separately - PerImage already covers them - so
// OutputPerMTok only prices TEXT (and thinking) tokens in the response.
type ImageModelPricing struct {
	PerImage      map[string]float64 `yaml:"perImage"`
	InputPerMTok  float64            `yaml:"inputPerMTok"`
	OutputPerMTok float64            `yaml:"outputPerMTok"`
}

// TextPricing holds per-token prices for the text model.
type TextPricing struct {
	InputPerMTok  float64 `yaml:"inputPerMTok"`
	OutputPerMTok float64 `yaml:"outputPerMTok"`
}

// PreservationConfig holds the preservation-check thresholds.
type PreservationConfig struct {
	EdgeScoreFlagThreshold float64 `yaml:"edgeScoreFlagThreshold"`
	EdgeDilationPx         int     `yaml:"edgeDilationPx"`
}

// AuthConfig holds Clerk authentication settings. ClerkSecretKey is normally
// supplied via the CLERK_SECRET_KEY env var rather than checked into
// config.yaml. When it's empty, auth is disabled - see internal/api/auth.go.
type AuthConfig struct {
	ClerkSecretKey string `yaml:"clerkSecretKey"`
}

// Load reads and parses the YAML config file at path, then applies environment
// variable overrides (GOOGLE_CLOUD_PROJECT / GOOGLE_CLOUD_LOCATION) for the
// Vertex section, CLERK_SECRET_KEY for the Auth section, and PORT for the
// server's listen port.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if v := os.Getenv("GOOGLE_CLOUD_PROJECT"); v != "" {
		cfg.Vertex.Project = v
	}
	if v := os.Getenv("GOOGLE_CLOUD_LOCATION"); v != "" {
		cfg.Vertex.Location = v
	}
	if v := os.Getenv("GOOGLE_CLOUD_TEXT_LOCATION"); v != "" {
		cfg.Vertex.TextLocation = v
	}
	if cfg.Vertex.TextLocation == "" {
		cfg.Vertex.TextLocation = cfg.Vertex.Location
	}
	if v := os.Getenv("CLERK_SECRET_KEY"); v != "" {
		cfg.Auth.ClerkSecretKey = v
	}
	if v := os.Getenv("STORAGE"); v != "" {
		cfg.Storage.Backend = v
	}
	if v := os.Getenv("STORAGE_PROJECT"); v != "" {
		cfg.Storage.Project = v
	}
	if v := os.Getenv("BLOB_BUCKET"); v != "" {
		cfg.Storage.BlobBucket = v
	}
	if v := os.Getenv("FIRESTORE_DATABASE"); v != "" {
		cfg.Storage.FirestoreDatabase = v
	}
	if v := os.Getenv("SIGNER_SERVICE_ACCOUNT"); v != "" {
		cfg.Storage.SignerServiceAccount = v
	}
	if cfg.Storage.Backend == "" {
		cfg.Storage.Backend = "memory"
	}

	if v := os.Getenv("RENDER_QUEUE"); v != "" {
		cfg.Jobs.Queue = v
	}
	if v := os.Getenv("RENDER_TASKS_QUEUE"); v != "" {
		cfg.Jobs.TasksQueue = v
	}
	if v := os.Getenv("RENDER_WORKER_URL"); v != "" {
		cfg.Jobs.WorkerURL = v
	}
	if v := os.Getenv("RENDER_WORKER_AUDIENCE"); v != "" {
		cfg.Jobs.WorkerAudience = v
	}
	if v := os.Getenv("RENDER_TASKS_INVOKER_SA"); v != "" {
		cfg.Jobs.InvokerServiceAccount = v
	}
	if cfg.Jobs.Queue == "" {
		cfg.Jobs.Queue = "inline"
	}
	if cfg.Jobs.WorkerAudience == "" {
		cfg.Jobs.WorkerAudience = cfg.Jobs.WorkerURL
	}
	switch cfg.Jobs.Queue {
	case "inline":
	case "cloudtasks":
		var missing []string
		if cfg.Jobs.TasksQueue == "" {
			missing = append(missing, "jobs.tasksQueue / RENDER_TASKS_QUEUE")
		}
		if cfg.Jobs.WorkerURL == "" {
			missing = append(missing, "jobs.workerUrl / RENDER_WORKER_URL")
		}
		if cfg.Jobs.InvokerServiceAccount == "" {
			missing = append(missing, "jobs.invokerServiceAccount / RENDER_TASKS_INVOKER_SA")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("jobs.queue=cloudtasks requires %s", strings.Join(missing, ", "))
		}
	default:
		return nil, fmt.Errorf("unknown jobs.queue %q (want \"inline\" or \"cloudtasks\")", cfg.Jobs.Queue)
	}

	// Cloud Run (and most PaaS platforms) inject PORT and require the server to
	// bind to it, overriding whatever config.yaml says. Keep 8080 as the
	// default for local dev when PORT isn't set.
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 {
			return nil, fmt.Errorf("invalid PORT env var %q: must be a positive integer", v)
		}
		cfg.Server.Port = p
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.RenderTimeoutSec == 0 {
		cfg.Server.RenderTimeoutSec = 180
	}

	return &cfg, nil
}
