---
title: "Deploy Your First Application"
description: "Configure storage, import an image, launch into a VPC subnet, add DNS/LB/ingress."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Deploy Your First Application

This tutorial takes a `.cap` image and gets it publicly reachable through a load-balanced hostname in under 20 minutes.

**Time:** 20 minutes
**Personas:** [Application Developer](../personas/developer.md), [SRE](../personas/sre.md)

---

## Step 0 — Platform prerequisites

An admin must configure networking and storage once per host:

| Requirement | Where |
| --- | --- |
| **VPC + subnet** | **Network → VPCs** (AIO deploy creates `default-vpc` / `default` automatically) |
| **Default storage pool** | **Admin → Storage** (AIO deploy registers one automatically) |

![Admin storage pool](/assets/images/screenshots/31-admin-storage.png)

---

## Step 1 — Import an Image

Navigate to **Compute → Images** and click **Import**.

![Images](/assets/images/screenshots/06-images.png)

Upload your `.cap` file. Capper verifies the SHA-256 digest and extracts the image manifest. Once status shows **ready**, the image can be used to launch instances.

If you do not have a `.cap` image yet, build one with:

```bash
go run ./cmd/capper create myapp.cap examples/myapp/capper.json
```

---

## Step 2 — Launch an Instance

Navigate to **Compute → Instances → Launch**.

![Instances](/assets/images/screenshots/02-instances.png)

Select your image, capsule type, and **VPC + subnet** (required):

![Launch — networking step](/assets/images/screenshots/33-launch-instance-networking.png)

The wizard blocks launch until a default storage pool exists. Click **Launch**. The instance moves through `created → starting → running`.

**API equivalent:**

```bash
curl -X POST /api/v1/instances \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"image":"myapp","subnetId":"<subnet-id>","name":"web-1"}'
```

---

## Step 3 — Configure DNS

Navigate to **Network → DNS** and create a zone for your domain.

![DNS](/assets/images/screenshots/07-dns.png)

Add an A record pointing your application hostname at the instance private IP or the load balancer VIP from the next step.

---

## Step 4 — Create a Load Balancer

Navigate to **Network → Load Balancers** and click **Create LB**.

![Load Balancers](/assets/images/screenshots/08-load-balancers.png)

Provide:

- **`subnetId`** — same VPC subnet as your instances (required)
- **Listen port** — e.g. `443`
- **Backends** — instance private IPs or label selectors

---

## Step 5 — Add an Ingress Rule

Navigate to **Network → Ingress** to expose the load balancer externally.

![Ingress](/assets/images/screenshots/09-ingress.png)

Set hostname, upstream LB, and TLS certificate. Once active, `curl https://your-hostname/` should reach your app.

---

## Step 6 — Deploy as a Stack

For repeatable deployments, encode resources in a Stack template. Navigate to **Platform → Stacks**.

![Stacks](/assets/images/screenshots/17-stacks.png)

Stacks declare instances (with `subnetId`), load balancers, and DNS — **not** legacy flat `networks[]`:

```json
{
  "name": "my-app",
  "instances": [
    { "name": "web", "image": "myapp", "subnetId": "<subnet-id>" }
  ],
  "dns": [
    { "zone": "app.local", "name": "www", "type": "A", "values": ["10.0.1.10"] }
  ]
}
```

```bash
curl -X POST /api/v1/stacks -H "Content-Type: application/json" -d @my-stack.json
```

---

## What's Next

- [Scale a Workload](scale-workload.md) — compute groups and autoscale
- [Harden Your Platform](harden-platform.md) — firewalls, KMS, IAM
