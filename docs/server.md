# Server Setup

Orchid expects a Linux hypervisor host with KVM/QEMU, libvirt, a `default` NAT network, and a storage pool at `/var/lib/libvirt/images`.

## Image Layout

| Resource | Value |
| -------- | ----- |
| Base OS | Debian 12 (`generic` qcow2) |
| Shared base | `orchid-base.qcow2` symlink to the current versioned Orchid base image with Nix, Node.js, Go, PI coding agent, Codex CLI, zsh, direnv, `fd`, `ripgrep`, default Codex config, and common operator tools |
| VM disk | Thin qcow2 overlay backed by `orchid-base.qcow2` |
| Auth | SSH key from `~/.config/orchid/config.toml` |

## Host Setup

Run the following on the hypervisor host with root privileges.

Install the required host packages:

```bash
sudo apt install -y virtinst cloud-image-utils genisoimage qemu-utils sshpass wget
```

Make sure the host has KVM/QEMU, libvirt, `qemu-img`, `virt-install`, `cloud-localds`, `ssh`, and `sshpass` available. `orchid server install` uses those tools directly and no longer requires a repo checkout on the hypervisor.

### Install the daemon

Install the CLI with the same command used in the README, then register or refresh the checked-in systemd service with `sudo`:

```bash
go install github.com/mrs-electronics-inc/orchid@latest
sudo orchid server install
```

That command installs `/usr/local/bin/orchid`, downloads the Debian 12 base image if needed, builds the shared Orchid base image if it is missing, writes `orchid.service` to `/etc/systemd/system`, reloads systemd, enables the service, and restarts it if it is already running. If `orchid` is not on `PATH` after `go install`, use the binary from `$(go env GOPATH)/bin` or your `GOBIN`.

Run `orchid server status` after install to confirm the daemon is enabled and active.

The daemon listens on `/run/orchid/orchid.sock`, and laptop-side commands reach it through `ssh <hypervisor> orchid server proxy`.

Use `orchid server status` on the host to confirm the service state, `orchid server build-base` to refresh the shared base image later, and `orchid server run` if you want to run the daemon in the foreground during local debugging.

Once the daemon is installed, `orchid list`, `orchid create-vm`, and `orchid destroy-vm` use it for VM discovery and lifecycle work.

## Troubleshooting

If `sudo orchid server build-base` fails with `Host does not support any virtualization options` or `Unable to start event thread: Resource temporarily unavailable`, libvirtd is probably hitting a systemd task limit on the host. Increase the service limit and restart libvirtd:

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

## Maintenance

Orchid uses a two-stage image pipeline:

1. `debian-12-base.qcow2`
   Downloaded from Debian cloud images and kept pristine.
2. `orchid-base.qcow2`
   Built from the Debian base and preloaded with:
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

Rebuild the shared base image after changing the default toolchain:

```bash
sudo orchid server build-base
```
