// SPDX-License-Identifier: MIT

package handlers

import (
	"image"
	"image/color"
	"image/draw"
	"math/rand/v2"
	"testing"

	xdraw "golang.org/x/image/draw"
)

// randomImages returns one filled image per type the pipeline can meet.
func randomImages() []image.Image {
	rng := rand.New(rand.NewPCG(1, 2))
	fill := func(img draw.Image, withAlpha bool) {
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				a := uint8(255)
				if withAlpha {
					a = uint8(rng.IntN(256))
				}
				img.Set(x, y, color.NRGBA{uint8(rng.IntN(256)), uint8(rng.IntN(256)), uint8(rng.IntN(256)), a})
			}
		}
	}
	ycc := image.NewYCbCr(image.Rect(0, 0, 301, 203), image.YCbCrSubsampleRatio420)
	for i := range ycc.Y {
		ycc.Y[i] = uint8(rng.IntN(256))
	}
	for i := range ycc.Cb {
		ycc.Cb[i], ycc.Cr[i] = uint8(rng.IntN(256)), uint8(rng.IntN(256))
	}
	rgba := image.NewRGBA(image.Rect(7, 11, 7+257, 11+181)) // non-zero origin
	fill(rgba, true)
	nrgba := image.NewNRGBA(image.Rect(0, 0, 190, 330))
	fill(nrgba, true)
	gray := image.NewGray(image.Rect(0, 0, 222, 111))
	fill(gray, false)
	paletted := image.NewPaletted(image.Rect(0, 0, 140, 90), color.Palette{
		color.RGBA{0, 0, 0, 255}, color.RGBA{255, 255, 255, 255},
		color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 128},
	})
	for i := range paletted.Pix {
		paletted.Pix[i] = uint8(rng.IntN(len(paletted.Palette)))
	}
	gray16 := image.NewGray16(image.Rect(0, 0, 99, 77))
	for i := range gray16.Pix {
		gray16.Pix[i] = uint8(rng.IntN(256))
	}
	nrgba64 := image.NewNRGBA64(image.Rect(0, 0, 88, 66))
	fill(nrgba64, true)
	return []image.Image{ycc, rgba, nrgba, gray, paletted, gray16, nrgba64}
}

// boxImage reports whether the box filter handles the type itself. The kernel
// fallback covers the rest, which must keep working.
func boxImage(img image.Image) bool {
	switch img.(type) {
	case *image.YCbCr, *image.RGBA, *image.NRGBA, *image.Gray:
		return true
	}
	return false
}

// TestResizeBoundsAndPassthrough checks the two contracts every caller relies
// on: the destination size, and that a scale of 1 or more returns the source
// unchanged (no upscale, no copy).
func TestResizeBoundsAndPassthrough(t *testing.T) {
	for _, src := range randomImages() {
		b := src.Bounds()
		for _, scale := range []float64{0.9, 0.5, 0.1} {
			got := resize(src, scale)
			want := image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale)))
			if got.Bounds() != want {
				t.Errorf("%T scale %.2f: bounds %v, want %v", src, scale, got.Bounds(), want)
			}
		}
		for _, scale := range []float64{1, 1.5} {
			if got := resize(src, scale); got != image.Image(src) {
				t.Errorf("%T scale %.2f: got a new image instead of the source", src, scale)
			}
		}
	}
}

