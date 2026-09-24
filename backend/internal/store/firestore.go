package store

import (
	"context"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FirestoreStore implements Repository over Cloud Firestore.
//
// Layout (image bytes live in a BlobStore, never here - docs hold blob IDs):
//
//	projects/{pid}                         core doc + inline assets[]
//	projects/{pid}/views/{vid}             one doc per view + inline masks[]
//	projects/{pid}/views/{vid}/renders/{rid}
//	projects/{pid}/renderJobs/{jid}        one doc per async render job
//
// The project doc denormalizes viewCount, renderCount and a thumbnail blob ID
// so ListProjects is a single indexed query with no subcollection reads. A
// render's IsStyleAnchor is derived at read time from the project's
// styleAnchorRenderId, so setting the anchor touches only the project doc.
type FirestoreStore struct {
	client *firestore.Client
}

var _ Repository = (*FirestoreStore)(nil)

// NewFirestore builds a Firestore-backed repository. projectID is the GCP
// project that owns the Firestore database; empty means auto-detect from the
// runtime credentials / metadata server (the ambient Cloud Run project).
// database is the Firestore database id; empty means the project default.
func NewFirestore(ctx context.Context, projectID, database string) (*FirestoreStore, error) {
	if projectID == "" {
		projectID = firestore.DetectProjectID
	}
	var (
		client *firestore.Client
		err    error
	)
	if database == "" {
		client, err = firestore.NewClient(ctx, projectID)
	} else {
		client, err = firestore.NewClientWithDatabase(ctx, projectID, database)
	}
	if err != nil {
		return nil, err
	}
	return &FirestoreStore{client: client}, nil
}

func (f *FirestoreStore) projects() *firestore.CollectionRef {
	return f.client.Collection("projects")
}

// --- Firestore document shapes ---------------------------------------------

type projectDoc struct {
	OwnerID             string        `firestore:"ownerId"`
	OrgID               *string       `firestore:"orgId"`
	Name                string        `firestore:"name"`
	CreatedAt           time.Time     `firestore:"createdAt"`
	UpdatedAt           time.Time     `firestore:"updatedAt"`
	DeletedAt           *time.Time    `firestore:"deletedAt,omitempty"`
	Style               StyleSettings `firestore:"style"`
	Assets              []*Asset      `firestore:"assets"`
	StyleAnchorRenderID *string       `firestore:"styleAnchorRenderId"`
	ViewCount           int           `firestore:"viewCount"`
	RenderCount         int           `firestore:"renderCount"`
	ThumbnailImageID    string        `firestore:"thumbnailImageId"`
}

type viewDoc struct {
	Name              string    `firestore:"name"`
	ScreenshotImageID string    `firestore:"screenshotImageId"`
	HasScreenshot     bool      `firestore:"hasScreenshot"`
	Width             int       `firestore:"width"`
	Height            int       `firestore:"height"`
	Masks             []*Mask   `firestore:"masks"`
	Inventory         string    `firestore:"inventory"`
	CreatedAt         time.Time `firestore:"createdAt"`
}

func (pd *projectDoc) toProject(id string) *Project {
	assets := pd.Assets
	if assets == nil {
		assets = []*Asset{}
	}
	return &Project{
		ID:                  id,
		OwnerID:             pd.OwnerID,
		OrgID:               clonePtr(pd.OrgID),
		Name:                pd.Name,
		CreatedAt:           pd.CreatedAt,
		UpdatedAt:           pd.UpdatedAt,
		DeletedAt:           clonePtr(pd.DeletedAt),
		Style:               pd.Style,
		Assets:              assets,
		Views:               []*View{},
		StyleAnchorRenderID: clonePtr(pd.StyleAnchorRenderID),
	}
}

func (vd *viewDoc) toView(id string) *View {
	masks := vd.Masks
	if masks == nil {
		masks = []*Mask{}
	}
	return &View{
		ID:                id,
		Name:              vd.Name,
		ScreenshotImageID: vd.ScreenshotImageID,
		HasScreenshot:     vd.HasScreenshot,
		Width:             vd.Width,
		Height:            vd.Height,
		Masks:             masks,
		Inventory:         vd.Inventory,
		Renders:           []*Render{},
	}
}

func isNotFound(err error) bool { return status.Code(err) == codes.NotFound }

// projectDocFromSnap decodes a project document snapshot, treating a
// soft-deleted project (DeletedAt set) as not found. Every method below that
// loads the project's core doc goes through this single helper - whether
// inside a transaction or not - so a deleted project is consistently
// unreachable, even by a direct call that bypasses the API's ownership gate.
func projectDocFromSnap(snap *firestore.DocumentSnapshot) (projectDoc, error) {
	var pd projectDoc
	if err := snap.DataTo(&pd); err != nil {
		return pd, err
	}
	if pd.DeletedAt != nil {
		return pd, ErrNotFound
	}
	return pd, nil
}

// --- Projects ----------------------------------------------------------------

func (f *FirestoreStore) CreateProject(ownerID, name string) *Project {
	ctx := context.Background()
	now := time.Now().UTC()
	ref := f.projects().NewDoc()
	pd := projectDoc{
		OwnerID:   ownerID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Style:     defaultStyle(),
		Assets:    []*Asset{},
	}
	if _, err := ref.Set(ctx, &pd); err != nil {
		// CreateProject has no error return (mirroring MemoryStore); a failure
		// here is logged and surfaces to the caller as a project that fails to
		// load on the next request.
		logStoreErr("firestore: creating project %s: %v", ref.ID, err)
	}
	return pd.toProject(ref.ID)
}

// ListProjects queries by ownerId only (no orderBy - results are sorted in Go
// by sortSummariesNewestFirst, so this needs no composite index) and filters
// out soft-deleted projects here rather than adding a "deletedAt == null"
// clause to the query, which would need its own composite index alongside
// ownerId; skipping them in Go keeps this a single simple-index query.
func (f *FirestoreStore) ListProjects(ownerID string) ([]ProjectSummary, error) {
	ctx := context.Background()
	snaps, err := f.projects().Where("ownerId", "==", ownerID).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]ProjectSummary, 0, len(snaps))
	for _, snap := range snaps {
		var pd projectDoc
		if err := snap.DataTo(&pd); err != nil {
			return nil, err
		}
		if pd.DeletedAt != nil {
			continue
		}
		out = append(out, ProjectSummary{
			ID:               snap.Ref.ID,
			Name:             pd.Name,
			CreatedAt:        pd.CreatedAt,
			UpdatedAt:        pd.UpdatedAt,
			ViewCount:        pd.ViewCount,
			RenderCount:      pd.RenderCount,
			ThumbnailImageID: pd.ThumbnailImageID,
		})
	}
	sortSummariesNewestFirst(out)
	return out, nil
}

