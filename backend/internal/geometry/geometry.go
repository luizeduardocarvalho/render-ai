// Package geometry implements the pure-Go (no cgo, no OpenCV) image
// processing used to assemble a render: compositing the region map and
// computing edge maps / edge IoU for the preservation check.
package geometry

import "image"

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// isPainted reports whether a mask-bitmap pixel counts as "painted" (white),
// using the white-on-black convention shared by masks, edge maps and the
// preservation exclusion region.
func isPainted(gray *image.Gray, x, y int) bool {
	return gray.GrayAt(x, y).Y >= 128
}
