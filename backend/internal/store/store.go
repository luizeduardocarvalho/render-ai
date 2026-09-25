package store

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is the in-process implementation of Repository and BlobStore:
// projects plus their image blobs, guarded by a single mutex. It is safe for
// concurrent use and loses everything on restart - the zero-config store used
// for local dev when Firestore/GCS are not configured. The deployed backend
// uses FirestoreStore + GCSStore instead.
type MemoryStore struct {
	mu         sync.Mutex
	projects   map[string]*Project
	blobs      map[string]Blob
	renderJobs map[string]*RenderJob   // keyed by jobKey(pid, jid)
	accounts   map[string]*userAccount // keyed by userID ("" when auth is disabled)
	assetLibs  map[string][]*Asset     // keyed by ownerID, oldest first
}

// userAccount is one user's credit balance and ledger.
type userAccount struct {
	balanceUnits int64
	ledger       []CreditLedgerEntry // oldest first
}

// Compile-time checks that MemoryStore satisfies both storage interfaces.
var (
	_ Repository = (*MemoryStore)(nil)
	_ BlobStore  = (*MemoryStore)(nil)
)

// NewMemory creates an empty in-memory store.
func NewMemory() *MemoryStore {
	return &MemoryStore{
		projects:   make(map[string]*Project),
		blobs:      make(map[string]Blob),
		renderJobs: make(map[string]*RenderJob),
		accounts:   make(map[string]*userAccount),
		assetLibs:  make(map[string][]*Asset),
	}
}

// jobKey is the MemoryStore's renderJobs map key: jobs are scoped to a
// project, so the same job id could otherwise collide across projects.
func jobKey(pid, jid string) string { return pid + "/" + jid }

func newID() string { return uuid.NewString() }

// --- Blobs -----------------------------------------------------------------

// PutBlob stores data under a new random ID and returns it. It never fails -
// the map write cannot error - but returns an error to satisfy BlobStore
// alongside GCSStore, whose writes can.
func (s *MemoryStore) PutBlob(data []byte, contentType string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID()
	s.blobs[id] = Blob{Data: data, ContentType: contentType}
	return id, nil
}

// PutBlobAt stores data under a caller-chosen ID, overwriting any existing
// blob there. Used for mask bitmaps, whose image ID is the mask's own ID. It
// never fails; see PutBlob.
func (s *MemoryStore) PutBlobAt(id string, data []byte, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs[id] = Blob{Data: data, ContentType: contentType}
	return nil
}

// GetBlob fetches a stored blob by ID.
func (s *MemoryStore) GetBlob(id string) (Blob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.blobs[id]
	return b, ok
}

// DeleteBlob removes a blob by ID. Missing blobs are a no-op.
func (s *MemoryStore) DeleteBlob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blobs, id)
}

// SignedURL returns the same-origin path the frontend fetches the blob from.
// There is nothing to sign for the in-memory store - the ttl is ignored.
func (s *MemoryStore) SignedURL(id string, _ time.Duration) (string, error) {
	return "/api/images/" + id, nil
}

// --- Projects ----------------------------------------------------------------

// CreateProject creates a new project owned by ownerID, with default style
// settings.
func (s *MemoryStore) CreateProject(ownerID, name string) *Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	p := &Project{
		ID:        newID(),
		OwnerID:   ownerID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Style:     defaultStyle(),
		Assets:    []*Asset{},
		Views:     []*View{},
	}
	s.projects[p.ID] = p
	return p.clone()
}

// ListProjects returns lightweight summaries of every project owned by
// ownerID, newest first. Soft-deleted projects are excluded.
func (s *MemoryStore) ListProjects(ownerID string) ([]ProjectSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProjectSummary, 0)
	for _, p := range s.projects {
		if p.OwnerID != ownerID || p.DeletedAt != nil {
			continue
		}
		out = append(out, summarize(p))
	}
	sortSummariesNewestFirst(out)
	return out, nil
}

