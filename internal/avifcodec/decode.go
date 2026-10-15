// SPDX-License-Identifier: MIT

// Package avifcodec runs a pinned libavif/dav1d build in a resource-limited
// WASM sandbox. It deliberately does not register a global image decoder.
package avifcodec

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	// The caller reserves this entire envelope BEFORE reading the input. WASM
	// allocations, compressed Go input and the detached output all fit in it.
	WorkingBytes   int64 = memoryPages*65536 + MaxInputBytes + MaxOutputBytes
	MaxInputBytes  int64 = 32 << 20
	MaxOutputBytes int64 = 64 << 20
	memoryPages          = 192 << 20 / 65536
	decodeTimeout        = 30 * time.Second
)

var (
	ErrUnsupported = errors.New("unsupported AVIF feature")
	ErrUnavailable = errors.New("AVIF decoder unavailable")
)

//go:embed lib/avif.wasm.gz
var compressedModule []byte

var engine struct {
	init     initialization
	rt       wazero.Runtime
	compiled wazero.CompiledModule
}

func initialize() error {
	// Initialization is lazy: JPEG/PNG/WebP requests never compile this module.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rt := wazero.NewRuntimeWithConfig(context.Background(), wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesThreads).
		WithMemoryLimitPages(memoryPages).WithCloseOnContextDone(true))
	ready := false
	defer func() {
		if !ready {
			_ = rt.Close(context.Background())
		}
	}()
	z, err := gzip.NewReader(bytes.NewReader(compressedModule))
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(z, (8<<20)+1))
	_ = z.Close()
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return errors.New("AVIF module exceeds trusted size limit")
	}
	cm, err := rt.CompileModule(ctx, data)
	if err != nil {
		return err
	}
	if _, err = wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return err
	}
	engine.rt, engine.compiled = rt, cm
	ready = true
	return nil
}

