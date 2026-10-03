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

// resize must stay pixel-equivalent to the BiLinear.Scale call it replaced
// (which allocated a dstWidth x srcHeight float64 buffer), for every source
// type the decoders produce and for sources whose bounds do not start at 0.
func TestResizeMatchesKernelScale(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	fill := func(img draw.Image) {
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				a := uint8(255)
				if _, ok := img.(*image.NRGBA); ok {
					a = uint8(rng.IntN(256)) // exercise premultiplication
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
	fill(rgba)
	nrgba := image.NewNRGBA(image.Rect(0, 0, 190, 330))
	fill(nrgba)
	gray := image.NewGray(image.Rect(0, 0, 222, 111))
	fill(gray)

	for _, src := range []image.Image{ycc, rgba, nrgba, gray} {
		for _, scale := range []float64{0.5, 0.37, 0.1} {
			got := resize(src, scale).(*image.RGBA)
			want := image.NewRGBA(got.Bounds())
			xdraw.BiLinear.Scale(want, want.Bounds(), src, src.Bounds(), draw.Src, nil)
			maxDiff := 0
			for i := range got.Pix {
				d := int(got.Pix[i]) - int(want.Pix[i])
				maxDiff = max(maxDiff, d, -d)
			}
			if maxDiff > 1 {
				t.Errorf("%T scale %.2f: max channel difference %d, want <= 1", src, scale, maxDiff)
			}
		}
	}
	if resize(rgba, 1) != image.Image(rgba) {
		t.Error("scale >= 1 must return the source unchanged")
	}
}
