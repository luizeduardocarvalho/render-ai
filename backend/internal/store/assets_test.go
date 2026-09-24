package store

import "testing"

// TestMemoryStoreAssetLibraryCRUD covers the user-wide asset library: create,
// newest-first listing, get, update, setting a reference and delete - all
// scoped per owner.
func TestMemoryStoreAssetLibraryCRUD(t *testing.T) {
	s := NewMemory()

	a1, err := s.CreateAsset("user-a", "Sofa", "a sofa", "#ff0000")
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if a1.CreatedAt.IsZero() {
		t.Fatal("CreateAsset did not set CreatedAt")
	}
	a2, err := s.CreateAsset("user-a", "Lamp", "a lamp", "#00ff00")
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}

	list, err := s.ListAssets("user-a")
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(list) != 2 || list[0].ID != a2.ID || list[1].ID != a1.ID {
		t.Fatalf("ListAssets not newest-first: %+v", list)
	}

	if other, err := s.ListAssets("user-b"); err != nil || len(other) != 0 {
		t.Fatalf("ListAssets for a different owner: %+v, %v", other, err)
	}

	got, err := s.GetAsset("user-a", a1.ID)
	if err != nil || got.Name != "Sofa" {
		t.Fatalf("GetAsset: %+v, %v", got, err)
	}
	if _, err := s.GetAsset("user-b", a1.ID); err != ErrNotFound {
		t.Fatalf("GetAsset with the wrong owner: want ErrNotFound, got %v", err)
	}

	updated, err := s.UpdateAsset("user-a", a1.ID, "Sofa v2", "a nicer sofa", "#0000ff")
	if err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	if updated.Name != "Sofa v2" || updated.Color != "#0000ff" {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if _, err := s.UpdateAsset("user-b", a1.ID, "x", "y", "z"); err != ErrNotFound {
		t.Fatalf("UpdateAsset with the wrong owner: want ErrNotFound, got %v", err)
	}

	withRef, err := s.SetAssetReference("user-a", a1.ID, "blob-1")
	if err != nil {
		t.Fatalf("SetAssetReference: %v", err)
	}
	if !withRef.HasReferenceImage || withRef.ReferenceImageID != "blob-1" {
		t.Fatalf("unexpected reference: %+v", withRef)
	}

	if err := s.DeleteAsset("user-a", a1.ID); err != nil {
		t.Fatalf("DeleteAsset: %v", err)
	}
	if _, err := s.GetAsset("user-a", a1.ID); err != ErrNotFound {
		t.Fatalf("GetAsset after delete: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteAsset("user-a", a1.ID); err != ErrNotFound {
		t.Fatalf("deleting an already-deleted asset: want ErrNotFound, got %v", err)
	}
	// The other asset in the library is untouched.
	if _, err := s.GetAsset("user-a", a2.ID); err != nil {
		t.Fatalf("GetAsset for the surviving asset: %v", err)
	}
}

// TestMemoryStoreListAssetsReturnsACopy confirms a caller mutating the
// returned slice/asset can't corrupt the store.
func TestMemoryStoreListAssetsReturnsACopy(t *testing.T) {
	s := NewMemory()
	a, err := s.CreateAsset("user-a", "Sofa", "a sofa", "#ff0000")
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	list, err := s.ListAssets("user-a")
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	list[0].Name = "mutated"
	again, err := s.GetAsset("user-a", a.ID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if again.Name != "Sofa" {
		t.Fatalf("ListAssets leaked a mutation: %+v", again)
	}
}

// TestMemoryStoreImportAssetsIsIdempotentAndKeepsIds covers the migration
// helper's contract: assets land in the library under their original ids,
// and importing the same (or overlapping) legacy data again never clobbers a
// library edit made since.
func TestMemoryStoreImportAssetsIsIdempotentAndKeepsIds(t *testing.T) {
	s := NewMemory()
	legacy := []*Asset{
		{ID: "asset-1", Name: "Sofa", Description: "a sofa", Color: "#ff0000"},
		{ID: "asset-2", Name: "Lamp", Description: "a lamp", Color: "#00ff00"},
	}

	if err := s.ImportAssets("user-a", legacy); err != nil {
		t.Fatalf("ImportAssets: %v", err)
	}
	list, err := s.ListAssets("user-a")
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d assets, want 2", len(list))
	}
	got, err := s.GetAsset("user-a", "asset-1")
	if err != nil || got.Name != "Sofa" {
		t.Fatalf("GetAsset asset-1: %+v, %v", got, err)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("ImportAssets did not backfill CreatedAt for a legacy asset with none")
	}

	if _, err := s.UpdateAsset("user-a", "asset-1", "Sofa (edited)", "a sofa", "#ff0000"); err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	if err := s.ImportAssets("user-a", legacy); err != nil {
		t.Fatalf("ImportAssets (2nd time): %v", err)
	}
	got, err = s.GetAsset("user-a", "asset-1")
	if err != nil {
		t.Fatalf("GetAsset asset-1 (after re-import): %v", err)
	}
	if got.Name != "Sofa (edited)" {
		t.Fatalf("re-importing clobbered a library edit: %+v", got)
	}
	list, err = s.ListAssets("user-a")
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("re-importing duplicated assets: got %d, want 2", len(list))
	}
}

// TestMemoryStoreClearProjectAssets covers the migration's other half:
// emptying the project doc's legacy embedded Assets slice.
func TestMemoryStoreClearProjectAssets(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	if err := s.SeedLegacyProjectAssets(p.ID, []*Asset{{ID: "asset-1", Name: "Sofa"}}); err != nil {
		t.Fatalf("SeedLegacyProjectAssets: %v", err)
	}
	got, err := s.GetProject(p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if len(got.Assets) != 1 {
		t.Fatalf("seed did not take: %+v", got.Assets)
	}

	if err := s.ClearProjectAssets(p.ID); err != nil {
		t.Fatalf("ClearProjectAssets: %v", err)
	}
	got, err = s.GetProject(p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if len(got.Assets) != 0 {
		t.Fatalf("Assets not cleared: %+v", got.Assets)
	}
}
