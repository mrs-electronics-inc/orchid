#!/usr/bin/env bash
set -euo pipefail

IMAGES="/var/lib/libvirt/images"
BASE="${IMAGES}/debian-12-base.qcow2"
CONNECT="qemu:///system"

usage() {
  cat >&2 <<EOF
Usage: $0 <repo-url> [options]

Arguments:
  repo-url                     Git repository URL (name derived automatically)

Options:
  --name "vm-name"             Override the VM name (default: repo name)
EOF
  exit 1
}

[[ $# -ge 1 ]] || usage

REPO_URL="$1"
shift

VM_NAME=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) VM_NAME="$2"; shift 2 ;;
    *)      echo "Unknown option: $1" >&2; usage ;;
  esac
done

# Derive VM name from repo URL if not provided, prefixed with username
if [[ -z "${VM_NAME}" ]]; then
  VM_OWNER="${SUDO_USER:-$(whoami)}"
  VM_NAME="${VM_OWNER}-$(basename "${REPO_URL}" .git)"
fi

echo "Creating VM '${VM_NAME}' for ${REPO_URL}..."

# 1. Create thin-provisioned disk
qemu-img create -f qcow2 -b "${BASE}" -F qcow2 "${IMAGES}/${VM_NAME}.qcow2" 10G

# 2. Write cloud-init configs
cat > "/tmp/${VM_NAME}-user-data" <<EOF
#cloud-config
hostname: ${VM_NAME}
ssh_pwauth: true
users:
  - name: dev
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: false
chpasswd:
  expire: false
  users:
    - name: dev
      password: dev
      type: text
packages:
  - git
  - curl
  - xz-utils
package_update: true
write_files:
  - path: /etc/ssh/sshd_config.d/orchid.conf
    content: |
      PasswordAuthentication yes
runcmd:
  - systemctl restart sshd
  - |
    # Install Nix (multi-user daemon mode)
    curl -L https://nixos.org/nix/install | sh -s -- --daemon --yes
  - |
    # Clone the repo
    su - dev -c 'git clone ${REPO_URL} /home/dev/${VM_NAME}'
EOF

cat > "/tmp/${VM_NAME}-meta-data" <<EOF
instance-id: ${VM_NAME}
local-hostname: ${VM_NAME}
EOF

cat > "/tmp/${VM_NAME}-network-config" <<EOF
version: 2
ethernets:
  default:
    match:
      name: "e*"
    dhcp4: true
EOF

# 3. Create seed ISO
cloud-localds --network-config="/tmp/${VM_NAME}-network-config" \
  "${IMAGES}/${VM_NAME}-seed.iso" \
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
  --network "network=default,model=virtio" \
  --graphics none \
  --console pty,target_type=serial \
  --noautoconsole \
  --import

# 5. Wait for IP
echo "Waiting for VM to get an IP..."
for i in $(seq 1 30); do
  IP="$(virsh -c "${CONNECT}" domifaddr "${VM_NAME}" 2>/dev/null | awk '/ipv4/ {split($4,a,"/"); print a[1]; exit}')"
  if [[ -z "${IP}" ]]; then
    MAC="$(virsh -c "${CONNECT}" domiflist "${VM_NAME}" 2>/dev/null | awk 'NR > 2 && $5 != "-" {print $5; exit}' | tr '[:upper:]' '[:lower:]')"
    if [[ -n "${MAC}" ]]; then
      IP="$(virsh -c "${CONNECT}" net-dhcp-leases default 2>/dev/null | awk -v mac="${MAC}" 'tolower($0) ~ mac && /ipv4/ {split($5,a,"/"); print a[1]; exit}')"
    fi
  fi
  [[ -n "${IP}" ]] && break
  echo "  attempt ${i}/30: no IP yet"
  sleep 2
done

if [[ -n "${IP}" ]]; then
  echo ""
  echo "VM '${VM_NAME}' is ready!"
  echo "  ssh dev@${IP}"
  echo ""
  echo "Nix will be installed on first boot (may take a few minutes)."
else
  echo ""
  echo "VM '${VM_NAME}' started but no IP yet. Check manually:"
  echo "  virsh -c ${CONNECT} domifaddr ${VM_NAME}"
fi
