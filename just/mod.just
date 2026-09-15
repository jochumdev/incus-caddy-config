# The module and code hygiene gates.

# Run this before committing or pushing.
pre-commit:
    just tidy
    just lint

# Lint all files. The path scopes the Go step.
lint path="./...":
    golangci-lint run {{ path }}

# Lint and fix all files.
fix path="./...":
    golangci-lint run --fix {{ path }}

# Run `go mod tidy`.
[doc("Run `go mod tidy`")]
tidy:
    go mod tidy
