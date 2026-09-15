ARG GO_VERSION=1.27.1-alpine3.24
ARG BASE=docker.io/library/busybox:glibc

FROM docker.io/library/golang:${GO_VERSION} AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -tags=netgo -ldflags="-w -s -X main.version=${VERSION}" -trimpath -o /out/caddy-config ./cmd/caddy-config && \
    /out/caddy-config version && \
    /out/caddy-config run --help > /dev/null

FROM ${BASE}
COPY --from=build /out/caddy-config /usr/local/bin/caddy-config

WORKDIR /

EXPOSE 9153

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --spider http://127.0.0.1:9153/ready || exit 1

CMD ["caddy-config", "run"]