// TestBoxResizeUniform checks that a flat colour survives a downscale
// untouched, alpha included.
func TestBoxResizeUniform(t *testing.T) {
	uniform := color.NRGBA{10, 20, 30, 128}
	sources := []image.Image{
		image.NewYCbCr(image.Rect(0, 0, 200, 120), image.YCbCrSubsampleRatio420),
		image.NewRGBA(image.Rect(0, 0, 200, 120)),
		image.NewNRGBA(image.Rect(0, 0, 200, 120)),
		image.NewGray(image.Rect(0, 0, 200, 120)),
	}
	for i := range sources[0].(*image.YCbCr).Y {
		sources[0].(*image.YCbCr).Y[i] = 200
	}
	for i := range sources[0].(*image.YCbCr).Cb {
		sources[0].(*image.YCbCr).Cb[i], sources[0].(*image.YCbCr).Cr[i] = 100, 150
	}
	draw.Draw(sources[1].(*image.RGBA), sources[1].Bounds(), image.NewUniform(uniform), image.Point{}, draw.Src)
	draw.Draw(sources[2].(*image.NRGBA), sources[2].Bounds(), image.NewUniform(uniform), image.Point{}, draw.Src)
	draw.Draw(sources[3].(*image.Gray), sources[3].Bounds(), image.NewUniform(color.Gray{77}), image.Point{}, draw.Src)

	for _, src := range sources {
		for _, scale := range []float64{0.9, 0.5, 0.13} {
			got := resize(src, scale)
			b := got.Bounds()
			wr, wg, wb, wa := src.At(src.Bounds().Min.X, src.Bounds().Min.Y).RGBA()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r, g, bl, a := got.At(x, y).RGBA()
					if absDiff(r, wr) > 0x101 || absDiff(g, wg) > 0x101 || absDiff(bl, wb) > 0x101 || absDiff(a, wa) > 0x101 {
						t.Fatalf("%T scale %.2f: pixel (%d,%d) = %d,%d,%d,%d, want %d,%d,%d,%d",
							src, scale, x, y, r, g, bl, a, wr, wg, wb, wa)
					}
				}
			}
		}
	}
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestBoxYCbCrAveragesEachPlane compares the fast plane-average path with the
// definition it implements: every destination pixel is the mean of the source
// rectangle behind it, addressed here independently through YStride/CStride.
func TestBoxYCbCrAveragesEachPlane(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 301, 203), image.YCbCrSubsampleRatio420)
	rng := rand.New(rand.NewPCG(3, 4))
	for i := range src.Y {
		src.Y[i] = uint8(rng.IntN(256))
	}
	for i := range src.Cb {
		src.Cb[i], src.Cr[i] = uint8(rng.IntN(256)), uint8(rng.IntN(256))
	}
	for _, scale := range []float64{0.9, 0.5, 0.2} {
		b := src.Bounds()
		w, h := max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))
		got := resize(src, scale).(*image.YCbCr)
		if got.SubsampleRatio != src.SubsampleRatio {
			t.Fatalf("scale %.2f: subsample ratio changed to %v", scale, got.SubsampleRatio)
		}
		planes := []struct {
			name           string
			src, dst       []byte
			sw, sh, dw, dh int
			srcStride      int
			dstStride      int
		}{
			{"Y", src.Y, got.Y, b.Dx(), b.Dy(), w, h, src.YStride, got.YStride},
			{"Cb", src.Cb, got.Cb, (b.Dx() + 1) / 2, (b.Dy() + 1) / 2, (w + 1) / 2, (h + 1) / 2, src.CStride, got.CStride},
			{"Cr", src.Cr, got.Cr, (b.Dx() + 1) / 2, (b.Dy() + 1) / 2, (w + 1) / 2, (h + 1) / 2, src.CStride, got.CStride},
		}
		for _, p := range planes {
			for y := range p.dh {
				for x := range p.dw {
					sx0 := x * p.sw / p.dw
					sx1 := max(sx0+1, (x+1)*p.sw/p.dw)
					sy0 := y * p.sh / p.dh
					sy1 := max(sy0+1, (y+1)*p.sh/p.dh)
					sum, n := 0, 0
					for sy := sy0; sy < sy1; sy++ {
						for sx := sx0; sx < sx1; sx++ {
							sum += int(p.src[sy*p.srcStride+sx])
							n++
						}
					}
					if want := byte((sum + n/2) / n); p.dst[y*p.dstStride+x] != want {
						t.Fatalf("scale %.2f %s plane (%d,%d): got %d, want %d", scale, p.name, x, y, p.dst[y*p.dstStride+x], want)
					}
				}
			}
		}
	}
}

