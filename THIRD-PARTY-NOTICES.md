# Third-party notices

Lanpaper (this repository) is licensed under the MIT License — see
[LICENSE](LICENSE). This file lists everything that is **not** written by the
Lanpaper authors but is compiled into, or shipped with, a release.

Every component below is permissively licensed (MIT, MIT-0, BSD-3-Clause, OFL-1.1).
**There is no copyleft component** — no GPL, LGPL, AGPL or MPL code is linked
into the binary or served by it — so Lanpaper may be offered as a hosted
service, bundled with proprietary software, or redistributed under an
additional commercial license without any obligation to publish source code.
The only obligations are attribution (reproducing the notices below) and, for
BSD-3-Clause, not using a contributor's name to endorse a derived product.

| Component | Version | License | Used for | Present in |
| --- | --- | --- | --- | --- |
| [SeriousBug/webp-go-pure](https://github.com/SeriousBug/webp-go-pure) | v1.2.0 | MIT | WebP **encoding** (thumbnails and re-encoded media) | linked into the binary |
| [golang.org/x/image](https://github.com/golang/image) | v0.46.0 | BSD-3-Clause | BMP/TIFF decoding, WebP **decoding**, bilinear resizing | linked into the binary |
| [joho/godotenv](https://github.com/joho/godotenv) | v1.5.1 | MIT | loading an optional `.env` file at startup | linked into the binary |
| [ncruces/go-sqlite3](https://github.com/ncruces/go-sqlite3) | v0.35.6 | MIT | pure-Go SQLite metadata backend and `database/sql` driver | linked into the binary |
| [ncruces/go-sqlite3-wasm](https://github.com/ncruces/go-sqlite3-wasm) | v6.3.35304 | MIT-0 | embedded SQLite WASM runtime | linked into the binary |
| [ncruces/julianday](https://github.com/ncruces/julianday) | v1.0.0 | MIT | SQLite date/time support | linked into the binary |
| Go standard library | per the build toolchain | BSD-3-Clause | HTTP server, JSON, crypto, image codecs | linked into the binary |
| [Golos Text](https://github.com/googlefonts/golos-text) | as shipped (`@fontsource-variable/golos-text` 5.3.0) | SIL OFL 1.1 | the whole interface, one variable font (latin + cyrillic subsets) | `static/fonts/golos-text-*.woff2` |

Build- and CI-time only (not distributed in any artifact): `gofmt`,
`go vet`, `govulncheck`, the GitHub Actions runners and the pinned actions in
[.github/workflows](.github/workflows), Node.js for the front-end tests, and
Docker/Buildx for the image build.

## Font licensing note

The SIL Open Font License 1.1 permits embedding, bundling and redistribution
with an application, and places no restriction on documents or images produced
with the fonts. The only prohibition is selling the fonts by themselves. The
full license texts ship next to the font files:

- [static/fonts/OFL-Golos-Text.txt](static/fonts/OFL-Golos-Text.txt)

The font is also listed in the service worker's precache, so an offline admin
panel keeps its typography.

## Trademarks

"Lanpaper" is the name of this project. Neither the MIT License nor any notice
in this file grants a right to use the names or logos of the project, its
authors, or any third-party component listed above, except as required for
attribution. A commercial offering built on this code should not present itself
as "Lanpaper" without permission.

## License texts

### MIT License (joho/godotenv, SeriousBug/webp-go-pure)

```
Copyright (c) 2013 John Barton                                  (godotenv)
Copyright (c) 2026 MITH@mmk                                     (webp-go-pure)
Copyright (c) 2026 Kaan Barmore-Genc                            (webp-go-pure)
Copyright (c) 2023 Nuno Cruces                                   (go-sqlite3)
Copyright (c) 2022 Nuno Cruces                                   (julianday)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### BSD 3-Clause License (golang.org/x/image, Go standard library)

```
Copyright 2009 The Go Authors.                          (x/image, Go stdlib)
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software
   without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## Keeping this file current

CI verifies dependencies with `go mod download && go mod verify` and scans them
with `govulncheck`. When a dependency is added or bumped, update the table
above in the same change: `go.mod` is the authoritative list, and a release
must not ship a component that is not accounted for here.
