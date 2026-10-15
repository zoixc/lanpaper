// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"golang.org/x/image/webp"

	"lanpaper/config"
	"lanpaper/storage"
)

// The EXIF orientation tag says how a viewer must turn the pixels a camera
// stored. The compression pass drops EXIF on purpose — it also carries the
// location — so the turn has to be baked into the pixels; otherwise a photo
// from a phone is served sideways in the panel, on the display and in the
// preview, while the very same file looks correct in every other application.

// pictureJPEG encodes a width x height JPEG with a red left half and a blue
// right half, so a flip or a turn shows in the corner colours.
func pictureJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			if x < width/2 {
				img.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// exifJPEG builds a 16x8 JPEG carrying an EXIF Orientation tag of the given
// value, the way a camera writes it. orientation 0 means no EXIF at all;
// bigEndian picks the byte order; notFirst puts the segment after the first
// one, so the segment walk is exercised.
func exifJPEG(t *testing.T, orientation uint16, bigEndian, notFirst bool) []byte {
	t.Helper()
	return withOrientation(t, pictureJPEG(t, 16, 8), orientation, bigEndian, notFirst)
}

// withOrientation inserts an EXIF APP1 segment into a JPEG. orientation 0
// returns the file untouched.
func withOrientation(t *testing.T, encoded []byte, orientation uint16, bigEndian, notFirst bool) []byte {
	t.Helper()
	if orientation == 0 {
		return encoded
	}
	order := []byte{'I', 'I', 0x2A, 0x00} // little endian ("II*\0")
	tiff := append([]byte{}, order...)
	tiff = append(tiff, 0x08, 0x00, 0x00, 0x00) // IFD0 follows the header
	tiff = append(tiff, 0x01, 0x00)             // one entry in it
	if bigEndian {
		tiff[0], tiff[1], tiff[2], tiff[3] = 'M', 'M', 0x00, 0x2A
		tiff[4], tiff[5], tiff[6], tiff[7] = 0x00, 0x00, 0x00, 0x08
		tiff[8], tiff[9] = 0x00, 0x01
		// tag 0x0112, SHORT, count 1, value in the first two bytes of the field
		tiff = append(tiff, 0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01,
			byte(orientation>>8), byte(orientation), 0x00, 0x00)
	} else {
		tiff = append(tiff, 0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00,
			byte(orientation), 0x00, 0x00, 0x00)
	}
	tiff = append(tiff, 0x00, 0x00, 0x00, 0x00) // no next IFD
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte((len(payload) + 2) & 0xFF)}

	// SOI first, then whatever the encoder writes; the segment walk must find
	// the tag behind that segment too.
	split := 2
	if notFirst && split+4 <= len(encoded) && encoded[split] == 0xFF {
		size := int(encoded[split+2])<<8 | int(encoded[split+3])
		split += 2 + size
	}
	out := append([]byte{}, encoded[:split]...)
	out = append(out, segment...)
	out = append(out, payload...)
	return append(out, encoded[split:]...)
}

// hasExifOrientation reports whether the tag survived in a file's bytes.
func hasExifOrientation(t *testing.T, data []byte) bool {
	t.Helper()
	return imageOrientation(bytes.NewReader(data), "jpg") > orientationNormal
}

