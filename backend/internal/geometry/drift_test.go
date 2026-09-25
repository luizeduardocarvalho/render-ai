package geometry

import (
	"image"
	"image/color"
	"testing"
)

func TestRegionDriftIsZeroForAnUnchangedAreaAndLargeForAnotherMaterial(t *testing.T) {
	const w, h = 400, 240
	mask := rectMask(w, h, image.Rect(0, 100, w, h))
	before := lawn(w, h)

	if d, share, ok := RegionDrift(before, lawn(w, h), mask); !ok || d != 0 || share != 0 {
		t.Errorf("unchanged area: drift %v share %v ok %v, want 0 0 true", d, share, ok)
	}

	// The same lawn, finer and brighter in detail but with the same look overall.
	sharper := lawn(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := sharper.NRGBAAt(x, y)
			if (x+y)%2 == 0 {
				c.G = min(255, c.G+8)
			} else {
				c.G -= 8
			}
			sharper.SetNRGBA(x, y, c)
		}
	}
	if d, share, _ := RegionDrift(before, sharper, mask); d > 3 || share != 0 {
		t.Errorf("same look with different detail: drift %.1f share %.2f, want under 3 and 0", d, share)
	}

	// Pebbles: gray-brown instead of green.
	pebbles := image.NewNRGBA(before.Bounds())
	for i := 0; i < len(pebbles.Pix); i += 4 {
		n := uint8((i / 4 * 11) % 30)
		pebbles.Pix[i], pebbles.Pix[i+1], pebbles.Pix[i+2], pebbles.Pix[i+3] = 120+n, 112+n, 105+n, 255
	}
	if d, share, _ := RegionDrift(before, pebbles, mask); d < 15 || share != 1 {
		t.Errorf("grass turned to pebbles: drift %.1f share %.2f, want 15 or more and 1", d, share)
	}

	// Pebbles over only the right half: the mean is diluted, the share is not.
	half := lawn(w, h)
	for y := 0; y < h; y++ {
		for x := w / 2; x < w; x++ {
			half.SetNRGBA(x, y, pebbles.NRGBAAt(x, y))
		}
	}
	d, share, _ := RegionDrift(before, half, mask)
	if share < 0.4 || share > 0.6 {
		t.Errorf("pebbles over half the area: share %.2f, want about 0.5 (mean %.1f)", share, d)
	}
}

// Only the masked area counts.
func TestRegionDriftIgnoresWhatIsOutsideTheMask(t *testing.T) {
	const w, h = 400, 240
	before := lawn(w, h)
	after := lawn(w, h)
	for y := 0; y < 100; y++ { // change only the top, outside the mask
		for x := 0; x < w; x++ {
			after.SetNRGBA(x, y, color.NRGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	if d, share, ok := RegionDrift(before, after, rectMask(w, h, image.Rect(0, 100, w, h))); !ok || d != 0 || share != 0 {
		t.Errorf("drift %v share %v ok %v, want 0 0 true: the change is outside the mask", d, share, ok)
	}
}

// A region smaller than a block, and a thin one, still get measured.
func TestRegionDriftMeasuresSmallAndThinRegions(t *testing.T) {
	const w, h = 400, 240
	before := lawn(w, h)
	after := driftTestImage(w, h, color.NRGBA{R: 200, G: 30, B: 30, A: 255})
	for _, r := range []image.Rectangle{image.Rect(100, 100, 106, 105), image.Rect(0, 100, w, 108)} {
		if d, _, ok := RegionDrift(before, after, rectMask(w, h, r)); !ok || d < 15 {
			t.Errorf("region %v: drift %v ok %v, want a large drift", r, d, ok)
		}
	}
	if _, _, ok := RegionDrift(before, after, image.NewGray(image.Rect(0, 0, w, h))); ok {
		t.Errorf("an empty mask has no drift")
	}
}

func driftTestImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}
