set dotenv-load

default:
    @just --list

# Build the Orchid CLI
build:
    mkdir -p bin
    go build -o ./bin/orchid .

# Run the Orchid CLI locally, e.g. `just run list`
run *args:
    sh -c 'if [ "$#" -gt 0 ]; then exec go run . "$@"; else go run . --help || true; fi' sh {{args}}
