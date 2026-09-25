package geometry

import (
	"image"
	"image/color"
	"testing"
)

func paintRect(img *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

// grayScene is a w x h image of one flat gray, so it gives no hue reason to
// prefer any swatch.
func grayScene(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	paintRect(img, img.Bounds(), color.NRGBA{R: 180, G: 180, B: 180, A: 255})
	return img
}

// maskOf is a white-on-black bitmap with r painted.
func maskOf(w, h int, r image.Rectangle) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	paintRect(img, img.Bounds(), color.NRGBA{A: 255})
	paintRect(img, r, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	return img
}

func names(sw []Swatch) []string {
	out := make([]string, len(sw))
	for i, s := range sw {
		out[i] = s.Name
	}
	return out
}

func TestAssignSwatchesGrayScenePicksPaletteOrder(t *testing.T) {
	scene := grayScene(200, 100)
	groups := [][]image.Image{
		{maskOf(200, 100, image.Rect(10, 10, 60, 60))},
		{maskOf(200, 100, image.Rect(100, 10, 160, 60))},
		{maskOf(200, 100, image.Rect(10, 70, 60, 95))},
	}
	got, err := AssignSwatches(scene, groups, RenderPalette)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"magenta", "cyan", "purple"}
	for i, w := range want {
		if got[i].Name != w {
			t.Fatalf("swatches = %v, want %v", names(got), want)
		}
	}
}

// A mask sitting on a magenta object must not be tinted magenta: the tint
// would vanish into the object.
func TestAssignSwatchesAvoidsHueUnderTheMask(t *testing.T) {
	scene := grayScene(200, 100)
	objectArea := image.Rect(20, 20, 80, 80)
	paintRect(scene, objectArea, color.NRGBA{R: 210, G: 40, B: 180, A: 255}) // magenta sofa

	got, err := AssignSwatches(scene, [][]image.Image{{maskOf(200, 100, objectArea)}}, RenderPalette)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name == "magenta" {
		t.Fatalf("picked magenta for a mask over a magenta object, want a different hue")
	}
	if got[0].Name != "cyan" {
		t.Errorf("picked %s, want cyan (next in palette order)", got[0].Name)
	}
}

// A real object of the same hue elsewhere in the scene also steers the
// choice away, so the model is not tempted to read it as a region.
func TestAssignSwatchesAvoidsHueElsewhereInTheScene(t *testing.T) {
	scene := grayScene(200, 100)
	paintRect(scene, image.Rect(120, 20, 190, 90), color.NRGBA{R: 210, G: 40, B: 180, A: 255}) // magenta object, not masked

	got, err := AssignSwatches(scene, [][]image.Image{{maskOf(200, 100, image.Rect(10, 10, 60, 60))}}, RenderPalette)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "cyan" {
		t.Errorf("picked %s, want cyan because magenta appears in the scene", got[0].Name)
	}
}

func TestAssignSwatchesNeverSharesASwatch(t *testing.T) {
	scene := grayScene(200, 100)
	var groups [][]image.Image
	for range RenderPalette {
		groups = append(groups, []image.Image{maskOf(200, 100, image.Rect(0, 0, 50, 50))})
	}
	got, err := AssignSwatches(scene, groups, RenderPalette)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range got {
		if seen[s.Name] {
			t.Fatalf("swatch %s assigned twice: %v", s.Name, names(got))
		}
		seen[s.Name] = true
	}
}

func TestAssignSwatchesRejectsBadInput(t *testing.T) {
	scene := grayScene(200, 100)
	tooMany := make([][]image.Image, len(RenderPalette)+1)
	if _, err := AssignSwatches(scene, tooMany, RenderPalette); err == nil {
		t.Error("expected an error for more groups than palette swatches")
	}
	wrongSize := [][]image.Image{{maskOf(50, 50, image.Rect(0, 0, 10, 10))}}
	if _, err := AssignSwatches(scene, wrongSize, RenderPalette); err == nil {
		t.Error("expected an error for a mask bitmap that does not match the screenshot size")
	}
}

func TestAssignSwatchesIsDeterministic(t *testing.T) {
	scene := grayScene(200, 100)
	paintRect(scene, image.Rect(120, 20, 190, 90), color.NRGBA{R: 40, G: 190, B: 220, A: 255})
	groups := [][]image.Image{
		{maskOf(200, 100, image.Rect(10, 10, 60, 60))},
		{maskOf(200, 100, image.Rect(120, 20, 190, 90))},
	}
	a, _ := AssignSwatches(scene, groups, RenderPalette)
	b, _ := AssignSwatches(scene, groups, RenderPalette)
	if len(a) != 2 || a[0] != b[0] || a[1] != b[1] {
		t.Fatalf("not deterministic: %v vs %v", names(a), names(b))
	}
}

func TestRenderPaletteHuesAreSeparated(t *testing.T) {
	// Two neighbours closer than this would be hard for the model to tell
	// apart (yellow and orange, the closest pair, are last in the order).
	for i, a := range RenderPalette {
		for j, b := range RenderPalette {
			if i >= j {
				continue
			}
			d := hueOf(a.Color) - hueOf(b.Color)
			if d < 0 {
				d = -d
			}
			if d > 180 {
				d = 360 - d
			}
			if d < 20 {
				t.Errorf("%s and %s are only %.0f degrees apart", a.Name, b.Name, d)
			}
		}
	}
}
