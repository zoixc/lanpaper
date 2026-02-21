# BUILD STAGE
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with optimizations
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build \
    -ldflags="-w -s -X main.Version=v0.8.3" \
    -trimpath \
    -o /lanpaper \
    ./cmd/lanpaper

# RUNTIME STAGE
FROM scratch AS runtime

# Copy binary
COPY --from=builder /lanpaper /lanpaper

# Copy static files
COPY --from=builder /app/static /static
COPY --from=builder /app/config /config

# Expose port
EXPOSE 8080

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/lanpaper", "/health"] || exit 1

# Run
ENTRYPOINT ["/lanpaper"]