// premultiplied draws any image into an *image.RGBA, which is the colour space
// image.At().RGBA() reports in.
func premultiplied(src image.Image) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// referenceBox averages the source rectangle through image.At, an obviously
// correct implementation the fast paths are compared against.
func referenceBox(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		sy0 := y * b.Dy() / h
		sy1 := max(sy0+1, (y+1)*b.Dy()/h)
		for x := range w {
			sx0 := x * b.Dx() / w
			sx1 := max(sx0+1, (x+1)*b.Dx()/w)
			var sr, sg, sb, sa, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, bl, a := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					sr, sg, sb, sa = sr+uint64(r), sg+uint64(g), sb+uint64(bl), sa+uint64(a)
					n++
				}
			}
			dst.SetRGBA64(x, y, color.RGBA64{
				uint16((sr + n/2) / n), uint16((sg + n/2) / n),
				uint16((sb + n/2) / n), uint16((sa + n/2) / n)})
		}
	}
	return dst
}

// TestBoxResizeMatchesReference checks the fast RGBA/NRGBA/Gray paths against
// referenceBox. The arithmetic is identical up to rounding, so one level of
// difference per channel is allowed, and the mean must be far below that.
func TestBoxResizeMatchesReference(t *testing.T) {
	for _, src := range randomImages() {
		if !boxImage(src) || isYCbCr(src) {
			continue // YCbCr has its own plane-exact test above
		}
		b := src.Bounds()
		for _, scale := range []float64{0.9, 0.5, 0.2} {
			w, h := max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))
			got := premultiplied(resize(src, scale))
			want := referenceBox(src, w, h)
			var sum, maxDiff, n int
			for i := range want.Pix {
				d := absDiff(uint32(got.Pix[i]), uint32(want.Pix[i]))
				sum += int(d)
				n++
				if int(d) > maxDiff {
					maxDiff = int(d)
				}
			}
			mean := float64(sum) / float64(n)
			if mean > 0.6 {
				t.Errorf("%T scale %.2f: mean channel difference %.2f, want <= 0.6", src, scale, mean)
			}
			if maxDiff > 2 {
				t.Errorf("%T scale %.2f: max channel difference %d, want <= 2", src, scale, maxDiff)
			}
		}
	}
}

func isYCbCr(img image.Image) bool {
	_, ok := img.(*image.YCbCr)
	return ok
}

// TestBoxResizeTransparency checks that a partly transparent edge keeps its
// colour: a half-opaque red next to fully transparent pixels must stay red at
// half alpha. Averaging the stored (straight) channels instead would average
// the red with the transparent pixels' colour and darken it.
func TestBoxResizeTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			a := uint8(255)
			if x >= 2 {
				a = 0
			}
			src.SetNRGBA(x, y, color.NRGBA{R: 255, A: a})
		}
	}
	got := resize(src, 0.25).(*image.NRGBA)
	if b := got.Bounds(); b.Dx() != 1 || b.Dy() != 1 {
		t.Fatalf("bounds %v, want 1x1", b)
	}
	c := got.NRGBAAt(0, 0)
	if c.R < 254 || c.G != 0 || c.B != 0 {
		t.Fatalf("colour %+v, want opaque red: the transparent half must not tint the result", c)
	}
	if c.A < 127 || c.A > 128 {
		t.Fatalf("alpha %d, want ~127", c.A)
	}
}

