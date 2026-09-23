package render

import "math"

// supportedAspectRatios are the values accepted by genai.ImageConfig.AspectRatio.
var supportedAspectRatios = []struct {
	label string
	ratio float64
}{
	{"1:1", 1.0 / 1.0},
	{"2:3", 2.0 / 3.0},
	{"3:2", 3.0 / 2.0},
	{"3:4", 3.0 / 4.0},
	{"4:3", 4.0 / 3.0},
	{"9:16", 9.0 / 16.0},
	{"16:9", 16.0 / 9.0},
	{"21:9", 21.0 / 9.0},
}

// PickAspectRatio returns the supported aspect ratio label closest to the
// screenshot's own width/height ratio.
func PickAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return "16:9"
	}
	target := float64(width) / float64(height)
	best := supportedAspectRatios[0]
	bestDiff := math.Abs(target - best.ratio)
	for _, ar := range supportedAspectRatios[1:] {
		if diff := math.Abs(target - ar.ratio); diff < bestDiff {
			best, bestDiff = ar, diff
		}
	}
	return best.label
}
