FROM --platform=$BUILDPLATFORM golang:1.25.3-alpine AS builder

# Устанавливаем необходимые инструменты для сборки
RUN apk add --no-cache gcc musl-dev libwebp-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=v0.8.7
ARG TARGETARCH
ARG TARGETOS

# Включаем CGO для работы с webp
RUN CGO_ENABLED=1 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
    -ldflags="-w -s -X main.Version=${VERSION} -linkmode external -extldflags '-static'" \
    -trimpath \
    -o /lanpaper \
    ./main.go

FROM alpine:3.20

# Добавляем runtime библиотеки
RUN apk --no-cache add ca-certificates tzdata libwebp

COPY --from=builder /lanpaper /lanpaper
COPY --from=builder /app/static /static
COPY --from=builder /app/config /config

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["/lanpaper", "health"] || exit 1
ENTRYPOINT ["/lanpaper"]
