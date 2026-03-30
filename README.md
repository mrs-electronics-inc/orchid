# orchid

_Because why not._

Lightweight, disposable Debian 12 VMs for running coding agents against single projects.

## Requirements

- Linux host with KVM/QEMU and libvirt
- A `default` libvirt network (NAT) and storage pool

## VM Spec (per instance)

| Resource | Value                               |
| -------- | ----------------------------------- |
| OS       | Debian 12 (cloud image)             |
| vCPU     | 1                                   |
| RAM      | 2 GB                                |
| Disk     | 10 GB (qcow2, thin-provisioned)     |
| Access   | SSH only (cloud-init key injection) |

## Prerequisites

Install dependencies (one-time):

```bash
apt install -y virtinst cloud-image-utils genisoimage
```

Download the Debian 12 generic cloud image (one-time):

```bash
cd /var/lib/libvirt/images
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2 \
  -O debian-12-base.qcow2
```

## Usage

```bash
# Basic VM (auto-detects SSH key from ~/.ssh/)
just create-vm myproject

# With extra packages
just create-vm specture --packages "golang"

# With a specific SSH key
just create-vm myproject --ssh-key "ssh-ed25519 AAAA..."
```

The script creates the VM and waits for it to get a DHCP address, then prints the SSH command.

Default packages installed: `git`, `curl`, `build-essential`.

## Lifecycle Commands

```bash
virsh -c qemu:///system start <vm-name>       # start a stopped VM
virsh -c qemu:///system shutdown <vm-name>     # graceful shutdown
virsh -c qemu:///system destroy <vm-name>      # force stop
virsh -c qemu:///system undefine <vm-name>     # remove VM definition
```

Clean up disk artifacts after undefine:

```bash
rm /var/lib/libvirt/images/<vm-name>.qcow2
rm /var/lib/libvirt/images/<vm-name>-seed.iso
```

## SSH Config

Add a wildcard entry to `~/.ssh/config` to reach any VM through the hypervisor host:

```
Host 192.168.122.*
    User dev
    ProxyJump <hypervisor-host>
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
```

Then connect to any VM by IP:

```bash
ssh 192.168.122.x
```

## License

[MIT](LICENSE)
