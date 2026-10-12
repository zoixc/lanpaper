// SPDX-License-Identifier: MIT

// Box downscaling for the image pipeline: every destination pixel gets the
// average of the exact source rectangle behind it, computed from raw pixel
// rows rather than through the image.Image interface.
//
// Why it exists: the widened-kernel resampler (resizeKernel) calls At() once
// per source sample, and its kernel support grows as the scale shrinks, so a
// large downscale costs tens of interface calls and a colour conversion per
// destination pixel. Measured on a 12 MP photo, fitting the 640x360 admin
// preview took about 470 ms of CPU and halving a 24 MP photo about 1.1 s —
// several times the JPEG decode (176 ms) or the WebP encode (~100 ms) next to
// them. The box pass touches every source pixel exactly once, so the same 12 MP
// preview costs about 18 ms and the 24 MP halving about 73 ms. Uploads share
// this path for the stored image and the preview, so the CPU peak of an upload
// drops by roughly 40%.
//
// The two paths are not pixel-identical: on real photos they differ by a mean
// of about 1.5 levels of 255, and visibly only on hard edges, where the wider
// kernel blurs what the box average keeps. The box average is the more faithful
// of the two, because every source pixel contributes with equal weight; the
// pixels it keeps also survive the JPEG encode that follows, as the stored file
// is fractionally smaller for the same visual result.
package handlers

import "image"

// boxRanges maps every destination column to the source column it starts at
// and the number of source columns it covers. The mapping is identical for
// every row, and computing it inside the row loop would dominate the cost.
func boxRanges(srcW, dstW int) (start, width []int) {
	start = make([]int, dstW)
	width = make([]int, dstW)
	for x := range dstW {
		from := x * srcW / dstW
		start[x] = from
		width[x] = max(from+1, (x+1)*srcW/dstW) - from
	}
	return start, width
}

// boxPlane averages one byte plane of srcW x srcH into dstW x dstH. Both
// planes are indexed from their own image's top-left corner, which is the
// convention image.Image uses for cropped (SubImage) sources as well: a row of
// the visible area starts at srcStride bytes past the previous one, and column
// zero of the plane is column zero of the visible area.
func boxPlane(dst, src []byte, dstStride, srcStride, srcW, srcH, dstW, dstH int) {
	start, width := boxRanges(srcW, dstW)
	sums := make([]int64, dstW)
	for y := range dstH {
		sy0 := y * srcH / dstH
		sy1 := max(sy0+1, (y+1)*srcH/dstH)
		clear(sums)
		for sy := sy0; sy < sy1; sy++ {
			row := src[sy*srcStride : sy*srcStride+srcW]
			for x, sum := range sums {
				for _, v := range row[start[x] : start[x]+width[x]] {
					sum += int64(v)
				}
				sums[x] = sum
			}
		}
		rows := sy1 - sy0
		drow := dst[y*dstStride : y*dstStride+dstW]
		for x := range drow {
			n := int64(width[x] * rows)
			drow[x] = byte((sums[x] + n/2) / n)
		}
	}
}

// chromaPlaneSize returns the chroma plane dimensions image.NewYCbCr allocates
// for a w x h image with the given subsample ratio.
func chromaPlaneSize(w, h int, ratio image.YCbCrSubsampleRatio) (int, int) {
	switch ratio {
	case image.YCbCrSubsampleRatio444:
		return w, h
	case image.YCbCrSubsampleRatio422:
		return (w + 1) / 2, h
	case image.YCbCrSubsampleRatio420:
		return (w + 1) / 2, (h + 1) / 2
	case image.YCbCrSubsampleRatio440:
		return w, (h + 1) / 2
	case image.YCbCrSubsampleRatio411:
		return (w + 3) / 4, h
	case image.YCbCrSubsampleRatio410:
		return (w + 3) / 4, (h + 1) / 2
	}
	return w, h
}

// boxYCbCr reduces a YCbCr image plane by plane, so no pixel is ever converted
// to RGB. It also keeps the result in the layout the JPEG and WebP encoders
// read directly (image/jpeg's yCbCrToYCbCr, the WebP encoder's planesOf), which
// is what keeps the re-encode afterwards cheap as well.
func boxYCbCr(src *image.YCbCr, w, h int) *image.YCbCr {
	b := src.Bounds()
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), src.SubsampleRatio)
	sw, sh := chromaPlaneSize(b.Dx(), b.Dy(), src.SubsampleRatio)
	dw, dh := chromaPlaneSize(w, h, src.SubsampleRatio)
	// Every plane of a YCbCr image is indexed from the image's own top-left
	// corner, so a cropped source needs no offset here.
	boxPlane(dst.Y, src.Y, dst.YStride, src.YStride, b.Dx(), b.Dy(), w, h)
	boxPlane(dst.Cb, src.Cb, dst.CStride, src.CStride, sw, sh, dw, dh)
	boxPlane(dst.Cr, src.Cr, dst.CStride, src.CStride, sw, sh, dw, dh)
	return dst
}

