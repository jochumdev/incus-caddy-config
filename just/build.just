# Building and running incus-templated.

# Build binary into bin/incus-templated
build:
    @mkdir -p bin
    go build -o bin/incus-templated ./cmd/incus-templated

# Run incus-templated directly
run *args:
    @go run ./cmd/incus-templated {{ args }}
