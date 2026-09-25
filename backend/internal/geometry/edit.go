package geometry

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// EditFeatherPx is the feather width the edit compositor uses for an image
// whose shorter side is minSide: about 1% of it, so the soft edge scales with
// the render, clamped to a sensible range.
func EditFeatherPx(minSide int) int {
	return clampInt(int(math.Round(float64(minSide)*0.01)), 4, 40)
}

// EditAlpha builds the blend mask for an Edit: the union of every region
// bitmap (white-on-black, any size - each is scaled to w x h first), grown by
// featherPx/2 so the new content can blend a little past the brush edge, then
// blurred with sigma featherPx/2 into a soft 0-255 alpha.
//
// The alpha is exactly 0 farther than the dilation plus the blur kernel's
// reach from any painted pixel; BlendEdit relies on that to leave the rest of
// the image byte-identical. The work is limited to the painted bounding box
// plus that reach, since the rest is 0 by construction.
func EditAlpha(regions []image.Image, w, h, featherPx int) (*image.Gray, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid edit size %dx%d", w, h)
	}
	union := image.NewGray(image.Rect(0, 0, w, h))
	for i, r := range regions {
		b := r.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			return nil, fmt.Errorf("region %d: empty bitmap", i)
		}
		gray := ToGray(r)
		if b.Dx() != w || b.Dy() != h {
			gray = ResizeGray(gray, w, h)
		}
		for j, v := range gray.Pix {
			// Row stride equals width for both freshly built images.
			if v >= 128 {
				union.Pix[j] = 255
			}
		}
	}

	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		row := union.Pix[y*union.Stride : y*union.Stride+w]
		for x, v := range row {
			if v == 0 {
				continue
			}
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	alpha := image.NewGray(image.Rect(0, 0, w, h))
	if maxX < 0 {
		return alpha, nil
	}

	dilate := featherPx / 2
	sigma := math.Max(float64(featherPx)/2, 0.5)
	reach := dilate + len(gaussianKernel(sigma))/2 + 1
	crop := image.Rect(max(0, minX-reach), max(0, minY-reach), min(w, maxX+1+reach), min(h, maxY+1+reach))

	cropped := image.NewGray(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	for y := 0; y < crop.Dy(); y++ {
		copy(cropped.Pix[y*cropped.Stride:y*cropped.Stride+crop.Dx()],
			union.Pix[(crop.Min.Y+y)*union.Stride+crop.Min.X:(crop.Min.Y+y)*union.Stride+crop.Max.X])
	}
	soft := gaussianBlur(Dilate(cropped, dilate), sigma, 0.5)
	for y := 0; y < crop.Dy(); y++ {
		copy(alpha.Pix[(crop.Min.Y+y)*alpha.Stride+crop.Min.X:(crop.Min.Y+y)*alpha.Stride+crop.Max.X],
			soft.Pix[y*soft.Stride:y*soft.Stride+crop.Dx()])
	}
	return alpha, nil
}

// BlendEdit returns src with gen mixed in by alpha (0 keeps src, 255 takes
// gen). gen is scaled to src's size first if it differs. Where alpha is 0 the
// output pixel is copied from src untouched, so everything outside the edit
// is identical to the source.
func BlendEdit(src, gen image.Image, alpha *image.Gray) (*image.NRGBA, error) {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if ab := alpha.Bounds(); ab.Dx() != w || ab.Dy() != h {
		return nil, fmt.Errorf("alpha size %dx%d does not match source size %dx%d", ab.Dx(), ab.Dy(), w, h)
	}
	base := toNRGBA(src)
	var generated *image.NRGBA
	if gb := gen.Bounds(); gb.Dx() == w && gb.Dy() == h {
		generated = toNRGBA(gen)
	} else {
		generated = ResizeBilinear(gen, w, h)
	}

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srow := base.Pix[y*base.Stride : y*base.Stride+w*4]
		grow := generated.Pix[y*generated.Stride : y*generated.Stride+w*4]
		orow := out.Pix[y*out.Stride : y*out.Stride+w*4]
		arow := alpha.Pix[y*alpha.Stride : y*alpha.Stride+w]
		for x := 0; x < w; x++ {
			i := x * 4
			a := int(arow[x])
			switch a {
			case 0:
				copy(orow[i:i+4], srow[i:i+4])
			case 255:
				copy(orow[i:i+3], grow[i:i+3])
				orow[i+3] = 255
			default:
				for c := 0; c < 3; c++ {
					orow[i+c] = uint8((int(srow[i+c])*(255-a) + int(grow[i+c])*a + 127) / 255)
				}
				orow[i+3] = 255
			}
		}
	}
	return out, nil
}

