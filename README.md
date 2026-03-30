# orchid

_Because why not._

Lightweight, disposable Debian 12 VMs with Nix for running coding agents. Each VM clones a repo and provisions Nix so its `flake.nix` can provide the dev environment. VMs default to a `dev` user with password `dev`.

## Requirements

- Linux host with KVM/QEMU and libvirt
- A `default` libvirt network (NAT) and storage pool

## VM Spec (per instance)

| Resource | Value                           |
| -------- | ------------------------------- |
| OS       | Debian 12 (`generic` qcow2)     |
| vCPU     | 1                               |
| RAM      | 2 GB                            |
| Disk     | 10 GB (qcow2, thin-provisioned) |
| Auth     | `dev` / `dev`                  |
| Packages | Nix (installed on first boot)   |

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
```

This installs host dependencies and downloads the Debian 12 `generic` base image.

## Usage

Point orchid at a Git repo URL. It derives the VM name and provisions a VM with Nix ready to go. By default, VM names will be prefixed by the username on the hypervisor. This avoid collisions between different developer's VMs.

```bash
# Create a VM
sudo just create-vm https://github.com/specture-system/specture

# Override the VM name
sudo just create-vm https://github.com/specture-system/specture --name my-dev

# Remove a VM and its disk artifacts
sudo just destroy-vm addison-specture
```

On first boot, cloud-init will install Nix in multi-user daemon mode. The VM is ready for `nix develop` once Nix finishes installing (~2-3 min).

You can log in over the serial console or SSH with username `dev` and password `dev`.

## Lifecycle Commands

Use orchid to remove the VM definition and disk artifacts together:

```bash
sudo just destroy-vm <vm-name>
```

## SSH Config (on your laptop)

VMs are on a NAT network only reachable from the hypervisor host. To SSH from your laptop, first find your libvirt subnet on the hypervisor:

```bash
virsh -c qemu:///system net-dumpxml default | grep 'ip address'
# Example output: <ip address='192.168.122.1' netmask='255.255.255.0'>
```

Then add a wildcard entry to `~/.ssh/config` on your laptop using that subnet:

```
Host <subnet>.*
    User dev
    ProxyJump <hypervisor-host>
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
```

Then connect to any VM by IP:

```bash
ssh <vm-ip>
```

## License

[MIT](LICENSE)
