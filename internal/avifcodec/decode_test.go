// SPDX-License-Identifier: MIT

package avifcodec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeStillAndDeep(t *testing.T) {
	for _, name := range []string{"test8.avif", "test10.avif", "test_rot.avif"} {
		t.Run(name, func(t *testing.T) {
			validated := false
			img, err := Decode(context.Background(), bytes.NewReader(fixture(t, name)), func(cfg image.Config) error {
				validated = true
				if cfg.Width < 1 || cfg.Height < 1 {
					t.Fatal(cfg)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !validated || img.Bounds().Empty() {
				t.Fatal("missing validation/image")
			}
			// A second instance must not overwrite pixels returned from a closed one.
			before := img.At(0, 0)
			if _, err := Decode(context.Background(), bytes.NewReader(fixture(t, "test8.avif")), nil); err != nil {
				t.Fatal(err)
			}
			if img.At(0, 0) != before {
				t.Fatal("output aliases WASM memory")
			}
		})
	}
}

func TestDecodeRejectsAnimationAndInvalidData(t *testing.T) {
	if _, err := Decode(context.Background(), bytes.NewReader(fixture(t, "test.avifs")), nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("animation: %v", err)
	}
	for _, data := range [][]byte{nil, []byte("not an image"), fixture(t, "test8.avif")[:128]} {
		if _, err := Decode(context.Background(), bytes.NewReader(data), nil); err == nil {
			t.Fatal("invalid file accepted")
		}
	}
}

func TestDecodeCancellationAndValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Decode(ctx, bytes.NewReader(fixture(t, "test8.avif")), nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	sentinel := errors.New("no pixel budget")
	if _, err := Decode(context.Background(), bytes.NewReader(fixture(t, "test8.avif")), func(image.Config) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestSandboxMemoryAndExecutionLimits(t *testing.T) {
	if err := engine.init.wait(t.Context(), initialize); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mod, err := engine.rt.InstantiateModule(ctx, engine.compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(context.Background())
	if _, ok := mod.Memory().Grow(memoryPages); ok {
		t.Fatal("WASM escaped its hard memory limit")
	}
	// A hostile codec that loops forever is interrupted, not merely abandoned
	// in a goroutine. Minimal WASM module exports an infinite-loop function.
	spin := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 4, 1, 96, 0, 0, 3, 2, 1, 0, 7, 8, 1, 4, 's', 'p', 'i', 'n', 0, 0, 10, 9, 1, 7, 0, 3, 64, 12, 0, 11, 11}
	m, err := engine.rt.Instantiate(ctx, spin)
	if err != nil {
		t.Fatal(err)
	}
	short, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, err = m.ExportedFunction("spin").Call(short); err == nil {
		t.Fatal("unbounded execution")
	}
	if !m.IsClosed() {
		t.Fatal("canceled module not closed")
	}
}

func TestDetachOrientationAndEndian(t *testing.T) {
	// Two rows: 1 2 3 / 4 5 6. Reference EXIF mappings for each irot/imir pair.
	input := []byte{1, 0, 0, 255, 2, 0, 0, 255, 3, 0, 0, 255, 4, 0, 0, 255, 5, 0, 0, 255, 6, 0, 0, 255}
	tests := []struct {
		t    transform
		want []byte
	}{
		{transform{0, -1}, []byte{1, 2, 3, 4, 5, 6}},
		{transform{1, -1}, []byte{3, 6, 2, 5, 1, 4}},
		{transform{2, -1}, []byte{6, 5, 4, 3, 2, 1}},
		{transform{3, -1}, []byte{4, 1, 5, 2, 6, 3}},
		{transform{0, 0}, []byte{4, 5, 6, 1, 2, 3}},
		{transform{0, 1}, []byte{3, 2, 1, 6, 5, 4}},
		{transform{1, 0}, []byte{1, 4, 2, 5, 3, 6}},
		{transform{1, 1}, []byte{6, 3, 5, 2, 4, 1}},
	}
	for _, tc := range tests {
		got := detach(input, 3, 2, 4, tc.t).(*image.RGBA)
		for i, want := range tc.want {
			if got.Pix[4*i] != want {
				t.Fatalf("%v pixel %d: %v", tc.t, i, got.Pix)
			}
		}
	}
	deep := detach([]byte{0x34, 0x12, 0, 0, 0, 0, 0xff, 0xff}, 1, 1, 8, transform{mirror: -1}).(*image.RGBA64)
	if binary.BigEndian.Uint16(deep.Pix) != 0x1234 {
		t.Fatal("incorrect 16-bit byte order")
	}
}

func FuzzContainer(f *testing.F) {
	f.Add(fixture(f, "test8.avif"))
	f.Add(fixture(f, "test_rot.avif"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = inspectProperties(b) })
}

func BenchmarkDecode(b *testing.B) {
	data := fixture(b, "test8.avif")
	if path := os.Getenv("LANPAPER_AVIF_BENCH_FILE"); path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
	}
	if _, err := Decode(context.Background(), bytes.NewReader(data), nil); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(context.Background(), bytes.NewReader(data), nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCorruptBitstreamIsNotValidatedByHeaderAlone(t *testing.T) {
	data := fixture(t, "test8.avif")
	idx := bytes.Index(data, []byte("mdat"))
	if idx < 0 {
		t.Fatal("no mdat")
	}
	for i := idx + 4; i < len(data); i++ {
		data[i] = 0
	}
	parsed := false
	if _, err := Decode(context.Background(), bytes.NewReader(data), func(image.Config) error { parsed = true; return nil }); err == nil {
		t.Fatal("corrupted AV1 stream accepted")
	}
	if !parsed {
		t.Fatal("test did not reach full decode")
	}
}

func TestAlphaAndRotatedFixture(t *testing.T) {
	img, err := Decode(context.Background(), bytes.NewReader(fixture(t, "alpha.avif")), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	if p.A < 126 || p.A > 130 || p.R < 250 || p.G > 5 || p.B > 5 {
		t.Fatalf("alpha/premultiplication: %+v", p)
	}
	img, err = Decode(context.Background(), bytes.NewReader(fixture(t, "test_rot.avif")), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Upstream fixture is 640x480 with a 90-degree rotation.
	if img.Bounds().Dx() != 480 || img.Bounds().Dy() != 640 {
		t.Fatalf("rotation ignored: %v", img.Bounds())
	}
}

func TestUnsupportedColourAndCrop(t *testing.T) {
	original := fixture(t, "test8.avif")
	for _, tc := range []struct {
		name string
		edit func([]byte)
	}{
		{"HDR", func(b []byte) {
			i := bytes.Index(b, []byte("nclx"))
			if i < 0 {
				t.Fatal("no nclx")
			}
			binary.BigEndian.PutUint16(b[i+6:], 16)
		}},
		{"wide gamut", func(b []byte) { i := bytes.Index(b, []byte("nclx")); binary.BigEndian.PutUint16(b[i+4:], 9) }},
		{"ICC", func(b []byte) { i := bytes.Index(b, []byte("nclx")); copy(b[i:], "prof") }},
		{"crop", func(b []byte) { i := bytes.Index(b, []byte("ispe")); copy(b[i:], "clap") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bytes.Clone(original)
			tc.edit(b)
			if _, err := Decode(context.Background(), bytes.NewReader(b), nil); !errors.Is(err, ErrUnsupported) {
				t.Fatal(err)
			}
		})
	}
}

func TestAVIFInputAndOutputLimits(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "large-*.avif")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = f.Truncate(MaxInputBytes + 1); err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(context.Background(), f, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	data := fixture(t, "test8.avif")
	i := bytes.Index(data, []byte("ispe"))
	if i < 0 {
		t.Fatal("no ispe")
	}
	binary.BigEndian.PutUint32(data[i+8:], 8192)
	binary.BigEndian.PutUint32(data[i+12:], 8192)
	if _, err = Decode(context.Background(), bytes.NewReader(data), nil); err == nil {
		t.Fatal("oversized dimensions accepted")
	}
}

func TestPinnedModuleDigest(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(compressedModule)); got != "92ffd604d276359c2df829effc615cd9f286c36ece5761644929e4396eb97360" {
		t.Fatal("AVIF module changed without provenance review", got)
	}
}
