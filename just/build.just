# Building and running caddy-config.

# Build binary into bin/caddy-config
build:
    @mkdir -p bin
    go build -o bin/caddy-config ./cmd/caddy-config

# Run caddy-config directly
run *args:
    @go run ./cmd/caddy-config {{ args }}
