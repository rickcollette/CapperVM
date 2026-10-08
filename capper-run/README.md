<div align="center">

# 🚀 Capper

### A self-hosted, multi-tenant cloud control plane — in a single binary

*Compute · Networking · Storage · Identity · Topology · Serverless · Observability*
*…driven by one control plane, reachable from a CLI, REST API, Go SDK, and Web UI.*

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Web UI](https://img.shields.io/badge/Web%20UI-React%20%2B%20Vite-61DAFB?style=for-the-badge&logo=react&logoColor=black)
![Database](https://img.shields.io/badge/Store-SQLite%20%7C%20CapDB-003B57?style=for-the-badge&logo=sqlite&logoColor=white)
![Platform](https://img.shields.io/badge/Platform-Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)
![Status](https://img.shields.io/badge/status-1.0.1-EC4899?style=for-the-badge)

</div>

---

> [!WARNING]
> **Do not run untrusted `.cap` images.** Capsule isolation is best-effort,
> not a security boundary.

Capper started as a local `.cap` capsule runner and grew into a full platform:
compute, networking, storage, identity, topology, certificates, observability,
serverless, and public IP management — all behind one control plane, exposed
identically across **four interfaces**.

## 🏗️ Architecture

```mermaid
flowchart TB
    CLI["🖥️ CLI<br/><code>capper …</code>"]
    API["🌐 REST API<br/><code>/api/v1</code>"]
    SDK["📦 Go SDK<br/><code>cappersdk</code>"]
    WEB["✨ Web UI<br/>CapperWeb"]

    subgraph CP["🧠 Control Plane"]
        direction TB
        CORE["Controller · Auth · Scheduler"]
        subgraph SUBS[" "]
            direction LR
            C1["⚙️ Compute"]
            C2["🌐 Networking"]
            C3["💾 Storage"]
            C4["🔐 IAM &<br/>Multi-tenancy"]
            C5["🗺️ Topology"]
            C6["📊 Observability"]
            C7["λ Serverless"]
            C8["🛡️ Security"]
        end
    end

    subgraph DATA["🗄️ Control-plane store"]
        direction LR
        SQLITE[("🪶 SQLite<br/>pure-Go, default")]
        CAPDB[("🔗 CapDB<br/>networked, opt-in")]
    end

    subgraph FLEET["🛰️ Fleet"]
        direction LR
        N1["🤖 capper-agent"]
        N2["🤖 capper-agent"]
        N3["🤖 capper-agent"]
    end

    CLI & API & SDK & WEB --> CORE
    CORE --> SUBS
    CP --> SQLITE
    CP -. "-tags capdb" .-> CAPDB
    CORE <== "heartbeat · inventory · metrics" ==> FLEET

    classDef iface fill:#6366F1,stroke:#312E81,color:#fff,stroke-width:2px;
    classDef core  fill:#F59E0B,stroke:#92400E,color:#1F2937,stroke-width:2px;
    classDef sub   fill:#10B981,stroke:#065F46,color:#04221A,stroke-width:2px;
    classDef data  fill:#3B82F6,stroke:#1E3A8A,color:#fff,stroke-width:2px;
    classDef node  fill:#EC4899,stroke:#831843,color:#fff,stroke-width:2px;

    class CLI,API,SDK,WEB iface;
    class CORE core;
    class C1,C2,C3,C4,C5,C6,C7,C8 sub;
    class SQLITE,CAPDB data;
    class N1,N2,N3 node;

    style CP fill:#FEF3C7,stroke:#F59E0B,stroke-width:3px,color:#1F2937;
    style DATA fill:#DBEAFE,stroke:#3B82F6,stroke-width:3px,color:#1E3A8A;
    style FLEET fill:#FCE7F3,stroke:#EC4899,stroke-width:3px,color:#831843;
    style SUBS fill:#D1FAE5,stroke:#10B981,stroke-width:2px;
```

## 🧩 Subsystems

| Area | What it provides |
|---|---|
| ⚙️ **Compute** | `.cap` capsule instances (bwrap/chroot/crun/runc), images, templates, instance types, GPU inventory, compute groups + autoscale |
| 🌐 **Networking** | virtual networks, VPCs + subnets, firewalls, load balancers, DNS, ingress, **Public IPAM / Elastic IPs** |
| 💾 **Storage** | block volumes, S3-compatible object store, snapshots, CSD shared/replicated volumes, backups |
| 🔐 **Multi-tenancy** | organizations → accounts → projects, IAM (users/groups/roles/policies), managed policies, assume-role, quotas, governance, audit |
| 🗺️ **Topology** | realms → regions → zones → nodes, node pools, service roles, the `capper-agent` daemon, placement scheduler |
| 🚚 **VPC Mobility** | plan → approve → execute → cutover migration of VPC workloads across realms/regions |
| 📜 **Certificates** | ACME / Let's Encrypt issuance, renewal scheduler, bindings, internal CA |
| 📊 **Observability** | unified resource inventory, config drift, metrics, resource events, alerts |
| λ **Serverless** | Lambda-style **Functions** (triggers, invocations) and managed **MCP servers** with per-tool IAM + approval gates |
| 🛡️ **Security** | KMS, secrets, image posture scanning, SBOM, marketplace review |

> [!NOTE]
> Every subsystem is exposed **consistently across all four interfaces** (CLI,
> REST API, Go SDK, Web UI) and is covered by tests.

## 🔌 Interfaces

- **CLI** — `capper <subsystem> <verb>` (e.g. `capper instances list`, `capper org create`, `capper fn invoke`). Run `capper --help`.
- **REST API** — `capper api start` serves `/api/v1/…` with bearer-token auth.
- **Go SDK** — `import cappersdk "capper/sdk/go"` → `c := cappersdk.New(url, token)`; groups include `c.Instances`, `c.IAM`, `c.Functions`, `c.IPAM`, and ~40 more.
- **Web UI** — **CapperWeb** (Vite + React), served via `capper api start --console <dist>`.

## ⚡ Quick start

<details open>
<summary><b>Run a capsule</b></summary>

```bash
sh examples/alpine/bootstrap.sh
go run ./cmd/capper --store /tmp/capper-alpine create alpine.cap examples/alpine/capper.json
go run ./cmd/capper --store /tmp/capper-alpine run alpine.cap
go run ./cmd/capper --store /tmp/capper-alpine list instances
```

> [!TIP]
> Capper prefers Bubblewrap (`bwrap`) with unprivileged user namespaces and falls
> back to chroot (may need `sudo`). Choose with `--runtime bwrap|chroot|crun|runc`,
> and cap resources with `--memory 128M --cpu-time 60 --file-size 16M`.

</details>

<details>
<summary><b>Run the control plane</b></summary>

```bash
make capper-run            # builds a fresh bundle into capper-run/, serves http://127.0.0.1:8687
make capper-run-status
make capper-run-stop

# overrides
CAPPER_RUN_API_ADDR=127.0.0.1:8690 make capper-run
CAPPER_RUN_CONSOLE=/path/to/CapperWeb/dist make capper-run   # serve the Web UI

# …or start the API directly
capper api start --listen 127.0.0.1:8686 --console /path/to/CapperWeb/dist
```

</details>

<details>
<summary><b>All-in-one node</b></summary>

```bash
capper aio init --backend capdb   # storage layout + local topology + TLS + units
capper aio up                     # start API, daemon, and local services
capper aio status
capper aio upgrade --channel stable   # seamless, auto-rollback upgrades
```

</details>

<details>
<summary><b>Join a worker node</b></summary>

```bash
capper node join my-node --token <join-token> --address 10.0.0.5 --role compute
capper node approve my-node        # on the control plane
```

The `capper-agent` daemon (`cmd/capper-agent`) sends heartbeats, reports inventory
and version, pushes host metrics, and supervises services.

</details>

## 🗄️ Storage backend

By default Capper persists control-plane state in a single embedded **SQLite**
database (`modernc.org/sqlite`, WAL + busy timeout) — pure-Go, no external process.

For networked, connection-pooled storage it can instead talk to **CapDB 3.7.2**
— a SQLite fork with a TLS client/server protocol, a native pool, a volume
store, and WAL replication primitives. The engine is
[rickcollette/CapDB](https://github.com/rickcollette/CapDB). `make capdb-fetch`
checks it out at `CAPDB_REF` (default `v3.7.2`) under `./CapDB`. The SQL
dialect is unchanged.

All-in-one installs still run one co-located `capdb-server` on a `--db-root`
directory. The bundle also ships the `capdb` shell for online `.backup`.

```bash
make capdb-fetch          # clone CapDB and detach at CAPDB_REF
make capdb                # client lib, capdb-server, and the capdb shell
go build -tags capdb ./cmd/capper
make test-capdb           # driver conformance suite
```

See [`docs/src/operator-guide/capdb-backend.md`](docs/src/operator-guide/capdb-backend.md).

## 🛠️ Build & test

```bash
make build                # stamped binaries into bin/
go test ./...
go vet ./...
cd ../CapperWeb && npm run build   # Web UI
```

## 📚 Documentation

Operator and concept docs live under [`docs/`](docs/) (built with the toolchain in
`docs/config.yml` + `docs/nav.yml`). Start with the
[Upgrades guide](docs/src/operator-guide/upgrades.md) and the
[CapDB backend](docs/src/operator-guide/capdb-backend.md).

<div align="center">

---

*Built with Go 🐹 · React ⚛️ · SQLite/CapDB 🗄️ — self-hosted, single-binary, multi-tenant.*

</div>
