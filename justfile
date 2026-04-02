set dotenv-load

default:
    @just --list

# Build the Orchid CLI
build:
    mkdir -p bin
    CGO_ENABLED=0 go build -o ./bin/orchid .