func (f *FirestoreStore) ProjectOwner(pid string) (string, error) {
	ctx := context.Background()
	snap, err := f.projects().Doc(pid).Get(ctx)
	if isNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	pd, err := projectDocFromSnap(snap)
	if err != nil {
		return "", err
	}
	return pd.OwnerID, nil
}

func (f *FirestoreStore) GetProject(pid string) (*Project, error) {
	return f.getProjectFull(context.Background(), pid)
}

// getProjectFull assembles a Project from its core doc, its views subcollection
// and each view's renders subcollection, ordering views and renders by
// creation time (sorted in Go, so no composite Firestore index is required).
func (f *FirestoreStore) getProjectFull(ctx context.Context, pid string) (*Project, error) {
	pref := f.projects().Doc(pid)
	psnap, err := pref.Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	pd, err := projectDocFromSnap(psnap)
	if err != nil {
		return nil, err
	}
	p := pd.toProject(pid)

	vsnaps, err := pref.Collection("views").Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	type viewWithTime struct {
		v  *View
		at time.Time
	}
	views := make([]viewWithTime, 0, len(vsnaps))
	for _, vs := range vsnaps {
		var vd viewDoc
		if err := vs.DataTo(&vd); err != nil {
			return nil, err
		}
		v := vd.toView(vs.Ref.ID)

		rsnaps, err := vs.Ref.Collection("renders").Documents(ctx).GetAll()
		if err != nil {
			return nil, err
		}
		for _, rs := range rsnaps {
			var r Render
			if err := rs.DataTo(&r); err != nil {
				return nil, err
			}
			r.ID = rs.Ref.ID
			r.IsStyleAnchor = p.StyleAnchorRenderID != nil && *p.StyleAnchorRenderID == r.ID
			v.Renders = append(v.Renders, &r)
		}
		sort.SliceStable(v.Renders, func(i, j int) bool {
			return v.Renders[i].CreatedAt.Before(v.Renders[j].CreatedAt)
		})
		views = append(views, viewWithTime{v: v, at: vd.CreatedAt})
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].at.Before(views[j].at) })
	for _, vw := range views {
		p.Views = append(p.Views, vw.v)
	}
	return p, nil
}

