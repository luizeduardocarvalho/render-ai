package geometry

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// checkerboard builds a synthetic grayscale image with enough high-contrast
// structure that a Sobel edge map will find real edges.
func checkerboard(w, h, cell int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{A: 255}
			if (x/cell+y/cell)%2 == 0 {
				c.R, c.G, c.B = 240, 240, 240
			} else {
				c.R, c.G, c.B = 10, 10, 10
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func TestCompositeRegionMapDimensions(t *testing.T) {
	const w, h = 64, 48
	base := checkerboard(w, h, 8)

	bitmap := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(bitmap, image.Rect(10, 10, 30, 30), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	regions := []Region{
		{Bitmap: bitmap, Color: color.RGBA{R: 255, G: 0, B: 0, A: 255}},
	}

	out, err := CompositeRegionMap(base, regions)
	if err != nil {
		t.Fatalf("CompositeRegionMap: %v", err)
	}
	if out.Bounds().Dx() != w || out.Bounds().Dy() != h {
		t.Fatalf("output dims = %dx%d, want %dx%d", out.Bounds().Dx(), out.Bounds().Dy(), w, h)
	}

	// A pixel inside the painted region should have shifted toward red.
	inside := out.NRGBAAt(20, 20)
	if inside.R < 100 {
		t.Errorf("painted region pixel R = %d, want a strong red tint", inside.R)
	}

	// A pixel outside the painted region should be unchanged from base.
	baseOutside := color.NRGBAModel.Convert(base.At(50, 40)).(color.NRGBA)
	outOutside := out.NRGBAAt(50, 40)
	if outOutside != baseOutside {
		t.Errorf("unpainted pixel changed: got %+v, want %+v", outOutside, baseOutside)
	}
}

func TestCompositeRegionMapSizeMismatch(t *testing.T) {
	base := checkerboard(64, 48, 8)
	bitmap := image.NewGray(image.Rect(0, 0, 32, 32))
	_, err := CompositeRegionMap(base, []Region{{Bitmap: bitmap, Color: color.RGBA{A: 255}}})
	if err == nil {
		t.Fatal("expected error for mismatched mask bitmap size, got nil")
	}
}

func TestEdgeMapIsWhiteOnBlackAndNonEmpty(t *testing.T) {
	img := checkerboard(64, 64, 8)
	edges := EdgeMap(img)

	bounds := edges.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Fatalf("edge map dims = %dx%d, want 64x64", bounds.Dx(), bounds.Dy())
	}

	whiteCount, otherCount := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			v := edges.GrayAt(x, y).Y
			switch v {
			case 0:
				// black background, fine
			case 255:
				whiteCount++
			default:
				otherCount++
			}
		}
	}
	if otherCount != 0 {
		t.Errorf("edge map has %d non-binary pixels, want strictly 0/255", otherCount)
	}
	if whiteCount == 0 {
		t.Error("edge map has no white edge pixels for a high-contrast checkerboard input")
	}
}

func TestEdgeIoUSelfIsNearOne(t *testing.T) {
	img := checkerboard(80, 80, 10)
	edges := EdgeMap(img)

	score, err := EdgeIoU(edges, edges, 2, nil)
	if err != nil {
		t.Fatalf("EdgeIoU: %v", err)
	}
	if score < 0.95 {
		t.Errorf("EdgeIoU(x, x) = %f, want ~1.0", score)
	}
}

func TestEdgeIoUSizeMismatch(t *testing.T) {
	a := image.NewGray(image.Rect(0, 0, 10, 10))
	b := image.NewGray(image.Rect(0, 0, 20, 20))
	if _, err := EdgeIoU(a, b, 0, nil); err == nil {
		t.Fatal("expected error for mismatched edge map sizes, got nil")
	}
}

func TestEdgeIoUExcludesMaskedRegion(t *testing.T) {
	// Two edge maps that agree everywhere except inside a region we exclude.
	a := image.NewGray(image.Rect(0, 0, 20, 20))
	b := image.NewGray(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if x == 5 {
				a.SetGray(x, y, color.Gray{Y: 255})
				b.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	// Disagreement confined entirely to x in [10,15).
	for y := 0; y < 20; y++ {
		a.SetGray(10, y, color.Gray{Y: 255})
	}
	exclude := image.NewGray(image.Rect(0, 0, 20, 20))
	draw.Draw(exclude, image.Rect(9, 0, 16, 20), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	score, err := EdgeIoU(a, b, 0, exclude)
	if err != nil {
		t.Fatalf("EdgeIoU: %v", err)
	}
	if score < 0.999 {
		t.Errorf("EdgeIoU with excluded disagreement = %f, want ~1.0", score)
	}
}
