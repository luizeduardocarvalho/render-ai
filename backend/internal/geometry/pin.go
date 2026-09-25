package geometry

import (
	"fmt"
	"image"
	"math"
)

const (
	// pinLowWidth is the width the low-frequency comparison is made at. Colour
	// and lighting live in the broad strokes of an image, so comparing them on a
	// small copy is as good as on the full one and costs almost nothing, however
	// large the render is.
	pinLowWidth = 640
	// pinSigma is how far, in pixels of that small copy, a colour is smeared
	// before two images are compared. At a 4K render (5504 px wide) it is about
	// 26 px: wider than any texture the model repaints (brick grain, a wicker
	// weave), narrower than the objects and light pools that make up a room.
	pinSigma = 3.0
)

// PinLowFrequency gives gen the colour and lighting of ref while keeping gen's
// own fine detail. gen is a re-drawn version of ref (an upscale): it has the
// detail we want, but the model shifts colours a little on the way (paler
// bricks, a warmer cast), which for an approved render is a defect.
//
// The two images are compared blurred - the low frequencies are what carry
// colour and light - and the difference is added back to gen, so gen's blurred
// colours become ref's exactly while everything finer than the blur, that is
// all of gen's added detail, is untouched.
//
// ref may be any size but must show the same picture as gen (the same aspect
// ratio). Both are taken to be opaque, as a render is.
//
// gen is changed in place when it is an *image.NRGBA or *image.RGBA (a 4K
// render is 68 MB per copy; the worker cannot afford two), and the returned
// image is then gen itself. Any other type is converted to a new *image.NRGBA
// first.
func PinLowFrequency(gen, ref image.Image) (image.Image, error) {
	gb, rb := gen.Bounds(), ref.Bounds()
	if gb.Dx() <= 0 || gb.Dy() <= 0 || rb.Dx() <= 0 || rb.Dy() <= 0 {
		return nil, fmt.Errorf("pinning colours: empty image (%v, %v)", gb, rb)
	}

	pix, stride, w, h, out := opaquePixels(gen)
	lw := min(pinLowWidth, w)
	lh := max(1, int(math.Round(float64(h)*float64(lw)/float64(w))))

	genLow := blurRGB(downsampleRGB(gen, lw, lh), lw, lh, pinSigma)
	refLow := blurRGB(downsampleRGB(ref, lw, lh), lw, lh, pinSigma)
	for i := range genLow {
		genLow[i] = refLow[i] - genLow[i] // the correction, per low-res pixel
	}

	// Per-column sampling positions into the correction, shared by every row.
	type tap struct {
		i0, i1 int
		w1     float32
	}
	taps := make([]tap, w)
	for x := range taps {
		f := clampFloat((float64(x)+0.5)*float64(lw)/float64(w)-0.5, 0, float64(lw-1))
		i0 := int(f)
		taps[x] = tap{i0: i0 * 3, i1: min(i0+1, lw-1) * 3, w1: float32(f - float64(i0))}
	}
	for y := 0; y < h; y++ {
		f := clampFloat((float64(y)+0.5)*float64(lh)/float64(h)-0.5, 0, float64(lh-1))
		y0 := int(f)
		y1 := min(y0+1, lh-1)
		wy := float32(f - float64(y0))
		top, bottom := genLow[y0*lw*3:(y0+1)*lw*3], genLow[y1*lw*3:(y1+1)*lw*3]
		row := pix[y*stride : y*stride+w*4]
		for x, t := range taps {
			for c := 0; c < 3; c++ {
				a := top[t.i0+c] + (top[t.i1+c]-top[t.i0+c])*t.w1
				b := bottom[t.i0+c] + (bottom[t.i1+c]-bottom[t.i0+c])*t.w1
				v := float32(row[x*4+c]) + a + (b-a)*wy + 0.5
				switch {
				case v < 0:
					row[x*4+c] = 0
				case v > 255:
					row[x*4+c] = 255
				default:
					row[x*4+c] = uint8(v)
				}
			}
		}
	}
	return out, nil
}