// mutateProject runs fn against the project's core doc inside a transaction,
// bumps updatedAt, and writes it back. Returns ErrNotFound if the project is
// absent.
func (f *FirestoreStore) mutateProject(ctx context.Context, pid string, fn func(pd *projectDoc) error) error {
	ref := f.projects().Doc(pid)
	return f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		pd, err := projectDocFromSnap(snap)
		if err != nil {
			return err
		}
		if err := fn(&pd); err != nil {
			return err
		}
		pd.UpdatedAt = time.Now().UTC()
		return tx.Set(ref, &pd)
	})
}

// DeleteProject soft-deletes a project by setting deletedAt on its core doc.
// Nothing else is touched - views, renders and their blobs are left in place
// so the project can be restored - but projectDocFromSnap then makes every
// other method treat it as gone. Returns ErrNotFound if the project doesn't
// exist or is already deleted.
//
// TODO: a scheduled purge job should hard-delete projects whose deletedAt is
// more than 30 days old - the project doc, its views/renders subcollections,
// and their blobs from the BlobStore. Not implemented here; see
// PERSISTENCE_HANDOFF.md.
func (f *FirestoreStore) DeleteProject(pid string) error {
	return f.mutateProject(context.Background(), pid, func(pd *projectDoc) error {
		now := time.Now().UTC()
		pd.DeletedAt = &now
		return nil
	})
}

func (f *FirestoreStore) UpdateStyle(pid string, style StyleSettings) (*Project, error) {
	ctx := context.Background()
	if err := f.mutateProject(ctx, pid, func(pd *projectDoc) error {
		pd.Style = style
		return nil
	}); err != nil {
		return nil, err
	}
	return f.getProjectFull(ctx, pid)
}

func (f *FirestoreStore) SetAnchor(pid string, renderID *string) (*Project, error) {
	ctx := context.Background()
	if renderID != nil {
		full, err := f.getProjectFull(ctx, pid)
		if err != nil {
			return nil, err
		}
		if !renderExists(full, *renderID) {
			return nil, ErrNotFound
		}
	}
	if err := f.mutateProject(ctx, pid, func(pd *projectDoc) error {
		pd.StyleAnchorRenderID = clonePtr(renderID)
		return nil
	}); err != nil {
		return nil, err
	}
	return f.getProjectFull(ctx, pid)
}

func renderExists(p *Project, renderID string) bool {
	for _, v := range p.Views {
		for _, r := range v.Renders {
			if r.ID == renderID {
				return true
			}
		}
	}
	return false
}

// --- Assets ------------------------------------------------------------------

func (f *FirestoreStore) CreateAsset(pid, name, description, color string) (*Asset, error) {
	a := &Asset{ID: newID(), Name: name, Description: description, Color: color}
	if err := f.mutateProject(context.Background(), pid, func(pd *projectDoc) error {
		pd.Assets = append(pd.Assets, a)
		return nil
	}); err != nil {
		return nil, err
	}
	c := *a
	return &c, nil
}

