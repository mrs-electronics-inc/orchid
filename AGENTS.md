# AGENTS.md

This file is the local memory for future agents working in this repo. Keep it short, specific, and actionable.

## Always Do

- Use the existing `just` recipes for the main workflow.
- Keep `README.md`, `justfile`, and the VM scripts in sync when changing the image flow.
- Run a shell syntax check on modified bash scripts before handing off.
- Preserve the shared-base model: common tooling belongs in `orchid-base.qcow2`, not in per-VM bootstrap logic.
- Use conventional commit messages for every Git commit, for example `feat: ...`, `fix: ...`, or `docs: ...`.

## Ask First

- Anything that changes the shared base image contents or default disk sizing.
- Anything that touches system locations like `/var/lib/libvirt/images` outside the documented workflow.
- Any change that would rewrite or remove existing VM images or the `orchid-base.qcow2` symlink.

## Never Do

- Reintroduce per-VM installs of shared tools that are already baked into the Orchid base image.
- Make broad documentation changes that duplicate repo facts the code already makes obvious.

## Long Term Memory

- `orchid-base.qcow2` is a symlink to the current versioned shared base image.
- `scripts/create-vm.sh` should stay light: repo clone, login-shell wiring, and VM-specific cloud-init only.
- `scripts/build-base.sh` is the place for shared toolchain changes.