// liveProject looks up a project by id, treating a soft-deleted project the
// same as a missing one (ErrNotFound). Every method below that loads a
// project by id goes through this single helper, so deletion is enforced
// consistently. Callers must hold s.mu.
func (s *MemoryStore) liveProject(pid string) (*Project, error) {
	p, ok := s.projects[pid]
	if !ok || p.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return p, nil
}

// ProjectOwner returns the owner id of a project, or ErrNotFound.
func (s *MemoryStore) ProjectOwner(pid string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.liveProject(pid)
	if err != nil {
		return "", err
	}
	return p.OwnerID, nil
}

// GetProject returns a deep copy of the project.
func (s *MemoryStore) GetProject(pid string) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.liveProject(pid)
	if err != nil {
		return nil, err
	}
	return p.clone(), nil
}

// withProject runs fn against the live project under the store lock, bumps its
// UpdatedAt, then returns a deep copy of the (possibly mutated) project.
func (s *MemoryStore) withProject(pid string, fn func(p *Project) error) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.liveProject(pid)
	if err != nil {
		return nil, err
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	p.UpdatedAt = time.Now().UTC()
	return p.clone(), nil
}

// DeleteProject soft-deletes a project: it (and its views/renders/blobs) is
// left in place, but liveProject makes every other method treat it as gone.
// Returns ErrNotFound if the project doesn't exist or is already deleted.
//
// TODO: a scheduled purge job should hard-delete projects whose DeletedAt is
// more than 30 days old - removing the project, its views/renders and their
// blobs from the BlobStore. Not implemented here; see PERSISTENCE_HANDOFF.md.
func (s *MemoryStore) DeleteProject(pid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.liveProject(pid)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	p.DeletedAt = &now
	p.UpdatedAt = now
	return nil
}

// UpdateStyle replaces the project's style settings.
func (s *MemoryStore) UpdateStyle(pid string, style StyleSettings) (*Project, error) {
	return s.withProject(pid, func(p *Project) error {
		p.Style = style
		return nil
	})
}

// SetAnchor sets or clears the project's style-anchor render. renderID may be
// nil to clear it. It also keeps each render's IsStyleAnchor flag in sync.
func (s *MemoryStore) SetAnchor(pid string, renderID *string) (*Project, error) {
	return s.withProject(pid, func(p *Project) error {
		if renderID != nil {
			found := false
			for _, v := range p.Views {
				for _, r := range v.Renders {
					if r.ID == *renderID {
						found = true
					}
				}
			}
			if !found {
				return ErrNotFound
			}
		}
		for _, v := range p.Views {
			for _, r := range v.Renders {
				r.IsStyleAnchor = renderID != nil && r.ID == *renderID
			}
		}
		p.StyleAnchorRenderID = clonePtr(renderID)
		return nil
	})
}

// --- Asset library -----------------------------------------------------------
//
// The library is user-wide (keyed by ownerID, a Clerk user id or "" when auth
// is disabled), not project-scoped - see Repository's doc comment and
// API_CONTRACT.md's asset library section. ClearProjectAssets and
// ImportAssets exist only to support the API layer's lazy migration of
// legacy project-embedded assets into this library.

// SeedLegacyProjectAssets is a test-only helper that directly sets a
// project's legacy embedded Assets slice, simulating data written before the
// asset library existed (production code never writes to project.Assets
// directly any more - see ClearProjectAssets and the API layer's
// withLibraryAssets). Used by the migration test in internal/api.
func (s *MemoryStore) SeedLegacyProjectAssets(pid string, assets []*Asset) error {
	_, err := s.withProject(pid, func(p *Project) error {
		p.Assets = assets
		return nil
	})
	return err
}

// ClearProjectAssets empties a project's legacy embedded Assets slice.
func (s *MemoryStore) ClearProjectAssets(pid string) error {
	_, err := s.withProject(pid, func(p *Project) error {
		p.Assets = []*Asset{}
		return nil
	})
	return err
}

