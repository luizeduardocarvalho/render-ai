package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/jobs"
	"render-ai/backend/internal/store"
)

func newAssetTestServer() (*Server, *store.MemoryStore) {
	st := store.NewMemory()
	return NewServer(st, st, nil, nil, &config.Config{}, "", jobs.NewInline()), st
}

func withCallerContext(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDCtxKey, userID))
}

// TestAssetLibraryCRUD covers the user-wide library routes end to end:
// create, newest-first listing, update and delete, all scoped to the caller
// - a different user's library is unaffected and can't reach another user's
// assets.
func TestAssetLibraryCRUD(t *testing.T) {
	s, _ := newAssetTestServer()

	create := func(userID, name string) store.Asset {
		r := withCallerContext(jsonRequest(t, assetRequest{Name: name, Description: "d", Color: "#ff0000"}), userID)
		rec := httptest.NewRecorder()
		if err := s.createLibraryAsset(rec, r); err != nil {
			t.Fatalf("createLibraryAsset: %v", err)
		}
		var a store.Asset
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("decoding asset: %v", err)
		}
		return a
	}

	a1 := create("user-a", "Sofa")
	a2 := create("user-a", "Lamp")

	list := func(userID string) []store.Asset {
		r := withCallerContext(httptest.NewRequest(http.MethodGet, "/api/assets", nil), userID)
		rec := httptest.NewRecorder()
		if err := s.listLibraryAssets(rec, r); err != nil {
			t.Fatalf("listLibraryAssets: %v", err)
		}
		var out []store.Asset
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding list: %v", err)
		}
		return out
	}

	got := list("user-a")
	if len(got) != 2 || got[0].ID != a2.ID || got[1].ID != a1.ID {
		t.Fatalf("list not newest-first: %+v", got)
	}
	if other := list("user-b"); len(other) != 0 {
		t.Fatalf("a different user's library was not empty: %+v", other)
	}

	upd := withCallerContext(jsonRequest(t, assetRequest{Name: "Sofa v2", Description: "d2", Color: "#00ff00"}), "user-a")
	upd.SetPathValue("aid", a1.ID)
	rec := httptest.NewRecorder()
	if err := s.updateLibraryAsset(rec, upd); err != nil {
		t.Fatalf("updateLibraryAsset: %v", err)
	}
	var updated store.Asset
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decoding updated asset: %v", err)
	}
	if updated.Name != "Sofa v2" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	// A different user can't update or delete it.
	otherUpd := withCallerContext(jsonRequest(t, assetRequest{Name: "hijacked"}), "user-b")
	otherUpd.SetPathValue("aid", a1.ID)
	if err := s.updateLibraryAsset(httptest.NewRecorder(), otherUpd); err == nil {
		t.Fatal("a different user updating another user's asset should fail")
	}
	otherDel := withCallerContext(httptest.NewRequest(http.MethodDelete, "/", nil), "user-b")
	otherDel.SetPathValue("aid", a1.ID)
	if err := s.deleteLibraryAsset(httptest.NewRecorder(), otherDel); err == nil {
		t.Fatal("a different user deleting another user's asset should fail")
	}

	del := withCallerContext(httptest.NewRequest(http.MethodDelete, "/", nil), "user-a")
	del.SetPathValue("aid", a1.ID)
	rec = httptest.NewRecorder()
	if err := s.deleteLibraryAsset(rec, del); err != nil {
		t.Fatalf("deleteLibraryAsset: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	if got := list("user-a"); len(got) != 1 || got[0].ID != a2.ID {
		t.Fatalf("library after delete: %+v, want just %s", got, a2.ID)
	}
}

// TestProjectAssetMigrationKeepsMaskBindings covers the lazy asset-library
// migration: a legacy project with an asset embedded directly in the doc
// (and a mask painted and bound to it) gets that asset moved into the
// owner's library under the SAME id the first time the project is loaded -
// and the mask binding, and render's resolution of it (qualifyingMasks),
// keep working afterwards.
func TestProjectAssetMigrationKeepsMaskBindings(t *testing.T) {
	s, st := newAssetTestServer()
	p := st.CreateProject("user-a", "P")
	view, err := st.CreateView(p.ID, "front", "shot-1", 10, 10)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	mask, err := st.CreateMask(p.ID, view.ID, nil)
	if err != nil {
		t.Fatalf("CreateMask: %v", err)
	}
	if _, err := st.SetMaskBitmap(p.ID, view.ID, mask.ID); err != nil {
		t.Fatalf("SetMaskBitmap: %v", err)
	}

	const legacyAssetID = "legacy-asset-1"
	if err := st.SeedLegacyProjectAssets(p.ID, []*store.Asset{
		{ID: legacyAssetID, Name: "Sofa", Description: "d", Color: "#ff0000"},
	}); err != nil {
		t.Fatalf("SeedLegacyProjectAssets: %v", err)
	}
	legacyID := legacyAssetID
	if _, err := st.UpdateMask(p.ID, view.ID, mask.ID, &legacyID, true, nil); err != nil {
		t.Fatalf("UpdateMask: %v", err)
	}

	if lib, err := st.ListAssets("user-a"); err != nil || len(lib) != 0 {
		t.Fatalf("library before migration: %+v, %v", lib, err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("pid", p.ID)
	rec := httptest.NewRecorder()
	if err := s.getProject(rec, r); err != nil {
		t.Fatalf("getProject: %v", err)
	}
	var got store.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding project: %v", err)
	}
	if len(got.Assets) != 1 || got.Assets[0].ID != legacyAssetID {
		t.Fatalf("migrated project.assets = %+v, want the one legacy asset under the same id", got.Assets)
	}

	lib, err := st.ListAssets("user-a")
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(lib) != 1 || lib[0].ID != legacyAssetID {
		t.Fatalf("library after migration: %+v", lib)
	}

	// The project doc's own embedded copy is cleared - the migration only
	// needs to run once - but re-reading through the same handler must still
	// report the asset (now served from the library).
	rec = httptest.NewRecorder()
	if err := s.getProject(rec, r); err != nil {
		t.Fatalf("getProject (2nd time): %v", err)
	}
	var got2 store.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got2); err != nil {
		t.Fatalf("decoding project (2nd time): %v", err)
	}
	if len(got2.Assets) != 1 || got2.Assets[0].ID != legacyAssetID {
		t.Fatalf("project.assets after a second load = %+v, want unchanged", got2.Assets)
	}

	// Render's asset resolution must still find the asset through the
	// mask's unchanged AssetID.
	resolved, err := s.withLibraryAssets(st.GetProject(p.ID))
	if err != nil {
		t.Fatalf("withLibraryAssets: %v", err)
	}
	qualifying := qualifyingMasks(resolved, resolved.Views[0])
	if len(qualifying) != 1 || qualifying[0].asset.ID != legacyAssetID {
		t.Fatalf("qualifyingMasks after migration = %+v, want the mask still resolving to %s", qualifying, legacyAssetID)
	}
}