func TestImageOrientationReadsTheTag(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		ext  string
		want int
	}{
		{"little endian, rotate 90 CW", exifJPEG(t, 6, false, false), "jpg", 6},
		{"big endian, rotate 270 CW", exifJPEG(t, 8, true, false), "jpg", 8},
		{"after the JFIF segment", exifJPEG(t, 6, false, true), "jpg", 6},
		{"tag says 1", exifJPEG(t, 1, false, false), "jpg", orientationNormal},
		{"no EXIF at all", exifJPEG(t, 0, false, false), "jpg", orientationNormal},
		{"not a JPEG", []byte("this is not a picture, just some bytes here"), "jpg", orientationNormal},
		{"empty", nil, "jpg", orientationNormal},
		{"a JPEG segment saying Exif but too short", exifJPEG(t, 6, false, false)[:14], "jpg", orientationNormal},
		{"AVIF is not read", []byte("\x00\x00\x00\x18ftypavif\x00\x00\x00\x00"), "avif", orientationNormal},
		{"a WebP without the chunk", []byte("RIFF\x20\x00\x00\x00WEBPVP8L\x14\x00\x00\x00"), "webp", orientationNormal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := imageOrientation(bytes.NewReader(tc.data), tc.ext); got != tc.want {
				t.Errorf("imageOrientation = %d, want %d", got, tc.want)
			}
		})
	}

	// A WebP keeps its EXIF in a RIFF chunk, with or without the "Exif\0\0"
	// prefix some writers add.
	tiff := []byte{'I', 'I', 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"with the prefix", append([]byte("Exif\x00\x00"), tiff...)},
		{"without the prefix", tiff},
	} {
		chunk := append([]byte("VP8L"), 0x05, 0x00, 0x00, 0x00, 'x', 'x', 'x', 'x', 'x', 0x00)
		exif := append([]byte("EXIF"), byte(len(tc.payload)), 0x00, 0x00, 0x00)
		exif = append(exif, tc.payload...)
		if len(tc.payload)%2 == 1 {
			exif = append(exif, 0x00)
		}
		body := append(append(append([]byte("WEBP"), chunk...), exif...), []byte("xxxx")...)
		riff := append([]byte("RIFF"), byte(len(body)), 0x00, 0x00, 0x00)
		riff = append(riff, body...)
		if got := imageOrientation(bytes.NewReader(riff), "webp"); got != 6 {
			t.Errorf("WebP %s: imageOrientation = %d, want 6", tc.name, got)
		}
	}

	// A TIFF is its own EXIF block. The reader must be rewound, or the decode
	// that follows would see an empty file.
	data := exifJPEG(t, 6, false, false)
	reader := bytes.NewReader(data)
	if got := imageOrientation(reader, "jpg"); got != 6 {
		t.Fatalf("first read = %d, want 6", got)
	}
	if _, err := jpeg.Decode(reader); err != nil {
		t.Errorf("reader not rewound after reading the tag: %v", err)
	}
}

// uploadOrientedJPEG runs the real upload path — file or URL — with the given
// compression settings and returns the service it used.
func uploadOrientedJPEG(t *testing.T, via string, quality int, data []byte) {
	t.Helper()
	respond := roundTripFunc(nil)
	source := ""
	if via == "url" {
		respond = mediaResponse(http.StatusOK, data)
		source = "https://cdn.test/photo.jpg"
	}
	service := remoteUploadLink(t, respond)
	config.Current.Compression = config.CompressionConfig{Quality: quality, Scale: 100}
	w := httptest.NewRecorder()
	service.Upload(w, avifRequest(t, "photo", "", source, data))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

// TestUploadBakesExifOrientationWhenReEncoding covers the default path: the
// picture is decoded and encoded again, which drops the tag, so the turn has to
// be in the pixels of the stored file — and of the preview, which never had a
// tag of its own.
func TestUploadBakesExifOrientationWhenReEncoding(t *testing.T) {
	// Colours of the upright picture, corner by corner, for the tags a camera
	// actually writes (see TestApplyOrientationPermutations for all eight).
	cases := []struct {
		name                    string
		orientation             uint16
		wantWidth, wantHeight   int
		topLeft, topRight       string
		bottomLeft, bottomRight string
	}{
		{"as shot", 1, 16, 8, "red", "blue", "red", "blue"},
		{"upside down", 3, 16, 8, "blue", "red", "blue", "red"},
		{"rotate 90 CW (the upright phone photo)", 6, 8, 16, "red", "red", "blue", "blue"},
		{"rotate 270 CW", 8, 8, 16, "blue", "blue", "red", "red"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, via := range []string{"file", "url"} {
				t.Run(via, func(t *testing.T) {
					data := exifJPEG(t, tc.orientation, false, false)
					// quality 90: the picture goes through the encoder, which is
					// what loses the tag.
					uploadOrientedJPEG(t, via, 90, data)
					stored := decodeStoredJPEG(t)
					if b := stored.Bounds(); b.Dx() != tc.wantWidth || b.Dy() != tc.wantHeight {
						t.Fatalf("stored %dx%d, want %dx%d", b.Dx(), b.Dy(), tc.wantWidth, tc.wantHeight)
					}
					assertCorners(t, "stored image", stored, tc.topLeft, tc.topRight, tc.bottomLeft, tc.bottomRight)
					assertPreviewOrientation(t, tc.wantHeight > tc.wantWidth, tc.topLeft, tc.bottomRight)
					// The tag itself must be gone: a public link should not serve
					// the metadata, and nothing depends on it any more.
					wp, _ := storage.Global.Get("photo")
					if raw, err := os.ReadFile(wp.ImagePath); err == nil && hasExifOrientation(t, raw) {
						t.Error("the stored file still carries an EXIF orientation tag")
					}
				})
			}
		})
	}
}