func (f *FirestoreStore) UpdateAsset(pid, aid, name, description, color string) (*Asset, error) {
	var updated *Asset
	if err := f.mutateProject(context.Background(), pid, func(pd *projectDoc) error {
		for _, a := range pd.Assets {
			if a.ID == aid {
				a.Name, a.Description, a.Color = name, description, color
				c := *a
				updated = &c
				return nil
			}
		}
		return ErrNotFound
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

func (f *FirestoreStore) DeleteAsset(pid, aid string) error {
	ctx := context.Background()
	if err := f.mutateProject(ctx, pid, func(pd *projectDoc) error {
		idx := -1
		for i, a := range pd.Assets {
			if a.ID == aid {
				idx = i
				break
			}
		}
		if idx == -1 {
			return ErrNotFound
		}
		pd.Assets = append(pd.Assets[:idx], pd.Assets[idx+1:]...)
		return nil
	}); err != nil {
		return err
	}
	// Un-assign the deleted asset from any masks referencing it. Masks live on
	// view docs (a separate collection), so this is a follow-up pass, done
	// per-view so each write stays a single-document update.
	return f.clearAssetFromMasks(ctx, pid, aid)
}

// clearAssetFromMasks nulls out AssetID on every mask (across all views) that
// referenced the given asset. Each view is updated in its own transaction; a
// view with no matching mask is left untouched.
func (f *FirestoreStore) clearAssetFromMasks(ctx context.Context, pid, aid string) error {
	pref := f.projects().Doc(pid)
	vsnaps, err := pref.Collection("views").Documents(ctx).GetAll()
	if err != nil {
		return err
	}
	for _, vs := range vsnaps {
		var vd viewDoc
		if err := vs.DataTo(&vd); err != nil {
			return err
		}
		affected := false
		for _, m := range vd.Masks {
			if m.AssetID != nil && *m.AssetID == aid {
				affected = true
				break
			}
		}
		if !affected {
			continue
		}
		if err := f.mutateView(ctx, pid, vs.Ref.ID, func(vd *viewDoc) error {
			for _, m := range vd.Masks {
				if m.AssetID != nil && *m.AssetID == aid {
					m.AssetID = nil
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (f *FirestoreStore) SetAssetReference(pid, aid, imageID string) (*Asset, error) {
	var updated *Asset
	if err := f.mutateProject(context.Background(), pid, func(pd *projectDoc) error {
		for _, a := range pd.Assets {
			if a.ID == aid {
				a.ReferenceImageID = imageID
				a.HasReferenceImage = true
				c := *a
				updated = &c
				return nil
			}
		}
		return ErrNotFound
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

// --- Views -------------------------------------------------------------------

func (f *FirestoreStore) CreateView(pid, name, screenshotImageID string, width, height int) (*View, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	vref := pref.Collection("views").NewDoc()
	now := time.Now().UTC()
	vd := viewDoc{
		Name:              name,
		ScreenshotImageID: screenshotImageID,
		HasScreenshot:     true,
		Width:             width,
		Height:            height,
		Masks:             []*Mask{},
		CreatedAt:         now,
	}
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		psnap, err := tx.Get(pref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		pd, err := projectDocFromSnap(psnap)
		if err != nil {
			return err
		}
		if err := tx.Set(vref, &vd); err != nil {
			return err
		}
		pd.ViewCount++
		if pd.ThumbnailImageID == "" {
			pd.ThumbnailImageID = screenshotImageID
		}
		pd.UpdatedAt = now
		return tx.Set(pref, &pd)
	})
	if err != nil {
		return nil, err
	}
	return vd.toView(vref.ID), nil
}

func (f *FirestoreStore) GetView(pid, vid string) (*View, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	psnap, err := pref.Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	pd, err := projectDocFromSnap(psnap)
	if err != nil {
		return nil, err
	}

	vref := pref.Collection("views").Doc(vid)
	vsnap, err := vref.Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var vd viewDoc
	if err := vsnap.DataTo(&vd); err != nil {
		return nil, err
	}
	v := vd.toView(vid)

	rsnaps, err := vref.Collection("renders").Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	for _, rs := range rsnaps {
		var r Render
		if err := rs.DataTo(&r); err != nil {
			return nil, err
		}
		r.ID = rs.Ref.ID
		r.IsStyleAnchor = pd.StyleAnchorRenderID != nil && *pd.StyleAnchorRenderID == r.ID
		v.Renders = append(v.Renders, &r)
	}
	sort.SliceStable(v.Renders, func(i, j int) bool {
		return v.Renders[i].CreatedAt.Before(v.Renders[j].CreatedAt)
	})
	return v, nil
}

func (f *FirestoreStore) DeleteView(pid, vid string) ([]string, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	vref := pref.Collection("views").Doc(vid)
	var blobIDs []string
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		blobIDs = nil // reset on transaction retry
		psnap, err := tx.Get(pref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		pd, err := projectDocFromSnap(psnap)
		if err != nil {
			return err
		}
		vsnap, err := tx.Get(vref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var vd viewDoc
		if err := vsnap.DataTo(&vd); err != nil {
			return err
		}
		// Reads first (transaction rule): the view's renders, and the sibling
		// views (to re-pick a thumbnail if this view supplied it).
		rsnaps, err := tx.Documents(vref.Collection("renders")).GetAll()
		if err != nil {
			return err
		}
		var siblingSnaps []*firestore.DocumentSnapshot
		if pd.ThumbnailImageID == vd.ScreenshotImageID {
			siblingSnaps, err = tx.Documents(pref.Collection("views")).GetAll()
			if err != nil {
				return err
			}
		}

		// Gather orphaned blob IDs: screenshot, mask bitmaps, render results.
		if vd.ScreenshotImageID != "" {
			blobIDs = append(blobIDs, vd.ScreenshotImageID)
		}
		for _, m := range vd.Masks {
			if m.HasBitmap {
				blobIDs = append(blobIDs, m.ID)
			}
		}
		for _, rs := range rsnaps {
			var r Render
			if err := rs.DataTo(&r); err != nil {
				return err
			}
			if r.ResultImageID != "" {
				blobIDs = append(blobIDs, r.ResultImageID)
			}
		}

		// Writes.
		for _, rs := range rsnaps {
			if err := tx.Delete(rs.Ref); err != nil {
				return err
			}
		}
		if err := tx.Delete(vref); err != nil {
			return err
		}
		pd.ViewCount--
		pd.RenderCount -= len(rsnaps)
		if pd.ThumbnailImageID == vd.ScreenshotImageID {
			pd.ThumbnailImageID = ""
			for _, ss := range siblingSnaps {
				if ss.Ref.ID == vid {
					continue
				}
				var sib viewDoc
				if err := ss.DataTo(&sib); err != nil {
					return err
				}
				if sib.HasScreenshot {
					pd.ThumbnailImageID = sib.ScreenshotImageID
					break
				}
			}
		}
		pd.UpdatedAt = time.Now().UTC()
		return tx.Set(pref, &pd)
	})
	if err != nil {
		return nil, err
	}
	return blobIDs, nil
}

func (f *FirestoreStore) SetInventory(pid, vid, inventory string) (*View, error) {
	if err := f.mutateView(context.Background(), pid, vid, func(vd *viewDoc) error {
		vd.Inventory = inventory
		return nil
	}); err != nil {
		return nil, err
	}
	return f.GetView(pid, vid)
}

// mutateView runs fn against a view doc inside a transaction and bumps the
// parent project's updatedAt. Returns ErrNotFound if the view is absent.
func (f *FirestoreStore) mutateView(ctx context.Context, pid, vid string, fn func(vd *viewDoc) error) error {
	pref := f.projects().Doc(pid)
	vref := pref.Collection("views").Doc(vid)
	return f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		vsnap, err := tx.Get(vref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var vd viewDoc
		if err := vsnap.DataTo(&vd); err != nil {
			return err
		}
		if err := fn(&vd); err != nil {
			return err
		}
		if err := tx.Set(vref, &vd); err != nil {
			return err
		}
		return tx.Update(pref, []firestore.Update{{Path: "updatedAt", Value: time.Now().UTC()}})
	})
}

// --- Masks -------------------------------------------------------------------

func (f *FirestoreStore) CreateMask(pid, vid string, assetID *string) (*Mask, error) {
	m := &Mask{ID: newID(), AssetID: clonePtr(assetID)}
	if err := f.mutateView(context.Background(), pid, vid, func(vd *viewDoc) error {
		vd.Masks = append(vd.Masks, m)
		return nil
	}); err != nil {
		return nil, err
	}
	c := *m
	c.AssetID = clonePtr(m.AssetID)
	return &c, nil
}

func (f *FirestoreStore) UpdateMask(pid, vid, mid string, assetID *string, assetIDSet bool, hidden *bool) (*Mask, error) {
	var updated *Mask
	if err := f.mutateView(context.Background(), pid, vid, func(vd *viewDoc) error {
		for _, m := range vd.Masks {
			if m.ID == mid {
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
			}
		}
		return ErrNotFound
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

func (f *FirestoreStore) SetMaskBitmap(pid, vid, mid string) (*Mask, error) {
	var updated *Mask
	if err := f.mutateView(context.Background(), pid, vid, func(vd *viewDoc) error {
		for _, m := range vd.Masks {
			if m.ID == mid {
				m.HasBitmap = true
				c := *m
				c.AssetID = clonePtr(m.AssetID)
				updated = &c
				return nil
			}
		}
		return ErrNotFound
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

func (f *FirestoreStore) DeleteMask(pid, vid, mid string) error {
	return f.mutateView(context.Background(), pid, vid, func(vd *viewDoc) error {
		idx := -1
		for i, m := range vd.Masks {
			if m.ID == mid {
				idx = i
				break
			}
		}
		if idx == -1 {
			return ErrNotFound
		}
		vd.Masks = append(vd.Masks[:idx], vd.Masks[idx+1:]...)
		return nil
	})
}

// --- Renders -----------------------------------------------------------------

func (f *FirestoreStore) AddRender(pid, vid string, r *Render) (*Render, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	vref := pref.Collection("views").Doc(vid)
	rref := vref.Collection("renders").Doc(r.ID)
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		psnap, err := tx.Get(pref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		pd, err := projectDocFromSnap(psnap)
		if err != nil {
			return err
		}
		if _, err := tx.Get(vref); err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Set(rref, r); err != nil {
			return err
		}
		pd.RenderCount++
		pd.UpdatedAt = time.Now().UTC()
		return tx.Set(pref, &pd)
	})
	if err != nil {
		return nil, err
	}
	return r.clone(), nil
}

func (f *FirestoreStore) FindRender(pid, renderID string) (*Render, error) {
	full, err := f.getProjectFull(context.Background(), pid)
	if err != nil {
		return nil, err
	}
	for _, v := range full.Views {
		for _, r := range v.Renders {
			if r.ID == renderID {
				return r, nil
			}
		}
	}
	return nil, ErrNotFound
}

// --- Render jobs -------------------------------------------------------------

type renderJobDoc struct {
	ViewID     string               `firestore:"viewId"`
	CreatedAt  time.Time            `firestore:"createdAt"`
	UpdatedAt  time.Time            `firestore:"updatedAt"`
	Request    RenderJobRequest     `firestore:"request"`
	Variations []RenderJobVariation `firestore:"variations"`
}

func renderJobDocFrom(j *RenderJob) renderJobDoc {
	return renderJobDoc{
		ViewID:     j.ViewID,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
		Request:    j.Request,
		Variations: j.Variations,
	}
}

func (jd *renderJobDoc) toRenderJob(id string) *RenderJob {
	return (&RenderJob{
		ID:         id,
		ViewID:     jd.ViewID,
		CreatedAt:  jd.CreatedAt,
		UpdatedAt:  jd.UpdatedAt,
		Request:    jd.Request,
		Variations: jd.Variations,
	}).clone()
}

func (f *FirestoreStore) renderJobs(pid string) *firestore.CollectionRef {
	return f.projects().Doc(pid).Collection("renderJobs")
}

// projectAlive checks the project's core doc exists and is not soft-deleted,
// without loading its views/renders - the cheap existence check every
// render-job method needs before touching the renderJobs subcollection.
func (f *FirestoreStore) projectAlive(ctx context.Context, pref *firestore.DocumentRef) error {
	psnap, err := pref.Get(ctx)
	if isNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = projectDocFromSnap(psnap)
	return err
}

// CreateRenderJob stores job (whose ID and Variations are already set by the
// caller) under the project.
func (f *FirestoreStore) CreateRenderJob(pid string, job *RenderJob) (*RenderJob, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	jref := f.renderJobs(pid).Doc(job.ID)
	doc := renderJobDocFrom(job)
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := f.projectAliveTx(tx, pref); err != nil {
			return err
		}
		return tx.Set(jref, &doc)
	})
	if err != nil {
		return nil, err
	}
	return doc.toRenderJob(job.ID), nil
}

// projectAliveTx is projectAlive's transaction-bound counterpart (tx.Get
// reads must go through the transaction, not the client, inside
// RunTransaction).
func (f *FirestoreStore) projectAliveTx(tx *firestore.Transaction, pref *firestore.DocumentRef) error {
	psnap, err := tx.Get(pref)
	if isNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = projectDocFromSnap(psnap)
	return err
}

// GetRenderJob returns one render job.
func (f *FirestoreStore) GetRenderJob(pid, jid string) (*RenderJob, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	if err := f.projectAlive(ctx, pref); err != nil {
		return nil, err
	}
	jsnap, err := f.renderJobs(pid).Doc(jid).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var jd renderJobDoc
	if err := jsnap.DataTo(&jd); err != nil {
		return nil, err
	}
	return jd.toRenderJob(jid), nil
}

// UpdateRenderJob runs fn against the job inside a Firestore transaction and
// writes back the result. Returns ErrNotFound if the project or job doesn't
// exist.
func (f *FirestoreStore) UpdateRenderJob(pid, jid string, fn func(j *RenderJob) error) (*RenderJob, error) {
	ctx := context.Background()
	pref := f.projects().Doc(pid)
	jref := f.renderJobs(pid).Doc(jid)
	var result *RenderJob
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := f.projectAliveTx(tx, pref); err != nil {
			return err
		}
		jsnap, err := tx.Get(jref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var jd renderJobDoc
		if err := jsnap.DataTo(&jd); err != nil {
			return err
		}
		job := jd.toRenderJob(jid)
		if err := fn(job); err != nil {
			return err
		}
		job.UpdatedAt = time.Now().UTC()
		result = job
		newDoc := renderJobDocFrom(job)
		return tx.Set(jref, &newDoc)
	})
	if err != nil {
		return nil, err
	}
	return result.clone(), nil
}