// ListAssets returns ownerID's library, newest first.
func (s *MemoryStore) ListAssets(ownerID string) ([]*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lib := s.assetLibs[ownerID]
	out := make([]*Asset, len(lib))
	for i, a := range lib {
		c := *a
		out[len(lib)-1-i] = &c
	}
	return out, nil
}

// findLibraryAsset returns the (mutable, store-owned) asset with id aid in
// ownerID's library. Callers must hold s.mu.
func (s *MemoryStore) findLibraryAsset(ownerID, aid string) *Asset {
	for _, a := range s.assetLibs[ownerID] {
		if a.ID == aid {
			return a
		}
	}
	return nil
}

// GetAsset returns one asset from ownerID's library.
func (s *MemoryStore) GetAsset(ownerID, aid string) (*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findLibraryAsset(ownerID, aid)
	if a == nil {
		return nil, ErrNotFound
	}
	c := *a
	return &c, nil
}

// CreateAsset adds a new asset to ownerID's library.
func (s *MemoryStore) CreateAsset(ownerID, name, description, color string) (*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := &Asset{ID: newID(), Name: name, Description: description, Color: color, CreatedAt: time.Now().UTC()}
	s.assetLibs[ownerID] = append(s.assetLibs[ownerID], a)
	c := *a
	return &c, nil
}

// UpdateAsset updates an existing library asset's name, description and color.
func (s *MemoryStore) UpdateAsset(ownerID, aid, name, description, color string) (*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findLibraryAsset(ownerID, aid)
	if a == nil {
		return nil, ErrNotFound
	}
	a.Name, a.Description, a.Color = name, description, color
	c := *a
	return &c, nil
}

