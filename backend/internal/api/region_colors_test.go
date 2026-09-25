package api

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/geometry"
	"render-ai/backend/internal/jobs"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

const regionTestW, regionTestH = 160, 90

func fillRect(img *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	draw.Draw(img, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

// regionFixture is a project with one gray view, ready for masks and assets.
type regionFixture struct {
	t    *testing.T
	repo *store.MemoryStore
	s    *Server
	pid  string
	view *store.View
}

func newRegionFixture(t *testing.T, scene func(*image.NRGBA)) *regionFixture {
	t.Helper()
	repo := store.NewMemory()
	p := repo.CreateProject("owner-1", "P")

	img := image.NewNRGBA(image.Rect(0, 0, regionTestW, regionTestH))
	fillRect(img, img.Bounds(), color.NRGBA{R: 180, G: 180, B: 180, A: 255})
	if scene != nil {
		scene(img)
	}
	shotID, err := repo.PutBlob(encodePNG(t, img), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	view, err := repo.CreateView(p.ID, "v", shotID, regionTestW, regionTestH)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(repo, repo, nil, nil, &config.Config{}, "../../prompts/render.tmpl", jobs.NewInline())
	return &regionFixture{t: t, repo: repo, s: s, pid: p.ID, view: view}
}

// addAsset creates a library asset; withPhoto uploads a reference image and
// missingBlob then deletes it so the asset still claims to have one.
func (f *regionFixture) addAsset(name, hex string, withPhoto, missingBlob bool) *store.Asset {
	f.t.Helper()
	a, err := f.repo.CreateAsset("owner-1", name, name+" description", hex)
	if err != nil {
		f.t.Fatal(err)
	}
	if withPhoto {
		ref := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		id, err := f.repo.PutBlob(encodePNG(f.t, ref), "image/png")
		if err != nil {
			f.t.Fatal(err)
		}
		if a, err = f.repo.SetAssetReference("owner-1", a.ID, id); err != nil {
			f.t.Fatal(err)
		}
		if missingBlob {
			f.repo.DeleteBlob(id)
		}
	}
	return a
}

func (f *regionFixture) addMask(a *store.Asset, r image.Rectangle) {
	f.t.Helper()
	m, err := f.repo.CreateMask(f.pid, f.view.ID, &a.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	bm := image.NewNRGBA(image.Rect(0, 0, regionTestW, regionTestH))
	fillRect(bm, bm.Bounds(), color.NRGBA{A: 255})
	fillRect(bm, r, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if err := f.repo.PutBlobAt(m.ID, encodePNG(f.t, bm), "image/png"); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.repo.SetMaskBitmap(f.pid, f.view.ID, m.ID); err != nil {
		f.t.Fatal(err)
	}
}

func (f *regionFixture) assemble() *renderAssembly {
	f.t.Helper()
	asm, err := f.s.assembleRenderRequest(f.pid, f.view.ID, store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K})
	if err != nil {
		f.t.Fatalf("assembleRenderRequest: %v", err)
	}
	return asm
}

// Regions are named by palette color, never by the asset's own hex, and two
// assets never share a name.
func TestRenderPromptNamesRegionsByPaletteColor(t *testing.T) {
	f := newRegionFixture(t, nil)
	sofa := f.addAsset("Sofa", "#2980b9", true, false)
	floor := f.addAsset("Floor", "#e67e22", true, false)
	f.addMask(sofa, image.Rect(10, 10, 60, 60))
	f.addMask(floor, image.Rect(80, 10, 150, 80))

	asm := f.assemble()
	prompt := asm.req.Prompt
	for _, hex := range []string{"#2980b9", "#e67e22"} {
		if strings.Contains(prompt, hex) {
			t.Errorf("prompt contains the asset's hex color %s", hex)
		}
	}
	// A gray scene gives no reason to deviate from palette order.
	for _, want := range []string{
		"the magenta tinted region.",
		"the cyan tinted region.",
		"magenta tinted region -> IMAGE 4: Sofa description",
		"cyan tinted region -> IMAGE 5: Floor description",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n%s", want, prompt)
		}
	}
	// screenshot, region map, edge map, then one reference photo per asset.
	if len(asm.req.Images) != 5 {
		t.Errorf("images = %d, want 5", len(asm.req.Images))
	}
	if asm.qualifyingCount != 2 {
		t.Errorf("qualifyingCount = %d, want 2", asm.qualifyingCount)
	}
}

// A real magenta object under the mask must push the asset off magenta, or
// the tint would disappear into the object.
func TestRegionColorAvoidsRealObjectOfSameHue(t *testing.T) {
	objectArea := image.Rect(10, 10, 60, 60)
	f := newRegionFixture(t, func(img *image.NRGBA) {
		fillRect(img, objectArea, color.NRGBA{R: 210, G: 40, B: 180, A: 255})
	})
	sofa := f.addAsset("Sofa", "#ff00ff", true, false)
	f.addMask(sofa, objectArea)

	prompt := f.assemble().req.Prompt
	if strings.Contains(prompt, "magenta tinted region") {
		t.Errorf("mask over a magenta object was tinted magenta\n%s", prompt)
	}
	if !strings.Contains(prompt, "cyan tinted region -> IMAGE 4: Sofa description") {
		t.Errorf("expected the next palette color, cyan\n%s", prompt)
	}
}

// An asset whose reference photo cannot be loaded is dropped together with
// its masks, so no region is tinted without a matching product image.
func TestMasksOfAssetWithMissingPhotoAreDropped(t *testing.T) {
	f := newRegionFixture(t, nil)
	good := f.addAsset("Sofa", "#111111", true, false)
	broken := f.addAsset("Lamp", "#222222", true, true)
	f.addMask(good, image.Rect(10, 10, 60, 60))
	f.addMask(broken, image.Rect(80, 10, 150, 80))

	asm := f.assemble()
	if strings.Contains(asm.req.Prompt, "Lamp") {
		t.Errorf("prompt mentions the asset whose photo is missing")
	}
	if asm.qualifyingCount != 1 || len(asm.maskBitmaps) != 1 {
		t.Errorf("qualifyingCount = %d, maskBitmaps = %d, want 1 and 1", asm.qualifyingCount, len(asm.maskBitmaps))
	}
	if len(asm.req.Images) != 4 { // screenshot, region map, edge map, one photo
		t.Errorf("images = %d, want 4", len(asm.req.Images))
	}
}

// More distinct assets than palette colors: the extra ones are dropped with
// their masks and the request stays inside the image budget.
func TestRegionSetCapsAssetsAtThePaletteSize(t *testing.T) {
	f := newRegionFixture(t, nil)
	n := len(geometry.RenderPalette) + 2
	for i := 0; i < n; i++ {
		a := f.addAsset(fmt.Sprintf("Asset%d", i), "#333333", true, false)
		f.addMask(a, image.Rect(i*10, 0, i*10+8, 20))
	}

	asm := f.assemble()
	if got, want := len(asm.req.Images), 3+len(geometry.RenderPalette); got != want {
		t.Errorf("images = %d, want %d", got, want)
	}
	if len(asm.req.Images) > renderpkg.MaxInputImages {
		t.Errorf("images = %d exceeds the budget of %d", len(asm.req.Images), renderpkg.MaxInputImages)
	}
	if strings.Contains(asm.req.Prompt, fmt.Sprintf("Asset%d", n-1)) {
		t.Errorf("last asset should have been dropped")
	}
}
