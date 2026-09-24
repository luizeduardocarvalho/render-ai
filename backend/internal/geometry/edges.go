package geometry

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Defaults for the simplified Canny-style pipeline: Gaussian blur to reduce
// noise, Sobel gradient magnitude, then a fixed threshold on the normalized
// magnitude. This is intentionally simple (no non-max suppression or
// hysteresis) but produces clean white-edges-on-black output good enough to
// anchor geometry and to score preservation via IoU.
const (
	defaultBlurSigma     = 1.4
	defaultEdgeThreshold = 60 // 0-255, applied to normalized gradient magnitude
)

// ToGray converts any image to 8-bit grayscale.
func ToGray(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	draw.Draw(gray, bounds, img, bounds.Min, draw.Src)
	return gray
}

// GaussianBlur applies a separable Gaussian blur with the given sigma,
// clamping at the image edges. Results are truncated to 8 bits.
func GaussianBlur(gray *image.Gray, sigma float64) *image.Gray {
	return gaussianBlur(gray, sigma, 0)
}

// gaussianBlur is GaussianBlur with bias added to every sum before it is cut
// to 8 bits: 0 truncates, 0.5 rounds to nearest. Rounding is what keeps a
// fully white area at exactly 255, which the edit compositor needs.
func gaussianBlur(gray *image.Gray, sigma, bias float64) *image.Gray {
	kernel := gaussianKernel(sigma)
	radius := len(kernel) / 2
	bounds := gray.Bounds()

	tmp := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sum := 0.0
			for k := -radius; k <= radius; k++ {
				xx := clampInt(x+k, bounds.Min.X, bounds.Max.X-1)
				sum += float64(gray.GrayAt(xx, y).Y) * kernel[k+radius]
			}
			tmp.SetGray(x, y, color.Gray{Y: uint8(clampFloat(sum+bias, 0, 255))})
		}
	}

	out := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sum := 0.0
			for k := -radius; k <= radius; k++ {
				yy := clampInt(y+k, bounds.Min.Y, bounds.Max.Y-1)
				sum += float64(tmp.GrayAt(x, yy).Y) * kernel[k+radius]
			}
			out.SetGray(x, y, color.Gray{Y: uint8(clampFloat(sum+bias, 0, 255))})
		}
	}
	return out
}

func gaussianKernel(sigma float64) []float64 {
	radius := int(math.Ceil(3 * sigma))
	if radius < 1 {
		radius = 1
	}
	size := 2*radius + 1
	kernel := make([]float64, size)
	sum := 0.0
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		kernel[i+radius] = v
		sum += v
	}
	for i := range kernel {
		kernel[i] /= sum
	}
	return kernel
}

var sobelX = [3][3]int{{-1, 0, 1}, {-2, 0, 2}, {-1, 0, 1}}
var sobelY = [3][3]int{{-1, -2, -1}, {0, 0, 0}, {1, 2, 1}}

// sobelMagnitude computes the Sobel gradient magnitude and returns it
// normalized to 0-255 (so the brightest edge in the image is pure white).
func sobelMagnitude(gray *image.Gray) *image.Gray {
	bounds := gray.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	mag := make([]float64, w*h)
	maxMag := 0.0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var gx, gy float64
			for ky := -1; ky <= 1; ky++ {
				yy := clampInt(bounds.Min.Y+y+ky, bounds.Min.Y, bounds.Max.Y-1)
				for kx := -1; kx <= 1; kx++ {
					xx := clampInt(bounds.Min.X+x+kx, bounds.Min.X, bounds.Max.X-1)
					v := float64(gray.GrayAt(xx, yy).Y)
					gx += v * float64(sobelX[ky+1][kx+1])
					gy += v * float64(sobelY[ky+1][kx+1])
				}
			}
			m := math.Hypot(gx, gy)
			mag[y*w+x] = m
			if m > maxMag {
				maxMag = m
			}
		}
	}

	out := image.NewGray(bounds)
	if maxMag == 0 {
		maxMag = 1
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			norm := mag[y*w+x] / maxMag * 255
			out.SetGray(bounds.Min.X+x, bounds.Min.Y+y, color.Gray{Y: uint8(clampFloat(norm, 0, 255))})
		}
	}
	return out
}

