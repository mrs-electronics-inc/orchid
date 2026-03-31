#!/usr/bin/env bash
set -euo pipefail

IMAGES="/var/lib/libvirt/images"
DEBIAN_BASE="${IMAGES}/debian-12-base.qcow2"
BASE_LINK="${IMAGES}/orchid-base.qcow2"
BASE_VERSION="orchid-base-$(date -u +%Y%m%d%H%M%S).qcow2"
BASE_IMAGE="${IMAGES}/${BASE_VERSION}"
BUILD_VM="orchid-base-build-$(date -u +%Y%m%d%H%M%S)"
BUILD_DISK="${IMAGES}/${BUILD_VM}.qcow2"
SEED_IMAGE="${IMAGES}/${BUILD_VM}-seed.iso"
CONNECT="qemu:///system"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/orchid-lib.sh"
VIRT_TYPE="$(orchid_select_virt_type)"
TMP_DIR="$(mktemp -d "/tmp/${BUILD_VM}.XXXXXX")"

cleanup() {
  rm -rf "${TMP_DIR}"
  if virsh -c "${CONNECT}" dominfo "${BUILD_VM}" >/dev/null 2>&1; then
    virsh -c "${CONNECT}" destroy "${BUILD_VM}" >/dev/null 2>&1 || true
    virsh -c "${CONNECT}" undefine "${BUILD_VM}" >/dev/null 2>&1 || true
  fi
  rm -f "${SEED_IMAGE}" "${BUILD_DISK}"
}

trap cleanup EXIT

[[ -f "${DEBIAN_BASE}" ]] || {
  echo "Missing Debian base image: ${DEBIAN_BASE}" >&2
  echo "Run: sudo just setup" >&2
  exit 1
}

if [[ -e "${BASE_LINK}" && ! -L "${BASE_LINK}" ]]; then
  echo "Refusing to replace non-symlink base image at ${BASE_LINK}." >&2
  echo "Rename or remove it first, then rerun: sudo just build-base" >&2
  exit 1
fi

echo "Creating shared Orchid base image '${BASE_VERSION}'..."
qemu-img create -f qcow2 -b "${DEBIAN_BASE}" -F qcow2 "${BUILD_DISK}" 30G

cat > "${TMP_DIR}/user-data" <<EOF
#cloud-config
hostname: orchid-base
ssh_pwauth: true
locale: false
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
  - locales
  - xz-utils
package_update: true
write_files:
  - path: /etc/ssh/sshd_config.d/orchid.conf
    content: |
      PasswordAuthentication yes
      AcceptEnv TERM
  - path: /usr/local/bin/orchid-bootstrap.sh
    permissions: '0755'
    content: |
      #!/usr/bin/env bash
      set -euxo pipefail
      exec > >(tee -a /var/log/orchid-bootstrap.log) 2>&1

      systemctl restart sshd
      update-locale LANG=C.UTF-8

      export HOME=/root
      curl -L https://nixos.org/nix/install | sh -s -- --daemon --yes

      mkdir -p /etc/nix /etc/profile.d
      if ! grep -q '^experimental-features = .*flakes' /etc/nix/nix.conf 2>/dev/null; then
        printf '\nexperimental-features = nix-command flakes\n' >> /etc/nix/nix.conf
      fi

      cat > /etc/profile.d/orchid-path.sh <<'ORCHID_PATH'
      export PATH="/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:/usr/local/bin:\${PATH}"
      ORCHID_PATH
      chmod 0644 /etc/profile.d/orchid-path.sh

      export PATH="/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:\${PATH}"
      nix profile install nixpkgs#helix nixpkgs#zellij nixpkgs#nodejs nixpkgs#go

      NPM_CONFIG_PREFIX=/usr/local npm install -g @mariozechner/pi-coding-agent
runcmd:
  - /usr/local/bin/orchid-bootstrap.sh
EOF

cat > "${TMP_DIR}/meta-data" <<EOF
instance-id: ${BUILD_VM}
local-hostname: orchid-base
EOF

cat > "${TMP_DIR}/network-config" <<EOF
version: 2
ethernets:
  default:
    match:
      name: "e*"
    dhcp4: true
EOF

cloud-localds --network-config="${TMP_DIR}/network-config" \
  "${SEED_IMAGE}" \
  "${TMP_DIR}/user-data" \
  "${TMP_DIR}/meta-data"

virt-install \
  --connect "${CONNECT}" \
  --virt-type "${VIRT_TYPE}" \
  --name "${BUILD_VM}" \
  --memory 2048 \
  --vcpus 1 \
  --disk "path=${BUILD_DISK},format=qcow2" \
  --disk "path=${SEED_IMAGE},device=cdrom" \
  --security type=none \
  --os-variant debian12 \
  --network "network=default,model=virtio" \
  --graphics none \
  --console pty,target_type=serial \
  --noautoconsole \
  --import

echo "Waiting for base builder VM to get an IP..."
IP="$(orchid_wait_for_ip "${CONNECT}" "${BUILD_VM}" 60)" || {
  echo "Build VM did not receive an IP address." >&2
  exit 1
}

echo "Waiting for SSH to become available..."
orchid_wait_for_ssh "${IP}" || {
  echo "Build VM SSH did not become ready in time." >&2
  exit 1
}

echo "Waiting for cloud-init to finish..."
if ! orchid_wait_for_cloud_init "${IP}"; then
  exit 1
fi

echo "Cleaning the image for cloning..."
sshpass -p dev ssh \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o ConnectTimeout=5 \
  dev@"${IP}" '
    sudo cloud-init clean --logs --seed &&
    sudo rm -f /etc/ssh/ssh_host_* &&
    sudo truncate -s 0 /etc/machine-id &&
    sudo rm -f /var/lib/dbus/machine-id &&
    sudo sync &&
    sudo shutdown -h now
  ' >/dev/null 2>&1 || true

echo "Waiting for builder VM to shut down..."
for attempt in $(seq 1 60); do
  STATE="$(virsh -c "${CONNECT}" domstate "${BUILD_VM}" 2>/dev/null || true)"
  if [[ "${STATE}" == "shut off" ]]; then
    break
  fi
  echo "  attempt ${attempt}/60: state is '${STATE:-unknown}'"
  sleep 2
done

STATE="$(virsh -c "${CONNECT}" domstate "${BUILD_VM}" 2>/dev/null || true)"
if [[ "${STATE}" != "shut off" ]]; then
  echo "Builder VM did not shut down cleanly." >&2
  exit 1
fi

virsh -c "${CONNECT}" undefine "${BUILD_VM}"
mv "${BUILD_DISK}" "${BASE_IMAGE}"
ln -sfn "${BASE_VERSION}" "${BASE_LINK}"

echo ""
echo "Shared Orchid base image is ready."
echo "  ${BASE_IMAGE}"
echo ""
echo "Current base image link:"
echo "  ${BASE_LINK} -> ${BASE_VERSION}"
echo ""
echo "Old versioned Orchid base images are kept in place so existing overlays keep working."
