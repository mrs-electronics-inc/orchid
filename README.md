# orchid

_Because why not._

Lightweight, disposable Debian 12 VMs with Nix for running coding agents. Orchid keeps per-VM disks small by building a shared base image with the common toolchain already installed, then creating thin qcow2 overlays for each repo-specific VM. The shared base also gives `dev` a zsh shell with the `robbyrussell` theme, `direnv` for repo-local environments, and a default Codex config. VM login is key-based; password login is disabled in the guest.

## VM Spec (per instance)

| Resource | Value |
| -------- | ----- |
| vCPU     | 1     |
| RAM      | 2 GB  |
| Disk     | Thin qcow2 overlay; physical usage grows with repo-specific writes |

## Server Docs

Host setup, base image maintenance, and troubleshooting live in [docs/server.md](docs/server.md).

The hypervisor now runs an Orchid daemon managed with `orchid server install`, `orchid server status`, `orchid server run`, and `orchid server proxy`. The `orchid list` command talks to that daemon over SSH.

## Install

Install with Go:

```bash
go install github.com/mrs-electronics-inc/orchid@latest
```

## Config

Configure the hypervisor once:

```bash
orchid config set hypervisor <hypervisor-host>
```

Configure the SSH identity once:

```bash
orchid config set identity-file <path-to-identity>
```

The config file lives at `~/.config/orchid/config.toml` and uses simple TOML keys:

```toml
hypervisor = "<hypervisor-host>"
identity_file = "<path-to-identity>"
```

## Create Virtual Machine

`orchid create-vm` runs on your laptop, talks to the configured hypervisor daemon, derives the VM name, and submits a job to provision a VM from the shared Orchid base image. By default, VM names are prefixed by the local username so different developers do not collide.

```bash
orchid create-vm <repo-url>

orchid create-vm --name <vm-name> <repo-url>
```

The CLI prints job stage transitions while the daemon creates the disk, writes cloud-init seed data, starts the VM, waits for IP/SSH/cloud-init, and then prints `orchid connect <vm-name>`.

On first boot, cloud-init performs only VM-specific setup: setting the hostname, cloning the target repo, installing the authorized key, and dropping a repo-local `.envrc` so `direnv` loads the flake when the checkout has a `flake.nix`.

## Connect

```bash
orchid connect <vm-name>
```

Once `hypervisor` and `identity_file` are set in `~/.config/orchid/config.toml`, `orchid connect` does not need flags.

Use `orchid list` to inspect the VMs on the configured hypervisor:

```bash
orchid list
```

## License

[MIT](LICENSE)
