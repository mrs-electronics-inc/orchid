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

# 1. Create thin-provisioned disk
qemu-img create -f qcow2 -b "${BASE}" -F qcow2 "${IMAGES}/${VM_NAME}.qcow2" 10G

# 2. Write cloud-init configs
cat > "/tmp/${VM_NAME}-user-data" <<EOF
#cloud-config
hostname: ${VM_NAME}
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
      # Keep terminal compatibility but avoid forwarding client locale vars
      # into the VM, which may not have those locales generated.
      AcceptEnv TERM
  - path: /usr/local/bin/orchid-bootstrap.sh
    permissions: '0755'
    content: |
      #!/usr/bin/env bash
      set -euxo pipefail
      exec > >(tee -a /var/log/orchid-bootstrap.log) 2>&1

      systemctl restart sshd
      update-locale LANG=C.UTF-8

      # Install Nix (multi-user daemon mode)
      export HOME=/root
      curl -L https://nixos.org/nix/install | sh -s -- --daemon --yes

      # Enable the modern Nix CLI for flake-based dev shells.
      mkdir -p /etc/nix
      if ! grep -q '^experimental-features = .*flakes' /etc/nix/nix.conf 2>/dev/null; then
        printf '\nexperimental-features = nix-command flakes\n' >> /etc/nix/nix.conf
      fi

      # Clone the repo for the dev user if it is not already present.
      if [[ ! -d "/home/dev/${REPO_NAME}/.git" ]]; then
        su - dev -c 'git clone ${REPO_URL} /home/dev/${REPO_NAME}'
      fi

      cat > /usr/local/bin/orchid-dev-shell.sh <<'ORCHID_SHELL'
      #!/usr/bin/env bash
      set -euo pipefail

      repo_dir="__REPO_DIR__"

      cd "\${repo_dir}"
      export PATH="\${HOME}/.npm-global/bin:\${PATH}"

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

      su - dev -c '
        export PATH="/nix/var/nix/profiles/default/bin:/home/dev/.npm-global/bin:\${PATH}"
        mkdir -p /home/dev/.npm-global
        nix profile install nixpkgs#helix nixpkgs#zellij nixpkgs#nodejs
        NPM_CONFIG_PREFIX=/home/dev/.npm-global npm install -g @mariozechner/pi-coding-agent
      '
runcmd:
  - /usr/local/bin/orchid-bootstrap.sh
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
  IP="$(virsh -c "${CONNECT}" domifaddr "${VM_NAME}" 2>/dev/null | awk '/ipv4/ && ip == "" {split($4,a,"/"); ip=a[1]} END {print ip}')"
  if [[ -z "${IP}" ]]; then
    MAC="$(virsh -c "${CONNECT}" domiflist "${VM_NAME}" 2>/dev/null | awk 'NR > 2 && $5 != "-" && mac == "" {mac=$5} END {print mac}' | tr '[:upper:]' '[:lower:]')"
    if [[ -n "${MAC}" ]]; then
      IP="$(virsh -c "${CONNECT}" net-dhcp-leases default 2>/dev/null | awk -v mac="${MAC}" 'tolower($0) ~ mac && /ipv4/ && ip == "" {split($5,a,"/"); ip=a[1]} END {print ip}')"
    fi
  fi
  [[ -n "${IP}" ]] && break
  echo "  attempt ${i}/30: no IP yet"
  sleep 2
done

if [[ -n "${IP}" ]]; then
  CLOUD_INIT_VERIFIED=0
  if command -v sshpass >/dev/null 2>&1; then
    echo "Waiting for SSH to become available..."
    for i in $(seq 1 60); do
      if sshpass -p dev ssh \
        -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=5 \
        dev@"${IP}" true >/dev/null 2>&1; then
        break
      fi
      echo "  attempt ${i}/60: ssh not ready yet"
      sleep 2
    done

    echo "Waiting for cloud-init to finish..."
    if ! sshpass -p dev ssh -tt \
      -o StrictHostKeyChecking=no \
      -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=5 \
      dev@"${IP}" 'sudo cloud-init status --wait && sudo cloud-init status --long'; then
      echo ""
      echo "cloud-init reported a failure. Recent bootstrap log:"
      sshpass -p dev ssh \
        -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=5 \
        dev@"${IP}" 'sudo tail -n 200 /var/log/orchid-bootstrap.log || sudo tail -n 200 /var/log/cloud-init-output.log || true'
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
