package geometry

import "image"

// Resize scales img to exactly w x h using nearest-neighbor sampling. It is
// used to bring a render result (whose pixel dimensions follow the requested
// "1K"/"2K"/"4K" output size, not the original screenshot) back to the
// screenshot's resolution so their edge maps can be compared pixel-for-pixel
// in the preservation check.
func Resize(img image.Image, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return out
	}
	src := img.Bounds()
	sw, sh := src.Dx(), src.Dy()
	if sw <= 0 || sh <= 0 {
		return out
	}
	for y := 0; y < h; y++ {
		sy := src.Min.Y + y*sh/h
		for x := 0; x < w; x++ {
			sx := src.Min.X + x*sw/w
			out.Set(x, y, img.At(sx, sy))
		}
	}
	return out
}
