package geometry

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Defaults for RegionColorDistance, taken from the offline prototype on the
// cena 05 scene (solid sofa fabric and a plaid armchair against their
// reference photos).
const (
	// colorPaletteSize is how many colors a region and a reference photo are
	// each reduced to.
	colorPaletteSize = 12
	// colorLightnessWeight scales lightness in the color distance. Lightness is
	// weighed below hue and chroma because a render is lit differently from a
	// product photo, but not ignored: a neutral asset (cream, white) has almost
	// no chroma to compare.
	colorLightnessWeight = 0.5
	// colorRefSize is the side, in pixels, the reference photo is shrunk to
	// before its palette is taken.
	colorRefSize = 60
	// colorSampleRows is roughly how many pixel rows of the image are sampled
	// for a region's palette.
	colorSampleRows = 270
	// colorErodeFraction is how much of the image width a mask is shrunk by on
	// each side before sampling, so a loosely painted mask does not pull in the
	// wall or floor around the object.
	colorErodeFraction = 0.003
)

// PaletteColor is one entry of a Palette: a color in CIELAB and the share of
// the sampled pixels it stands for.
type PaletteColor struct {
	Weight  float64
	L, A, B float64
}

// RGBToLab converts an sRGB color (D65) to CIELAB.
func RGBToLab(r, g, b uint8) (l, a, bb float64) {
	lin := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	lr, lg, lb := lin(r), lin(g), lin(b)
	x := (0.4124*lr + 0.3576*lg + 0.1805*lb) / 0.95047
	y := 0.2126*lr + 0.7152*lg + 0.0722*lb
	z := (0.0193*lr + 0.1192*lg + 0.9505*lb) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}

// Palette reduces pixels to at most k representative colors by median cut in
// RGB space: the box of pixels with the widest channel range is split at its
// median until there are k boxes. Each box becomes one PaletteColor (its mean,
// in CIELAB, weighted by its share of the pixels). Returns nil for no pixels.
func Palette(pixels []color.RGBA, k int) []PaletteColor {
	if len(pixels) == 0 || k < 1 {
		return nil
	}
	work := append([]color.RGBA(nil), pixels...)
	boxes := [][]color.RGBA{work}
	for len(boxes) < k {
		best, bestRange, bestChannel := -1, 0, 0
		for i, box := range boxes {
			if len(box) < 2 {
				continue
			}
			if rng, ch := widestChannel(box); rng > bestRange {
				best, bestRange, bestChannel = i, rng, ch
			}
		}
		if best < 0 {
			break // every box is a single color
		}
		box := boxes[best]
		sort.Slice(box, func(i, j int) bool { return channel(box[i], bestChannel) < channel(box[j], bestChannel) })
		mid := splitPoint(box, bestChannel)
		boxes[best] = box[:mid]
		boxes = append(boxes, box[mid:])
	}

	out := make([]PaletteColor, 0, len(boxes))
	for _, box := range boxes {
		var r, g, b float64
		for _, p := range box {
			r += float64(p.R)
			g += float64(p.G)
			b += float64(p.B)
		}
		n := float64(len(box))
		l, a, bb := RGBToLab(uint8(r/n+0.5), uint8(g/n+0.5), uint8(b/n+0.5))
		out = append(out, PaletteColor{Weight: n / float64(len(pixels)), L: l, A: a, B: bb})
	}
	return out
}

// splitPoint is where to cut a box sorted by channel ch: at the median, moved
// to the nearest change of value, so pixels of one color never end up in two
// boxes (which would spend the palette's budget on duplicates). The box must
// have more than one value on ch.
func splitPoint(box []color.RGBA, ch int) int {
	mid := len(box) / 2
	v := channel(box[mid], ch)
	lo, hi := mid, mid
	for lo > 0 && channel(box[lo-1], ch) == v {
		lo--
	}
	for hi < len(box) && channel(box[hi], ch) == v {
		hi++
	}
	if lo > 0 && (hi == len(box) || mid-lo <= hi-mid) {
		return lo
	}
	return hi
}

func channel(c color.RGBA, ch int) uint8 {
	switch ch {
	case 0:
		return c.R
	case 1:
		return c.G
	default:
		return c.B
	}
}

