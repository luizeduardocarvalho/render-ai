package store

import "testing"

// A Render clone must preserve the nil-vs-empty distinction of the preservation
// inventory slices. An empty (non-nil) slice must stay non-nil so the JSON
// response carries [] instead of null - the frontend reads .length on it.
func TestRenderClonePreservesEmptyInventorySlices(t *testing.T) {
	r := &Render{
		Preservation: &PreservationReport{
			Inventory: PreservationInventory{
				Removed: []string{},         // empty, non-nil
				Added:   []string{"a sofa"}, // populated
				Moved:   nil,                // nil stays nil
			},
		},
	}

	got := r.clone()

	if got.Preservation.Inventory.Removed == nil {
		t.Error("Removed: empty slice was collapsed to nil during clone")
	}
	if len(got.Preservation.Inventory.Removed) != 0 {
		t.Errorf("Removed: want length 0, got %d", len(got.Preservation.Inventory.Removed))
	}
	if len(got.Preservation.Inventory.Added) != 1 || got.Preservation.Inventory.Added[0] != "a sofa" {
		t.Errorf("Added: want [a sofa], got %v", got.Preservation.Inventory.Added)
	}
	if got.Preservation.Inventory.Moved != nil {
		t.Errorf("Moved: want nil, got %v", got.Preservation.Inventory.Moved)
	}

	// The clone must be independent of the original.
	got.Preservation.Inventory.Added[0] = "mutated"
	if r.Preservation.Inventory.Added[0] != "a sofa" {
		t.Error("clone shares backing array with the original")
	}
}
