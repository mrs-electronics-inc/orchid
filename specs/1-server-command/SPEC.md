# Server Command Spec

## Goal

Add an `orchid server` command group that installs, manages, and proxies a simple Orchid daemon running on the hypervisor.

This replaces the current pattern where the laptop-side CLI performs privileged host operations over ad hoc SSH commands. The daemon becomes the single owner of privileged VM lifecycle actions on the hypervisor.

## Why

The current `create-vm` flow depends on remote privileged commands like `qemu-img`, `cloud-localds`, and `virt-install`. That requires non-interactive `sudo` over SSH, which is fragile and not a good long-term interface.

A long-running Orchid service on the hypervisor solves that problem:

- privileged operations run locally on the hypervisor
- laptop-side commands stop depending on per-command `sudo`
- progress reporting has a natural home
- VM lifecycle logic becomes easier to reason about and test

## Command Shape

Add a new top-level command group:

```text
orchid server install
orchid server status
orchid server proxy
orchid server run
```

### `orchid server install`

Installs the Orchid daemon as a systemd service on the hypervisor.

Responsibilities:

- install the `orchid` binary to a stable location on the hypervisor
- install the checked-in systemd unit file
- reload systemd
- enable and start the Orchid service

This is an admin/setup command. It is allowed to require elevated privileges on the hypervisor.

### `orchid server status`

Reports whether the Orchid service is installed and running on the hypervisor.

It should be concise by default. It can use systemd state as the source of truth.

### `orchid server proxy`

Runs on the hypervisor and bridges stdin/stdout to the Orchid Unix socket.

This is the transport helper that lets the laptop CLI speak HTTP over the daemon's Unix socket through SSH. It should be intentionally small and dumb: connect to the socket and copy bytes in both directions.

### `orchid server run`

Runs the Orchid daemon itself. This is the process systemd will start.

It should not be a general-purpose debug framework. For normal operation it should use a fixed socket path and a fixed HTTP API.

## Service Shape

The daemon should:

- run as a root-owned systemd service on the hypervisor
- listen on a fixed Unix socket: `/run/orchid/orchid.sock`
- expose a minimal HTTP+JSON API over that socket
- own all privileged VM lifecycle operations

Do not expose a public TCP port.

Do not make the socket path a user-facing flag in the normal interface.

## Trust Boundary

SSH remains the remote access boundary.

The daemon does not need its own authentication layer in the first version because:

- the socket is local to the hypervisor
- the laptop reaches it through SSH
- the daemon only accepts local socket connections

This keeps the first version simple and avoids unnecessary auth complexity.

## API Shape

Use HTTP+JSON over the Unix socket.

Start with `/v1` versioning immediately.

Initial endpoints:

- `GET /v1/health`
- `GET /v1/vms`
- `GET /v1/vms/{name}/ip`
- `POST /v1/vms`
- `DELETE /v1/vms/{name}`
- `GET /v1/jobs/{id}`

## Job Model

Long-running operations should be job-based from the start.

This is especially important for `create-vm`, which already has multiple long-running stages and needs progress reporting.

### Job States

- `queued`
- `running`
- `succeeded`
- `failed`

### Job Stages

For VM creation, expected stages are:

- `validating_request`
- `creating_disk`
- `writing_seed`
- `starting_vm`
- `waiting_for_ip`
- `waiting_for_ssh`
- `waiting_for_cloud_init`
- `ready`

### Job Status Payload

The job status response should include at least:

- `job_id`
- `state`
- `stage`
- `message`
- `vm_name`
- `ip` when known
- `error` when failed

## Client Flow

After the daemon exists, laptop-side commands should move to it incrementally.

### `orchid list`

- CLI connects to the daemon through SSH and `orchid server proxy`
- daemon returns VM list

### `orchid create-vm`

- CLI reads local config
- CLI reads local identity material
- CLI sends a create request to the daemon
- daemon returns `job_id`
- CLI polls job status and prints stage transitions
- on success, CLI prints `orchid connect <vm-name>`

### `orchid connect`

`connect` should stay a client-side SSH action.

The daemon is responsible for IP resolution, but the final guest SSH session remains a client concern.

## Systemd Installation

Ship a checked-in unit file in the repo. The service should be installed by `orchid server install`, not by dynamic unit generation in the client.

The service should:

- run `orchid server run`
- own `/run/orchid`
- create the runtime socket path used by the daemon

Socket activation is not required in the first version. A plain service is the simpler starting point.

## Non-Goals

For the first version, do not add:

- gRPC
- public TCP listeners
- a database
- a queue system
- persistent job storage
- a custom auth layer

Keep the daemon local, small, and focused on Orchid operations only.

## First Implementation Slice

Build the smallest useful slice in this order:

1. `orchid server run`
2. `orchid server proxy`
3. checked-in systemd unit
4. `orchid server install`
5. `orchid server status`
6. `GET /v1/health`
7. move `list` behind the daemon

After that, move `create-vm` to the job-based daemon API.
