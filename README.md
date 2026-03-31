# orchid

_Because why not._

Lightweight, disposable Debian 12 VMs with Nix for running coding agents. Orchid keeps per-VM disks small by building a shared base image with the common toolchain already installed, then creating thin qcow2 overlays for each repo-specific VM. The shared base also gives `dev` a zsh shell with the `robbyrussell` theme, `direnv` for repo-local environments, and a default Codex config. VM login is key-based; password login is disabled in the guest.

## Requirements

- Linux host with KVM/QEMU and libvirt
- A `default` libvirt network (NAT) and storage pool

## Image Layout

| Resource | Value                           |
| -------- | ------------------------------- |
| Base OS  | Debian 12 (`generic` qcow2)     |
| Shared base | `orchid-base.qcow2` symlink to the current versioned Orchid base image with Nix, Node.js, Go, PI coding agent, Codex CLI, zsh, direnv, `fd`, `ripgrep`, default Codex config, and common operator tools |
| VM disk  | Thin qcow2 overlay backed by `orchid-base.qcow2` |
| Auth     | SSH key from `~/.config/orchid/config.toml` |

## VM Spec (per instance)

| Resource | Value |
| -------- | ----- |
| vCPU     | 1     |
| RAM      | 2 GB  |
| Disk     | Thin qcow2 overlay; physical usage grows with repo-specific writes |

## Prerequisites

Run the following on the **hypervisor host**.

`orchid` still creates disks in the system libvirt storage pool at `/var/lib/libvirt/images` and talks to `qemu:///system`, so the host-side setup/build commands below should be run with root privileges. `orchid create-vm` itself runs on your laptop and SSHes to the configured hypervisor.

### Create a shared workspace

```bash
sudo groupadd --system orchid
sudo usermod -aG orchid "$USER"
sudo mkdir -p /srv/orchid
sudo chgrp -R orchid /srv/orchid
sudo chmod -R 2775 /srv/orchid
newgrp orchid
```

Add any future users to the `orchid` group so they can manage VMs in the shared workspace.

### Clone and set up

```bash
git clone https://github.com/mrs-electronics-inc/orchid.git /srv/orchid
git config --global --add safe.directory /srv/orchid
cd /srv/orchid
sudo just setup
sudo just build-base
```

`just setup` installs host dependencies and downloads the Debian 12 `generic` base image. `just build-base` creates a new versioned Orchid base image and refreshes `orchid-base.qcow2` to point at it.

## Troubleshooting

If `sudo just build-base` fails with `Host does not support any virtualization options` or `Unable to start event thread: Resource temporarily unavailable`, libvirtd is probably hitting a systemd task limit on the host. Increase the service limit and restart libvirtd:

```bash
sudo systemctl edit libvirtd
```

```ini
[Service]
TasksMax=65536
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart libvirtd
```

## Usage

Point orchid at a Git repo URL. `orchid create-vm` runs on your laptop, talks to the configured hypervisor, derives the VM name, and provisions a VM from the shared Orchid base image. By default, VM names are prefixed by the local username so different developers do not collide.

```bash
orchid create-vm --identity-file ~/.ssh/id_ed25519 https://github.com/specture-system/specture

orchid create-vm \
  --identity-file ~/.ssh/id_ed25519 \
  --name my-dev \
  https://github.com/specture-system/specture

sudo just destroy-vm addison-specture
```

On first boot, cloud-init performs only VM-specific setup: setting the hostname, cloning the target repo, installing the authorized key, and dropping a repo-local `.envrc` so `direnv` loads the flake when the checkout has a `flake.nix`. The shared base already provides Nix, zsh, the `robbyrussell` theme, `direnv`, and the common toolchain.

Use `orchid connect <vm-name>` from your laptop to resolve the current IP and open SSH through the configured hypervisor without managing per-VM aliases. Password login is disabled in the guest.

## Base Image Workflow

Orchid uses a two-stage image pipeline:

1. `debian-12-base.qcow2`
   Downloaded from Debian cloud images and kept pristine.
2. `orchid-base.qcow2`
   Built once from the Debian base and preloaded with:
   - Nix with flakes enabled
   - Node.js
   - Go
   - PI coding agent
   - Codex CLI
   - zsh with the `robbyrussell` theme
   - `direnv`
   - default Codex config in `~/.codex/config.toml`
   - `git`, `curl`, `helix`, `zellij`, `fd`, and `ripgrep`
3. Per-VM overlay
   Created for each repo and used only for repo checkout, repo-specific closures, and transient build output.

This keeps common Nix store contents in the shared base layer so dozens of VMs do not each pay to install the same baseline toolchain.

## Lifecycle Commands

Use orchid to remove the VM definition and disk artifacts together:

```bash
sudo just destroy-vm <vm-name>
```

Rebuild the shared base image after changing the default toolchain:

```bash
sudo just build-base
```

## Orchid CLI

Install with Go:

```bash
go install github.com/mrs-electronics-inc/orchid@latest
```

Configure the hypervisor once:

```bash
orchid config set hypervisor cs02
```

Configure the SSH identity once:

```bash
orchid config set identity-file ~/.ssh/id_ed25519
```

The config file lives at `~/.config/orchid/config.toml` and uses simple TOML keys:

```toml
hypervisor = "cs02"
identity_file = "/home/addison/.ssh/id_ed25519"
```

One-off overrides still come from flags:

```bash
orchid connect --hypervisor cs02 --identity-file ~/.ssh/id_ed25519 addison-specture
```

If both `hypervisor` and `identity_file` are already set in `~/.config/orchid/config.toml`, the flags can be omitted.

## License

[MIT](LICENSE)
