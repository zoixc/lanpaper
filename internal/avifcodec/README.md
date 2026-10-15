# Bounded AVIF decoder

This package is intentionally an explicit adapter, not an `image.RegisterFormat`
import. Existing JPEG/PNG/GIF/WebP/BMP/TIFF paths never initialize it.

## Provenance and updating

`lib/avif.wasm.gz`, the C bridge, build recipes and upstream fixtures were copied
from `gen2brain/avif` commit
`6cb8f35756bea8198ca1763953f5f19358bdfecf` (MIT wrapper):
https://github.com/gen2brain/avif/tree/6cb8f35756bea8198ca1763953f5f19358bdfecf

Compressed module SHA-256:
`92ffd604d276359c2df829effc615cd9f286c36ece5761644929e4396eb97360`.

The upstream recipe identifies libavif v1.4.2, dav1d 1.5.3, libaom v3.14.1,
and libyuv's `stable` ref. The bundled module also contains the unused encoder;
Lanpaper exposes no AVIF encoding API. The upstream recipe does not pin the
libyuv commit or WASI SDK version: do not claim a bit-reproducible build from
these recipes alone. Keep the exact upstream artifact hash when updating.

`lib/Makefile`, `avif.c`, `toolchain.cmake`, and `wasi.meson` are upstream source
and build instructions, not part of the normal Lanpaper build. With the upstream
WASI SDK and native build tools, `make avif.wasm` builds the module; compress it
with `gzip -n -9`. Review codec versions, hashes, licenses and fixtures together.
`govulncheck` can check wazero but **cannot audit the C codecs embedded in WASM**;
those need upstream security-advisory review on updates.

Licenses for the wrapper, codecs, libyuv and WASI/compiler runtime are retained
in `lib/` and reproduced in the root `THIRD-PARTY-NOTICES.md` for binary/Docker
redistribution. Upstream test fixtures inherit the wrapper repository's MIT
license. `testdata/alpha.avif` is a project-generated 32x24 constant RGBA
(255, 0, 0, 128) image, encoded with Pillow 12.3.0 at quality 100; it contains no
third-party photograph.

## Safety policy

- CGO-free, WASM-only, with no dynamic system-library discovery.
- No host filesystem, network, environment, stdout or stderr capabilities.
- Native-code compilation needs executable anonymous mappings on supported
  hosts. Do not use systemd `MemoryDenyWriteExecute=true` for an AVIF-enabled
  deployment; see `docs/DEPLOYMENT.md`. Temporary files remain non-executable.
- Runtime/module compilation is lazy and cached once. A canceled caller stops
  waiting while the one trusted compiler finishes under its own deadline; a
  failed initialization is cached and fails closed with HTTP 503 until restart.
  A fresh instance is closed after each decode; returned pixels are copied,
  not retained WASM views.
- A 192 MiB hard WASM linear-memory limit, 32 MiB exact-sized compressed input,
  and 64 MiB detached output. The caller reserves the entire 288 MiB envelope
  in the existing shared image byte budget before input allocation. Runtime,
  metadata-parser and image-processing overhead are additional, as they are
  for other codecs; this is not an RSS limit for the process.
- 30-second decode context, inherited caller cancellation, and wazero's
  `WithCloseOnContextDone(true)` terminate execution, including infinite loops.
  Initial trusted-module compilation has a separate 60-second ceiling.
- Read dimensions before allocating pixel output and before reserving the
  shared pixel budget. AVIF limits: 16,777,216 pixels at 8-bit or 8,388,608 at
  10/12-bit, 16,384 pixels per side; memory-heavy streams may still fail below
  these limits. No full animation decode is used, even in original-byte mode.
- Static SDR only. HDR, wide-gamut and ICC colour, clean-aperture crops and
  sequences are explicitly rejected, not silently tone-mapped/flattened.
  Rotation and mirroring are applied for the primary item. Original-byte mode
  leaves the source untouched; only its preview is transformed.
- The pinned C bridge returns success if `NextImage` fails before frame one.
  The adapter therefore seeds the frame-delay output with an impossible
  sentinel and requires an actual successful-frame write. A regression test
  corrupts the AV1 payload while keeping the container parseable.

To run the focused tests and parser fuzzing:

```sh
go test -race ./internal/avifcodec ./handlers
go test ./internal/avifcodec -run '^$' -fuzz FuzzContainer -fuzztime 10s
```
