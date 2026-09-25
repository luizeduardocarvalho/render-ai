package geometry

import (
	"image"
	"math"
)

// The edit overlay paints each region in a flat label color, and the model is
// told the colors are labels only. It still sometimes traces the border of a
// region in that color, leaving a colored outline along the region's edge
// (including along the edges of the image, where a region reaches them) that
// the compositor would then keep. ScrubOverlayColors removes those lines.
const (
	// A pixel counts as a leak of a region's label color if it is saturated
	// enough, close enough in hue and not too far in lightness. Hue rather than
	// a plain color distance, because the anti-aliased fringe of a drawn line is
	// a paler or darker version of the same hue, and the fringe is what is left
	// behind if only the core of the line is caught.
	overlayLeakMinChroma = 40.0 // CIELAB chroma; keeps pale and gray pixels out
	overlayLeakMaxHue    = 22.0 // degrees of CIELAB hue
	overlayLeakMaxLight  = 40.0 // CIELAB lightness
	// overlayBandFraction is how far from a region's border, as a fraction of
	// the image's shorter side, a leak is looked for on either side of it.
	overlayBandFraction = 0.02
	// overlayGrowPx widens what was found so the anti-aliased fringe of a line
	// goes with it.
	overlayGrowPx = 3
)

// ScrubOverlayColors repairs, in place, pixels of img that still carry a
// region's overlay color near that region's border, and returns how many it
// repaired. img is the model's answer at the source's size; each Region is a
// bitmap (white is inside, any size) and the label color it was painted in on
// the overlay.
//
// Only a narrow band around each region's border is searched, and only for its
// own label color, so a region that is itself green or blue is not disturbed
// where it is a natural green or blue away from the border. A repaired pixel
// takes the color of the untouched pixel that mirrors it across the edge of the
// damaged strip, so the texture on both sides carries on through it.
func ScrubOverlayColors(img *image.NRGBA, regions []Region) int {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return 0
	}
	band := max(3, int(float64(min(w, h))*overlayBandFraction))

	leak := image.NewGray(image.Rect(0, 0, w, h))
	for _, r := range regions {
		mask := ToGray(r.Bitmap)
		if mb := mask.Bounds(); mb.Dx() != w || mb.Dy() != h {
			mask = ResizeGray(mask, w, h)
		}
		near := borderBand(mask, band)
		tl, ta, tb := RGBToLab(r.Color.R, r.Color.G, r.Color.B)
		targetHue := hueDegrees(ta, tb)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if near.Pix[y*near.Stride+x] == 0 {
					continue
				}
				i := y*img.Stride + x*4
				l, a, bb := RGBToLab(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
				if chroma(a, bb) >= overlayLeakMinChroma &&
					hueGap(hueDegrees(a, bb), targetHue) <= overlayLeakMaxHue &&
					math.Abs(l-tl) <= overlayLeakMaxLight {
					leak.Pix[y*leak.Stride+x] = 255
				}
			}
		}
	}

	holes := Dilate(leak, overlayGrowPx)
	return fillFromNeighbours(img, holes)
}

func chroma(a, b float64) float64 { return math.Sqrt(a*a + b*b) }

func hueDegrees(a, b float64) float64 { return math.Atan2(b, a) * 180 / math.Pi }

// hueGap is the angle between two hues, 0 to 180 degrees.
func hueGap(h1, h2 float64) float64 {
	d := math.Abs(h1 - h2)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// borderBand is white within band pixels of the border of the white area of
// mask, on both sides of it. The image's own edge counts as a border: a
// region that reaches it has a border there too.
func borderBand(mask *image.Gray, band int) *image.Gray {
	w, h := mask.Bounds().Dx(), mask.Bounds().Dy()
	padded := image.NewGray(image.Rect(0, 0, w+2*band, h+2*band))
	for y := 0; y < h; y++ {
		copy(padded.Pix[(y+band)*padded.Stride+band:(y+band)*padded.Stride+band+w], mask.Pix[y*mask.Stride:y*mask.Stride+w])
	}
	grown := Dilate(padded, band)
	shrunk := Erode(padded, band)
	out := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pi := (y+band)*padded.Stride + x + band
			if grown.Pix[pi] >= 128 && shrunk.Pix[pi] < 128 {
				out.Pix[y*out.Stride+x] = 255
			}
		}
	}
	return out
}

// fillFromNeighbours overwrites every pixel of img where holes is white and
// returns how many it overwrote. Working from the outside of each hole inward,
// it finds for every hole pixel the nearest pixel that is not a hole (its
// anchor), then takes the color of the pixel on the far side of that anchor,
// as if the surrounding texture were reflected across the edge of the hole. If
// that mirror pixel is itself a hole or off the image, the anchor's own color
// is used. If there is nothing to copy from (the whole image is a hole) img is
// left as it is.
func fillFromNeighbours(img *image.NRGBA, holes *image.Gray) int {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	isHole := make([]bool, w*h)
	anchor := make([]int, w*h) // for a hole pixel: index of its anchor once found
	var pending []int
	for i := range isHole {
		if holes.Pix[(i/w)*holes.Stride+i%w] >= 128 {
			isHole[i] = true
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 || len(pending) == w*h {
		return 0
	}
	original := append([]uint8(nil), img.Pix...)
	wasHole := append([]bool(nil), isHole...)

	type found struct{ idx, anchor int }
	var order []found
	for len(pending) > 0 {
		var round []found
		var still []int
		for _, i := range pending {
			x, y := i%w, i/w
			// Which neighbour to try first rotates with position, so anchors do
			// not all lean one way.
			from := -1
			for k := 0; k < 4 && from < 0; k++ {
				switch (k + x + y) % 4 {
				case 0:
					if x > 0 && !isHole[i-1] {
						from = i - 1
					}
				case 1:
					if x < w-1 && !isHole[i+1] {
						from = i + 1
					}
				case 2:
					if y > 0 && !isHole[i-w] {
						from = i - w
					}
				case 3:
					if y < h-1 && !isHole[i+w] {
						from = i + w
					}
				}
			}
			if from < 0 {
				still = append(still, i)
				continue
			}
			a := from
			if wasHole[from] {
				a = anchor[from]
			}
			round = append(round, found{idx: i, anchor: a})
		}
		if len(round) == 0 {
			break
		}
		for _, f := range round {
			anchor[f.idx] = f.anchor
			isHole[f.idx] = false
		}
		order = append(order, round...)
		pending = still
	}

	for _, f := range order {
		px, py := f.idx%w, f.idx/w
		ax, ay := f.anchor%w, f.anchor/w
		mx, my := 2*ax-px, 2*ay-py
		src := f.anchor
		if mx >= 0 && mx < w && my >= 0 && my < h && !wasHole[my*w+mx] {
			src = my*w + mx
		}
		d, s := (py*img.Stride)+px*4, (src/w)*img.Stride+(src%w)*4
		copy(img.Pix[d:d+3], original[s:s+3])
	}
	return len(order)
}
