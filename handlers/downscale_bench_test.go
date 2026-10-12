// SPDX-License-Identifier: MIT

package handlers

import (
	"image"
	"io"
	"runtime"
	"testing"

	"github.com/SeriousBug/webp-go-pure/std"
)

// benchImage builds a synthetic photo-like source of the requested size. The
// benchmark needs a raw, uncompressed source, so it never measures a decoder
// it is not testing, and the pixels are identical on every machine.
func benchImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := range w {
			row[x*4+0] = uint8(x * 255 / w)
			row[x*4+1] = uint8(y * 255 / h)
			row[x*4+2] = uint8((x ^ y) & 255)
			row[x*4+3] = 255
		}
	}
	return img
}

// BenchmarkResize12MPThumbnail measures fitting the admin preview: the
// dominant CPU cost of an upload, and the reason the box filter exists.
func BenchmarkResize12MPThumbnail(b *testing.B) {
	img := benchImage(4128, 3072)
	scale := min(640.0/float64(4128), 360.0/float64(3072))
	b.ReportAllocs()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(resize(img, scale))
	}
}

// BenchmarkResizeKernel12MPThumbnail keeps the previous implementation
// measured next to the box filter, so the gain stays visible in future runs.
func BenchmarkResizeKernel12MPThumbnail(b *testing.B) {
	img := benchImage(4128, 3072)
	b.ReportAllocs()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(resizeKernel(img, 640, 360))
	}
}

// BenchmarkResize24MP1080 measures the compression path on a large upload.
func BenchmarkResize24MP1080(b *testing.B) {
	img := benchImage(6000, 4000)
	b.ReportAllocs()
	b.SetBytes(int64(len(img.Pix)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(resize(img, 1080.0/4000.0))
	}
}

// BenchmarkUploadPipeline12MP measures what a non-lossless upload costs after
// the decode: preview resize, WebP encode, compression resize and JPEG encode.
func BenchmarkUploadPipeline12MP(b *testing.B) {
	img := benchImage(4128, 3072)
	previewScale := min(640.0/float64(4128), 360.0/float64(3072))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		preview := resize(img, previewScale)
		if err := webp.Encode(io.Discard, preview, &webp.Options{Quality: 80, Effort: webpEffort}); err != nil {
			b.Fatal(err)
		}
		stored := resize(img, 1080.0/3072.0)
		if err := encodeImage(io.Discard, stored, "jpg", 85); err != nil {
			b.Fatal(err)
		}
	}
}

// TestBenchImageIsUsable guards the fixture above: a flat image would let the
// resampler and encoder benchmarks silently measure almost nothing.
func TestBenchImageIsUsable(t *testing.T) {
	img := benchImage(4128, 3072)
	colours := map[uint32]bool{}
	for y := 0; y < img.Bounds().Dy(); y += 97 {
		for x := 0; x < img.Bounds().Dx(); x += 89 {
			r, g, b, _ := img.At(x, y).RGBA()
			colours[r^g^b] = true
		}
	}
	if len(colours) < 50 {
		t.Fatalf("benchmark fixture is nearly flat: %d distinct colours", len(colours))
	}
}
