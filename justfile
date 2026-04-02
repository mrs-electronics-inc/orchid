set dotenv-load

default:
    @just --list

# Build the Orchid CLI
build:
    mkdir -p bin
    CGO_ENABLED=0 go build -o ./bin/orchid .

# Remove a VM and its disk artifacts: sudo just destroy-vm <vm-name>
destroy-vm vm_name:
    ./scripts/destroy-vm.sh {{vm_name}}
