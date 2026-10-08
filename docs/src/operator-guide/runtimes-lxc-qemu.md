---
title: "LXC and QEMU runtimes"
description: "Host packages and security notes for Capper LXC and QEMU backends."
owner: "docs"
status: "stable"
reviewed: "2026-10-07"
outputs:
  - markdown
  - web
---

# LXC and QEMU runtimes

Capper can run capsules with process isolation (`bwrap` / `chroot` / `crun` / `runc`)
or with heavier backends:

| Mode | What it runs | Typical host packages |
| --- | --- | --- |
| `lxc` | LXC container over the extracted rootfs | `lxc` / `lxc-utils` (`lxc-start`, `lxc-attach`, `lxc-stop`) |
| `qemu` | KVM/TCG VM from rootfs→qcow2 + nocloud seed | `qemu-system-*`, `qemu-img`, `libguestfs-tools` (`virt-make-fs`), `genisoimage`/`xorriso`, optional `socat` |

## Selecting a runtime

- **Host default**: `capper --runtime <mode> …` (also used by the API controller).
- **Per instance**: create/run with `runtimeMode` / `--runtime-mode`.
- **Capsule preference**: optional `preferredRuntime` on the capsule manifest.
- Resolution order: request → capsule preference → host default.
- `auto` still prefers bubblewrap then chroot; it never silently picks LXC or QEMU.

```bash
capper run alpine.cap --runtime-mode lxc
capper run alpine.cap --runtime-mode qemu
```

API:

```json
{ "image": "alpine", "subnetId": "…", "runtimeMode": "lxc" }
```

## Doctor checks

`capper doctor` reports LXC tools, `qemu-system-*`, `qemu-img`, and `/dev/kvm`
availability. Missing KVM is a warning: QEMU falls back to TCG (slower).

## Security warning

LXC and QEMU widen the attack surface versus bubblewrap. Prefer `bwrap`/`crun`
for untrusted workloads. QEMU guests share a larger host kernel/userspace
interface (KVM, TAP, disk images). Treat VM and LXC hosts as privileged
control-plane nodes.

## QEMU notes

- First start converts `instDir/rootfs` into `disk.qcow2` via `virt-make-fs`.
- Networking uses a host TAP bridged into the Capper VPC bridge when attached.
- Console/PTY attaches to the QEMU serial socket (not a guest shell attach).
- Guest `exec` is not supported in this Capper version.