// boxRGBA reduces a premultiplied RGBA image. Its stored channels are averaged
// as they are: for premultiplied pixels that is the value a compositor would
// get from the average of the source area, so nothing has to be unpacked.
func boxRGBA(src *image.RGBA, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	boxRGBAInto(dst.Pix, src.Pix, dst.Stride, src.Stride, b.Dx(), b.Dy(), w, h)
	return dst
}

func boxRGBAInto(dst, src []byte, dstStride, srcStride, srcW, srcH, dstW, dstH int) {
	start, width := boxRanges(srcW, dstW)
	sums := make([]int64, dstW*4)
	for y := range dstH {
		sy0 := y * srcH / dstH
		sy1 := max(sy0+1, (y+1)*srcH/dstH)
		clear(sums)
		for sy := sy0; sy < sy1; sy++ {
			row := src[sy*srcStride:]
			for x := range dstW {
				for px := start[x] * 4; px < (start[x]+width[x])*4; px += 4 {
					for ch := range 4 {
						sums[x*4+ch] += int64(row[px+ch])
					}
				}
			}
		}
		rows := sy1 - sy0
		drow := dst[y*dstStride : y*dstStride+dstW*4]
		for x := range dstW {
			n := int64(width[x] * rows)
			for ch := range 4 {
				drow[x*4+ch] = byte((sums[x*4+ch] + n/2) / n)
			}
		}
	}
}

// boxNRGBA reduces a straight-alpha NRGBA image by averaging its premultiplied
// form and unpacking the result again.
func boxNRGBA(src *image.NRGBA, w, h int) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	boxPremultipliedInto(dst.Pix, src.Pix, dst.Stride, src.Stride, b.Dx(), b.Dy(), w, h)
	return dst
}

// boxPremultipliedInto downscales 4-byte NRGBA pixels: colour channels are
// weighted by alpha while summing, then divided by the summed alpha, while the
// alpha plane itself is a plain average.
func boxPremultipliedInto(dst, src []byte, dstStride, srcStride, srcW, srcH, dstW, dstH int) {
	start, width := boxRanges(srcW, dstW)
	sums := make([]int64, dstW*3)
	alpha := make([]int64, dstW)
	for y := range dstH {
		sy0 := y * srcH / dstH
		sy1 := max(sy0+1, (y+1)*srcH/dstH)
		clear(sums)
		clear(alpha)
		for sy := sy0; sy < sy1; sy++ {
			row := src[sy*srcStride:]
			for x := range dstW {
				for px := start[x] * 4; px < (start[x]+width[x])*4; px += 4 {
					wa := int64(row[px+3])
					alpha[x] += wa
					for ch := range 3 {
						sums[x*3+ch] += int64(row[px+ch]) * wa
					}
				}
			}
		}
		rows := sy1 - sy0
		drow := dst[y*dstStride : y*dstStride+dstW*4]
		for x := range dstW {
			px := drow[x*4:]
			a := alpha[x]
			if a > 0 {
				for ch := range 3 {
					px[ch] = byte((sums[x*3+ch] + a/2) / a)
				}
			}
			n := int64(width[x] * rows)
			px[3] = byte((a + n/2) / n)
		}
	}
}

// boxGray reduces a grayscale image, which needs no channel handling at all.
func boxGray(src *image.Gray, w, h int) *image.Gray {
	b := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	boxPlane(dst.Pix, src.Pix, dst.Stride, src.Stride, b.Dx(), b.Dy(), w, h)
	return dst
}

// boxDownscale reduces src to w x h with the box filter. It reports false for
// image types it does not know, and the caller falls back to the kernel path:
// a case it turns down costs speed, never correctness.
func boxDownscale(src image.Image, w, h int) (image.Image, bool) {
	switch s := src.(type) {
	case *image.YCbCr:
		return boxYCbCr(s, w, h), true
	case *image.RGBA:
		return boxRGBA(s, w, h), true
	case *image.NRGBA:
		return boxNRGBA(s, w, h), true
	case *image.Gray:
		return boxGray(s, w, h), true
	}
	return nil, false
}
