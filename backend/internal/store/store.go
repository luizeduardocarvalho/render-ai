package store

import (
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
	mu       sync.Mutex
	projects map[string]*Project
	blobs    map[string]Blob
}

// Compile-time checks that MemoryStore satisfies both storage interfaces.
var (
	_ Repository = (*MemoryStore)(nil)
	_ BlobStore  = (*MemoryStore)(nil)
)

// NewMemory creates an empty in-memory store.
func NewMemory() *MemoryStore {
	return &MemoryStore{
		projects: make(map[string]*Project),
		blobs:    make(map[string]Blob),
	}
}

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
// ownerID, newest first.
func (s *MemoryStore) ListProjects(ownerID string) ([]ProjectSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProjectSummary, 0)
	for _, p := range s.projects {
		if p.OwnerID != ownerID {
			continue
		}
		out = append(out, summarize(p))
	}
	sortSummariesNewestFirst(out)
	return out, nil
}

// ProjectOwner returns the owner id of a project, or ErrNotFound.
func (s *MemoryStore) ProjectOwner(pid string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return "", ErrNotFound
	}
	return p.OwnerID, nil
}

// GetProject returns a deep copy of the project.
func (s *MemoryStore) GetProject(pid string) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return nil, ErrNotFound
	}
	return p.clone(), nil
}

// withProject runs fn against the live project under the store lock, bumps its
// UpdatedAt, then returns a deep copy of the (possibly mutated) project.
func (s *MemoryStore) withProject(pid string, fn func(p *Project) error) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return nil, ErrNotFound
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	p.UpdatedAt = time.Now().UTC()
	return p.clone(), nil
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

// --- Assets ------------------------------------------------------------------

// CreateAsset adds a new library asset to the project.
func (s *MemoryStore) CreateAsset(pid, name, description, color string) (*Asset, error) {
	var created *Asset
	_, err := s.withProject(pid, func(p *Project) error {
		a := &Asset{ID: newID(), Name: name, Description: description, Color: color}
		p.Assets = append(p.Assets, a)
		c := *a
		created = &c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateAsset updates an existing asset's name, description and color.
func (s *MemoryStore) UpdateAsset(pid, aid, name, description, color string) (*Asset, error) {
	var updated *Asset
	_, err := s.withProject(pid, func(p *Project) error {
		for _, a := range p.Assets {
			if a.ID == aid {
				a.Name = name
				a.Description = description
				a.Color = color
				c := *a
				updated = &c
				return nil
			}
		}
		return ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteAsset removes an asset from the project, and un-assigns it from any
// masks that referenced it (across all views).
func (s *MemoryStore) DeleteAsset(pid, aid string) error {
	_, err := s.withProject(pid, func(p *Project) error {
		idx := -1
		for i, a := range p.Assets {
			if a.ID == aid {
				idx = i
				break
			}
		}
		if idx == -1 {
			return ErrNotFound
		}
		p.Assets = append(p.Assets[:idx], p.Assets[idx+1:]...)
		for _, v := range p.Views {
			for _, m := range v.Masks {
				if m.AssetID != nil && *m.AssetID == aid {
					m.AssetID = nil
				}
			}
		}
		return nil
	})
	return err
}

// SetAssetReference attaches a reference photo blob to an asset.
func (s *MemoryStore) SetAssetReference(pid, aid, imageID string) (*Asset, error) {
	var updated *Asset
	_, err := s.withProject(pid, func(p *Project) error {
		for _, a := range p.Assets {
			if a.ID == aid {
				a.ReferenceImageID = imageID
				a.HasReferenceImage = true
				c := *a
				updated = &c
				return nil
			}
		}
		return ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
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
	p, ok := s.projects[pid]
	if !ok {
		return nil, ErrNotFound
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
	p, ok := s.projects[pid]
	if !ok {
		return nil, ErrNotFound
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
