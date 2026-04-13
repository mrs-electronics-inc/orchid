set dotenv-load

default:
    @just --list

# Download module dependencies
deps:
    @go mod download

# Build the Orchid CLI
build:
    @mkdir -p bin
    @go build -ldflags "-X main.version=dev -X main.commit=$(git rev-parse --short=7 HEAD 2>/dev/null || true)" -o ./bin/orchid ./cmd/orchid

# Format Go source files
format:
    @go fmt ./...

# Run static analysis
lint:
    @go vet ./...

# Run the Go test suite
test:
    @go test ./...

# Run the standard repository checks
check: format lint test

# Run the Orchid CLI locally, e.g. `just run list`
run *args: build
    @./bin/orchid {{args}}
