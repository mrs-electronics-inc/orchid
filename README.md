# orchid

_Because why not._

Lightweight, disposable Debian 12 VMs with Nix for running coding agents. Orchid keeps per-VM disks small by building a shared base image with the common toolchain already installed, then creating thin qcow2 overlays for each repo-specific VM. The shared base also gives `dev` a zsh shell with the `robbyrussell` theme, `direnv` for repo-local environments, a user-writable npm global prefix, and a default Codex config. VM login is key-based; password login is disabled in the guest.

VM lifecycle commands live under `orchid vm`.

## VM Spec (per instance)

| Resource | Value                                                              |
| -------- | ------------------------------------------------------------------ |
| vCPU     | 1                                                                  |
| RAM      | 2 GB                                                               |
| Disk     | Thin qcow2 overlay; physical usage grows with repo-specific writes |

## Server Docs

Host setup, base image maintenance, and troubleshooting live in [docs/server.md](docs/server.md). The SSH user that proxies to the hypervisor daemon must be in the `orchid` group.

## Install

Orchid is distributed through [GitHub Releases](https://github.com/mrs-electronics-inc/orchid/releases). Download the latest archive for your platform, extract `orchid`, and place it on your `PATH`.

You can also install with Go:

```bash
go install github.com/mrs-electronics-inc/orchid/cmd/orchid@latest
```

Verify the build with:

```bash
orchid --version
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

`orchid vm create` runs on your laptop, talks to the configured hypervisor daemon, derives the VM name, and submits a job to provision a VM from the shared Orchid base image. By default, VM names are prefixed by the local username so different developers do not collide.

```bash
orchid vm create <repo-url>

orchid vm create --name <vm-name> <repo-url>
```

The CLI prints job stage transitions while the daemon creates the disk, writes cloud-init seed data, starts the VM, waits for IP/SSH/cloud-init, verifies the repo checkout, warms the flake dev shell with `nix develop` on the hypervisor, and then prints `orchid vm connect <vm-name>`. `orchid vm connect` forces `TERM=xterm-256color` so the guest uses a standard terminal type.

On first boot, cloud-init performs only VM-specific setup: setting the hostname, cloning the target repo, installing the authorized key, and dropping a repo-local `.envrc` so `direnv` loads the flake when the checkout has a `flake.nix`.

Use `orchid vm destroy <vm-name>` to remove the VM and its disk artifacts.

## Connect

```bash
orchid vm connect <vm-name>
```

Once `hypervisor` and `identity_file` are set in `~/.config/orchid/config.toml`, `orchid vm connect` does not need flags.

Use `orchid vm list` to inspect the VMs on the configured hypervisor:

```bash
orchid vm list
```

## License

[MIT](LICENSE)
