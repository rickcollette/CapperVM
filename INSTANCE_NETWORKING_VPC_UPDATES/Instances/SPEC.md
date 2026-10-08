# Capper Instances — AWS-Style Redesign SPEC

Status: Draft for implementation  
Owner: Capper Compute / Runtime / Networking / WebUI  
Target: Backend, API, SDK, CLI, WebUI, Docs, Tests  
Non-goal: This is not code. This is the product and engineering contract.

## 1. Purpose

Capper Instances must become AWS EC2-like virtual compute resources from the user’s point of view. Today, Capper can create and manage capsule-backed instances, but the model is still too small: image, name, status, IP, labels, and lifecycle actions. The redesign makes instances first-class cloud compute objects with instance types, launch templates, ENIs, block devices, key pairs, metadata, IAM roles, placement, monitoring, termination protection, tags, console access, and networking parity with the VPC model.

The goal is not to clone EC2 implementation internals. The goal is to provide the options users expect when launching and operating a cloud instance.

## 2. Current Capper Baseline

The uploaded Capper snapshot already includes instance API/SDK operations for list, create, get, start, stop, delete, plus image, network, DNS, LB, KMS, storage, GPU, instance type, compute group, scheduler, resource monitor, and runtime components. Current instance creation accepts image, name, labels, environment, and command. The runtime has support for resource limits, bubblewrap/chroot/OCI modes, network namespaces, proc masking, shell/PTY access, and process lifecycle. The SDK and tests already exercise instance lifecycle and pagination.

This spec expands that foundation into an AWS-style instance contract.

## 3. Product Model

### 3.1 Instance

Required instance fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | Stable ID, for example `i_...`. |
| arn/crn | yes | Canonical resource name. |
| orgId | yes | Owning org. |
| accountId | yes | Owning account. |
| projectId | yes | Owning project. |
| name | yes | Display name. |
| hostname | yes | Guest hostname. |
| imageId | yes | Source image/capsule/AMI-like artifact. |
| imageRef | yes | Human image ref. |
| instanceType | yes | Compute sizing class. |
| state | yes | `pending`, `running`, `stopping`, `stopped`, `shutting-down`, `terminated`, `rebooting`, `error`. |
| stateReason | no | Last state message. |
| lifecycle | yes | `normal`, `spot`, `scheduled`, `reserved`, `ephemeral`. |
| regionId | yes | Region. |
| zoneId | yes | Zone. |
| nodeId | no | Assigned node. |
| placementGroupId | no | Placement group. |
| tenancy | yes | `default`, `dedicated-node`, `host-pinned`. |
| vpcId | yes | Parent VPC. |
| subnetId | yes | Primary subnet. |
| primaryEniId | yes | Primary ENI. |
| privateIpAddress | yes | Primary private IP. |
| publicIpAddress | no | Associated public IP, if any. |
| ipv6Addresses | no | IPv6 addresses. |
| securityGroupIds | yes | Attached SGs. |
| keyName | no | SSH key pair name. |
| iamRoleId | no | Instance role/profile. |
| metadataOptions | yes | Metadata service config. |
| userDataHash | no | Hash only; raw user data must be secret-protected. |
| rootBlockDevice | yes | Root volume/device spec. |
| blockDeviceMappings | no | Extra volumes/devices. |
| ebsOptimized | yes | Storage optimization flag. |
| monitoring | yes | `basic`, `detailed`, `disabled`. |
| terminationProtection | yes | Prevent accidental terminate. |
| shutdownBehavior | yes | `stop` or `terminate`. |
| sourceDestCheck | yes | Needed for appliance instances. |
| tags | no | AWS-style tags. |
| labels | no | Capper internal labels. |
| createdBy | yes | Principal. |
| launchedAt | no | Launch timestamp. |
| createdAt | yes | Create timestamp. |
| updatedAt | yes | Update timestamp. |
| terminatedAt | no | Termination timestamp. |

### 3.2 Instance Type

Instance types define CPU, memory, storage, network, and accelerator capacity.

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `it_...`. |
| name | yes | Example: `cap.t3.small`, `cap.c1.large`, `cap.gpu.a100.1x`. |
| family | yes | `general`, `compute`, `memory`, `storage`, `gpu`, `network`, `burstable`. |
| generation | yes | Numeric or semantic generation. |
| cpuCount | yes | vCPU count. |
| cpuShares | no | Relative scheduler shares. |
| memoryBytes | yes | Memory limit. |
| localDiskBytes | no | Ephemeral storage. |
| networkPerformance | yes | `low`, `moderate`, `high`, `very-high`, or numeric Mbps. |
| maxEnis | yes | Maximum ENIs. |
| ipv4PerEni | yes | IPs per ENI. |
| ipv6PerEni | yes | IPv6 per ENI. |
| gpuEligible | yes | GPU support. |
| gpuCount | no | Required GPUs. |
| gpuMemoryBytes | no | GPU memory. |
| architecture | yes | `x86_64`, `arm64`, future. |
| bareMetal | yes | Whether maps to whole node. |
| status | yes | `available`, `deprecated`, `disabled`. |
| regions | no | Region availability. |
| zones | no | Zone availability. |