// opaquePixels returns the pixel buffer of img as 4 bytes per pixel (R, G, B
// first) together with its stride and size, and the image that owns the buffer.
// It is img itself when img is an *image.NRGBA or *image.RGBA starting at the
// origin, otherwise a converted copy.
func opaquePixels(img image.Image) (pix []uint8, stride, w, h int, owner image.Image) {
	switch m := img.(type) {
	case *image.NRGBA:
		if m.Rect.Min == (image.Point{}) {
			return m.Pix, m.Stride, m.Rect.Dx(), m.Rect.Dy(), m
		}
	case *image.RGBA:
		if m.Rect.Min == (image.Point{}) {
			return m.Pix, m.Stride, m.Rect.Dx(), m.Rect.Dy(), m
		}
	}
	n := toNRGBA(img)
	return n.Pix, n.Stride, n.Rect.Dx(), n.Rect.Dy(), n
}

// downsampleRGB returns img scaled to w x h by averaging the block of source
// pixels each output pixel covers, as 3 floats (R, G, B) per pixel. Should w or
// h exceed the source's size, source pixels are repeated.
func downsampleRGB(img image.Image, w, h int) []float32 {
	pix, stride, sw, sh, _ := opaquePixels(img)
	out := make([]float32, w*h*3)
	sums := make([]float64, w*3)
	for ty := 0; ty < h; ty++ {
		y0, y1 := ty*sh/h, max(ty*sh/h+1, (ty+1)*sh/h)
		for i := range sums {
			sums[i] = 0
		}
		for sy := y0; sy < y1; sy++ {
			row := pix[sy*stride : sy*stride+sw*4]
			for tx := 0; tx < w; tx++ {
				x0, x1 := tx*sw/w, max(tx*sw/w+1, (tx+1)*sw/w)
				var r, g, b uint32
				for sx := x0; sx < x1; sx++ {
					r += uint32(row[sx*4])
					g += uint32(row[sx*4+1])
					b += uint32(row[sx*4+2])
				}
				sums[tx*3] += float64(r)
				sums[tx*3+1] += float64(g)
				sums[tx*3+2] += float64(b)
			}
		}
		rows := float64(y1 - y0)
		for tx := 0; tx < w; tx++ {
			x0, x1 := tx*sw/w, max(tx*sw/w+1, (tx+1)*sw/w)
			n := rows * float64(x1-x0)
			for c := 0; c < 3; c++ {
				out[(ty*w+tx)*3+c] = float32(sums[tx*3+c] / n)
			}
		}
	}
	return out
}

// blurRGB gaussian-blurs buf (w x h pixels, 3 floats each) in place and returns
// it. Pixels past the edge repeat the edge's own.
func blurRGB(buf []float32, w, h int, sigma float64) []float32 {
	radius := int(math.Ceil(3 * sigma))
	kernel := make([]float32, 2*radius+1)
	var sum float32
	for i := range kernel {
		d := float64(i - radius)
		kernel[i] = float32(math.Exp(-d * d / (2 * sigma * sigma)))
		sum += kernel[i]
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	tmp := make([]float32, len(buf))
	for y := 0; y < h; y++ { // horizontal pass: buf -> tmp
		for x := 0; x < w; x++ {
			var r, g, b float32
			for k, kv := range kernel {
				i := (y*w + min(max(x+k-radius, 0), w-1)) * 3
				r += kv * buf[i]
				g += kv * buf[i+1]
				b += kv * buf[i+2]
			}
			o := (y*w + x) * 3
			tmp[o], tmp[o+1], tmp[o+2] = r, g, b
		}
	}
	for y := 0; y < h; y++ { // vertical pass: tmp -> buf
		for x := 0; x < w; x++ {
			var r, g, b float32
			for k, kv := range kernel {
				i := (min(max(y+k-radius, 0), h-1)*w + x) * 3
				r += kv * tmp[i]
				g += kv * tmp[i+1]
				b += kv * tmp[i+2]
			}
			o := (y*w + x) * 3
			buf[o], buf[o+1], buf[o+2] = r, g, b
		}
	}
	return buf
}
