package store

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a project, view, mask, asset or render ID does
// not exist (or, for project-scoped calls, when it belongs to a different
// owner and the caller must not learn it exists).
var ErrNotFound = errors.New("not found")

// ErrInsufficientCredits is returned by AdjustCredits when applying deltaUnits
// would take a user's balance below zero. The write is not applied - the
// balance and ledger are left exactly as they were.
var ErrInsufficientCredits = errors.New("insufficient credits")

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
	// ClearProjectAssets empties a project's legacy embedded Assets slice.
	// Used exactly once per project by the asset-library migration (see the
	// API layer's withLibraryAssets), after those assets have been imported
	// into the owner's library under the same ids via ImportAssets - so a
	// project only ever needs migrating the first time it's loaded after the
	// library shipped.
	ClearProjectAssets(pid string) error

	// Asset library: a user-wide library of assets shared by all of that
	// user's projects (see API_CONTRACT.md's asset library section), keyed
	// by ownerID (a Clerk user id, or "" when auth is disabled - see
	// internal/api/auth.go). The project-scoped asset routes are back-compat
	// wrappers that resolve {pid} to its owner and call these directly (see
	// internal/api/assets.go).
	ListAssets(ownerID string) ([]*Asset, error)
	GetAsset(ownerID, aid string) (*Asset, error)
	CreateAsset(ownerID, name, description, color string) (*Asset, error)
	UpdateAsset(ownerID, aid, name, description, color string) (*Asset, error)
	DeleteAsset(ownerID, aid string) error
	SetAssetReference(ownerID, aid, imageID string) (*Asset, error)
	// ImportAssets upserts-by-id into ownerID's library: an asset whose id is
	// already present is left untouched (so migrating the same legacy
	// project twice, or migrating two projects that happen to share stale
	// data, is idempotent and never clobbers a library edit made since the
	// first migration). Used only by the migration in withLibraryAssets.
	ImportAssets(ownerID string, assets []*Asset) error

	// Credits. Balances and ledger deltas are stored as integer "units" -
	// see internal/api/credits.go for the unit<->credit conversion the API
	// layer applies before this ever reaches JSON.
	//
	// GetCredits returns 0, nil for a user with no account yet (never
	// ErrNotFound) - every user implicitly starts at a zero balance.
	GetCredits(userID string) (int64, error)
	// AdjustCredits atomically applies deltaUnits to userID's balance and
	// appends entry (Reason set by the caller; ID/CreatedAt/DeltaUnits/
	// BalanceAfterUnits are filled in here) to their ledger. If the result
	// would be negative, nothing is written and it returns
	// ErrInsufficientCredits. Safe for concurrent callers - two renders that
	// would jointly overdraw the balance can never both succeed.
	AdjustCredits(userID string, deltaUnits int64, entry CreditLedgerEntry) (balanceUnits int64, err error)
	// ListCreditLedger returns up to limit entries (limit <= 0 means no
	// limit), newest first.
	ListCreditLedger(userID string, limit int) ([]CreditLedgerEntry, error)

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
	// ListRenderJobs returns the project's jobs last updated at or after
	// since, newest created first. ErrNotFound if the project doesn't exist
	// or is soft-deleted.
	ListRenderJobs(pid string, since time.Time) ([]*RenderJob, error)
	// MarkRenderJobSeen sets the job's SeenAt to at, unless it is already set.
	// It does not touch UpdatedAt, which the 24h window of the notification list
	// is measured on: seeing a job must not keep it listed longer.
	// ErrNotFound if the project or job doesn't exist.
	MarkRenderJobSeen(pid, jid string, at time.Time) error
	// ViewNames returns the names of the project's views by view id, without
	// loading the views' masks or renders. ErrNotFound if the project doesn't
	// exist or is soft-deleted.
	ViewNames(pid string) (map[string]string, error)
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
