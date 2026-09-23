package store

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a project, view, mask, asset or render ID
// does not exist.
var ErrNotFound = errors.New("not found")

// Blob is a stored image (screenshot, reference photo, mask bitmap, or
// render result) with its content type.
type Blob struct {
	Data        []byte
	ContentType string
}

// Store is the whole in-memory database: projects plus their image blobs,
// guarded by a single mutex. It is safe for concurrent use.
type Store struct {
	mu       sync.Mutex
	projects map[string]*Project
	blobs    map[string]Blob
}

// New creates an empty Store.
func New() *Store {
	return &Store{
		projects: make(map[string]*Project),
		blobs:    make(map[string]Blob),
	}
}

func newID() string { return uuid.NewString() }

// --- Blobs -----------------------------------------------------------------

// PutBlob stores data under a new random ID and returns it.
func (s *Store) PutBlob(data []byte, contentType string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID()
	s.blobs[id] = Blob{Data: data, ContentType: contentType}
	return id
}

// PutBlobAt stores data under a caller-chosen ID, overwriting any existing
// blob there. Used for mask bitmaps, whose image ID is the mask's own ID.
func (s *Store) PutBlobAt(id string, data []byte, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs[id] = Blob{Data: data, ContentType: contentType}
}

// GetBlob fetches a stored blob by ID.
func (s *Store) GetBlob(id string) (Blob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.blobs[id]
	return b, ok
}

func (s *Store) deleteBlob(id string) {
	delete(s.blobs, id)
}

// --- Projects ----------------------------------------------------------------

// CreateProject creates a new project with default style settings.
func (s *Store) CreateProject(name string) *Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := &Project{
		ID:     newID(),
		Name:   name,
		Style:  defaultStyle(),
		Assets: []*Asset{},
		Views:  []*View{},
	}
	s.projects[p.ID] = p
	return p.clone()
}

// GetProject returns a deep copy of the project.
func (s *Store) GetProject(pid string) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return nil, ErrNotFound
	}
	return p.clone(), nil
}

// withProject runs fn against the live project under the store lock, then
// returns a deep copy of the (possibly mutated) project.
func (s *Store) withProject(pid string, fn func(p *Project) error) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return nil, ErrNotFound
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	return p.clone(), nil
}

// UpdateStyle replaces the project's style settings.
func (s *Store) UpdateStyle(pid string, style StyleSettings) (*Project, error) {
	return s.withProject(pid, func(p *Project) error {
		p.Style = style
		return nil
	})
}

// SetAnchor sets or clears the project's style-anchor render. renderID may be
// nil to clear it. It also keeps each render's IsStyleAnchor flag in sync.
func (s *Store) SetAnchor(pid string, renderID *string) (*Project, error) {
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
func (s *Store) CreateAsset(pid, name, description, color string) (*Asset, error) {
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
func (s *Store) UpdateAsset(pid, aid, name, description, color string) (*Asset, error) {
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
func (s *Store) DeleteAsset(pid, aid string) error {
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
func (s *Store) SetAssetReference(pid, aid, imageID string) (*Asset, error) {
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
func (s *Store) CreateView(pid, name, screenshotImageID string, width, height int) (*View, error) {
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
func (s *Store) GetView(pid, vid string) (*View, error) {
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

// DeleteView removes a view and its blobs (screenshot, mask bitmaps) from
// the project.
func (s *Store) DeleteView(pid, vid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return ErrNotFound
	}
	idx := -1
	for i, v := range p.Views {
		if v.ID == vid {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNotFound
	}
	v := p.Views[idx]
	s.deleteBlob(v.ScreenshotImageID)
	for _, m := range v.Masks {
		s.deleteBlob(m.ID)
	}
	for _, r := range v.Renders {
		s.deleteBlob(r.ResultImageID)
	}
	p.Views = append(p.Views[:idx], p.Views[idx+1:]...)
	return nil
}

// SetInventory overwrites a view's cached object inventory text.
func (s *Store) SetInventory(pid, vid, inventory string) (*View, error) {
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
func (s *Store) CreateMask(pid, vid string, assetID *string) (*Mask, error) {
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
func (s *Store) UpdateMask(pid, vid, mid string, assetID *string, assetIDSet bool, hidden *bool) (*Mask, error) {
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
func (s *Store) SetMaskBitmap(pid, vid, mid string) (*Mask, error) {
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

// DeleteMask removes a mask (and its bitmap blob, if any) from a view.
func (s *Store) DeleteMask(pid, vid, mid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[pid]
	if !ok {
		return ErrNotFound
	}
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
	s.deleteBlob(mid)
	v.Masks = append(v.Masks[:idx], v.Masks[idx+1:]...)
	return nil
}

// --- Renders ---------------------------------------------------------------

// AddRender appends a completed render to a view's history.
func (s *Store) AddRender(pid, vid string, r *Render) (*Render, error) {
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
func (s *Store) FindRender(pid, renderID string) (*Render, error) {
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
