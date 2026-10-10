# SPDX-License-Identifier: MIT
# --- Stage 1: Builder ---
# Build args let CI use an authenticated/rate-limit-independent registry mirror
# while release and local builds retain the familiar Docker Hub defaults.
ARG GO_IMAGE=golang:1.27-alpine
ARG ALPINE_IMAGE=alpine:3.24
FROM ${GO_IMAGE} AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

ARG VERSION=dev

# Static binary built with CGO off: no C toolchain is needed in the builder and
# no shared library is needed at runtime. WebP encoding is pure Go
# (github.com/SeriousBug/webp-go-pure).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /out/lanpaper .

# --- Stage 2: Runner ---
FROM ${ALPINE_IMAGE}

ARG VERSION=dev
LABEL org.opencontainers.image.title="Lanpaper" \
      org.opencontainers.image.description="Self-hosted wallpaper and media link server" \
      org.opencontainers.image.source="https://github.com/zoixc/lanpaper" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

# ca-certificates for outbound HTTPS downloads. wget (used by HEALTHCHECK)
# is part of busybox in the base image. The fixed IDs (uid 100, gid 101) match
# earlier images; bind-mounted data directories must be writable by them.
# zlib is pinned to a minimum version because alpine:3.24 ships 1.3.2-r0,
# which is affected by CVE-2026-85091; 1.3.2-r1 is the fixed build. A minimum
# (not an exact version) keeps later fixed revisions installable, and the build
# fails rather than ship the old library if the fix is not available.
RUN apk add --no-cache ca-certificates "zlib>=1.3.2-r1" \
    && addgroup -S -g 101 lanpaper && adduser -S -u 100 -G lanpaper lanpaper

WORKDIR /app

COPY --from=builder /out/lanpaper .
COPY admin.html login.html ./
COPY static ./static
# The licence and the third-party notices ship inside the image: a redistributor
# (or an auditor scanning a running container) must be able to read them without
# the repository.
COPY LICENSE THIRD-PARTY-NOTICES.md ./

# Fail the build early if an application asset is missing.
RUN for f in \
      static/css/style.css static/js/app.js static/js/api.js static/js/compressor.js \
      static/js/settings-menu.js static/js/export-import.js static/sw.js \
      static/manifest.json static/logo.svg static/favicon.svg \
      static/i18n/en.json static/icons/icon-512.png \
      static/fonts/golos-text-latin.woff2 static/fonts/golos-text-cyrillic.woff2 \
      LICENSE THIRD-PARTY-NOTICES.md ; \
    do test -f "$f" || { echo "ERROR: $f missing" >&2; exit 1; }; done

# Application code and assets stay root-owned (read-only for the service);
# only the runtime directories are writable by the unprivileged user.
RUN mkdir -p data/media data/previews external/images static/images/previews \
    && chown -R lanpaper:lanpaper data external static/images \
    && chmod 700 data

USER lanpaper

EXPOSE 8080

# Honours a custom PORT (with or without a leading colon).
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD p="${PORT:-8080}"; wget -qO- "http://127.0.0.1:${p#:}/health" || exit 1

CMD ["./lanpaper"]
