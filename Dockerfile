# --- Stage 1: Builder ---
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./

# Use cache mount for faster dependency downloads
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev

# Use cache mounts for faster builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -extldflags '-static'" \
    -o lanpaper .

# --- Stage 2: Runner ---
FROM alpine:3.24

# ca-certificates for HTTPS; wget is provided by busybox (already in Alpine)
# and is used only for the HEALTHCHECK — no extra packages needed.
RUN apk --no-cache add ca-certificates

# Run as non-root user for security
RUN addgroup -S lanpaper && adduser -S lanpaper -G lanpaper

WORKDIR /app

COPY --from=builder /app/lanpaper .
COPY admin.html .
COPY static ./static

# Verify critical static files exist
RUN echo "Verifying static files..." && \
    for f in \
      static/css/style.css \
      static/js/app.js \
      static/js/compressor.js \
      static/js/settings-menu.js \
      static/js/export-import.js \
      static/sw.js \
      static/manifest.json \
      static/logo.svg \
      static/favicon.svg \
      static/i18n/en.json \
      static/icons/icon-512.png \
    ; do test -f "$f" || (echo "ERROR: $f missing!" && exit 1) || exit 1; done && \
    echo "✓ All critical static files present" && \
    ls -lh static/css/ static/js/ static/*.svg

RUN mkdir -p data/media data/previews static/images/previews external/images \
    && chown -R lanpaper:lanpaper /app

USER lanpaper

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./lanpaper"]
