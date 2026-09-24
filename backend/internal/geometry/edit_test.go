package geometry

import (
	"image"
	"image/color"
	"testing"
)

// paintedRect returns a w x h white-on-black bitmap with r filled white.
func paintedRect(w, h int, r image.Rectangle) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, w, h))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			g.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	return g
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// TestDilateMatchesBruteForce pins the separable Dilate to the definition:
// a pixel is white if any pixel within the square window is white.
func TestDilateMatchesBruteForce(t *testing.T) {
	const w, h = 40, 30
	bin := image.NewGray(image.Rect(0, 0, w, h))
	for _, p := range []image.Point{{5, 5}, {20, 14}, {21, 14}, {39, 29}, {0, 29}} {
		bin.SetGray(p.X, p.Y, color.Gray{Y: 255})
	}
	for _, radius := range []int{0, 1, 3, 7} {
		got := Dilate(bin, radius)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				want := false
				for dy := -radius; dy <= radius && !want; dy++ {
					for dx := -radius; dx <= radius; dx++ {
						xx, yy := x+dx, y+dy
						if xx >= 0 && xx < w && yy >= 0 && yy < h && bin.GrayAt(xx, yy).Y >= 128 {
							want = true
							break
						}
					}
				}
				if (got.GrayAt(x, y).Y == 255) != want {
					t.Fatalf("radius %d: pixel (%d,%d) = %d, want painted=%v", radius, x, y, got.GrayAt(x, y).Y, want)
				}
			}
		}
	}
}

func TestEditFeatherPx(t *testing.T) {
	for _, tc := range []struct{ minSide, want int }{{100, 4}, {1200, 12}, {3072, 31}, {10000, 40}} {
		if got := EditFeatherPx(tc.minSide); got != tc.want {
			t.Errorf("EditFeatherPx(%d) = %d, want %d", tc.minSide, got, tc.want)
		}
	}
}

func TestEditAlphaShape(t *testing.T) {
	const w, h, feather = 200, 120, 10
	region := paintedRect(w, h, image.Rect(80, 50, 120, 70))
	alpha, err := EditAlpha([]image.Image{region}, w, h, feather)
	if err != nil {
		t.Fatal(err)
	}
	if got := alpha.GrayAt(100, 60).Y; got != 255 {
		t.Errorf("centre of region alpha = %d, want 255", got)
	}
	if got := alpha.GrayAt(0, 0).Y; got != 0 {
		t.Errorf("far corner alpha = %d, want 0", got)
	}
	// The dilation lets the edit reach past the brush edge...
	if got := alpha.GrayAt(78, 60).Y; got == 0 {
		t.Errorf("just outside the region alpha = 0, want a soft non-zero value")
	}
	// ...and the alpha is exactly 0 beyond dilation + blur reach.
	reach := feather/2 + 3*(feather/2) + 2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := max(80-x, x-119, 0)
			dy := max(50-y, y-69, 0)
			if max(dx, dy) > reach && alpha.GrayAt(x, y).Y != 0 {
				t.Fatalf("pixel (%d,%d) is %d beyond the feather reach, want 0", x, y, alpha.GrayAt(x, y).Y)
			}
		}
	}
}

func TestEditAlphaUnionAndScaling(t *testing.T) {
	// Two regions at half resolution are scaled up and unioned.
	a := paintedRect(100, 60, image.Rect(10, 10, 20, 20))
	b := paintedRect(100, 60, image.Rect(70, 30, 90, 50))
	alpha, err := EditAlpha([]image.Image{a, b}, 200, 120, 6)
	if err != nil {
		t.Fatal(err)
	}
	if alpha.GrayAt(30, 30).Y != 255 || alpha.GrayAt(160, 80).Y != 255 {
		t.Errorf("both regions should be fully opaque: got %d and %d", alpha.GrayAt(30, 30).Y, alpha.GrayAt(160, 80).Y)
	}
	if alpha.GrayAt(100, 100).Y != 0 {
		t.Errorf("gap between regions should be 0, got %d", alpha.GrayAt(100, 100).Y)
	}
}

func TestEditAlphaEmptyRegionIsZero(t *testing.T) {
	alpha, err := EditAlpha([]image.Image{image.NewGray(image.Rect(0, 0, 20, 20))}, 20, 20, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range alpha.Pix {
		if v != 0 {
			t.Fatal("an unpainted region must produce an all-zero alpha")
		}
	}
}

// TestBlendEditLeavesOutsideIdentical is the point of the compositor:
// wherever alpha is 0 the output equals the source exactly, and a generated
// image that differs everywhere only shows through inside the mask.
func TestBlendEditLeavesOutsideIdentical(t *testing.T) {
	const w, h = 160, 100
	src := checkerboard(w, h, 5)
	gen := solid(w, h, color.NRGBA{R: 200, G: 30, B: 30, A: 255})
	region := paintedRect(w, h, image.Rect(60, 40, 100, 60))
	alpha, err := EditAlpha([]image.Image{region}, w, h, 8)
	if err != nil {
		t.Fatal(err)
	}
	out, err := BlendEdit(src, gen, alpha)
	if err != nil {
		t.Fatal(err)
	}

	changed := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			want := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			got := out.NRGBAAt(x, y)
			if alpha.GrayAt(x, y).Y == 0 {
				if got != want {
					t.Fatalf("pixel (%d,%d) outside the mask changed: %v -> %v", x, y, want, got)
				}
			} else if got != want {
				changed++
			}
		}
	}
	if changed == 0 {
		t.Fatal("nothing changed inside the mask")
	}
	if got := out.NRGBAAt(80, 50); got != (color.NRGBA{R: 200, G: 30, B: 30, A: 255}) {
		t.Errorf("centre of the edit = %v, want the generated colour", got)
	}
}

func TestBlendEditScalesGeneratedImage(t *testing.T) {
	src := solid(100, 60, color.NRGBA{A: 255})
	gen := solid(400, 240, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	alpha := image.NewGray(image.Rect(0, 0, 100, 60))
	for i := range alpha.Pix {
		alpha.Pix[i] = 255
	}
	out, err := BlendEdit(src, gen, alpha)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 60 {
		t.Fatalf("output is %v, want the source's 100x60", out.Bounds())
	}
	if got := out.NRGBAAt(50, 30); got.R != 255 {
		t.Errorf("scaled generated pixel = %v, want white", got)
	}
}

func TestBlendEditRejectsMismatchedAlpha(t *testing.T) {
	if _, err := BlendEdit(solid(10, 10, color.NRGBA{A: 255}), solid(10, 10, color.NRGBA{A: 255}), image.NewGray(image.Rect(0, 0, 5, 5))); err == nil {
		t.Fatal("want an error for an alpha of the wrong size")
	}
}

func TestResizeBilinearGradient(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{R: 200, G: 200, B: 200, A: 255})
	out := ResizeBilinear(src, 8, 1)
	prev := -1
	for x := 0; x < 8; x++ {
		r := int(out.NRGBAAt(x, 0).R)
		if r < prev {
			t.Fatalf("gradient is not monotonic at x=%d: %d after %d", x, r, prev)
		}
		prev = r
	}
	if out.NRGBAAt(0, 0).R != 0 || out.NRGBAAt(7, 0).R != 200 {
		t.Errorf("ends = %d, %d, want 0 and 200", out.NRGBAAt(0, 0).R, out.NRGBAAt(7, 0).R)
	}
}
