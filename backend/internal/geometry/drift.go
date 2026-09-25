package geometry

import (
	"image"
	"math"
)

const (
	// driftBlockDivisor sets the block size RegionDrift averages over: the
	// image's shorter side divided by this (48px on a 1536px-high render), and
	// never under driftMinBlock. Blocks are big enough that fine texture (grass
	// blades, individual stones) averages out and only the color and tone of an
	// area is compared.
	driftBlockDivisor = 32
	driftMinBlock     = 4
	// driftMinCoverage is the share of a block that must lie inside the mask for
	// the block to count.
	driftMinCoverage = 0.5
	// driftChangedBlock is the drift from which a block counts as changed: about
	// halfway between the same look (0 to 6) and another material (15+).
	driftChangedBlock = 12.0
)

// RegionDrift measures how much the look of the masked area changed between
// before and after (same size as mask). It cuts the area into blocks and, for
// each, takes the CIELAB distance between the block's average color before and
// after, with lightness at half weight. Working on block averages means fine
// texture does not count, only what the area is like. It returns the mean of
// those distances and changedShare, the share of blocks at or above
// driftChangedBlock; the share is what shows a change that covers only part of
// the area, which the mean dilutes. ok is false if the mask holds no block.
//
// Roughly, a mean of 0 to 6 is the same look (grass made more realistic grass)
// and a block of 15 or more is a different material or color (grass turned to
// pebbles). A change meant to alter color or material, such as painting a wall
// green, scores high by design, so this says how much a region changed, not
// whether that was right.
func RegionDrift(before, after image.Image, mask *image.Gray) (drift, changedShare float64, ok bool) {
	mb := mask.Bounds()
	minX, minY, maxX, maxY := mb.Max.X, mb.Max.Y, mb.Min.X-1, mb.Min.Y-1
	for y := mb.Min.Y; y < mb.Max.Y; y++ {
		for x := mb.Min.X; x < mb.Max.X; x++ {
			if isPainted(mask, x, y) {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if maxX < minX {
		return 0, 0, false
	}
	bw, bh := maxX-minX+1, maxY-minY+1
	block := max(driftMinBlock, min(mb.Dx(), mb.Dy())/driftBlockDivisor)
	block = min(block, min(bw, bh)) // a region smaller than a block still gets one

	sum, n, changed := 0.0, 0, 0
	for by := minY; by+block <= maxY+1; by += block {
		for bx := minX; bx+block <= maxX+1; bx += block {
			var rb, gb, bb, ra, ga, ba float64
			painted := 0
			for y := by; y < by+block; y++ {
				for x := bx; x < bx+block; x++ {
					if !isPainted(mask, x, y) {
						continue
					}
					painted++
					r, g, b, _ := before.At(before.Bounds().Min.X+x-mb.Min.X, before.Bounds().Min.Y+y-mb.Min.Y).RGBA()
					rb, gb, bb = rb+float64(r>>8), gb+float64(g>>8), bb+float64(b>>8)
					r, g, b, _ = after.At(after.Bounds().Min.X+x-mb.Min.X, after.Bounds().Min.Y+y-mb.Min.Y).RGBA()
					ra, ga, ba = ra+float64(r>>8), ga+float64(g>>8), ba+float64(b>>8)
				}
			}
			if float64(painted) < driftMinCoverage*float64(block*block) {
				continue
			}
			p := float64(painted)
			l1, a1, b1 := RGBToLab(uint8(rb/p+0.5), uint8(gb/p+0.5), uint8(bb/p+0.5))
			l2, a2, b2 := RGBToLab(uint8(ra/p+0.5), uint8(ga/p+0.5), uint8(ba/p+0.5))
			d := math.Sqrt(0.25*(l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
			sum += d
			n++
			if d >= driftChangedBlock {
				changed++
			}
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return sum / float64(n), float64(changed) / float64(n), true
}
