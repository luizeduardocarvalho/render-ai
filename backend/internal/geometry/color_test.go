package geometry

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

var (
	rust      = color.RGBA{R: 180, G: 70, B: 40, A: 255}
	darkRust  = color.RGBA{R: 140, G: 52, B: 30, A: 255} // the same fabric, less lit
	blue      = color.RGBA{R: 40, G: 90, B: 190, A: 255}
	cream     = color.RGBA{R: 235, G: 228, B: 212, A: 255}
	darkBrown = color.RGBA{R: 70, G: 45, B: 30, A: 255}
)

func flat(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

// halves is a 1000x100 image with its left half painted in l and its right in
// r, plus a mask of the left half. It is wide enough that a mask is shrunk by
// a few pixels before sampling, as it is on a real screenshot.
func halves(l, r color.RGBA) (*image.RGBA, *image.Gray) {
	img := flat(1000, 100, r)
	draw.Draw(img, image.Rect(0, 0, 500, 100), &image.Uniform{C: l}, image.Point{}, draw.Src)
	mask := image.NewGray(img.Bounds())
	for y := 0; y < 100; y++ {
		for x := 0; x < 500; x++ {
			mask.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	return img, mask
}

func TestPaletteOfOneColorIsOneEntry(t *testing.T) {
	pal := Palette([]color.RGBA{rust, rust, rust}, 12)
	if len(pal) != 1 || pal[0].Weight != 1 {
		t.Fatalf("palette = %+v, want one entry of weight 1", pal)
	}
	if Palette(nil, 12) != nil {
		t.Fatalf("no pixels should give no palette")
	}
}

func TestPaletteSeparatesDistinctColorsByWeight(t *testing.T) {
	pixels := []color.RGBA{rust, rust, rust, blue}
	pal := Palette(pixels, 12)
	if len(pal) != 2 {
		t.Fatalf("palette = %+v, want 2 entries", pal)
	}
	weights := map[float64]bool{pal[0].Weight: true, pal[1].Weight: true}
	if !weights[0.75] || !weights[0.25] {
		t.Errorf("weights = %v, want 0.75 and 0.25", weights)
	}
}

func TestPaletteDistance(t *testing.T) {
	p := Palette([]color.RGBA{rust, blue}, 12)
	if d, ok := PaletteDistance(p, p, 0.5); !ok || d != 0 {
		t.Errorf("identical palettes: distance %v ok %v, want 0 true", d, ok)
	}
	if _, ok := PaletteDistance(p, nil, 0.5); ok {
		t.Errorf("an empty palette has no distance")
	}
}

func TestRegionColorDistanceTellsTheRightAssetFromTheWrongOne(t *testing.T) {
	img, mask := halves(rust, blue)

	same, ok := RegionColorDistance(img, mask, flat(32, 32, rust))
	if !ok {
		t.Fatal("region should have pixels")
	}
	litDifferently, _ := RegionColorDistance(img, mask, flat(32, 32, darkRust))
	wrong, _ := RegionColorDistance(img, mask, flat(32, 32, blue))

	if same > 1 {
		t.Errorf("same color scored %.1f, want ~0", same)
	}
	if litDifferently > 12 {
		t.Errorf("same fabric lit differently scored %.1f, want well under a wrong color's", litDifferently)
	}
	if wrong < 30 {
		t.Errorf("wrong color scored %.1f, want 30+", wrong)
	}
}

// A neutral asset has almost no chroma, so lightness has to carry the score.
func TestRegionColorDistanceUsesLightnessForNeutralAssets(t *testing.T) {
	img, mask := halves(darkBrown, cream)
	d, _ := RegionColorDistance(img, mask, flat(32, 32, cream))
	if d < 10 {
		t.Errorf("a dark region against a cream reference scored %.1f, want it clearly off", d)
	}
}

func TestRegionColorDistanceOfAnEmptyMaskIsNotOK(t *testing.T) {
	img := flat(100, 100, rust)
	if _, ok := RegionColorDistance(img, image.NewGray(img.Bounds()), flat(8, 8, rust)); ok {
		t.Errorf("an empty mask has no region to measure")
	}
}

// The mask is shrunk before sampling, so a sloppy edge that spills onto the
// wall does not count against the asset.
func TestRegionColorDistanceIgnoresASloppyMaskEdge(t *testing.T) {
	img, _ := halves(rust, blue)
	sloppy := image.NewGray(img.Bounds())
	for y := 0; y < 100; y++ {
		for x := 0; x < 502; x++ { // 2px onto the blue
			sloppy.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	d, _ := RegionColorDistance(img, sloppy, flat(32, 32, rust))
	if d > 1 {
		t.Errorf("spill onto the wall scored %.1f, want ~0", d)
	}
}

func TestErodeShrinksTheWhiteArea(t *testing.T) {
	bin := image.NewGray(image.Rect(0, 0, 20, 20))
	for y := 5; y < 15; y++ {
		for x := 5; x < 15; x++ {
			bin.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	out := Erode(bin, 2)
	count := 0
	for _, v := range out.Pix {
		if v == 255 {
			count++
		}
	}
	if count != 6*6 || out.GrayAt(5, 5).Y != 0 || out.GrayAt(7, 7).Y != 255 {
		t.Errorf("eroded 10x10 by 2 -> %d white pixels, want 36", count)
	}
}
