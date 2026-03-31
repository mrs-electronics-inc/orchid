#!/usr/bin/env bash
set -euo pipefail

IMAGES="/var/lib/libvirt/images"
BASE_LINK="${IMAGES}/orchid-base.qcow2"
CONNECT="qemu:///system"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/orchid-lib.sh"
VIRT_TYPE="$(orchid_select_virt_type)"
TMP_DIR=""

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

[[ -e "${BASE_LINK}" ]] || {
  echo "Missing Orchid base image: ${BASE_LINK}" >&2
  echo "Run: sudo just build-base" >&2
  exit 1
}

BASE="$(readlink -f "${BASE_LINK}")"

REPO_URL="$1"
shift

REPO_NAME="$(basename "${REPO_URL}" .git)"
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
  VM_NAME="${VM_OWNER}-${REPO_NAME}"
fi

echo "Creating VM '${VM_NAME}' for ${REPO_URL}..."

TMP_DIR="$(mktemp -d "/tmp/${VM_NAME}.XXXXXX")"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

# 1. Create thin-provisioned disk
qemu-img create -f qcow2 -b "${BASE}" -F qcow2 "${IMAGES}/${VM_NAME}.qcow2"

# 2. Write cloud-init configs
cat > "${TMP_DIR}/user-data" <<EOF
#cloud-config
hostname: ${VM_NAME}
ssh_pwauth: true
write_files:
  - path: /usr/local/bin/orchid-bootstrap.sh
    permissions: '0755'
    content: |
      #!/usr/bin/env bash
      set -euxo pipefail
      exec > >(tee -a /var/log/orchid-bootstrap.log) 2>&1

      systemctl restart sshd

      # Clone the repo for the dev user if it is not already present.
      if [[ ! -d "/home/dev/${REPO_NAME}/.git" ]]; then
        su - dev -c 'git clone ${REPO_URL} /home/dev/${REPO_NAME}'
      fi

      cat > /usr/local/bin/orchid-dev-shell.sh <<'ORCHID_SHELL'
      #!/usr/bin/env bash
      set -euo pipefail

      repo_dir="__REPO_DIR__"

      cd "\${repo_dir}"
      export PATH="/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:/usr/local/bin:\${PATH}"

      # Only auto-enter the flake shell for interactive login shells.
      case \$- in
        *i*) ;;
        *) return 0 2>/dev/null || exit 0 ;;
      esac

      if [[ -n "\${IN_NIX_SHELL:-}" ]]; then
        return 0 2>/dev/null || exit 0
      fi

      if [[ -f flake.nix ]]; then
        exec nix develop -c bash --login
      fi
      ORCHID_SHELL
      sed -i 's|__REPO_DIR__|/home/dev/${REPO_NAME}|' /usr/local/bin/orchid-dev-shell.sh
      chmod 0755 /usr/local/bin/orchid-dev-shell.sh

      cat > /home/dev/.bash_profile <<'ORCHID_PROFILE'
      # Source the normal interactive shell config first.
      if [[ -f ~/.bashrc ]]; then
        . ~/.bashrc
      fi

      . /usr/local/bin/orchid-dev-shell.sh
      ORCHID_PROFILE
      chown dev:dev /home/dev/.bash_profile
runcmd:
  - /usr/local/bin/orchid-bootstrap.sh
EOF

cat > "${TMP_DIR}/meta-data" <<EOF
instance-id: ${VM_NAME}
local-hostname: ${VM_NAME}
EOF

cat > "${TMP_DIR}/network-config" <<EOF
version: 2
ethernets:
  default:
    match:
      name: "e*"
    dhcp4: true
EOF

# 3. Create seed ISO
cloud-localds --network-config="${TMP_DIR}/network-config" \
  "${IMAGES}/${VM_NAME}-seed.iso" \
  "${TMP_DIR}/user-data" \
  "${TMP_DIR}/meta-data"

# 4. Launch VM
virt-install \
  --connect "${CONNECT}" \
  --virt-type "${VIRT_TYPE}" \
  --name "${VM_NAME}" \
  --memory 2048 \
  --vcpus 1 \
  --disk "path=${IMAGES}/${VM_NAME}.qcow2,format=qcow2" \
  --disk "path=${IMAGES}/${VM_NAME}-seed.iso,device=cdrom" \
  --security type=none \
  --os-variant debian12 \
  --network "network=default,model=virtio" \
  --graphics none \
  --console pty,target_type=serial \
  --noautoconsole \
  --import

# 5. Wait for IP
echo "Waiting for VM to get an IP..."
IP="$(orchid_wait_for_ip "${CONNECT}" "${VM_NAME}" 30)" || true

if [[ -n "${IP}" ]]; then
  CLOUD_INIT_VERIFIED=0
  if command -v sshpass >/dev/null 2>&1; then
    echo "Waiting for SSH to become available..."
    orchid_wait_for_ssh "${IP}" dev dev 60 || true

    echo "Waiting for cloud-init to finish..."
    if ! orchid_wait_for_cloud_init "${IP}"; then
      exit 1
    fi
    CLOUD_INIT_VERIFIED=1
  else
    echo "sshpass is not installed on the host, so cloud-init completion was not checked automatically."
    echo "After connecting, run: sudo cloud-init status --wait"
  fi

  echo ""
  echo "VM '${VM_NAME}' is ready!"
  echo "  ssh dev@${IP}"
  echo ""
  if [[ "${CLOUD_INIT_VERIFIED}" -eq 1 ]]; then
    echo "cloud-init completed."
  else
    echo "cloud-init completion was not verified."
  fi
else
  echo ""
  echo "VM '${VM_NAME}' started but no IP yet. Check manually:"
  echo "  virsh -c ${CONNECT} domifaddr ${VM_NAME}"
fi
