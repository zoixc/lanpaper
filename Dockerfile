# SPDX-License-Identifier: MIT
# --- Stage 1: Builder ---
FROM golang:1.27-alpine AS builder

# gcc/musl-dev: CGO is required by the WebP encoder (github.com/chai2010/webp).
RUN apk add --no-cache gcc musl-dev

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

ARG VERSION=dev

# Static binary: no shared libraries are needed at runtime.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.Version=${VERSION} -extldflags '-static'" \
    -o /out/lanpaper .

# --- Stage 2: Runner ---
FROM alpine:3.24

ARG VERSION=dev
LABEL org.opencontainers.image.title="Lanpaper" \
      org.opencontainers.image.description="Self-hosted wallpaper and media link server" \
      org.opencontainers.image.source="https://github.com/zoixc/lanpaper" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

# ca-certificates for outbound HTTPS downloads. wget (used by HEALTHCHECK)
# is part of busybox in the base image. The fixed IDs (uid 100, gid 101) match
# earlier images; bind-mounted data directories must be writable by them.
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 101 lanpaper && adduser -S -u 100 -G lanpaper lanpaper

WORKDIR /app

COPY --from=builder /out/lanpaper .
COPY admin.html .
COPY static ./static

# Fail the build early if an application asset is missing.
RUN for f in \
      static/css/style.css static/js/app.js static/js/compressor.js \
      static/js/settings-menu.js static/js/export-import.js static/sw.js \
      static/manifest.json static/logo.svg static/favicon.svg \
      static/i18n/en.json static/icons/icon-512.png \
      static/fonts/manrope-latin.woff2 ; \
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
