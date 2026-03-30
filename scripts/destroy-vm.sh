#!/usr/bin/env bash
set -euo pipefail

IMAGES="/var/lib/libvirt/images"
CONNECT="qemu:///system"

usage() {
  cat >&2 <<EOF
Usage: $0 <vm-name>

Arguments:
  vm-name                      Name of the VM to remove
EOF
  exit 1
}

[[ $# -eq 1 ]] || usage

VM_NAME="$1"
DISK_PATH="${IMAGES}/${VM_NAME}.qcow2"
SEED_PATH="${IMAGES}/${VM_NAME}-seed.iso"

echo "Removing VM '${VM_NAME}'..."

if virsh -c "${CONNECT}" dominfo "${VM_NAME}" >/dev/null 2>&1; then
  STATE="$(virsh -c "${CONNECT}" domstate "${VM_NAME}" 2>/dev/null || true)"
  if [[ "${STATE}" == "running" || "${STATE}" == "in shutdown" || "${STATE}" == "paused" ]]; then
    virsh -c "${CONNECT}" destroy "${VM_NAME}"
  fi

  virsh -c "${CONNECT}" undefine "${VM_NAME}"
else
  echo "VM definition '${VM_NAME}' does not exist."
fi

rm -f "${DISK_PATH}" "${SEED_PATH}"

echo "VM '${VM_NAME}' removed."
