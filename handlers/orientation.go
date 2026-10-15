// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"io"
)

// EXIF orientation, tag 0x0112, as the camera writes it: the stored pixels are
// what came off the sensor, and the tag says how a viewer must turn them for
// display. A phone held upright writes 6 or 8 — a quarter turn — which is why
// most portrait photos carry a value other than 1.
const (
	orientationNormal = 1
	orientationFlipX  = 2
	orientationRot180 = 3
	orientationFlipY  = 4
	orientationTrans  = 5
	orientationRot90  = 6
	orientationTransY = 7
	orientationRot270 = 8
)

// maxExifBlob bounds what is read while looking for the orientation tag. An
// APP1 segment is at most 64 KiB by the JPEG specification.
const maxExifBlob = 64 << 10

// imageOrientation returns the EXIF orientation of a JPEG, WebP or TIFF file,
// or orientationNormal when the file carries none (and for every other format).
// The reader is rewound afterwards, so callers may decode it again.
//
// Why it matters here: the re-encode drops EXIF on purpose (it also carries
// location data), so a tag that is not honoured is not merely lost — the
// picture is then served sideways, in the panel, on the display and in the
// preview, while the very same file looks correct everywhere else. The lossless
// mode keeps the bytes, and therefore the tag, so the orientation must be
// applied to the thumbnail alone there.
func imageOrientation(r io.ReadSeeker, ext string) int {
	if r == nil {
		return orientationNormal
	}
	blob := exifBlob(r, ext)
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return orientationNormal
	}
	return tiffOrientation(blob)
}

// exifBlob extracts the TIFF/Exif block of a file: the APP1 payload of a JPEG,
// the EXIF chunk of a WebP, or the file itself for a TIFF image.
func exifBlob(r io.ReadSeeker, ext string) []byte {
	switch ext {
	case "jpg", "jpeg":
		return jpegExifBlob(r)
	case "webp":
		return webpExifBlob(r)
	case "tiff", "tif":
		return readAt(r, 0, maxExifBlob)
	}
	return nil
}

// jpegExifBlob walks the JPEG segments up to the first scan and returns the
// payload of the first APP1 segment that declares itself as Exif.
func jpegExifBlob(r io.ReadSeeker) []byte {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	head := make([]byte, 2)
	if _, err := io.ReadFull(r, head); err != nil || head[0] != 0xFF || head[1] != 0xD8 {
		return nil
	}
	for segment := 0; segment < 64; segment++ {
		if _, err := io.ReadFull(r, head); err != nil || head[0] != 0xFF {
			return nil
		}
		// Fill bytes: any number of 0xFF before the marker.
		marker := head[1]
		for marker == 0xFF {
			if _, err := io.ReadFull(r, head); err != nil {
				return nil
			}
			marker = head[1]
		}
		switch {
		case marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7):
			continue // stand-alone markers carry no length
		case marker == 0xDA: // start of scan: no metadata follows
			return nil
		}
		var sizeBuf [2]byte
		if _, err := io.ReadFull(r, sizeBuf[:]); err != nil {
			return nil
		}
		size := int(binary.BigEndian.Uint16(sizeBuf[:])) - 2
		if size < 0 {
			return nil
		}
		if marker == 0xE1 {
			payload := make([]byte, min(size, maxExifBlob))
			if _, err := io.ReadFull(r, payload); err != nil {
				return nil
			}
			if !bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				return nil
			}
			return payload[6:]
		}
		if _, err := r.Seek(int64(size), io.SeekCurrent); err != nil {
			return nil
		}
	}
	return nil
}

// webpExifBlob returns the EXIF chunk of a RIFF/WEBP file. Some writers prefix
// it with "Exif\x00\x00", some do not.
func webpExifBlob(r io.ReadSeeker) []byte {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(r, header); err != nil ||
		!bytes.Equal(header[0:4], []byte("RIFF")) || !bytes.Equal(header[8:12], []byte("WEBP")) {
		return nil
	}
	for chunk := 0; chunk < 32; chunk++ {
		var head [8]byte
		if _, err := io.ReadFull(r, head[:]); err != nil {
			return nil
		}
		size := int(binary.LittleEndian.Uint32(head[4:]))
		if string(head[0:4]) == "EXIF" {
			payload := make([]byte, min(size, maxExifBlob))
			if _, err := io.ReadFull(r, payload); err != nil {
				return nil
			}
			return bytes.TrimPrefix(payload, []byte("Exif\x00\x00"))
		}
		// RIFF pads every chunk to an even size.
		if _, err := r.Seek(int64(size+(size&1)), io.SeekCurrent); err != nil {
			return nil
		}
	}
	return nil
}

func readAt(r io.ReadSeeker, offset int64, size int) []byte {
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	buf := make([]byte, size)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil
	}
	return buf[:n]
}