// Decode validates the container and obtains dimensions without decoding pixels,
// invokes validate to reserve the caller's pixel budget, then decodes ONE still
// image. The caller must already hold WorkingBytes in its shared memory budget.
// Output owns its pixels: it never aliases a closed WASM instance.
func Decode(ctx context.Context, r io.ReadSeeker, validate func(image.Config) error) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if _, err = r.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	size := end - start
	if size <= 0 {
		return nil, errors.New("empty AVIF")
	}
	if size > MaxInputBytes {
		return nil, fmt.Errorf("%w: input exceeds 32 MiB", ErrUnsupported)
	}
	// A single exact allocation avoids io.ReadAll's growing buffers and spare
	// capacity escaping the advertised compressed-input memory envelope.
	data := make([]byte, int(size))
	if _, err = io.ReadFull(r, data); err != nil {
		return nil, err
	}
	transform, err := inspectProperties(data)
	if err != nil {
		return nil, err
	}
	if err := engine.init.wait(ctx, initialize); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	// Trusted, once-only compilation has its own ceiling. Start the per-image
	// execution deadline afterwards so a cold server under CPU contention does
	// not spend a request's entire decode allowance compiling the module.
	ctx, cancel := context.WithTimeout(ctx, decodeTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// No filesystem, environment, sockets, stdout or stderr are exposed.
	mod, err := engine.rt.InstantiateModule(ctx, engine.compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, err
	}
	defer mod.Close(context.Background())
	alloc := func(n uint64) (uint32, error) {
		result, e := mod.ExportedFunction("malloc").Call(ctx, n)
		if e != nil {
			return 0, e
		}
		if len(result) != 1 || result[0] == 0 || result[0] > uint64(^uint32(0)) {
			return 0, errors.New("AVIF allocation failed")
		}
		return uint32(result[0]), nil
	}
	input, err := alloc(uint64(len(data)))
	if err != nil {
		return nil, err
	}
	if !mod.Memory().Write(input, data) {
		return nil, errors.New("AVIF input memory write failed")
	}
	fields, err := alloc(24) // width, height, depth, frame count, double delay
	if err != nil {
		return nil, err
	}
	decode := func(configOnly uint64, out uint32) error {
		result, e := mod.ExportedFunction("decode").Call(ctx, uint64(input), uint64(len(data)), configOnly, 0,
			uint64(fields), uint64(fields+4), uint64(fields+8), uint64(fields+12), uint64(fields+16), uint64(out))
		if e != nil {
			return e
		}
		if len(result) != 1 || result[0] != 1 {
			return errors.New("invalid AVIF bitstream")
		}
		return nil
	}
	if err = decode(1, 0); err != nil {
		return nil, err
	}
	header, ok := mod.Memory().Read(fields, 16)
	if !ok {
		return nil, errors.New("AVIF header memory read failed")
	}
	width := binary.LittleEndian.Uint32(header)
	height := binary.LittleEndian.Uint32(header[4:])
	depth := binary.LittleEndian.Uint32(header[8:])
	count := binary.LittleEndian.Uint32(header[12:])
	if count != 1 {
		return nil, fmt.Errorf("%w: animated images", ErrUnsupported)
	}
	if depth != 8 && depth != 10 && depth != 12 {
		return nil, errors.New("invalid AVIF depth")
	}
	bpp := int64(4)
	model := color.RGBAModel
	if depth > 8 {
		bpp = 8
		model = color.RGBA64Model
	}
	// Validate the multiplication before converting dimensions to int (32-bit
	// builds too). The codec also has a hard WASM memory ceiling on every call.
	pixels := uint64(width) * uint64(height)
	if width == 0 || height == 0 || width > 16384 || height > 16384 || pixels > uint64(MaxOutputBytes/bpp) {
		return nil, fmt.Errorf("%w: decoded image exceeds AVIF size limits", ErrUnsupported)
	}
	cfg := image.Config{Width: int(width), Height: int(height), ColorModel: model}
	if validate != nil {
		if err = validate(cfg); err != nil {
			return nil, err
		}
	}
	outSize := uint32(pixels * uint64(bpp))
	out, err := alloc(uint64(outSize))
	if err != nil {
		return nil, err
	}
	// The pinned C bridge can return success when NextImage fails before the
	// first frame. It writes the frame delay ONLY after a successful decode.
	// Seed that slot with an impossible sentinel so malformed bitstreams can
	// never pass validation just because their container parsed successfully.
	if !mod.Memory().WriteUint64Le(fields+16, ^uint64(0)) {
		return nil, errors.New("AVIF delay memory write failed")
	}
	if err = decode(0, out); err != nil {
		return nil, err
	}
	delay, ok := mod.Memory().ReadUint64Le(fields + 16)
	if !ok || delay == ^uint64(0) {
		return nil, errors.New("AVIF frame was not decoded")
	}
	src, ok := mod.Memory().Read(out, outSize)
	if !ok {
		return nil, errors.New("AVIF output memory read failed")
	}
	img := detach(src, cfg.Width, cfg.Height, int(bpp), transform)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return img, nil
}

// transform is a counter-clockwise rotation followed by an optional mirror.
type transform struct{ angle, mirror int }

func detach(src []byte, w, h, bpp int, t transform) image.Image {
	dw, dh := w, h
	if t.angle%2 != 0 {
		dw, dh = h, w
	}
	dst := make([]byte, len(src))
	if bpp == 4 && t.angle == 0 && t.mirror == -1 {
		copy(dst, src)
		return &image.RGBA{Pix: dst, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			// irot is counter-clockwise, followed by imir in rotated coordinates.
			switch t.angle {
			case 1:
				dx, dy = y, w-1-x
			case 2:
				dx, dy = w-1-x, h-1-y
			case 3:
				dx, dy = h-1-y, x
			}
			// AVIF axis 0 exchanges top/bottom; axis 1 exchanges left/right.
			if t.mirror == 0 {
				dy = dh - 1 - dy
			} else if t.mirror == 1 {
				dx = dw - 1 - dx
			}
			s, d := (y*w+x)*bpp, (dy*dw+dx)*bpp
			if bpp == 8 {
				for k := 0; k < 8; k += 2 {
					dst[d+k], dst[d+k+1] = src[s+k+1], src[s+k]
				}
			} else {
				copy(dst[d:d+4], src[s:s+4])
			}
		}
	}
	if bpp == 8 {
		return &image.RGBA64{Pix: dst, Stride: dw * 8, Rect: image.Rect(0, 0, dw, dh)}
	}
	return &image.RGBA{Pix: dst, Stride: dw * 4, Rect: image.Rect(0, 0, dw, dh)}
}
