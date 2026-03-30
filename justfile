set dotenv-load

images := "/var/lib/libvirt/images"

# Install dependencies and download the Debian 12 cloud image
setup:
    apt install -y virtinst cloud-image-utils genisoimage
    test -f {{images}}/debian-12-base.qcow2 || \
        wget -O {{images}}/debian-12-base.qcow2 \
        https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2

# Create a new VM from a repo: just create-vm <repo-url> [--name <name>]
create-vm +args:
    ./scripts/create-vm.sh {{args}}
