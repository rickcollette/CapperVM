# qemu-hello

Reuse the Alpine rootfs from `examples/alpine` (or any `.cap` with a Linux rootfs).

```bash
# Build / obtain an alpine.cap first, then:
capper run alpine.cap --runtime-mode qemu --name qemu-hello
```

See [LXC and QEMU runtimes](../../docs/src/operator-guide/runtimes-lxc-qemu.md) for host packages.
