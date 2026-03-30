#!/usr/bin/env bash
set -euo pipefail

IMAGES="/var/lib/libvirt/images"
BASE="${IMAGES}/debian-12-base.qcow2"
CONNECT="qemu:///system"

usage() {
  cat >&2 <<EOF
Usage: $0 <vm-name> [options]

Options:
  --packages "pkg1 pkg2 ..."   Additional packages to install
  --ssh-key "key"              SSH public key (default: auto-detect from ~/.ssh/)
EOF
  exit 1
}

[[ $# -ge 1 ]] || usage

VM_NAME="$1"
shift

EXTRA_PACKAGES=""
SSH_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --packages) EXTRA_PACKAGES="$2"; shift 2 ;;
    --ssh-key)  SSH_KEY="$2"; shift 2 ;;
    *)          echo "Unknown option: $1" >&2; usage ;;
  esac
done

# Auto-detect SSH key if not provided
if [[ -z "${SSH_KEY}" ]]; then
  for key in ~/.ssh/id_ed25519.pub ~/.ssh/id_rsa.pub ~/.ssh/id_ecdsa.pub; do
    if [[ -f "${key}" ]]; then
      SSH_KEY="$(cat "${key}")"
      break
    fi
  done
  [[ -n "${SSH_KEY}" ]] || { echo "No SSH key found. Pass --ssh-key or add one to ~/.ssh/" >&2; exit 1; }
fi

# Build package list
PACKAGES="git curl build-essential"
[[ -n "${EXTRA_PACKAGES}" ]] && PACKAGES="${PACKAGES} ${EXTRA_PACKAGES}"

# Format as YAML list
PACKAGES_YAML=""
for pkg in ${PACKAGES}; do
  PACKAGES_YAML="${PACKAGES_YAML}
  - ${pkg}"
done

# 1. Create thin-provisioned disk
qemu-img create -f qcow2 -b "${BASE}" -F qcow2 "${IMAGES}/${VM_NAME}.qcow2" 10G

# 2. Write cloud-init configs
cat > "/tmp/${VM_NAME}-user-data" <<EOF
#cloud-config
hostname: ${VM_NAME}
users:
  - name: dev
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - ${SSH_KEY}
packages:${PACKAGES_YAML}
package_update: true
EOF

cat > "/tmp/${VM_NAME}-meta-data" <<EOF
instance-id: ${VM_NAME}
local-hostname: ${VM_NAME}
EOF

# 3. Create seed ISO
cloud-localds "${IMAGES}/${VM_NAME}-seed.iso" \
  "/tmp/${VM_NAME}-user-data" \
  "/tmp/${VM_NAME}-meta-data"

# 4. Launch VM
virt-install \
  --connect "${CONNECT}" \
  --name "${VM_NAME}" \
  --memory 2048 \
  --vcpus 1 \
  --disk "path=${IMAGES}/${VM_NAME}.qcow2,format=qcow2" \
  --disk "path=${IMAGES}/${VM_NAME}-seed.iso,device=cdrom" \
  --os-variant debian12 \
  --network network=default \
  --graphics none \
  --console pty,target_type=serial \
  --noautoconsole \
  --import

# 5. Wait for IP
echo "Waiting for VM to get an IP..."
for i in $(seq 1 30); do
  IP=$(virsh -c "${CONNECT}" domifaddr "${VM_NAME}" 2>/dev/null | awk '/ipv4/ {split($4,a,"/"); print a[1]}')
  [[ -n "${IP}" ]] && break
  sleep 2
done

if [[ -n "${IP}" ]]; then
  echo ""
  echo "VM '${VM_NAME}' is ready!"
  echo "  ssh dev@${IP}"
else
  echo ""
  echo "VM '${VM_NAME}' started but no IP yet. Check manually:"
  echo "  virsh -c ${CONNECT} domifaddr ${VM_NAME}"
fi