// TestBoxResizeSubImage checks that a source whose bounds do not start at the
// origin is reduced like the same pixels copied to the origin. Every plane of
// an image is indexed from its own top-left corner, so a cropped source is a
// regression trap for a filter that works on raw pixels.
func TestBoxResizeSubImage(t *testing.T) {
	const w, h = 120, 120
	crop := image.Rect(16, 24, 104, 88) // even offsets: chroma stays aligned
	rng := rand.New(rand.NewPCG(5, 6))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range rgba.Pix {
		rgba.Pix[i] = uint8(rng.IntN(256))
	}
	gray := image.NewGray(image.Rect(0, 0, w, h))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(rng.IntN(256))
	}
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for i := range ycc.Y {
		ycc.Y[i] = uint8(rng.IntN(256))
	}
	for i := range ycc.Cb {
		ycc.Cb[i], ycc.Cr[i] = uint8(rng.IntN(256)), uint8(rng.IntN(256))
	}

	// Each case reduces a cropped source and an equivalent image built at the
	// origin, because plane indexing differs between the two.
	compare := func(name string, cropped, copied image.Image) {
		t.Helper()
		got := premultiplied(resize(cropped, 0.5))
		want := premultiplied(resize(copied, 0.5))
		if got.Bounds() != want.Bounds() {
			t.Fatalf("%s: bounds %v vs %v", name, got.Bounds(), want.Bounds())
		}
		for i := range want.Pix {
			if got.Pix[i] != want.Pix[i] {
				t.Fatalf("%s: pixel byte %d is %d for the cropped source and %d for the copy",
					name, i, got.Pix[i], want.Pix[i])
			}
		}
	}

	// RGBA and Gray copies keep the pixel values, so the comparison is exact.
	rgbaCopy := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(rgbaCopy, rgbaCopy.Bounds(), rgba, crop.Min, draw.Src)
	compare("rgba", rgba.SubImage(crop), rgbaCopy)

	grayCopy := image.NewGray(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(grayCopy, grayCopy.Bounds(), gray, crop.Min, draw.Src)
	compare("gray", gray.SubImage(crop), grayCopy)

	// A YCbCr copy is built plane by plane: copying it into an RGBA image would
	// clamp the deliberately out-of-range random chroma values and hide the
	// difference this test looks for.
	sub := ycc.SubImage(crop).(*image.YCbCr)
	yccCopy := image.NewYCbCr(image.Rect(0, 0, crop.Dx(), crop.Dy()), ycc.SubsampleRatio)
	for y := range crop.Dy() {
		copy(yccCopy.Y[y*yccCopy.YStride:], sub.Y[y*sub.YStride:y*sub.YStride+crop.Dx()])
	}
	for y := range (crop.Dy() + 1) / 2 {
		copy(yccCopy.Cb[y*yccCopy.CStride:], sub.Cb[y*sub.CStride:(y*sub.CStride)+(crop.Dx()+1)/2])
		copy(yccCopy.Cr[y*yccCopy.CStride:], sub.Cr[y*sub.CStride:(y*sub.CStride)+(crop.Dx()+1)/2])
	}
	compare("ycbcr", sub, yccCopy)
}

// TestResizeKernelFallback keeps the kernel path honest for the image types the
// box filter does not know: the result must match BiLinear.Scale, which is what
// the previous implementation used, to within one level of rounding.
func TestResizeKernelFallback(t *testing.T) {
	var checked int
	for _, src := range randomImages() {
		if boxImage(src) {
			continue
		}
		checked++
		for _, scale := range []float64{0.9, 0.5, 0.37, 0.1} {
			got := resize(src, scale).(*image.RGBA)
			want := image.NewRGBA(got.Bounds())
			xdraw.BiLinear.Scale(want, want.Bounds(), src, src.Bounds(), draw.Src, nil)
			maxDiff := 0
			for i := range got.Pix {
				d := absDiff(uint32(got.Pix[i]), uint32(want.Pix[i]))
				maxDiff = max(maxDiff, int(d))
			}
			if maxDiff > 1 {
				t.Errorf("%T scale %.2f: max channel difference %d, want <= 1", src, scale, maxDiff)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source type exercised the kernel fallback")
	}
}
