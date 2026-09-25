package api

import (
	"testing"

	"render-ai/backend/internal/store"
)

// Only a visible, painted mask bound to an existing asset that has a
// reference photo may steer a render. Every other mask is skipped so the
// region map never shows a colored area the prompt cannot explain.
func TestQualifyingMasksSkipsUnusableMasks(t *testing.T) {
	ptr := func(s string) *string { return &s }

	project := &store.Project{Assets: []*store.Asset{
		{ID: "with-photo", Name: "Sofa", Color: "#ff0000", ReferenceImageID: "ref-1", HasReferenceImage: true},
		{ID: "no-photo", Name: "Lamp", Color: "#00ff00"},
	}}
	view := &store.View{Masks: []*store.Mask{
		{ID: "ok", AssetID: ptr("with-photo"), HasBitmap: true},
		{ID: "no-asset", AssetID: nil, HasBitmap: true},
		{ID: "asset-deleted", AssetID: ptr("gone"), HasBitmap: true},
		{ID: "asset-without-photo", AssetID: ptr("no-photo"), HasBitmap: true},
		{ID: "hidden", AssetID: ptr("with-photo"), HasBitmap: true, Hidden: true},
		{ID: "not-painted", AssetID: ptr("with-photo"), HasBitmap: false},
	}}

	got := qualifyingMasks(project, view)
	if len(got) != 1 || got[0].mask.ID != "ok" {
		ids := make([]string, len(got))
		for i, q := range got {
			ids[i] = q.mask.ID
		}
		t.Fatalf("qualifying masks = %v, want only [ok]", ids)
	}
}
