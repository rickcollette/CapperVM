package types

// RestartPolicy controls what happens when an instance stops unexpectedly.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "never"
	RestartAlways    RestartPolicy = "always"
	RestartOnFailure RestartPolicy = "on-failure"
)

// Mount describes a bind or tmpfs mount to add to the capsule at launch.
type Mount struct {
	Source   string `json:"source,omitempty"` // host path; empty for tmpfs
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly,omitempty"`
	Type     string `json:"type,omitempty"` // "bind" (default) or "tmpfs"
}

// PortMapping publishes a container port on the host.
type PortMapping struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"` // "tcp" (default) or "udp"
}

// ImagePolicy declares which instance types are allowed or denied for a capsule image.
// AllowedInstanceTypes: if non-empty, only listed types may launch this image.
// DeniedInstanceTypes: listed types are always rejected.
type ImagePolicy struct {
	AllowedInstanceTypes []string `json:"allowedInstanceTypes,omitempty"`
	DeniedInstanceTypes  []string `json:"deniedInstanceTypes,omitempty"`
	RequireGPU           bool     `json:"requireGpu,omitempty"`
}

type CapsuleManifest struct {
	CapsuleVersion string            `json:"capsuleVersion"`
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Created        string            `json:"created"`
	Hostname       string            `json:"hostname,omitempty"`
	Entrypoint     []string          `json:"entrypoint"`
	Args           []string          `json:"args"`
	Env            map[string]string `json:"env"`
	WorkingDir     string            `json:"workingDir"`
	Shell          string            `json:"shell"`
	User           UserConfig        `json:"user"`
	RootFS         RootFSInfo        `json:"rootfs"`
	Network        NetworkConfig     `json:"network"`
	Resources      ResourceLimits    `json:"resources,omitempty"`
	Mounts         []Mount           `json:"mounts,omitempty"`
	RestartPolicy  RestartPolicy     `json:"restartPolicy,omitempty"`
	Policy         ImagePolicy       `json:"policy,omitempty"`
	UseCapinit        bool   `json:"useCapinit,omitempty"`
	MetadataToken     string `json:"metadataToken,omitempty"` // host-side token file path
	PreferredRuntime  string `json:"preferredRuntime,omitempty"` // optional: auto|bwrap|chroot|crun|runc|lxc|qemu
}

type RootFSInfo struct {
	Archive     string `json:"archive"`
	Digest      string `json:"digest"`
	Compression string `json:"compression"`
}
