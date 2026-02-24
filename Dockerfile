# --- Stage 1: Builder ---
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -extldflags '-static'" \
    -o lanpaper .

# --- Stage 2: Runner ---
FROM alpine:latest

RUN apk --no-cache add ca-certificates wget

# Run as non-root user for security
RUN addgroup -S lanpaper && adduser -S lanpaper -G lanpaper

WORKDIR /app

COPY --from=builder /app/lanpaper .
COPY admin.html .
COPY static ./static

RUN mkdir -p data static/images/previews external/images \
    && chown -R lanpaper:lanpaper /app

USER lanpaper

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./lanpaper"]