// TestUploadKeepsTheTagWhenBytesAreKeptButTurnsThePreview covers lossless mode:
// the original bytes stay as they are, tag included, because every viewer
// applies it. The thumbnail is a plain WebP without EXIF, so its turn has to
// happen at upload time, or the panel shows a sideways preview next to an
// upright picture.
func TestUploadKeepsTheTagWhenBytesAreKeptButTurnsThePreview(t *testing.T) {
	data := exifJPEG(t, 6, false, false)
	uploadOrientedJPEG(t, "file", 100, data)
	wp, ok := storage.Global.Get("photo")
	if !ok {
		t.Fatal("link missing")
	}
	stored, err := os.ReadFile(wp.ImagePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatal("lossless mode must keep the original bytes, EXIF tag included")
	}
	img := decodeStoredJPEG(t)
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 8 {
		t.Fatalf("stored pixels %dx%d, want the camera layout 16x8", b.Dx(), b.Dy())
	}
	// The preview must be upright despite the untouched file: the tag says a
	// quarter turn, so the camera layout (red left, blue right) becomes red
	// above, blue below.
	assertPreviewOrientation(t, true, "red", "blue")

	// Rebuilding previews must produce the same thumbnail: the tag is still in
	// the file, and the regeneration reads the pixels straight from it.
	if err := regenPreview(context.Background(), wp); err != nil {
		t.Fatal(err)
	}
	assertPreviewOrientation(t, true, "red", "blue")
}

// The thumbnail of a turned picture must land in the same box as one that was
// portrait to begin with: the box is 640x360, and applying it to the camera
// layout would give the upright thumbnail the box's sides instead of its own.
func TestPreviewBoxAppliesToTheUprightPicture(t *testing.T) {
	// Lossless mode: the stored bytes keep the tag and the camera layout, so
	// the thumbnail is the one thing that has to be turned here.
	turned := withOrientation(t, pictureJPEG(t, 1600, 800), 6, false, false)
	uploadOrientedJPEG(t, "file", 100, turned)
	turnedWidth, turnedHeight := previewSize(t)

	// The same picture, portrait from the start and with no tag at all.
	uploadOrientedJPEG(t, "file", 100, pictureJPEG(t, 800, 1600))
	nativeWidth, nativeHeight := previewSize(t)

	if turnedWidth != nativeWidth || turnedHeight != nativeHeight {
		t.Errorf("turned preview %dx%d, natively portrait preview %dx%d: they must match",
			turnedWidth, turnedHeight, nativeWidth, nativeHeight)
	}
	if turnedWidth > config.ThumbnailMaxWidth || turnedHeight > config.ThumbnailMaxHeight {
		t.Errorf("preview %dx%d is outside the %dx%d box",
			turnedWidth, turnedHeight, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight)
	}
	if turnedHeight <= turnedWidth {
		t.Errorf("preview %dx%d is not portrait", turnedWidth, turnedHeight)
	}
}

// applyOrientation is a pure permutation, so every one of the eight tags has an
// exact expected result.
func TestApplyOrientationPermutations(t *testing.T) {
	// 3x2 source, each pixel a letter, so a wrong mapping shows up:
	//   A B C
	//   D E F
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i, v := range []color.RGBA{{R: 1, A: 255}, {R: 2, A: 255}, {R: 3, A: 255},
		{R: 4, A: 255}, {R: 5, A: 255}, {R: 6, A: 255}} {
		src.SetRGBA(i%3, i/3, v)
	}
	letter := map[byte]string{1: "A", 2: "B", 3: "C", 4: "D", 5: "E", 6: "F"}
	rows := func(img image.Image) []string {
		b := img.Bounds()
		out := make([]string, 0, b.Dy())
		for y := range b.Dy() {
			row := ""
			for x := range b.Dx() {
				c := color.RGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.RGBA)
				row += letter[c.R]
			}
			out = append(out, row)
		}
		return out
	}
	for _, tc := range []struct {
		orientation int
		want        []string
	}{
		{orientationNormal, []string{"ABC", "DEF"}},
		{orientationFlipX, []string{"CBA", "FED"}},
		{orientationRot180, []string{"FED", "CBA"}},
		{orientationFlipY, []string{"DEF", "ABC"}},
		{orientationTrans, []string{"AD", "BE", "CF"}},
		{orientationRot90, []string{"DA", "EB", "FC"}},
		{orientationTransY, []string{"FC", "EB", "DA"}},
		{orientationRot270, []string{"CF", "BE", "AD"}},
	} {
		got := rows(applyOrientation(src, tc.orientation))
		if len(got) != len(tc.want) {
			t.Fatalf("orientation %d: %d rows, want %d", tc.orientation, len(got), len(tc.want))
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("orientation %d: row %d = %q, want %q", tc.orientation, i, got[i], tc.want[i])
			}
		}
	}
	// A value outside the defined range moves nothing, and neither does a tag
	// on an image with no pixels.
	if got := rows(applyOrientation(src, 9)); got[0] != "ABC" || got[1] != "DEF" {
		t.Errorf("orientation 9 changed the image: %q", got)
	}
	if got := applyOrientation(image.NewRGBA(image.Rectangle{}), orientationRot90); got.Bounds().Empty() != true {
		t.Errorf("an empty image came back as %v", got.Bounds())
	}
	// A 16-bit source keeps its depth: the permutation must not demote it to
	// 8 bits per channel.
	sixteen := image.NewNRGBA64(image.Rect(0, 0, 2, 1))
	sixteen.SetNRGBA64(0, 0, color.NRGBA64{R: 0x0001, A: 0xFFFF})
	sixteen.SetNRGBA64(1, 0, color.NRGBA64{R: 0xFFFF, A: 0xFFFF})
	turned, ok := applyOrientation(sixteen, orientationRot90).(*image.NRGBA64)
	if !ok {
		t.Fatalf("a 16-bit image became %T", applyOrientation(sixteen, orientationRot90))
	}
	if got := turned.NRGBA64At(0, 0).R; got != 0x0001 {
		t.Errorf("16-bit pixel = %#04x, want 0x0001", got)
	}
}

