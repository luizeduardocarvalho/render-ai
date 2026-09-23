package geometry

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

// regionOpacity is the fill opacity used when painting a mask's region onto
// the region map, per the API contract ("~60% opacity").
const regionOpacity = 0.6

// Region is one non-hidden mask to composite into the region map: its
// white-on-black bitmap (at screenshot resolution) and its asset's display
// color.
type Region struct {
	Bitmap image.Image
	Color  color.RGBA
}

// CompositeRegionMap draws each region filled in its asset's color at
// regionOpacity over a copy of base, and returns the result. base and every
// region bitmap must share the same pixel dimensions.
func CompositeRegionMap(base image.Image, regions []Region) (*image.NRGBA, error) {
	bounds := base.Bounds()
	out := image.NewNRGBA(bounds)
	draw.Draw(out, bounds, base, bounds.Min, draw.Src)

	for i, r := range regions {
		mb := r.Bitmap.Bounds()
		if mb.Dx() != bounds.Dx() || mb.Dy() != bounds.Dy() {
			return nil, fmt.Errorf("region %d: mask bitmap size %dx%d does not match screenshot size %dx%d",
				i, mb.Dx(), mb.Dy(), bounds.Dx(), bounds.Dy())
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			my := mb.Min.Y + (y - bounds.Min.Y)
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				mx := mb.Min.X + (x - bounds.Min.X)
				gray := color.GrayModel.Convert(r.Bitmap.At(mx, my)).(color.Gray)
				if gray.Y < 128 {
					continue
				}
				bg := out.NRGBAAt(x, y)
				out.SetNRGBA(x, y, blend(bg, r.Color, regionOpacity))
			}
		}
	}
	return out, nil
}

func blend(bg color.NRGBA, fg color.RGBA, alpha float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(clampFloat(float64(bg.R)*(1-alpha)+float64(fg.R)*alpha, 0, 255)),
		G: uint8(clampFloat(float64(bg.G)*(1-alpha)+float64(fg.G)*alpha, 0, 255)),
		B: uint8(clampFloat(float64(bg.B)*(1-alpha)+float64(fg.B)*alpha, 0, 255)),
		A: 255,
	}
}

// UnionMask ORs together several white-on-black bitmaps (all assumed to
// share bounds' dimensions) into a single white-on-black bitmap. Used to
// build the "exclude masked regions" input for the preservation edge IoU.
func UnionMask(bounds image.Rectangle, bitmaps []image.Image) *image.Gray {
	out := image.NewGray(bounds)
	for _, bm := range bitmaps {
		bb := bm.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			by := bb.Min.Y + (y - bounds.Min.Y)
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				bx := bb.Min.X + (x - bounds.Min.X)
				gray := color.GrayModel.Convert(bm.At(bx, by)).(color.Gray)
				if gray.Y >= 128 {
					out.SetGray(x, y, color.Gray{Y: 255})
				}
			}
		}
	}
	return out
}
