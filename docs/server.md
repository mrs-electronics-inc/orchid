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

### Install the daemon

After checking out the repo on the host, install the CLI with `go install .` and then register the checked-in systemd service:

```bash
go install .
"$(go env GOPATH)/bin/orchid" server install
```

That command installs `/usr/local/bin/orchid`, writes `orchid.service` to `/etc/systemd/system`, reloads systemd, and enables the service. If you set `GOBIN`, use that directory instead of `$(go env GOPATH)/bin`.

The daemon listens on `/run/orchid/orchid.sock`, and laptop-side commands reach it through `ssh <hypervisor> orchid server proxy`.

Use `orchid server status` on the host to confirm the service state, and `orchid server run` if you want to run the daemon in the foreground during local debugging.

Once the daemon is installed, `orchid list` and `orchid create-vm` use it for VM discovery and lifecycle work.

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

## Maintenance

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

Rebuild the shared base image after changing the default toolchain:

```bash
sudo just build-base
```
