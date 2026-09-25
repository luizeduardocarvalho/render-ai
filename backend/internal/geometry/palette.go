package geometry

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// Swatch is a named overlay color. The name goes into the prompt, the color
// onto the region map, so the model can match one to the other.
type Swatch struct {
	Name  string
	Color color.RGBA
}

// RenderPalette is the fixed set of colors the render's region map paints
// assets in. The order is the tie-break when the scene gives no reason to
// prefer one color over another: hues that rarely occur in architectural
// scenes come first (magenta, cyan, purple), and the ones that collide with
// wood, brick, terracotta and plants (yellow, orange, red) come last.
var RenderPalette = []Swatch{
	{"magenta", color.RGBA{R: 230, G: 30, B: 200, A: 255}},
	{"cyan", color.RGBA{R: 20, G: 210, B: 230, A: 255}},
	{"purple", color.RGBA{R: 130, G: 50, B: 210, A: 255}},
	{"blue", color.RGBA{R: 30, G: 90, B: 240, A: 255}},
	{"green", color.RGBA{R: 30, G: 190, B: 60, A: 255}},
	{"yellow", color.RGBA{R: 250, G: 220, B: 20, A: 255}},
	{"orange", color.RGBA{R: 250, G: 130, B: 20, A: 255}},
	{"red", color.RGBA{R: 230, G: 25, B: 25, A: 255}},
}

const (
	// A pixel only has a usable hue if it is saturated and bright enough;
	// grays, whites, beiges and near-blacks conflict with no overlay color.
	minSaturation = 0.25
	minValue      = 0.20
	// A scene pixel conflicts with an overlay color when their hues are
	// within this many degrees of each other.
	hueConflictDeg = 30
	// Colors under a mask matter most (the tint blends into them); the same
	// hue elsewhere in the scene only risks the model reading a real object
	// as a region, so it counts for less.
	sceneConflictWeight = 0.25
	// Roughly this many samples along the longer side are enough for a
	// stable hue histogram and keep a 4K screenshot cheap.
	sampleTarget = 300
)

// AssignSwatches picks one palette swatch per group of masks (one group per
// asset: all of that asset's mask bitmaps), preferring for each group the
// swatch whose hue is least present under its masks and, less strongly, in
// the rest of base. Groups are handled in order and never share a swatch, and
// the result is deterministic for the same inputs. When the scene gives no
// reason to prefer one swatch (a gray scene), groups take palette order.
//
// Bitmaps are white-on-black at base's size; a pixel counts as inside a mask
// at the same brightness CompositeRegionMap paints it.
func AssignSwatches(base image.Image, groups [][]image.Image, palette []Swatch) ([]Swatch, error) {
	if len(groups) > len(palette) {
		return nil, fmt.Errorf("%d groups do not fit a palette of %d swatches", len(groups), len(palette))
	}
	bounds := base.Bounds()
	for gi, bitmaps := range groups {
		for _, bm := range bitmaps {
			if bm.Bounds().Dx() != bounds.Dx() || bm.Bounds().Dy() != bounds.Dy() {
				return nil, fmt.Errorf("group %d: mask bitmap size %dx%d does not match screenshot size %dx%d",
					gi, bm.Bounds().Dx(), bm.Bounds().Dy(), bounds.Dx(), bounds.Dy())
			}
		}
	}

	step := max(1, max(bounds.Dx(), bounds.Dy())/sampleTarget)
	var scene hueHistogram
	underGroup := make([]hueHistogram, len(groups))
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			hue, ok := pixelHue(base.At(x, y))
			scene.add(hue, ok)
			for gi, bitmaps := range groups {
				if insideAny(bitmaps, x-bounds.Min.X, y-bounds.Min.Y) {
					underGroup[gi].add(hue, ok)
				}
			}
		}
	}

	used := make([]bool, len(palette))
	out := make([]Swatch, len(groups))
	for gi := range groups {
		best, bestScore := -1, math.Inf(1)
		for pi, sw := range palette {
			if used[pi] {
				continue
			}
			h := hueOf(sw.Color)
			score := underGroup[gi].share(h) + sceneConflictWeight*scene.share(h)
			if score < bestScore { // strict: palette order wins ties
				best, bestScore = pi, score
			}
		}
		used[best] = true
		out[gi] = palette[best]
	}
	return out, nil
}

// hueHistogram counts sampled pixels per whole degree of hue.
type hueHistogram struct {
	bins  [360]int
	total int
}

func (h *hueHistogram) add(hue float64, colorful bool) {
	h.total++
	if colorful {
		h.bins[int(hue)%360]++
	}
}

// share is the fraction of all counted pixels whose hue is within
// hueConflictDeg of hue. It is 0 when nothing was counted.
func (h *hueHistogram) share(hue float64) float64 {
	if h.total == 0 {
		return 0
	}
	n := 0
	for d := -hueConflictDeg; d <= hueConflictDeg; d++ {
		n += h.bins[((int(hue)+d)%360+360)%360]
	}
	return float64(n) / float64(h.total)
}

func insideAny(bitmaps []image.Image, x, y int) bool {
	for _, bm := range bitmaps {
		b := bm.Bounds()
		if color.GrayModel.Convert(bm.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y >= 128 {
			return true
		}
	}
	return false
}

func hueOf(c color.RGBA) float64 {
	hue, _ := pixelHue(c)
	return hue
}

// pixelHue returns c's hue in degrees [0,360) and whether the pixel is
// colorful enough for that hue to mean anything.
func pixelHue(c color.Color) (hue float64, colorful bool) {
	r16, g16, b16, _ := c.RGBA()
	r, g, b := float64(r16)/65535, float64(g16)/65535, float64(b16)/65535
	maxC, minC := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	delta := maxC - minC
	if maxC < minValue || delta == 0 || delta/maxC < minSaturation {
		return 0, false
	}
	switch maxC {
	case r:
		hue = math.Mod((g-b)/delta, 6)
	case g:
		hue = (b-r)/delta + 2
	default:
		hue = (r-g)/delta + 4
	}
	hue *= 60
	if hue < 0 {
		hue += 360
	}
	return hue, true
}
