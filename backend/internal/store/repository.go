package store

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a project, view, mask, asset or render ID does
// not exist (or, for project-scoped calls, when it belongs to a different
// owner and the caller must not learn it exists).
var ErrNotFound = errors.New("not found")

// Blob is a stored image (screenshot, reference photo, mask bitmap, or render
// result) with its content type.
type Blob struct {
	Data        []byte
	ContentType string
}

// ProjectSummary is the lightweight projection returned by
// Repository.ListProjects for the project picker - enough to render a card
// without loading every view, mask and render.
type ProjectSummary struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ViewCount        int       `json:"viewCount"`
	RenderCount      int       `json:"renderCount"`
	ThumbnailImageID string    `json:"thumbnailImageId,omitempty"`
}

// Repository is the structured-data store for render-ai: projects and their
// nested views, masks, assets and renders. Image bytes live in a BlobStore,
// never here - the metadata only references blob IDs.
//
// It has two implementations: MemoryStore (in-process, for local dev) and
// FirestoreStore (Cloud Firestore, for the deployed backend). All returned
// values are deep copies the caller may freely mutate or JSON-encode.
//
// Project-scoped methods return ErrNotFound both when the project is absent
// and when it belongs to a different owner - callers must not distinguish the
// two. Ownership is enforced one layer up (see the API layer), so these
// methods trust their pid argument.
type Repository interface {
	// Projects.
	CreateProject(ownerID, name string) *Project
	ListProjects(ownerID string) ([]ProjectSummary, error)
	ProjectOwner(pid string) (string, error)
	GetProject(pid string) (*Project, error)
	UpdateStyle(pid string, style StyleSettings) (*Project, error)
	SetAnchor(pid string, renderID *string) (*Project, error)
	// DeleteProject soft-deletes a project: its DeletedAt is set, but nothing
	// else is removed - views, renders and blobs stay, so the project can be
	// restored. Every other Repository method must then treat it as gone
	// (ListProjects excludes it, ProjectOwner/GetProject/etc return
	// ErrNotFound). Returns ErrNotFound if the project doesn't exist or is
	// already deleted.
	DeleteProject(pid string) error

	// Assets.
	CreateAsset(pid, name, description, color string) (*Asset, error)
	UpdateAsset(pid, aid, name, description, color string) (*Asset, error)
	DeleteAsset(pid, aid string) error
	SetAssetReference(pid, aid, imageID string) (*Asset, error)

	// Views. DeleteView returns the blob IDs orphaned by the deletion
	// (screenshot, mask bitmaps, render results) so the caller can remove them
	// from the BlobStore - the two stores are decoupled.
	CreateView(pid, name, screenshotImageID string, width, height int) (*View, error)
	GetView(pid, vid string) (*View, error)
	DeleteView(pid, vid string) ([]string, error)
	SetInventory(pid, vid, inventory string) (*View, error)

	// Masks. A mask's bitmap blob is stored under the mask's own ID, so no
	// separate blob-id return is needed - the caller deletes blob `mid`.
	CreateMask(pid, vid string, assetID *string) (*Mask, error)
	UpdateMask(pid, vid, mid string, assetID *string, assetIDSet bool, hidden *bool) (*Mask, error)
	SetMaskBitmap(pid, vid, mid string) (*Mask, error)
	DeleteMask(pid, vid, mid string) error

	// Renders.
	AddRender(pid, vid string, r *Render) (*Render, error)
	FindRender(pid, renderID string) (*Render, error)

	// Render jobs (async render queue - see internal/jobs and
	// internal/api/render.go). CreateRenderJob takes a job with its ID and
	// Variations already populated by the caller, mirroring AddRender.
	// UpdateRenderJob is an atomic read-modify-write: fn runs against the
	// live job (a Firestore transaction; MemoryStore under its lock) and its
	// return value is persisted iff fn succeeds. All three return
	// ErrNotFound if the project or job doesn't exist (soft-deleted project
	// included).
	CreateRenderJob(pid string, job *RenderJob) (*RenderJob, error)
	GetRenderJob(pid, jid string) (*RenderJob, error)
	UpdateRenderJob(pid, jid string, fn func(j *RenderJob) error) (*RenderJob, error)
}

// BlobStore is the binary-object store for image bytes, keyed by blob ID. It
// has two implementations: MemoryStore (in-process map) and GCSStore (a Google
// Cloud Storage bucket).
type BlobStore interface {
	// PutBlob stores data under a new random ID and returns it. An error means
	// the data was not durably stored - callers must not record a reference to
	// the returned ID (there won't be one) or otherwise proceed as if the
	// write succeeded.
	PutBlob(data []byte, contentType string) (string, error)
	// PutBlobAt stores data under a caller-chosen ID, overwriting any existing
	// blob there. Used for mask bitmaps, whose blob ID is the mask's own ID. An
	// error means the write did not durably succeed - callers must not record a
	// reference to id as if it now holds this data.
	PutBlobAt(id string, data []byte, contentType string) error
	// GetBlob fetches a stored blob by ID.
	GetBlob(id string) (Blob, bool)
	// DeleteBlob removes a blob by ID. Missing blobs are not an error.
	DeleteBlob(id string)
	// SignedURL returns a URL the browser can load the blob from directly,
	// valid for at least ttl. For GCS this is a V4 signed URL; for the
	// in-memory store it is the same-origin /api/images/{id} path (there is
	// nothing to sign in local dev).
	SignedURL(id string, ttl time.Duration) (string, error)
}
