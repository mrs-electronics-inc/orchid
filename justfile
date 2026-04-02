set dotenv-load

default:
    @just --list

# Build the Orchid CLI
build:
    mkdir -p bin
    go build -o ./bin/orchid .

# Run the Orchid CLI locally, e.g. `just run -- --help`
run *args:
    go run . {{args}}
