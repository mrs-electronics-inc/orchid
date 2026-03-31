set dotenv-load

images := "/var/lib/libvirt/images"

default:
    @just --list
    
# Install dependencies and download the Debian 12 base image
setup:
    sudo apt install -y virtinst cloud-image-utils genisoimage qemu-utils sshpass wget
    test -f {{images}}/debian-12-base.qcow2 || \
        sudo wget -O {{images}}/debian-12-base.qcow2 \
        https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2

# Build the shared Orchid base image with the common toolchain
build-base:
    ./scripts/build-base.sh

# Build the Orchid CLI
build:
    mkdir -p bin
    CGO_ENABLED=0 go build -o ./bin/orchid .

# Remove a VM and its disk artifacts: sudo just destroy-vm <vm-name>
destroy-vm vm_name:
    ./scripts/destroy-vm.sh {{vm_name}}