### 3.3 Image

Instances launch from Capper images, but the user-facing behavior should map to AMI-like expectations.

Required image behavior:

- Image ID and image alias/ref.
- Architecture compatibility check.
- Root filesystem/source digest.
- Owner account and visibility: private, shared, marketplace, public.
- Boot mode / runtime mode compatibility.
- SBOM/provenance visibility.
- Security scan status.
- Deprecation date and disabled flag.

### 3.4 Launch Template

Launch templates are reusable instance launch definitions.

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `lt_...`. |
| name | yes | Unique per project/account. |
| defaultVersion | yes | Default version number. |
| latestVersion | yes | Latest version. |
| versions | yes | Versioned configs. |
| tags | no | Tags. |

Launch template version must include every field required to launch an instance, except fields allowed to be overridden at launch time.

Launch template version fields:

- imageId/imageRef
- instanceType
- keyName
- securityGroupIds
- networkInterfaces
- blockDeviceMappings
- iamRoleId
- userData
- metadataOptions
- monitoring
- placement
- tags
- shutdownBehavior
- terminationProtection

### 3.5 Key Pair

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `key_...`. |
| name | yes | Unique per account/project. |
| publicKey | yes | Stored public key. |
| fingerprint | yes | Public key fingerprint. |
| keyType | yes | `rsa`, `ed25519`, `ecdsa`. |
| createdAt | yes | Timestamp. |
| tags | no | Tags. |

Rules:

- Capper stores public keys only.
- Private key generation may be offered once, but private key must never be retrievable after creation.
- Linux instances receive key injection through supported guest init path.

### 3.6 ENI — Elastic Network Interface

Instances attach to VPC through ENIs.

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `eni_...`. |
| vpcId | yes | VPC. |
| subnetId | yes | Subnet. |
| zoneId | yes | Zone. |
| instanceId | no | Attached instance. |
| attachmentIndex | no | Device index. |
| macAddress | yes | MAC. |
| privateIpAddresses | yes | Primary and secondary IPs. |
| ipv6Addresses | no | IPv6 addresses. |
| publicIpAssociation | no | Public IP mapping. |
| securityGroupIds | yes | Security groups. |
| sourceDestCheck | yes | Boolean. |
| status | yes | `available`, `attaching`, `in-use`, `detaching`, `deleted`. |
| deleteOnTermination | yes | Whether deleted with instance. |
| tags | no | Tags. |

Rules:

- Every instance has a primary ENI at device index 0.
- Additional ENIs are limited by instance type.
- ENIs cannot cross zones.
- ENIs cannot attach to instances in different zones.
- Public IPs associate to ENI private IPs, not directly to instances.

### 3.7 Block Device Mapping

Fields:

| Field | Required | Description |
| --- | --- | --- |
| deviceName | yes | Guest device name. |
| volumeId | no | Existing volume. |
| snapshotId | no | Snapshot source. |
| sizeBytes | yes | Requested size. |
| volumeType | yes | `standard`, `ssd`, `provisioned-iops`, `local`, `shared`. |
| iops | no | For provisioned storage. |
| throughput | no | Optional. |
| encrypted | yes | Encryption state. |
| kmsKeyId | no | KMS key. |
| deleteOnTermination | yes | Delete with instance. |
| bootIndex | no | Boot order. |

Rules:

- Root block device is required.
- Root volume can be ephemeral or persistent depending on image and launch settings.
- Delete-on-termination must be visible and editable before launch.

### 3.8 Metadata Options

Fields:

| Field | Required | Description |
| --- | --- | --- |
| enabled | yes | Metadata service available. |
| httpTokens | yes | `required` or `optional`. Required should be default. |
| hopLimit | yes | Default 1. |
| exposeUserData | yes | Whether user data is accessible from metadata service. |
| exposeTags | yes | Whether tags are visible to instance. |
| endpointMode | yes | `link-local`, `vsock`, `disabled`. |

Rules:

- New instances default to token-required metadata.
- User data may be passed at launch and is visible only according to metadata settings.
- Metadata tokens must be unique per instance and rotated on stop/start if configured.

