FROM golang:1.25.13-alpine AS build

WORKDIR /src
RUN apk add --no-cache build-base ca-certificates git tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/headless ./cmd/server && \
    go build -trimpath -ldflags="-s -w" -o /out/headless-backup ./cmd/headless-backup && \
    go build -trimpath -ldflags="-s -w" -o /out/headless-token ./cmd/headless-token

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S headless && adduser -S -G headless -h /var/lib/headless headless && \
    mkdir -p /var/lib/headless/media /etc/headless && \
    chown -R headless:headless /var/lib/headless /etc/headless

COPY --from=build /out/headless /usr/local/bin/headless
COPY --from=build /out/headless-backup /usr/local/bin/headless-backup
COPY --from=build /out/headless-token /usr/local/bin/headless-token

USER headless
WORKDIR /var/lib/headless

ENV APP_BIND=0.0.0.0:8080 \
    HEALTH_LIVE_PATH=/healthz \
    HEALTH_READY_PATH=/readyz \
    HEADLESS_STORAGE=bbolt \
    HEADLESS_BBOLT_PATH=/var/lib/headless/backend.db \
    HEADLESS_MEDIA_ROOT=/var/lib/headless/media

VOLUME ["/var/lib/headless"]
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1

ENTRYPOINT ["/usr/local/bin/headless"]