// ResizeBilinear scales img to exactly w x h with bilinear sampling. Unlike
// Resize (nearest-neighbour, fine for comparing edge maps) this is what to use
// when the pixels themselves are kept.
func ResizeBilinear(img image.Image, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	src := toNRGBA(img)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if w <= 0 || h <= 0 || sw <= 0 || sh <= 0 {
		return out
	}
	xScale, yScale := float64(sw)/float64(w), float64(sh)/float64(h)
	for y := 0; y < h; y++ {
		fy := clampFloat((float64(y)+0.5)*yScale-0.5, 0, float64(sh-1))
		y0 := int(fy)
		y1 := min(y0+1, sh-1)
		wy := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := clampFloat((float64(x)+0.5)*xScale-0.5, 0, float64(sw-1))
			x0 := int(fx)
			x1 := min(x0+1, sw-1)
			wx := fx - float64(x0)
			o := out.Pix[y*out.Stride+x*4 : y*out.Stride+x*4+4]
			for c := 0; c < 4; c++ {
				p00 := float64(src.Pix[y0*src.Stride+x0*4+c])
				p10 := float64(src.Pix[y0*src.Stride+x1*4+c])
				p01 := float64(src.Pix[y1*src.Stride+x0*4+c])
				p11 := float64(src.Pix[y1*src.Stride+x1*4+c])
				top := p00 + (p10-p00)*wx
				bottom := p01 + (p11-p01)*wx
				o[c] = uint8(clampFloat(top+(bottom-top)*wy+0.5, 0, 255))
			}
		}
	}
	return out
}

// ResizeGray scales a mask-like grayscale image to exactly w x h with bilinear
// sampling. It is ResizeBilinear for one channel, so a 4K mask costs a fifth
// of the memory of going through RGBA.
func ResizeGray(src *image.Gray, w, h int) *image.Gray {
	out := image.NewGray(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if w <= 0 || h <= 0 || sw <= 0 || sh <= 0 {
		return out
	}
	xScale, yScale := float64(sw)/float64(w), float64(sh)/float64(h)
	for y := 0; y < h; y++ {
		fy := clampFloat((float64(y)+0.5)*yScale-0.5, 0, float64(sh-1))
		y0 := int(fy)
		y1 := min(y0+1, sh-1)
		wy := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := clampFloat((float64(x)+0.5)*xScale-0.5, 0, float64(sw-1))
			x0 := int(fx)
			x1 := min(x0+1, sw-1)
			wx := fx - float64(x0)
			p00 := float64(src.Pix[y0*src.Stride+x0])
			p10 := float64(src.Pix[y0*src.Stride+x1])
			p01 := float64(src.Pix[y1*src.Stride+x0])
			p11 := float64(src.Pix[y1*src.Stride+x1])
			top := p00 + (p10-p00)*wx
			bottom := p01 + (p11-p01)*wx
			out.Pix[y*out.Stride+x] = uint8(clampFloat(top+(bottom-top)*wy+0.5, 0, 255))
		}
	}
	return out
}

// ToNRGBA returns img as a zero-origin *image.NRGBA, without copying when it
// already is one (so changing the result may change img).
func ToNRGBA(img image.Image) *image.NRGBA { return toNRGBA(img) }

// toNRGBA returns img as a zero-origin *image.NRGBA, without copying when it
// already is one.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// RegionPalette is the fixed set of colors the edit overlay paints regions
// in, in region order. The names go into the prompt so the model can tell the
// regions apart; the colors are far apart in hue so it can too.
var RegionPalette = []struct {
	Name  string
	Color color.RGBA
}{
	{"red", color.RGBA{R: 230, G: 25, B: 25, A: 255}},
	{"blue", color.RGBA{R: 30, G: 90, B: 240, A: 255}},
	{"green", color.RGBA{R: 30, G: 190, B: 60, A: 255}},
	{"yellow", color.RGBA{R: 250, G: 220, B: 20, A: 255}},
	{"magenta", color.RGBA{R: 230, G: 30, B: 200, A: 255}},
	{"cyan", color.RGBA{R: 20, G: 210, B: 230, A: 255}},
	{"orange", color.RGBA{R: 250, G: 130, B: 20, A: 255}},
	{"purple", color.RGBA{R: 130, G: 50, B: 210, A: 255}},
}