// --- helpers ---------------------------------------------------------------

func decodeStoredJPEG(t *testing.T) image.Image {
	t.Helper()
	wp, ok := storage.Global.Get("photo")
	if !ok {
		t.Fatal("link missing")
	}
	f, err := os.Open(wp.ImagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// cornerColour names the dominant channel at one point of an image.
func cornerColour(img image.Image, x, y int) string {
	b := img.Bounds()
	r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
	switch {
	case r > bl && r > g:
		return "red"
	case bl > r && bl > g:
		return "blue"
	}
	return "other"
}

func assertCorners(t *testing.T, what string, img image.Image, topLeft, topRight, bottomLeft, bottomRight string) {
	t.Helper()
	b := img.Bounds()
	for _, c := range []struct {
		name string
		x, y int
		want string
	}{
		{"top left", 0, 0, topLeft},
		{"top right", b.Dx() - 1, 0, topRight},
		{"bottom left", 0, b.Dy() - 1, bottomLeft},
		{"bottom right", b.Dx() - 1, b.Dy() - 1, bottomRight},
	} {
		if got := cornerColour(img, c.x, c.y); got != c.want {
			t.Errorf("%s %s (%d,%d): %s, want %s (%dx%d)",
				what, c.name, c.x, c.y, got, c.want, b.Dx(), b.Dy())
		}
	}
}

// previewSize returns the dimensions of the stored thumbnail.
func previewSize(t *testing.T) (int, int) {
	t.Helper()
	wp, ok := storage.Global.Get("photo")
	if !ok {
		t.Fatal("link missing")
	}
	f, err := os.Open(wp.PreviewPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	thumb, err := webp.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return thumb.Bounds().Dx(), thumb.Bounds().Dy()
}

// assertPreviewOrientation checks the WebP thumbnail: portrait stays portrait,
// and the two colours of the source picture sit in the corners the upright
// picture puts them in. Quadrants are sampled rather than edges, because the
// red/blue border runs down the middle of the landscape original.
func assertPreviewOrientation(t *testing.T, wantPortrait bool, topLeft, bottomRight string) {
	t.Helper()
	wp, ok := storage.Global.Get("photo")
	if !ok {
		t.Fatal("link missing")
	}
	f, err := os.Open(wp.PreviewPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	thumb, err := webp.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := thumb.Bounds()
	if portrait := b.Dy() > b.Dx(); portrait != wantPortrait {
		t.Errorf("preview %dx%d: portrait=%v, want %v", b.Dx(), b.Dy(), portrait, wantPortrait)
	}
	if got := cornerColour(thumb, b.Dx()/4, b.Dy()/4); got != topLeft {
		t.Errorf("preview top-left quadrant is %s, want %s", got, topLeft)
	}
	if got := cornerColour(thumb, 3*b.Dx()/4, 3*b.Dy()/4); got != bottomRight {
		t.Errorf("preview bottom-right quadrant is %s, want %s", got, bottomRight)
	}
}
