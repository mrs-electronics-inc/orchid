# orchid

_Because why not._

Lightweight, disposable Debian 12 VMs with Nix for running coding agents. Orchid keeps per-VM disks small by building a shared base image with the common toolchain already installed, then creating thin qcow2 overlays for each repo-specific VM. VMs default to a `dev` user with password `dev`.

## Requirements

- Linux host with KVM/QEMU and libvirt
- A `default` libvirt network (NAT) and storage pool

## Image Layout

| Resource | Value                           |
| -------- | ------------------------------- |
| Base OS  | Debian 12 (`generic` qcow2)     |
| Shared base | `orchid-base.qcow2` symlink to the current versioned Orchid base image with Nix, Node.js, Go, PI coding agent, and common operator tools |
| VM disk  | Thin qcow2 overlay backed by `orchid-base.qcow2` |
| Auth     | `dev` / `dev`                  |

## VM Spec (per instance)

| Resource | Value |
| -------- | ----- |
| vCPU     | 1     |
| RAM      | 2 GB  |
| Disk     | Thin qcow2 overlay; physical usage grows with repo-specific writes |

## Prerequisites

Run the following on the **hypervisor host**.

`orchid` currently creates disks in the system libvirt storage pool at `/var/lib/libvirt/images` and talks to `qemu:///system`, so the setup and VM creation commands below should be run as `root`.

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

Point orchid at a Git repo URL. It derives the VM name and provisions a VM from the shared Orchid base image. By default, VM names will be prefixed by the username on the hypervisor. This avoids collisions between different developers' VMs.

```bash
# Create a VM from the shared Orchid base image
sudo just create-vm https://github.com/specture-system/specture

# Override the VM name
sudo just create-vm https://github.com/specture-system/specture --name my-dev

# Remove a VM and its disk artifacts
sudo just destroy-vm addison-specture
```

On first boot, cloud-init performs only VM-specific setup: setting the hostname, cloning the target repo, and wiring the login shell so interactive sessions auto-enter `nix develop` when the repo has a `flake.nix`. Nix itself and the common toolchain are already present in the shared base image. `just create-vm` waits for cloud-init to finish before returning when `sshpass` is available on the hypervisor host.

You can log in over the serial console or SSH with username `dev` and password `dev`.
Use `orchid connect <vm-name>` from your laptop to resolve the current IP and open SSH without managing per-VM aliases.

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
   - `git`, `curl`, `helix`, and `zellij`
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

Set `ORCHID_HYPERVISOR` before using `orchid connect`. Orchid does not ship a default hypervisor host.

Connect to a VM by name:

```bash
ORCHID_HYPERVISOR=<hypervisor-host> \
orchid connect addison-specture
```

## License

[MIT](LICENSE)
