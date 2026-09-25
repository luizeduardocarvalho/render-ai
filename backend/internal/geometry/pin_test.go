package geometry

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// pinScene is a smooth, broad "room" - a lit gradient with a dark object in it -
// at the given size. It carries all its information in low frequencies, so any
// size of it shows the same picture.
func pinScene(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			c := color.NRGBA{R: uint8(90 + 100*fx), G: uint8(70 + 90*fy), B: uint8(60 + 40*fx*fy), A: 255}
			if fx > 0.3 && fx < 0.55 && fy > 0.4 && fy < 0.8 {
				c = color.NRGBA{R: 40, G: 30, B: 25, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// pinRedrawn is scene as a model might redraw it larger: the same picture with
// a colour cast (redder, less blue) and a fine checker of texture added.
func pinRedrawn(scene *image.NRGBA, w, h, texture int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	sw, sh := scene.Bounds().Dx(), scene.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := scene.NRGBAAt(x*sw/w, y*sh/h)
			t := texture
			if (x+y)%2 == 0 {
				t = -texture
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: clampByte(int(c.R) + 18 + t),
				G: clampByte(int(c.G) + t),
				B: clampByte(int(c.B) - 12 + t),
				A: 255,
			})
		}
	}
	return img
}

func clampByte(v int) uint8 { return uint8(min(max(v, 0), 255)) }

// meanRGB is the mean colour of the pixels of img inside r.
func meanRGB(img image.Image, r image.Rectangle) [3]float64 {
	var sum [3]float64
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			sum[0] += float64(c.R)
			sum[1] += float64(c.G)
			sum[2] += float64(c.B)
			n++
		}
	}
	for i := range sum {
		sum[i] /= float64(n)
	}
	return sum
}

// roughness is the mean absolute difference between horizontal neighbours: the
// amount of fine detail in the image.
func roughness(img image.Image) float64 {
	b := img.Bounds()
	var total float64
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x+1 < b.Max.X; x++ {
			l := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			r := color.NRGBAModel.Convert(img.At(x+1, y)).(color.NRGBA)
			total += math.Abs(float64(l.R)-float64(r.R)) + math.Abs(float64(l.G)-float64(r.G)) + math.Abs(float64(l.B)-float64(r.B))
			n += 3
		}
	}
	return total / float64(n)
}

func TestPinLowFrequencyRestoresColourAndKeepsDetail(t *testing.T) {
	ref := pinScene(400, 225)
	gen := pinRedrawn(ref, 800, 450, 12)
	detail := roughness(gen)

	// Everywhere the redrawn image must start out visibly off.
	regions := []image.Rectangle{
		image.Rect(20, 20, 200, 150),   // the open wall
		image.Rect(300, 200, 520, 380), // inside the dark object
		image.Rect(600, 40, 780, 400),  // the bright side
		image.Rect(100, 300, 400, 430), // the floor
	}
	for _, r := range regions {
		want := meanRGB(ref, image.Rect(r.Min.X/2, r.Min.Y/2, r.Max.X/2, r.Max.Y/2))
		if got := meanRGB(gen, r); math.Abs(got[0]-want[0]) < 10 {
			t.Fatalf("test setup: region %v is already within %.1f of the reference", r, math.Abs(got[0]-want[0]))
		}
	}

	out, err := PinLowFrequency(gen, ref)
	if err != nil {
		t.Fatalf("PinLowFrequency: %v", err)
	}

	for _, r := range regions {
		want := meanRGB(ref, image.Rect(r.Min.X/2, r.Min.Y/2, r.Max.X/2, r.Max.Y/2))
		got := meanRGB(out, r)
		for c, name := range []string{"red", "green", "blue"} {
			if d := math.Abs(got[c] - want[c]); d > 3 {
				t.Errorf("region %v mean %s = %.1f, want the reference's %.1f (off by %.1f)", r, name, got[c], want[c], d)
			}
		}
	}
	if got := roughness(out); math.Abs(got-detail) > detail*0.05 {
		t.Errorf("fine detail changed: roughness %.2f, was %.2f", got, detail)
	}
}

func TestPinLowFrequencyChangesTheImageInPlace(t *testing.T) {
	ref := pinScene(80, 45)
	nrgba := pinRedrawn(ref, 160, 90, 6)
	out, err := PinLowFrequency(nrgba, ref)
	if err != nil {
		t.Fatal(err)
	}
	if out != image.Image(nrgba) {
		t.Error("an *image.NRGBA should be pinned in place, not copied")
	}

	// A PNG of an opaque RGB render decodes to *image.RGBA.
	redrawn := pinRedrawn(ref, 160, 90, 6)
	rgba := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			c := redrawn.NRGBAAt(x, y)
			rgba.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
	}
	before := meanRGB(rgba, rgba.Bounds())
	out, err = PinLowFrequency(rgba, scaled(ref, 2))
	if err != nil {
		t.Fatal(err)
	}
	if out != image.Image(rgba) {
		t.Error("an *image.RGBA should be pinned in place, not copied")
	}
	if after := meanRGB(out, rgba.Bounds()); after == before {
		t.Error("the RGBA image was not changed")
	}
}

// scaled is img repeated n times in each direction.
func scaled(img *image.NRGBA, n int) *image.NRGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w*n, h*n))
	for y := 0; y < h*n; y++ {
		for x := 0; x < w*n; x++ {
			out.SetNRGBA(x, y, img.NRGBAAt(x/n, y/n))
		}
	}
	return out
}

func TestPinLowFrequencyConvertsOtherImageTypes(t *testing.T) {
	ref := pinScene(80, 45)
	gray := image.NewGray(image.Rect(0, 0, 160, 90))
	out, err := PinLowFrequency(gray, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.(*image.NRGBA); !ok {
		t.Errorf("returned %T, want a converted *image.NRGBA", out)
	}
	// The gray input is black; pinning it to the scene must bring it to the
	// scene's colours rather than leave it black.
	if got := meanRGB(out, out.Bounds()); got[0] < 50 {
		t.Errorf("mean red = %.1f, want the scene's (about 140)", got[0])
	}
}

func TestPinLowFrequencySaturatesInsteadOfWrapping(t *testing.T) {
	ref := flatNRGBA(64, 36, color.NRGBA{R: 250, G: 250, B: 250, A: 255})
	gen := flatNRGBA(64, 36, color.NRGBA{R: 20, G: 20, B: 20, A: 255})
	out, err := PinLowFrequency(gen, ref)
	if err != nil {
		t.Fatal(err)
	}
	if c := color.NRGBAModel.Convert(out.At(32, 18)).(color.NRGBA); c.R < 245 {
		t.Errorf("pixel = %v, want the reference's 250", c)
	}

	// And the other way, where a wrap would land near 255.
	out, err = PinLowFrequency(flatNRGBA(64, 36, color.NRGBA{R: 250, G: 250, B: 250, A: 255}), flatNRGBA(64, 36, color.NRGBA{R: 5, G: 5, B: 5, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	if c := color.NRGBAModel.Convert(out.At(32, 18)).(color.NRGBA); c.R > 10 {
		t.Errorf("pixel = %v, want the reference's 5", c)
	}
}

func flatNRGBA(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestPinLowFrequencyRejectsAnEmptyImage(t *testing.T) {
	if _, err := PinLowFrequency(image.NewNRGBA(image.Rectangle{}), pinScene(8, 8)); err == nil {
		t.Error("want an error for an empty image")
	}
	if _, err := PinLowFrequency(pinScene(8, 8), image.NewNRGBA(image.Rectangle{})); err == nil {
		t.Error("want an error for an empty reference")
	}
}