// tiffOrientation reads the Orientation tag from an IFD0 of a TIFF/Exif block.
// Every field is bounds-checked: the block comes from an uploaded file.
func tiffOrientation(blob []byte) int {
	if len(blob) < 8 {
		return orientationNormal
	}
	var order binary.ByteOrder
	switch {
	case blob[0] == 'I' && blob[1] == 'I':
		order = binary.LittleEndian
	case blob[0] == 'M' && blob[1] == 'M':
		order = binary.BigEndian
	default:
		return orientationNormal
	}
	if order.Uint16(blob[2:4]) != 42 {
		return orientationNormal
	}
	ifd := int(order.Uint32(blob[4:8]))
	if ifd < 8 || ifd+2 > len(blob) {
		return orientationNormal
	}
	count := int(order.Uint16(blob[ifd : ifd+2]))
	for i := 0; i < count; i++ {
		entry := ifd + 2 + i*12
		if entry+12 > len(blob) {
			return orientationNormal
		}
		if order.Uint16(blob[entry:entry+2]) != 0x0112 { // Orientation
			continue
		}
		// SHORT (3) with one value; anything else is not the tag it claims.
		if order.Uint16(blob[entry+2:entry+4]) != 3 || order.Uint32(blob[entry+4:entry+8]) < 1 {
			return orientationNormal
		}
		if value := int(order.Uint16(blob[entry+8 : entry+10])); value >= 1 && value <= 8 {
			return value
		}
		return orientationNormal
	}
	return orientationNormal
}

// applyOrientation returns img turned the way its EXIF orientation asks for, so
// every consumer downstream (the stored file, the thumbnail, the scale pass)
// works on upright pixels and needs to know nothing about EXIF.
//
// Images that keep their pixels in one flat buffer (RGBA, NRGBA, their 16-bit
// forms, grays) are permuted byte-wise, which costs one pass and no colour
// conversion. Everything else — a JPEG decodes to subsampled YCbCr — is
// materialised one row at a time: that keeps the intermediate at one row while
// the destination is allocated once. The price is one pass over the pixels and
// one image's worth of memory; BenchmarkApplyOrientation12MP measures both
// paths, and the same walk written through the image.Image interface beside
// them — the obvious version, which costs more time and an allocation per
// pixel.
func applyOrientation(img image.Image, orientation int) image.Image {
	if orientation <= orientationNormal || orientation > orientationRot270 || img == nil {
		return img
	}
	b := img.Bounds()
	if b.Empty() {
		return img
	}
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orientation >= orientationTrans { // the quarter turns swap the sides
		dw, dh = h, w
	}
	rect := image.Rect(0, 0, dw, dh)
	switch src := img.(type) {
	case *image.RGBA:
		out, stride := permuted(src.Pix, src.Stride, 4, b, orientation, dw, dh)
		return &image.RGBA{Pix: out, Stride: stride, Rect: rect}
	case *image.NRGBA:
		out, stride := permuted(src.Pix, src.Stride, 4, b, orientation, dw, dh)
		return &image.NRGBA{Pix: out, Stride: stride, Rect: rect}
	case *image.RGBA64:
		out, stride := permuted(src.Pix, src.Stride, 8, b, orientation, dw, dh)
		return &image.RGBA64{Pix: out, Stride: stride, Rect: rect}
	case *image.NRGBA64:
		out, stride := permuted(src.Pix, src.Stride, 8, b, orientation, dw, dh)
		return &image.NRGBA64{Pix: out, Stride: stride, Rect: rect}
	case *image.Gray16:
		out, stride := permuted(src.Pix, src.Stride, 2, b, orientation, dw, dh)
		return &image.Gray16{Pix: out, Stride: stride, Rect: rect}
	case *image.Gray:
		out, stride := permuted(src.Pix, src.Stride, 1, b, orientation, dw, dh)
		return &image.Gray{Pix: out, Stride: stride, Rect: rect}
	}
	// A JPEG (subsampled YCbCr) or a paletted GIF: let the converter do the
	// per-pixel work, one row at a time.
	dst := image.NewRGBA(rect)
	row := image.NewRGBA(image.Rect(0, 0, w, 1))
	for y := 0; y < h; y++ {
		draw.Draw(row, row.Bounds(), img, image.Pt(b.Min.X, b.Min.Y+y), draw.Src)
		for x := 0; x < w; x++ {
			dx, dy := orientedPoint(orientation, x, y, w, h)
			offset := dy*dst.Stride + dx*4
			copy(dst.Pix[offset:offset+4], row.Pix[x*4:x*4+4])
		}
	}
	return dst
}

// permuted copies pixels of pixSize bytes from one flat buffer into a new one,
// moving the pixel at (x, y) to where the orientation puts it.
func permuted(pix []byte, stride, pixSize int, b image.Rectangle, orientation, dw, dh int) ([]byte, int) {
	out := make([]byte, dw*dh*pixSize)
	outStride := dw * pixSize
	for y := 0; y < b.Dy(); y++ {
		srcRow := pix[(b.Min.Y+y)*stride+b.Min.X*pixSize:]
		for x := 0; x < b.Dx(); x++ {
			dx, dy := orientedPoint(orientation, x, y, b.Dx(), b.Dy())
			offset := dy*outStride + dx*pixSize
			copy(out[offset:offset+pixSize], srcRow[x*pixSize:x*pixSize+pixSize])
		}
	}
	return out, outStride
}

// orientedPoint maps a source pixel to its place in the upright picture.
func orientedPoint(orientation, x, y, w, h int) (int, int) {
	switch orientation {
	case orientationFlipX:
		return w - 1 - x, y
	case orientationRot180:
		return w - 1 - x, h - 1 - y
	case orientationFlipY:
		return x, h - 1 - y
	case orientationTrans:
		return y, x
	case orientationRot90:
		return h - 1 - y, x
	case orientationTransY:
		return h - 1 - y, w - 1 - x
	case orientationRot270:
		return y, w - 1 - x
	}
	return x, y
}
