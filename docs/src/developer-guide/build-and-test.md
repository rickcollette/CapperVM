---
title: "Build and test"
description: "The make targets, the green-build invariant, and the docs/CapDB builds."
owner: "docs"
status: "stable"
reviewed: "2026-06-12"
outputs:
  - markdown
  - web
  - pdf
---

# Build and test

## Core targets

```bash
make build          # pure-Go binary → bin/capper
make test           # go test ./...
make clean          # remove build output
go build ./...      # build all binaries
go vet ./...        # vet
```

The **green-build invariant**: `go build ./...`, `go vet ./...`, `go test ./...`,
and (when the API/Web UI is touched) CapperWeb `npm run build` must all pass before
a change lands.

## Run a local control plane

```bash
make capper-run            # build a bundle + start API+daemon on :8687
make capper-run-status
make capper-run-stop
```

## CapDB backend (optional, cgo)

```bash
make capdb-fetch    # clone CapDB and detach at CAPDB_REF (default v3.7.2)
make capdb          # build libcapdb_client.a, capdb-server, and the capdb shell
make build-capdb    # build capper with -tags capdb (cgo + OpenSSL)
make test-capdb     # driver conformance + integration tests against a live server
```

Unit tests run on the pure-Go `modernc` SQLite backend (`:memory:`) and stay
cgo-free; CapDB work stays behind `-tags capdb`.

## Web UI

```bash
cd ../CapperWeb && npm run build
capper api start --console ../CapperWeb/dist
```

## Documentation

```bash
make docs-check     # validate front matter + structure (run this before committing docs)
make docs-inventory # regenerate the module/route/type/test inventory
make docs           # check + inventory + markdown + web + pdf
make docs-serve     # preview locally
```

`make docs-check` must be green; it enforces front matter (`title`, valid
`status` of draft/review/stable/deprecated) on every page.

## Related

- [Repository layout](repository-layout.md) · [Adding a module](adding-a-module.md)
  · [CapDB backend](../operator-guide/capdb-backend.md)