### 3.9 Instance Role / Profile

Fields:

| Field | Required | Description |
| --- | --- | --- |
| id | yes | `iprofile_...`. |
| name | yes | Profile name. |
| roleId | yes | IAM role. |
| credentialDelivery | yes | Metadata service. |
| maxSessionSeconds | yes | Credential TTL. |

Rules:

- Instance applications use temporary credentials, not stored permanent tokens.
- Role changes must be audited.
- Metadata service should expose only scoped temporary credentials.

## 4. API Surface

All endpoints are under `/api/v1`. All list operations must support pagination, sorting, tag filtering, state filtering, project/account filtering, and region/zone filtering.

### 4.1 Instance Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/instances` | List instances. |
| POST | `/instances` | Run/launch instance. |
| GET | `/instances/{instanceId}` | Describe instance. |
| PATCH | `/instances/{instanceId}` | Update mutable metadata/settings. |
| DELETE | `/instances/{instanceId}` | Terminate instance. |
| POST | `/instances/{instanceId}/start` | Start stopped instance. |
| POST | `/instances/{instanceId}/stop` | Stop running instance. |
| POST | `/instances/{instanceId}/reboot` | Reboot instance. |
| POST | `/instances/{instanceId}/hibernate` | Future/optional. |
| POST | `/instances/{instanceId}/protect-termination` | Enable termination protection. |
| DELETE | `/instances/{instanceId}/protect-termination` | Disable termination protection. |
| GET | `/instances/{instanceId}/console` | Console/session metadata. |
| POST | `/instances/{instanceId}/console/sessions` | Open console session. |
| GET | `/instances/{instanceId}/logs` | Runtime stdout/stderr/log stream. |
| GET | `/instances/{instanceId}/metrics` | Instance metrics. |
| GET | `/instances/{instanceId}/events` | Instance lifecycle events. |
| GET | `/instances/{instanceId}/metadata-options` | Read metadata options. |
| PATCH | `/instances/{instanceId}/metadata-options` | Update metadata options. |
| POST | `/instances/{instanceId}/attach-volume` | Attach volume. |
| POST | `/instances/{instanceId}/detach-volume` | Detach volume. |
| POST | `/instances/{instanceId}/attach-network-interface` | Attach ENI. |
| POST | `/instances/{instanceId}/detach-network-interface` | Detach ENI. |
| POST | `/instances/{instanceId}/create-image` | Create image/snapshot from instance. |

### 4.2 Launch Template Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/launch-templates` | List templates. |
| POST | `/launch-templates` | Create template. |
| GET | `/launch-templates/{templateId}` | Describe template. |
| PATCH | `/launch-templates/{templateId}` | Update metadata/default version. |
| DELETE | `/launch-templates/{templateId}` | Delete template. |
| POST | `/launch-templates/{templateId}/versions` | Create new version. |
| GET | `/launch-templates/{templateId}/versions` | List versions. |
| DELETE | `/launch-templates/{templateId}/versions/{version}` | Delete version. |

### 4.3 Instance Type Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/instance-types` | List instance types. |
| GET | `/instance-types/{name}` | Describe instance type. |
| POST | `/admin/instance-types` | Create/admin only. |
| PATCH | `/admin/instance-types/{name}` | Update/admin only. |
| DELETE | `/admin/instance-types/{name}` | Disable/admin only. |

Compatibility note: if existing code exposes `/capsule-types`, keep it as a compatibility alias and move the product language to `instance-types` in WebUI/docs.

### 4.4 Key Pair Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/key-pairs` | List key pairs. |
| POST | `/key-pairs` | Import or create key pair. |
| GET | `/key-pairs/{keyName}` | Describe key pair. |
| DELETE | `/key-pairs/{keyName}` | Delete key pair. |

### 4.5 ENI Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/network-interfaces` | List ENIs. |
| POST | `/network-interfaces` | Create ENI. |
| GET | `/network-interfaces/{eniId}` | Describe ENI. |
| PATCH | `/network-interfaces/{eniId}` | Update SGs/source-dest-check/tags. |
| DELETE | `/network-interfaces/{eniId}` | Delete ENI. |
| POST | `/network-interfaces/{eniId}/attach` | Attach to instance. |
| POST | `/network-interfaces/{eniId}/detach` | Detach from instance. |
| POST | `/network-interfaces/{eniId}/private-ips` | Assign private IP. |
| DELETE | `/network-interfaces/{eniId}/private-ips/{ip}` | Unassign private IP. |

## 5. Launch Instance Request Contract

A launch request must support:

- name
- imageId or imageRef
- instanceType
- regionId
- zoneId or automatic placement
- vpcId
- subnetId
- privateIpAddress optional
- securityGroupIds
- keyName
- iamRoleId / instanceProfileId
- userData as secret input
- metadataOptions
- blockDeviceMappings
- networkInterfaces
- publicIp behavior: `none`, `auto`, `existing-allocation`, `new-allocation`
- monitoring
- shutdownBehavior
- terminationProtection
- tags
- labels
- placement group
- capacity reservation / node pool preference
- startup command override for capsule compatibility
- environment variables for capsule compatibility

Validation order:

1. IAM authorization.
2. Quota check.
3. Image exists and is launchable.
4. Instance type exists and is available in target region/zone.
5. Placement/capacity check.
6. VPC/subnet/security group/ENI validation.
7. IPAM reservation.
8. Volume preparation.
9. Runtime launch.
10. Resource monitor registration.
11. Audit event.

## 6. Backend Requirements

### 6.1 Required Packages

| Package | Responsibility |
| --- | --- |
| `internal/compute` | Instance model, launch templates, instance types, placement groups. |
| `internal/runtime` | Process/container runtime lifecycle. |
| `internal/network` | ENI/netns/tap/veth attachment. |
| `internal/vpc` | Subnet, SG, NACL, route validation. |
| `internal/storage` | Volumes, snapshots, block mappings. |
| `internal/ipam` | Private/public IP allocation. |
| `internal/iam` | Instance roles, permissions. |
| `internal/resourcemon` | Metrics, drift, events, alerts. |
| `internal/audit` | Audit logs. |
| `internal/quotas` | Limits. |
| `internal/scheduler` | Placement and capacity decisions. |

### 6.2 Datastore Tables

Required tables:

- `instances`
- `instance_state_transitions`
- `instance_types`
- `launch_templates`
- `launch_template_versions`
- `key_pairs`
- `network_interfaces`
- `network_interface_private_ips`
- `instance_block_devices`
- `instance_metadata_options`
- `instance_profiles`
- `placement_groups`
- `console_sessions`
- `instance_events`

### 6.3 State Machine

Allowed state transitions:

| From | To |
| --- | --- |
| none | pending |
| pending | running |
| pending | error |
| running | stopping |
| running | rebooting |
| running | shutting-down |
| stopping | stopped |
| stopped | pending |
| rebooting | running |
| shutting-down | terminated |
| error | stopped |
| error | terminated |

Rules:

- Terminated instances cannot be restarted.
- Stopped instances keep persistent volumes and ENIs unless configured otherwise.
- Ephemeral/local instance storage is lost on stop/terminate if documented by type.
- Termination protection blocks DELETE/terminate unless disabled first.

### 6.4 Runtime Requirements

The runtime layer must support:

- Resource limits matching instance type.
- CPU/memory/proc masking matching configured capacity.
- Runtime mode selection: auto, bwrap, chroot, crun, runc.
- Network namespace/ENI attachment before guest boot.
- Metadata token injection.
- User-data delivery.
- Logs capture.
- PTY console.
- Graceful stop and forced stop.
- Runtime startup error propagation into instance state reason.

## 7. WebUI Requirements

Add top-level WebUI section: **Compute > Instances**.

Pages:

- Instance list
- Launch instance wizard
- Instance detail dashboard
- Networking tab
- Storage tab
- Security tab
- Monitoring tab
- Console tab
- Logs tab
- Events tab
- Metadata/user data tab
- Image/snapshot tab
- Termination settings tab

### 7.1 Launch Wizard

Wizard steps:

1. Name and tags
2. Image selection
3. Instance type selection
4. Key pair
5. Network settings
6. Security groups
7. Storage
8. Advanced details
9. Review and launch

Image selection must show:

- Owner
- Visibility
- Architecture
- Digest
- Scan status
- SBOM/provenance available
- Deprecation status

Instance type selection must show:

- CPU
- Memory
- Local disk
- Network performance
- GPU
- Max ENIs/IPs
- Region/zone availability

Network settings must allow:

- VPC
- Subnet
- Auto/manual private IP
- Public IP behavior
- Existing ENI attach
- Create new ENI
- Source/destination check

Security must allow:

- Existing security group selection
- Create new security group inline
- Rule preview
- Warnings on broad ingress

Storage must allow:

- Root volume size/type/encryption/delete-on-termination
- Additional volumes
- Existing volume attach
- Snapshot source

Advanced details must allow:

- IAM role/profile
- User data
- Metadata token requirement
- Shutdown behavior
- Termination protection
- Monitoring level
- Placement group
- Host/node pool preference
- Environment/command override for capsule workloads

### 7.2 Instance Detail Dashboard

Must show:

