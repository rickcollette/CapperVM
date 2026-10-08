---
title: "Onboard a New Team"
description: "Create a VPC, IAM users and groups, assign policies, and set quotas."
owner: "docs"
status: "stable"
reviewed: "2026-06-19"
outputs:
  - markdown
  - web
  - pdf
---

# Onboard a New Team

This tutorial walks a platform engineer through the end-to-end process of giving a new team a fully isolated environment on Capper — their own network, identity, and resource limits.

**Time:** 15 minutes
**Persona:** [Platform Engineer](../personas/platform-engineer.md)

---

## Step 1 — Check the Dashboard

Before provisioning anything, confirm the platform is healthy. The dashboard shows daemon status, running instance counts, and recent audit events.

![Capper Dashboard](/assets/images/screenshots/01-dashboard.png)

A green **online** badge in the top-right confirms the API is reachable. The Recent Activity panel will record every action you take in this tutorial.

---

## Step 2 — Configure host storage

Navigate to **Admin → Storage**, register a pool, and set the **default instance pool**.
AIO installs do this automatically; verify before handing the environment to the team.

![Admin storage](/assets/images/screenshots/31-admin-storage.png)

---

## Step 3 — Create a VPC

Navigate to **Network → VPCs** and click **Create VPC**.

![VPCs](/assets/images/screenshots/03-vpcs.png)

Fill in:
- **Name** — e.g. `team-alpha`
- **CIDR** — e.g. `10.10.0.0/16`

After the VPC is created, expand the row and add at least one subnet:
- **Name** — `team-alpha-primary`
- **CIDR** — `10.10.1.0/24`
- **Zone** — your availability zone, e.g. `zone-a`

The subnet CIDR must fall within the VPC CIDR. Instances launched into this VPC will receive addresses from the subnet range.

---

## Step 4 — Create IAM Users

Navigate to **IAM → Users**.

![IAM Users](/assets/images/screenshots/11-iam.png)

Create one user per team member. Each user gets a unique ID prefixed with `usr_`. Share the user ID with the team member — they will use it to generate API tokens from the Tokens tab.

---

## Step 5 — Create a Group and Assign Members

Navigate to **IAM → Groups** and create a group named `team-alpha-developers`. Add each user created in the previous step as a member.

Groups make policy assignment easier: attach a policy once to the group rather than to each individual user.

---

## Step 6 — Simulate and Attach a Policy

Before attaching a policy, use the simulator to verify it grants exactly what you intend.

Navigate to **IAM → Simulate**.

![IAM Policy Simulator](/assets/images/screenshots/30-iam-policy-sim.png)

Enter the principal (the group or a user), the action (e.g. `instance:create`), and the resource (e.g. `instance/*`). If the result is `deny`, adjust the policy before attaching.

Navigate to **IAM → Policies**, create a policy scoped to the resources the team needs, then navigate back to Groups and attach the policy to `team-alpha-developers`.

---

## Step 7 — Set Quotas

Navigate to **System → Quotas** to cap how many resources the team can consume.

![Quotas](/assets/images/screenshots/19-quotas.png)

Recommended starting limits for a new team:
- `instances` — 10
- `storage_bytes` — 107374182400 (100 GiB)
- `vpcs` — 3

## Step 8 — Verify via Audit Log

Navigate to **IAM → Audit Log** and confirm all the actions from this tutorial appear — VPC creation, user creation, group membership changes, and policy attachments. This is the record you will reference during any future access review.

---

## What's Next

- [Deploy Your First Application](deploy-first-app.md) — hand this guide to the new team
- [Harden Your Platform](harden-platform.md) — review policies and run a posture scan