// DeleteAsset removes an asset from ownerID's library. It does not touch any
// project's masks - see internal/api/assets.go's deleteLibraryAsset doc
// comment for why that's fine.
func (s *MemoryStore) DeleteAsset(ownerID, aid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lib := s.assetLibs[ownerID]
	for i, a := range lib {
		if a.ID == aid {
			s.assetLibs[ownerID] = append(lib[:i:i], lib[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// SetAssetReference attaches a reference photo blob to a library asset.
func (s *MemoryStore) SetAssetReference(ownerID, aid, imageID string) (*Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.findLibraryAsset(ownerID, aid)
	if a == nil {
		return nil, ErrNotFound
	}
	a.ReferenceImageID = imageID
	a.HasReferenceImage = true
	c := *a
	return &c, nil
}

// ImportAssets upserts-by-id into ownerID's library: an id already present is
// left untouched. See Repository.ImportAssets.
func (s *MemoryStore) ImportAssets(ownerID string, assets []*Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lib := s.assetLibs[ownerID]
	existing := make(map[string]bool, len(lib))
	for _, a := range lib {
		existing[a.ID] = true
	}
	for _, a := range assets {
		if existing[a.ID] {
			continue
		}
		c := *a
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now().UTC()
		}
		lib = append(lib, &c)
		existing[a.ID] = true
	}
	s.assetLibs[ownerID] = lib
	return nil
}

// --- Credits -------------------------------------------------------------

// GetCredits returns userID's balance in integer units, or 0 for a user with
// no account yet.
func (s *MemoryStore) GetCredits(userID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[userID]
	if !ok {
		return 0, nil
	}
	return a.balanceUnits, nil
}

// AdjustCredits atomically applies deltaUnits to userID's balance (creating
// the account on first use) and appends entry to their ledger. See
// Repository.AdjustCredits.
func (s *MemoryStore) AdjustCredits(userID string, deltaUnits int64, entry CreditLedgerEntry) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[userID]
	if !ok {
		a = &userAccount{}
		s.accounts[userID] = a
	}
	newBalance := a.balanceUnits + deltaUnits
	if newBalance < 0 {
		return a.balanceUnits, ErrInsufficientCredits
	}
	entry.ID = newID()
	entry.CreatedAt = time.Now().UTC()
	entry.DeltaUnits = deltaUnits
	entry.BalanceAfterUnits = newBalance
	a.balanceUnits = newBalance
	a.ledger = append(a.ledger, entry.clone())
	return newBalance, nil
}

// ListCreditLedger returns up to limit entries (limit <= 0 means no limit)
// from userID's ledger, newest first.
func (s *MemoryStore) ListCreditLedger(userID string, limit int) ([]CreditLedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[userID]
	if !ok {
		return []CreditLedgerEntry{}, nil
	}
	n := len(a.ledger)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]CreditLedgerEntry, n)
	for i := 0; i < n; i++ {
		out[i] = a.ledger[len(a.ledger)-1-i].clone()
	}
	return out, nil
}

// --- Views -------------------------------------------------------------------

// CreateView adds a new view (screenshot) to the project.
func (s *MemoryStore) CreateView(pid, name, screenshotImageID string, width, height int) (*View, error) {
	var created *View
	_, err := s.withProject(pid, func(p *Project) error {
		v := &View{
			ID:                newID(),
			Name:              name,
			ScreenshotImageID: screenshotImageID,
			HasScreenshot:     true,
			Width:             width,
			Height:            height,
			Masks:             []*Mask{},
			Renders:           []*Render{},
		}
		p.Views = append(p.Views, v)
		created = v.clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func findView(p *Project, vid string) *View {
	for _, v := range p.Views {
		if v.ID == vid {
			return v
		}
	}
	return nil
}

// GetView returns a deep copy of one view.
func (s *MemoryStore) GetView(pid, vid string) (*View, error) {
	var found *View
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		found = v.clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// DeleteView removes a view from the project and returns the blob IDs it
// orphaned (screenshot, mask bitmaps, render results) so the caller can delete
// them from the BlobStore.
func (s *MemoryStore) DeleteView(pid, vid string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.liveProject(pid)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, v := range p.Views {
		if v.ID == vid {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrNotFound
	}
	v := p.Views[idx]
	blobIDs := viewBlobIDs(v)
	p.Views = append(p.Views[:idx], p.Views[idx+1:]...)
	p.UpdatedAt = time.Now().UTC()
	return blobIDs, nil
}

// SetInventory overwrites a view's cached object inventory text.
func (s *MemoryStore) SetInventory(pid, vid, inventory string) (*View, error) {
	var updated *View
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		v.Inventory = inventory
		updated = v.clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// --- Masks ---------------------------------------------------------------

// CreateMask adds a new (empty) mask to a view.
func (s *MemoryStore) CreateMask(pid, vid string, assetID *string) (*Mask, error) {
	var created *Mask
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		m := &Mask{ID: newID(), AssetID: clonePtr(assetID)}
		v.Masks = append(v.Masks, m)
		c := *m
		c.AssetID = clonePtr(m.AssetID)
		created = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func findMask(v *View, mid string) *Mask {
	for _, m := range v.Masks {
		if m.ID == mid {
			return m
		}
	}
	return nil
}

// UpdateMask applies partial updates to a mask. Nil pointers leave the
// corresponding field unchanged.
func (s *MemoryStore) UpdateMask(pid, vid, mid string, assetID *string, assetIDSet bool, hidden *bool) (*Mask, error) {
	var updated *Mask
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		m := findMask(v, mid)
		if m == nil {
			return ErrNotFound
		}
		if assetIDSet {
			m.AssetID = clonePtr(assetID)
		}
		if hidden != nil {
			m.Hidden = *hidden
		}
		c := *m
		c.AssetID = clonePtr(m.AssetID)
		updated = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// SetMaskBitmap marks a mask as having a bitmap. The caller is responsible
// for storing the actual PNG blob (under the mask's own ID, via PutBlobAt).
func (s *MemoryStore) SetMaskBitmap(pid, vid, mid string) (*Mask, error) {
	var updated *Mask
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		m := findMask(v, mid)
		if m == nil {
			return ErrNotFound
		}
		m.HasBitmap = true
		c := *m
		c.AssetID = clonePtr(m.AssetID)
		updated = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteMask removes a mask from a view. The mask's bitmap blob (stored under
// the mask's own ID) is the caller's to delete.
func (s *MemoryStore) DeleteMask(pid, vid, mid string) error {
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		idx := -1
		for i, m := range v.Masks {
			if m.ID == mid {
				idx = i
				break
			}
		}
		if idx == -1 {
			return ErrNotFound
		}
		v.Masks = append(v.Masks[:idx], v.Masks[idx+1:]...)
		return nil
	})
	return err
}

// --- Renders ---------------------------------------------------------------

// AddRender appends a completed render to a view's history.
func (s *MemoryStore) AddRender(pid, vid string, r *Render) (*Render, error) {
	var created *Render
	_, err := s.withProject(pid, func(p *Project) error {
		v := findView(p, vid)
		if v == nil {
			return ErrNotFound
		}
		v.Renders = append(v.Renders, r)
		created = r.clone()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// FindRender looks up a render by ID across every view in the project.
func (s *MemoryStore) FindRender(pid, renderID string) (*Render, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.liveProject(pid)
	if err != nil {
		return nil, err
	}
	for _, v := range p.Views {
		for _, r := range v.Renders {
			if r.ID == renderID {
				return r.clone(), nil
			}
		}
	}
	return nil, ErrNotFound
}

// --- Render jobs -------------------------------------------------------------

// CreateRenderJob stores job (whose ID and Variations are already set by the
// caller) under the project. Returns ErrNotFound if pid doesn't exist or is
// soft-deleted.
func (s *MemoryStore) CreateRenderJob(pid string, job *RenderJob) (*RenderJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.liveProject(pid); err != nil {
		return nil, err
	}
	s.renderJobs[jobKey(pid, job.ID)] = job.clone()
	return job.clone(), nil
}

// GetRenderJob returns a deep copy of one render job.
func (s *MemoryStore) GetRenderJob(pid, jid string) (*RenderJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.liveProject(pid); err != nil {
		return nil, err
	}
	j, ok := s.renderJobs[jobKey(pid, jid)]
	if !ok {
		return nil, ErrNotFound
	}
	return j.clone(), nil
}

// ListRenderJobs returns deep copies of the project's jobs last updated at or
// after since, newest created first.
func (s *MemoryStore) ListRenderJobs(pid string, since time.Time) ([]*RenderJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.liveProject(pid); err != nil {
		return nil, err
	}
	var out []*RenderJob
	for key, j := range s.renderJobs {
		if strings.HasPrefix(key, pid+"/") && !j.UpdatedAt.Before(since) {
			out = append(out, j.clone())
		}
	}
	sortRenderJobsNewestFirst(out)
	return out, nil
}

// UpdateRenderJob runs fn against a private clone of the job under the
// store lock (so concurrent worker claims of different variations serialize
// cleanly), and only writes it back - bumping UpdatedAt - if fn returns
// nil. A failing fn therefore leaves the stored job untouched, mirroring
// FirestoreStore's transaction rollback on error.
func (s *MemoryStore) UpdateRenderJob(pid, jid string, fn func(j *RenderJob) error) (*RenderJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.liveProject(pid); err != nil {
		return nil, err
	}
	j, ok := s.renderJobs[jobKey(pid, jid)]
	if !ok {
		return nil, ErrNotFound
	}
	working := j.clone()
	if err := fn(working); err != nil {
		return nil, err
	}
	working.UpdatedAt = time.Now().UTC()
	s.renderJobs[jobKey(pid, jid)] = working
	return working.clone(), nil
}
