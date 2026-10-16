// SPDX-License-Identifier: MIT

package handlers

import (
	"image"
	"runtime"
	"testing"
)

// BenchmarkApplyOrientation12MP measures the turn a phone photo needs, on the
// two shapes a decoder produces, plus the obvious implementation beside them.
// It is the reason applyOrientation permutes a flat pixel buffer and buffers
// one row at a time for a subsampled source, instead of reading every pixel
// through image.Image.
func BenchmarkApplyOrientation12MP(b *testing.B) {
	rgba := benchImage(4128, 3072)
	b.Run("RGBA buffer permutation", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(rgba.Pix)))
		for b.Loop() {
			runtime.KeepAlive(applyOrientation(rgba, orientationRot90))
		}
	})

	// A JPEG decodes to subsampled YCbCr, which has no flat pixel buffer: this
	// is the path the converter row buffer exists for.
	ycbcr := image.NewYCbCr(image.Rect(0, 0, 4128, 3072), image.YCbCrSubsampleRatio420)
	for i := range ycbcr.Y {
		ycbcr.Y[i] = uint8(i)
	}
	b.Run("YCbCr row conversion", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(ycbcr.Y) + len(ycbcr.Cb) + len(ycbcr.Cr)))
		for b.Loop() {
			runtime.KeepAlive(applyOrientation(ycbcr, orientationRot90))
		}
	})

	// The obvious implementation, kept here as the yardstick: read every pixel
	// through the image.Image interface and write it where the orientation puts
	// it. It is not used: it is the slowest of the three and the only one that
	// allocates on every pixel.
	b.Run("per-pixel copy through image.Image", func(b *testing.B) {
		dst := image.NewRGBA(image.Rect(0, 0, 3072, 4128))
		b.ReportAllocs()
		b.SetBytes(int64(len(rgba.Pix)))
		for b.Loop() {
			for y := range 3072 {
				for x := range 4128 {
					dst.Set(x, y, rgba.At(x, y))
				}
			}
		}
	})
}
