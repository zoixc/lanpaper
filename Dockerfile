# --- Stage 1: Builder ---
FROM golang:1.25-alpine AS builder

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
FROM alpine:3.21

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
    test -f static/css/style.css || (echo "ERROR: static/css/style.css missing!" && exit 1) && \
    test -f static/css/skeleton.css || (echo "ERROR: static/css/skeleton.css missing!" && exit 1) && \
    test -f static/css/settings-menu.css || (echo "ERROR: static/css/settings-menu.css missing!" && exit 1) && \
    test -f static/js/app.js || (echo "ERROR: static/js/app.js missing!" && exit 1) && \
    test -f static/js/compressor.js || (echo "ERROR: static/js/compressor.js missing!" && exit 1) && \
    test -f static/js/settings-menu.js || (echo "ERROR: static/js/settings-menu.js missing!" && exit 1) && \
    test -f static/logo.svg || (echo "ERROR: static/logo.svg missing!" && exit 1) && \
    test -f static/favicon.svg || (echo "ERROR: static/favicon.svg missing!" && exit 1) && \
    echo "✓ All critical static files present" && \
    ls -lh static/css/ static/js/ static/*.svg

RUN mkdir -p data static/images/previews external/images \
    && chown -R lanpaper:lanpaper /app

USER lanpaper

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./lanpaper"]
