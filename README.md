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
