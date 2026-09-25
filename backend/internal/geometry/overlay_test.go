package geometry

import (
	"image"
	"image/color"
	"testing"
)

var (
	labelRed  = color.RGBA{R: 230, G: 25, B: 25, A: 255}
	labelBlue = color.RGBA{R: 30, G: 90, B: 240, A: 255}
)

// lawn is a green, slightly noisy image with no trace of any label color.
func lawn(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8((x*7 + y*13) % 24)
			img.SetNRGBA(x, y, color.NRGBA{R: 55 + n/2, G: 110 + n, B: 40 + n/3, A: 255})
		}
	}
	return img
}

func rectMask(w, h int, r image.Rectangle) *image.Gray {
	m := image.NewGray(image.Rect(0, 0, w, h))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	return m
}

// outline paints a t-pixel frame of c just inside r, with a paler fringe of
// one more pixel on each side, like an anti-aliased drawn line.
func outline(img *image.NRGBA, r image.Rectangle, t int, c color.RGBA) {
	fringe := color.NRGBA{R: uint8((int(c.R) + 60) / 2), G: uint8((int(c.G) + 90) / 2), B: uint8((int(c.B) + 40) / 2), A: 255}
	for y := r.Min.Y - 1; y < r.Max.Y+1; y++ {
		for x := r.Min.X - 1; x < r.Max.X+1; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			d := min(min(x-r.Min.X, r.Max.X-1-x), min(y-r.Min.Y, r.Max.Y-1-y)) // depth inside r, negative outside
			switch {
			case d >= 0 && d < t:
				img.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255})
			case d == -1 || d == t:
				img.SetNRGBA(x, y, fringe)
			}
		}
	}
}

// labelLike counts pixels that still look like the label color: saturated red.
func labelLike(img *image.NRGBA) int {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 120 && img.Pix[i+1] < 90 && img.Pix[i+2] < 90 {
			n++
		}
	}
	return n
}

func TestScrubRemovesAnOutlineTracedAlongARegionBorder(t *testing.T) {
	const w, h = 400, 240
	region := image.Rect(80, 50, 320, 190)
	img := lawn(w, h)
	clean := lawn(w, h)
	outline(img, region, 5, labelRed)
	if labelLike(img) == 0 {
		t.Fatal("test setup: no outline was drawn")
	}

	n := ScrubOverlayColors(img, []Region{{Bitmap: rectMask(w, h, region), Color: labelRed}})

	if n == 0 {
		t.Fatal("nothing was repaired")
	}
	if left := labelLike(img); left != 0 {
		t.Errorf("%d label-colored pixels are left", left)
	}
	// The repair blends into the lawn instead of leaving a bald strip.
	for _, p := range []image.Point{{82, 120}, {200, 52}, {318, 120}, {200, 188}} {
		c := img.NRGBAAt(p.X, p.Y)
		if int(c.G) < int(c.R)+30 {
			t.Errorf("repaired pixel %v = %v, want lawn green", p, c)
		}
	}
	// Far from the border nothing changed at all.
	for _, p := range []image.Point{{5, 5}, {200, 120}, {395, 235}, {150, 100}} {
		if img.NRGBAAt(p.X, p.Y) != clean.NRGBAAt(p.X, p.Y) {
			t.Errorf("pixel %v away from the border changed", p)
		}
	}
}

// A region that reaches the edge of the image has a border there too.
func TestScrubRemovesAnOutlineAlongTheImageEdge(t *testing.T) {
	const w, h = 400, 240
	region := image.Rect(0, 100, w, h) // the bottom of the image, edge to edge
	img := lawn(w, h)
	outline(img, region, 5, labelBlue)
	img2 := lawn(w, h)
	outline(img2, region, 5, labelBlue)

	n := ScrubOverlayColors(img, []Region{{Bitmap: rectMask(w, h, region), Color: labelBlue}})
	if n == 0 {
		t.Fatal("nothing was repaired")
	}
	blueish := func(im *image.NRGBA) int {
		c := 0
		for i := 0; i < len(im.Pix); i += 4 {
			if im.Pix[i+2] > 150 && im.Pix[i] < 100 {
				c++
			}
		}
		return c
	}
	if before, after := blueish(img2), blueish(img); before == 0 || after != 0 {
		t.Errorf("blue pixels before %d, after %d, want some and then none", before, after)
	}
}

// Only the region's own label color is looked for, and only near its border.
func TestScrubLeavesOtherColorsAndTheInteriorAlone(t *testing.T) {
	const w, h = 400, 240
	region := image.Rect(80, 50, 320, 190)

	// A blue line on a region labelled red is not the overlay leaking.
	img := lawn(w, h)
	outline(img, region, 5, labelBlue)
	if n := ScrubOverlayColors(img, []Region{{Bitmap: rectMask(w, h, region), Color: labelRed}}); n != 0 {
		t.Errorf("repaired %d pixels of a color that is not the region's label", n)
	}

	// Red the scene legitimately has, well inside the region, is kept.
	img = lawn(w, h)
	for y := 100; y < 140; y++ {
		for x := 150; x < 250; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 235, G: 30, B: 25, A: 255}) // a red object
		}
	}
	want := append([]uint8(nil), img.Pix...)
	if n := ScrubOverlayColors(img, []Region{{Bitmap: rectMask(w, h, region), Color: labelRed}}); n != 0 {
		t.Errorf("repaired %d pixels of a red object in the middle of the region", n)
	}
	for i := range want {
		if img.Pix[i] != want[i] {
			t.Fatalf("the interior changed at byte %d", i)
		}
	}
}

func TestScrubDoesNothingWithoutRegions(t *testing.T) {
	img := lawn(50, 50)
	outline(img, image.Rect(5, 5, 45, 45), 3, labelRed)
	want := append([]uint8(nil), img.Pix...)
	if n := ScrubOverlayColors(img, nil); n != 0 {
		t.Errorf("repaired %d pixels with no regions", n)
	}
	for i := range want {
		if img.Pix[i] != want[i] {
			t.Fatalf("image changed at byte %d", i)
		}
	}
}

func TestHueGapWrapsAround(t *testing.T) {
	if got := hueGap(170, -170); got != 20 {
		t.Errorf("hueGap(170, -170) = %v, want 20", got)
	}
	if got := hueGap(-65, -76); got != 11 {
		t.Errorf("hueGap(-65, -76) = %v, want 11", got)
	}
}
