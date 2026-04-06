set dotenv-load

default:
    @just --list

# Download module dependencies
deps:
    @go mod download

# Build the Orchid CLI
build:
    @mkdir -p bin
    @go build -o ./bin/orchid .

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