- State and state reason
- Runtime mode
- Image
- Instance type
- Region/zone/node
- Private/public IPs
- ENIs
- Security groups
- Volumes
- Key pair
- IAM role
- CPU/memory/network/disk metrics
- Recent events
- Health status
- Drift status
- Actions available for current state

### 7.3 WebUI/API Sync Rules

- Every launch wizard field must map to a documented API field.
- Every API enum must be represented in UI constants.
- WebUI must not synthesize fake instance states not returned by API.
- WebUI actions must use the same endpoints as SDK/CLI.
- WebUI must display backend validation errors without losing field context.

## 8. SDK and CLI Requirements

SDK groups:

- Instances
- InstanceTypes
- LaunchTemplates
- KeyPairs
- NetworkInterfaces
- InstanceConsole
- InstanceMetrics
- InstanceEvents

CLI groups:

- `capper instance`
- `capper instance-type`
- `capper launch-template`
- `capper key-pair`
- `capper eni`
- `capper console`

CLI examples must cover:

- Launch instance into VPC/subnet.
- Launch from template.
- Attach/detach volume.
- Attach/detach ENI.
- Assign public IP.
- Open console.
- Stop/start/reboot/terminate.

## 9. Observability Requirements

Resource Monitor must track:

- Instance health.
- Runtime state.
- CPU percent.
- Memory used/limit.
- Disk read/write bytes.
- Network rx/tx bytes.
- Process count.
- Restart count.
- Last startup error.
- Security group drift.
- ENI/IP drift.
- Volume attachment drift.

Events:

- instance.create.requested
- instance.launch.scheduled
- instance.launch.started
- instance.running
- instance.stop.requested
- instance.stopped
- instance.rebooted
- instance.terminated
- instance.error
- eni.attached/detached
- volume.attached/detached
- security-groups.changed
- metadata-options.changed
- public-ip.associated/disassociated

Alerts:

- Instance failed to launch.
- Instance unhealthy.
- CPU high.
- Memory high.
- Disk full.
- Network saturation.
- Unexpected stop.
- Runtime unreachable.

## 10. IAM and Policy Actions

Required actions:

- `instance:run`
- `instance:list`
- `instance:get`
- `instance:update`
- `instance:start`
- `instance:stop`
- `instance:reboot`
- `instance:terminate`
- `instance:connect`
- `instance:console`
- `instance:logs`
- `instance:metrics`
- `instance:create-image`
- `instance:modify-metadata-options`
- `launch-template:*`
- `key-pair:*`
- `network-interface:*`
- `volume:attach`
- `volume:detach`
- `security-group:attach`
- `iam:pass-role`

Important: launching an instance with an instance role requires `iam:pass-role` or equivalent Capper permission.

## 11. Testing Requirements

Unit tests:

- Instance state transition validation.
- Launch request validation.
- Instance type compatibility.
- Metadata option defaults.
- Termination protection behavior.
- Key pair import/fingerprint.
- ENI attach constraints.
- Block device mapping validation.

Integration tests:

- Launch instance into a private subnet.
- Launch instance with existing ENI.
- Launch instance with public IP.
- Launch instance with key pair.
- Launch instance with user data.
- Launch instance with role/profile.
- Attach/detach volume.
- Attach/detach ENI.
- Stop/start/reboot/terminate.
- Console session works.
- Metrics/events appear in Resource Monitor.
- SDK, CLI, WebUI all launch using same request contract.

E2E tests:

- WebUI launch wizard creates a running instance.
- Instance detail page displays networking/storage/security accurately.
- Security group edit changes effective traffic policy.
- Termination protection blocks delete.
- Delete flow shows volume and ENI cleanup choices.

## 12. Migration Plan

Phase 1: Add expanded instance model while preserving current create payload.

Phase 2: Add launch templates, key pairs, ENIs, metadata options, and block device mappings.

Phase 3: Update runtime launch path to consume subnet/ENI/security group model.

Phase 4: Update WebUI launch wizard.

Phase 5: Update CLI/SDK and generated docs.

Phase 6: Mark old `network` field as compatibility-only and translate it to subnet/ENI.

## 13. Acceptance Criteria

This redesign is complete when:

- A user can launch an instance using AWS-style choices: image, type, key pair, VPC, subnet, SGs, storage, public IP, metadata, IAM role, tags.
- The instance can be described consistently through WebUI, API, SDK, CLI, and docs.
- Instance state transitions are reliable and audited.
- ENIs and volumes are first-class attachable resources.
- Resource Monitor shows instance health, metrics, events, and drift.
- Launch templates support versioned repeatable launches.
- Tests prevent WebUI/API/SDK drift.