// Threshold binarizes a grayscale image: pixels >= t become white (255),
// everything else becomes black (0).
func Threshold(gray *image.Gray, t uint8) *image.Gray {
	bounds := gray.Bounds()
	out := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if gray.GrayAt(x, y).Y >= t {
				out.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return out
}

// EdgeMap computes a simplified Canny-style edge map: grayscale -> Gaussian
// blur -> Sobel magnitude -> threshold. Output is white edges on black.
func EdgeMap(img image.Image) *image.Gray {
	gray := ToGray(img)
	blurred := GaussianBlur(gray, defaultBlurSigma)
	mag := sobelMagnitude(blurred)
	return Threshold(mag, defaultEdgeThreshold)
}

// Dilate grows white regions by radius pixels (square structuring element).
// A square element is separable, so this is two 1-D passes (horizontal, then
// vertical) instead of a (2r+1)^2 window per pixel - the edit compositor
// dilates by tens of pixels on multi-megapixel masks.
func Dilate(bin *image.Gray, radius int) *image.Gray {
	bounds := bin.Bounds()
	if radius <= 0 {
		clone := image.NewGray(bounds)
		copy(clone.Pix, bin.Pix)
		return clone
	}
	w, h := bounds.Dx(), bounds.Dy()

	// dilateLine writes 255 into dst[i] wherever src has a painted sample
	// within radius of i. prefix[i] counts painted samples in src[:i], so the
	// window [i-radius, i+radius] holds one iff its prefix difference is > 0.
	prefix := make([]int, max(w, h)+1)
	dilateLine := func(n int, src func(i int) bool, dst func(i int)) {
		for i := 0; i < n; i++ {
			prefix[i+1] = prefix[i]
			if src(i) {
				prefix[i+1]++
			}
		}
		for i := 0; i < n; i++ {
			lo, hi := max(0, i-radius), min(n, i+radius+1)
			if prefix[hi]-prefix[lo] > 0 {
				dst(i)
			}
		}
	}

	horiz := image.NewGray(bounds)
	for y := 0; y < h; y++ {
		row := bin.Pix[y*bin.Stride : y*bin.Stride+w]
		out := horiz.Pix[y*horiz.Stride : y*horiz.Stride+w]
		dilateLine(w, func(x int) bool { return row[x] >= 128 }, func(x int) { out[x] = 255 })
	}

	out := image.NewGray(bounds)
	for x := 0; x < w; x++ {
		dilateLine(h,
			func(y int) bool { return horiz.Pix[y*horiz.Stride+x] >= 128 },
			func(y int) { out.Pix[y*out.Stride+x] = 255 })
	}
	return out
}

// EdgeIoU computes the intersection-over-union of two binary edge maps
// (a and b, both white-on-black, same dimensions) after dilating each by
// dilatePx to tolerate sub-pixel drift. Pixels where exclude is white (e.g.
// masked/replaced regions) are left out of the score entirely. Returns 1.0
// if there are no edges anywhere in the (non-excluded) frame in either map,
// since there is nothing to disagree about.
func EdgeIoU(a, b *image.Gray, dilatePx int, exclude *image.Gray) (float64, error) {
	if a.Bounds().Dx() != b.Bounds().Dx() || a.Bounds().Dy() != b.Bounds().Dy() {
		return 0, fmt.Errorf("edge map size mismatch: %dx%d vs %dx%d",
			a.Bounds().Dx(), a.Bounds().Dy(), b.Bounds().Dx(), b.Bounds().Dy())
	}
	da := Dilate(a, dilatePx)
	db := Dilate(b, dilatePx)

	bounds := a.Bounds()
	var eb image.Rectangle
	if exclude != nil {
		eb = exclude.Bounds()
	}

	var inter, union int
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if exclude != nil {
				ex := eb.Min.X + (x - bounds.Min.X)
				ey := eb.Min.Y + (y - bounds.Min.Y)
				if isPainted(exclude, ex, ey) {
					continue
				}
			}
			ea := isPainted(da, x, y)
			ebv := isPainted(db, x, y)
			if ea && ebv {
				inter++
			}
			if ea || ebv {
				union++
			}
		}
	}
	if union == 0 {
		return 1.0, nil
	}
	return float64(inter) / float64(union), nil
}
