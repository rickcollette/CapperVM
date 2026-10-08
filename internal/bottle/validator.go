package bottle

import (
	"fmt"
	"strings"
)

// ValidateSpec checks required fields, parameter types, and dangerous
// capability declarations. Returns a slice of error strings.
func ValidateSpec(spec BottleSpec, params map[string]string) []string {
	var errs []string

	if spec.Kind != "bottle" && spec.Kind != "Bottle" {
		errs = append(errs, fmt.Sprintf("kind must be \"bottle\", got %q", spec.Kind))
	}
	if spec.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if spec.Metadata.Version == "" {
		errs = append(errs, "metadata.version is required")
	}
	if len(spec.Spec.Resources.Networks) > 0 {
		errs = append(errs, "resources.networks is no longer supported; migrate services to VPC subnet placement and remove service network references")
	}

	// Validate required parameters.
	for key, pspec := range spec.Spec.Parameters {
		if pspec.Required {
			val, supplied := params[key]
			if !supplied || val == "" {
				errs = append(errs, fmt.Sprintf("parameter %q is required but not supplied", key))
			}
		}
		if !validParamType(pspec.Type) {
			errs = append(errs, fmt.Sprintf("parameter %q has unknown type %q", key, pspec.Type))
		}
	}

	// Validate services reference defined networks and volumes.
	networkNames := make(map[string]bool)
	for _, n := range spec.Spec.Resources.Networks {
		networkNames[n.Name] = true
	}
	volumeNames := make(map[string]bool)
	for _, v := range spec.Spec.Resources.Volumes {
		volumeNames[v.Name] = true
	}
	for _, svc := range spec.Spec.Services {
		if svc.Name == "" {
			errs = append(errs, "service has no name")
		}
		if svc.Network != "" && !networkNames[svc.Network] {
			errs = append(errs, fmt.Sprintf("service %q references undefined network %q", svc.Name, svc.Network))
		}
		for _, vm := range svc.Volumes {
			if !volumeNames[vm.Name] {
				errs = append(errs, fmt.Sprintf("service %q references undefined volume %q", svc.Name, vm.Name))
			}
		}
	}

	// Block dangerous capabilities without explicit declaration.
	caps := spec.Spec.Capabilities
	if caps.RequiresHostMounts && !caps.RequiresHostMounts {
		errs = append(errs, "host mounts require capabilities.requiresHostMounts=true")
	}

	// Warn about downloads without checksums in build steps.
	if spec.Spec.Build != nil {
		for i, step := range spec.Spec.Build.Steps {
			if step.Download != nil && step.Download.Checksum == "" {
				errs = append(errs, fmt.Sprintf("build step %d: download of %q missing checksum", i+1, step.Download.URL))
			}
		}
	}

	return errs
}

var validParamTypes = map[string]bool{
	"string": true, "integer": true, "float": true, "boolean": true,
	"enum": true, "secret": true, "password": true, "path": true,
	"size": true, "port": true, "cidr": true, "hostname": true,
	"email": true, "url": true, "": true,
}

func validParamType(t string) bool {
	return validParamTypes[strings.ToLower(t)]
}