// widestChannel returns the RGB channel with the largest value range in box,
// and that range.
func widestChannel(box []color.RGBA) (rng, ch int) {
	lo := [3]uint8{255, 255, 255}
	hi := [3]uint8{}
	for _, p := range box {
		for c := 0; c < 3; c++ {
			v := channel(p, c)
			lo[c], hi[c] = min(lo[c], v), max(hi[c], v)
		}
	}
	for c := 0; c < 3; c++ {
		if r := int(hi[c]) - int(lo[c]); r > rng {
			rng, ch = r, c
		}
	}
	return rng, ch
}

// PaletteDistance is how far apart two palettes are: for every color in one,
// the distance to the nearest color of the other, averaged by weight, and the
// two directions averaged. It is 0 for identical palettes; around 5 for the
// same asset under different lighting; 20 or more for a clearly different
// color. ok is false if either palette is empty.
func PaletteDistance(p, q []PaletteColor, lightnessWeight float64) (dist float64, ok bool) {
	if len(p) == 0 || len(q) == 0 {
		return 0, false
	}
	oneWay := func(x, y []PaletteColor) float64 {
		sum := 0.0
		for _, c := range x {
			nearest := math.Inf(1)
			for _, d := range y {
				dl := lightnessWeight * (c.L - d.L)
				nearest = min(nearest, math.Sqrt((c.A-d.A)*(c.A-d.A)+(c.B-d.B)*(c.B-d.B)+dl*dl))
			}
			sum += c.Weight * nearest
		}
		return sum
	}
	return (oneWay(p, q) + oneWay(q, p)) / 2, true
}

// Erode shrinks the white area of a white-on-black bitmap by radius pixels
// (square structuring element).
func Erode(bin *image.Gray, radius int) *image.Gray {
	inv := image.NewGray(bin.Bounds())
	for i, v := range bin.Pix {
		if v < 128 {
			inv.Pix[i] = 255
		}
	}
	grown := Dilate(inv, radius)
	out := image.NewGray(bin.Bounds())
	for i, v := range grown.Pix {
		if v < 128 {
			out.Pix[i] = 255
		}
	}
	return out
}

// samplePixels returns the pixels of img (same size as mask) where mask is
// white, taking every step-th pixel in both directions.
func samplePixels(img image.Image, mask *image.Gray, step int) []color.RGBA {
	b := mask.Bounds()
	ib := img.Bounds()
	var out []color.RGBA
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			if !isPainted(mask, x, y) {
				continue
			}
			r, g, bl, _ := img.At(ib.Min.X+(x-b.Min.X), ib.Min.Y+(y-b.Min.Y)).RGBA()
			out = append(out, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255})
		}
	}
	return out
}

// RegionColorDistance measures how well the color of a region of img matches
// a reference photo of the asset placed there: the PaletteDistance between the
// region's colors (mask shrunk a little, sampled sparsely) and the reference's.
// img and mask must share dimensions. Lower is better; see PaletteDistance for
// what the numbers mean. ok is false if the region has no pixels.
//
// It compares palettes rather than average colors so that a patterned asset
// (plaid, a rug) is not reduced to a muddy mean that matches nothing.
func RegionColorDistance(img image.Image, mask *image.Gray, ref image.Image) (dist float64, ok bool) {
	b := mask.Bounds()
	step := max(1, min(b.Dx(), b.Dy())/colorSampleRows)
	pixels := samplePixels(img, Erode(mask, max(1, int(float64(b.Dx())*colorErodeFraction))), step)
	if len(pixels) == 0 {
		// A very thin region: use it as painted rather than nothing.
		pixels = samplePixels(img, mask, step)
	}
	refPixels := samplePixels(Resize(ref, colorRefSize, colorRefSize), fullMask(colorRefSize, colorRefSize), 1)
	return PaletteDistance(Palette(pixels, colorPaletteSize), Palette(refPixels, colorPaletteSize), colorLightnessWeight)
}

func fullMask(w, h int) *image.Gray {
	m := image.NewGray(image.Rect(0, 0, w, h))
	for i := range m.Pix {
		m.Pix[i] = 255
	}
	return m
}
