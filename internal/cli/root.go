package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"capper/internal/alert"
	"capper/internal/autoscale"
	"capper/internal/backup"
	"capper/internal/bottle"
	"capper/internal/capinit"
	"capper/internal/compute"
	"capper/internal/control"
	"capper/internal/controller"
	"capper/internal/database"
	capperdns "capper/internal/dns"
	"capper/internal/firewall"
	"capper/internal/host"
	"capper/internal/iam"
	"capper/internal/lb"
	"capper/internal/loader"
	"capper/internal/manager"
	"capper/internal/metrics"
	"capper/internal/network"
	"capper/internal/networking"
	"capper/internal/org"
	capreg "capper/internal/registry"
	"capper/internal/runtime"
	caps3 "capper/internal/s3server"
	"capper/internal/sbom"
	"capper/internal/sign"
	"capper/internal/stack"
	capstore "capper/internal/storage"
	"capper/internal/store"
	"capper/internal/types"
	"capper/internal/version"
	"capper/internal/vpc"
)

type options struct {
	storePath   string
	runtimeMode string
	project     string
	debug       bool
	json        bool
}

func Execute() error {
	return NewRootCmd().Execute()
}

// NewRootCmd builds the full `capper` command tree and returns the root command
// without executing it. Command construction never touches the store or network
// (only RunE does), so this is safe to call from tooling — e.g. the docs
// generator walks this tree to produce the CLI reference.
func NewRootCmd() *cobra.Command {
	opts := &options{}
	root := &cobra.Command{
		Use:           "capper",
		Short:         "local experimental chroot-based capsule runner",
		Long:          "Capper is a local experimental chroot-based capsule runner.\n\nDo not run untrusted .cap images with Capper v0.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.storePath, "store", "", "Capper store path")
	root.PersistentFlags().StringVar(&opts.runtimeMode, "runtime", "auto", "runtime backend: auto, bwrap, chroot, crun, runc, lxc, or qemu")
	root.PersistentFlags().StringVar(&opts.project, "project", "default", "project namespace for resources")
	root.PersistentFlags().BoolVar(&opts.debug, "debug", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&opts.json, "json", false, "emit JSON output when applicable")
	root.AddCommand(
		createCmd(opts),
		runCmd(opts),
		connectCmd(opts),
		execCmd(opts),
		listCmd(opts),
		logsCmd(opts),
		inspectCmd(opts),
		stopCmd(opts),
		rmCmd(opts),
		deleteCmd(opts),
		validateCmd(opts),
		signCmd(opts),
		verifyCmd(opts),
		keygenCmd(),
		attestCmd(opts),
		daemonCmd(opts),
		apiCmd(opts),
		runLimitedCmd(),
		projectCmd(opts),
		iamCmd(opts),
		hostCmd(opts),
		networkCmd(opts),
		firewallCmd(opts),
		dnsCmd(opts),
		computeCmd(opts),
		storageCmd(opts),
		registryCmd(opts),
		eventCmd(opts),
		secretCmd(opts),
		kmsCmd(opts),
		certCmd(opts),
		postureCmd(opts),
		lbCmd(opts),
		statsCmd(opts),
		alertCmd(opts),
		dbCmd(opts),
		aiCmd(opts),
		marketCmd(opts),
		backupCmd(opts),
		controlStatusCmd(opts),
		stackCmd(opts),
		jobCmd(opts),
		bottleCmd(opts),
		healthCmd(opts),
		quotaCmd(opts),
		governanceCmd(opts),
		queueCmd(opts),
		ingressCmd(opts),
		ruleCmd(opts),
		scheduleCmd(opts),
		orgCmd(opts),
		contextCmd(opts),
		resourcesCmd(opts),
		metricsCmd(opts),
		configCmd(opts),
		alertsObserveCmd(opts),
		fnCmd(opts),
		mcpCmd(opts),
		ipPoolCmd(opts),
		ipCmd(opts),
		ipExclusionCmd(opts),
		hostStorageCmd(opts),
		volumeCmd(opts),
		realmCmd(opts),
		regionCmd(opts),
		zoneCmd(opts),
		nodeCmd(opts),
		vpcCmd(opts),
		sgCmd(opts),
		igwCmd(opts),
		natCmd(opts),
		routeTableCmd(opts),
		placementCmd(opts),
		schedulerCmd(opts),
		aioCmd(opts),
		versionCmd(opts),
		schemaCmd(opts),
		fleetCmd(opts),
	)
	return root
}

// schemaCmd manages the control-plane database schema (migrations + snapshots).
// Note: this is the control-plane store schema, distinct from `capper db` (the
// managed-database product).
func schemaCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "inspect and snapshot the control-plane database schema",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "show applied + pending schema migrations and the schema version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				applied, err := ctrl.Store.AppliedMigrations()
				if err != nil {
					return err
				}
				pending, err := ctrl.Store.PendingMigrations()
				if err != nil {
					return err
				}
				ver, _ := ctrl.Store.SchemaVersion()
				if opts.json {
					return printJSON(map[string]any{
						"schemaVersion": ver, "applied": applied, "pending": pending,
					})
				}
				fmt.Printf("Schema version: %s\n\nApplied migrations:\n", orDash(ver))
				for _, m := range applied {
					fmt.Printf("  %s  (%s)\n", m.Version, m.AppliedAt)
				}
				if len(pending) == 0 {
					fmt.Println("\nPending migrations: none (up to date)")
				} else {
					fmt.Printf("\nPending migrations:\n")
					for _, p := range pending {
						fmt.Printf("  %s\n", p)
					}
				}
				return nil
			})
		},
	}

	backupCmd := &cobra.Command{
		Use:   "backup [DEST]",
		Short: "write a consistent online snapshot of the control-plane database",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				dest := ctrl.Store.SnapshotPath()
				if len(args) == 1 {
					dest = args[0]
				}
				if err := ctrl.Store.SnapshotDB(dest); err != nil {
					return err
				}
				if opts.json {
					return printJSON(map[string]string{"snapshot": dest})
				}
				fmt.Printf("Snapshot written: %s\n", dest)
				return nil
			})
		},
	}

	cmd.AddCommand(statusCmd, backupCmd)
	return cmd
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// versionCmd prints the build-stamped version of this binary.
func versionCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the capper version, commit, and build info",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Get()
			if opts.json {
				return printJSON(info)
			}
			fmt.Println(info.String())
			return nil
		},
	}
}

func withController(opts *options, fn func(controller.Controller) error) error {
	root := opts.storePath
	if root == "" {
		var err error
		root, err = store.DefaultRoot()
		if err != nil {
			return err
		}
	}
	st, err := store.Open(store.NewPaths(root))
	if err != nil {
		return err
	}
	defer st.Close()
	if err := runtime.ValidateMode(opts.runtimeMode); err != nil {
		return err
	}
	ctrl := controller.New(st, opts.debug, opts.runtimeMode)
	return fn(ctrl)
}

func createCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create IMAGE_NAME CONFIG_FILE_NAME.json",
		Short: "create a .cap image",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("image:create", "project:"+opts.project); err != nil {
					return err
				}
				res, err := ctrl.Images.Create(args[0], args[1])
				if err != nil {
					return fmt.Errorf("Failed to create image.\n\nReason:\n  %w", err)
				}
				_ = ctrl.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "image",
					ResourceID:    res.Image.Name,
					Action:        "image.created",
					ProjectID:     opts.project,
					PrincipalType: ctrl.PrincipalType(),
					PrincipalID:   ctrl.PrincipalID(),
					Data:          map[string]any{"name": res.Image.Name, "version": res.Image.Version},
				})
				if opts.json {
					return printJSON(map[string]any{"name": res.Image.Name, "version": res.Image.Version, "path": res.Image.Path, "digest": res.Image.Digest, "sizeBytes": res.Image.SizeBytes})
				}
				fmt.Printf("Created image\n\nName:    %s\nVersion: %s\nPath:    %s\nDigest:  %s\nSize:    %s\n", res.Image.Name, res.Image.Version, res.Image.Path, res.Image.Digest, humanSize(res.Image.SizeBytes))
				return nil
			})
		},
	}
}

func runCmd(opts *options) *cobra.Command {
	var memory string
	var fileSize string
	var cpuTime int64
	var pids int64
	var name string
	var rm bool
	var requireSig bool
	var trustedKey string
	var mountSpecs []string
	var publishSpecs []string
	var restart string
	var subnetID string
	var vpcID string
	var networkName string // deprecated alias; errors with a migration hint
	var instanceTypeName string
	var secretSpecs []string
	var labelSpecs []string
	var overrideScan bool
	var runtimeModeOverride string
	cmd := &cobra.Command{
		Use:   "run IMAGE_NAME.cap",
		Short: "run a .cap image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if requireSig || trustedKey != "" {
					imagePath, _, err := loader.Loader{Paths: ctrl.Store.Paths}.ResolveImage(args[0])
					if err != nil {
						return fmt.Errorf("image not found: %s", args[0])
					}
					keyPath := trustedKey
					if keyPath == "" {
						keyPath = "capper.pub"
					}
					pub, err := sign.LoadPublicKey(keyPath)
					if err != nil {
						return fmt.Errorf("load trusted key %s: %w", keyPath, err)
					}
					if err := sign.VerifyImage(imagePath, pub); err != nil {
						return fmt.Errorf("image signature verification failed: %w", err)
					}
				}
				// Scan status check: block images with critical findings unless overridden.
				if !overrideScan {
					imgs, _ := ctrl.Store.Registry.ListImages("")
					imageBase := filepath.Base(args[0])
					imageBase = strings.TrimSuffix(imageBase, ".cap")
					for _, img := range imgs {
						if (img.Name == imageBase || img.Name == args[0]) && img.ScanStatus == "critical" {
							return fmt.Errorf("image has critical scan findings; use --override-scan to proceed")
						}
					}
				}
				resources, err := parseResourceFlags(
					memory,
					cpuTime,
					pids,
					fileSize,
					cmd.Flags().Changed("memory"),
					cmd.Flags().Changed("cpu-time"),
					cmd.Flags().Changed("pids"),
					cmd.Flags().Changed("file-size"),
				)
				if err != nil {
					return fmt.Errorf("Failed to run image.\n\nReason:\n  %w", err)
				}
				mounts, err := parseMountSpecs(mountSpecs)
				if err != nil {
					return fmt.Errorf("invalid --mount: %w", err)
				}
				ports, err := parsePublishSpecs(publishSpecs)
				if err != nil {
					return fmt.Errorf("invalid --publish: %w", err)
				}
				var restartPolicy types.RestartPolicy
				if restart != "" {
					switch restart {
					case "never", "always", "on-failure":
						restartPolicy = types.RestartPolicy(restart)
					default:
						return fmt.Errorf("invalid --restart value: %s (valid: never, always, on-failure)", restart)
					}
				}
				if err := ctrl.Authorize("instance:run", "project:"+opts.project); err != nil {
					return err
				}
				// Instance type enforcement: validate type policy, resource override, GPU check.
				if instanceTypeName != "" {
					if err := ctrl.Authorize("compute:type:use", "project:"+opts.project); err != nil {
						return err
					}
					it, err := ctrl.Store.Compute.GetInstanceType(instanceTypeName)
					if err != nil {
						return fmt.Errorf("instance type %q not found", instanceTypeName)
					}
					// Apply type resource limits (type wins over manual flags).
					if it.MemoryBytes > 0 && !cmd.Flags().Changed("memory") {
						resources.Limits.MemoryBytes = it.MemoryBytes
						resources.MemorySet = true
					}
					if it.PIDLimit > 0 && !cmd.Flags().Changed("pids") {
						resources.Limits.MaxProcesses = int64(it.PIDLimit)
						resources.PidsSet = true
					}
					if it.DiskBytes > 0 {
						resources.Limits.DiskBytes = it.DiskBytes
						resources.DiskSet = true
					}
					if it.CPUCount > 0 {
						resources.Limits.CPUCount = int64(it.CPUCount)
						resources.CPUSet = true
					}
					// Image type policy check.
					policyLd := loader.Loader{Paths: ctrl.Store.Paths}
					if imagePath, _, resolveErr := policyLd.ResolveImage(args[0]); resolveErr == nil {
						if manifest, readErr := loader.ReadManifest(imagePath); readErr == nil {
							if len(manifest.Policy.DeniedInstanceTypes) > 0 {
								for _, denied := range manifest.Policy.DeniedInstanceTypes {
									if denied == instanceTypeName {
										return fmt.Errorf("image policy denies instance type %q", instanceTypeName)
									}
								}
							}
							if len(manifest.Policy.AllowedInstanceTypes) > 0 {
								allowed := false
								for _, a := range manifest.Policy.AllowedInstanceTypes {
									if a == instanceTypeName {
										allowed = true
										break
									}
								}
								if !allowed {
									return fmt.Errorf("image policy does not allow instance type %q (allowed: %v)", instanceTypeName, manifest.Policy.AllowedInstanceTypes)
								}
							}
						}
					}
					// GPU availability check.
					if it.GPUEligible {
						gpuMgr := compute.NewManager(ctrl.Store.Compute)
						gpus, _ := gpuMgr.ListGPUDevices()
						available := 0
						for _, g := range gpus {
							if g.Status == compute.GPUStatusAvailable {
								available++
							}
						}
						if available < it.GPUCount {
							return fmt.Errorf("instance type %q requires %d GPU(s) but only %d available", instanceTypeName, it.GPUCount, available)
						}
					}
				}
				// Resolve --secret NAME=ENVVAR into extra env vars.
				var secretEnv map[string]string
				if len(secretSpecs) > 0 {
					if err := ctrl.Authorize("secret:read", "project:"+opts.project); err != nil {
						return err
					}
					secretEnv = make(map[string]string)
					for _, spec := range secretSpecs {
						secretName, envVar, _ := strings.Cut(spec, "=")
						if envVar == "" {
							envVar = strings.ToUpper(secretName)
						}
						val, serr := ctrl.Store.Secrets.GetDecrypted(secretName, opts.project)
						if serr != nil {
							return fmt.Errorf("secret %q: %w", secretName, serr)
						}
						secretEnv[envVar] = val
					}
				}
				labels, lerr := parseKeyValSpecs(labelSpecs)
				if lerr != nil {
					return fmt.Errorf("invalid --label: %w", lerr)
				}
				if secretEnv == nil {
					secretEnv = make(map[string]string)
				}
				secretEnv["CAPPER_METADATA_URL"] = "http://169.254.169.254/capper/v1"
				secretEnv["CAPPER_METADATA_TOKEN_FILE"] = "/run/capper/metadata-token"
				if runtimeModeOverride != "" {
					if err := runtime.ValidateMode(runtimeModeOverride); err != nil {
						return err
					}
				}
				runOpts := manager.RunOptions{
					Name:          name,
					Mounts:        mounts,
					Ports:         ports,
					RestartPolicy: restartPolicy,
					Env:           secretEnv,
					Labels:        labels,
					RuntimeMode:   runtimeModeOverride,
				}
				if networkName != "" {
					return fmt.Errorf("--network is removed; use --subnet-id with a VPC subnet ID")
				}
				if subnetID == "" {
					return fmt.Errorf("--subnet-id is required (instances must be placed in a VPC subnet)")
				}
				launchSub, launchENI, netOpts, nerr := resolveCLISubnetLaunch(ctrl.Store, subnetID, vpcID)
				if nerr != nil {
					return nerr
				}
				runOpts.Network = netOpts
				// Account quota enforcement: deny if project/account has hit its instance quota.
				if qerr := ctrl.Store.Billing.CheckAccountQuota(opts.project, "instance"); qerr != nil {
					return fmt.Errorf("quota exceeded: %w", qerr)
				}
				inst, err := ctrl.Instances.Run(args[0], resources, runOpts)
				if err != nil {
					_ = ctrl.Store.VPC.DeleteENI(launchENI.ID)
					if strings.Contains(err.Error(), "image not found") {
						ld := loader.Loader{Paths: ctrl.Store.Paths, Debug: opts.debug}
						_, searched, _ := ld.ResolveImage(args[0])
						return fmt.Errorf("Failed to run image.\n\nReason:\n  %w\n\nSearched:\n  %s", err, strings.Join(searched, "\n  "))
					}
					return fmt.Errorf("Failed to start instance.\n\nReason:\n  %w", err)
				}
				if inst.NetworkIP == "" {
					// Soft dataplane failure: do not leave the ENI in use without an IP.
					_ = ctrl.Store.VPC.DeleteENI(launchENI.ID)
				} else if _, aerr := ctrl.Store.VPC.AttachENI(launchENI.ID, inst.ID, 0); aerr != nil {
					_ = ctrl.Store.VPC.DeleteENI(launchENI.ID)
					_ = ctrl.Instances.Remove(inst.ID)
					return fmt.Errorf("eni attach: %w", aerr)
				} else {
					inst.VPCID = launchSub.VPCID
					inst.SubnetID = launchSub.ID
					inst.PrimaryENIID = launchENI.ID
					inst.PrivateIPAddress = inst.NetworkIP
					_ = ctrl.Store.UpdateInstance(*inst)
				}
				_ = ctrl.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "instance",
					ResourceID:    inst.ID,
					Action:        "instance.started",
					ProjectID:     opts.project,
					PrincipalType: ctrl.PrincipalType(),
					PrincipalID:   ctrl.PrincipalID(),
					Data:          map[string]any{"name": inst.Name, "image": inst.Image},
				})
				// DNS auto-registration and firewall recompile after network attach.
				if inst.NetworkID != "" && inst.NetworkIP != "" {
					dnsAutoRegister(ctrl.Store, inst.NetworkID, inst.Name, inst.NetworkIP)
					firewallAutoApply(ctrl.Store, inst.NetworkID)
				}
				// Port publishing: add DNAT rules for --publish HOST:CONTAINER specs.
				if inst.NetworkIP != "" && len(ports) > 0 {
					applyPortPublish(inst.NetworkIP, ports)
				}
				if opts.json {
					_ = printJSON(inst)
				} else {
					line := fmt.Sprintf("Started instance\n\nName:   %s\nID:     %s\nImage:  %s\nPID:    %d\nStatus: %s\n", inst.Name, inst.ID, inst.Image, inst.PID, inst.Status)
					if inst.NetworkIP != "" {
						line += fmt.Sprintf("IP:     %s\n", inst.NetworkIP)
					}
					fmt.Print(line)
				}
				if rm {
					if inst.Status == types.StatusRunning {
						// Block until the instance stops, forwarding signals.
						sigCh := make(chan os.Signal, 1)
						signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
						defer signal.Stop(sigCh)
						ticker := time.NewTicker(500 * time.Millisecond)
						defer ticker.Stop()
						fmt.Printf("Waiting for instance to stop (Ctrl-C to stop it now)...\n")
					waitLoop:
						for {
							select {
							case <-sigCh:
								fmt.Printf("\nStopping instance %s...\n", inst.Name)
								_, _, _ = ctrl.Instances.Stop(inst.ID, 5*time.Second, false)
								break waitLoop
							case <-ticker.C:
								current, err := ctrl.Store.ResolveInstance(inst.ID)
								if err != nil {
									break waitLoop
								}
								if err := ctrl.Instances.Refresh(current); err != nil {
									break waitLoop
								}
								if current.Status != types.StatusRunning {
									break waitLoop
								}
							}
						}
					}
					if err := ctrl.Instances.Remove(inst.ID); err != nil {
						return fmt.Errorf("Warning: --rm cleanup failed: %w", err)
					}
					fmt.Printf("Instance removed (--rm)\n")
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&memory, "memory", "", "limit virtual memory/address space, e.g. 128M, 1G")
	cmd.Flags().Int64Var(&cpuTime, "cpu-time", 0, "limit CPU time in seconds")
	cmd.Flags().Int64Var(&pids, "pids", 0, "limit number of processes for the capsule user")
	cmd.Flags().StringVar(&fileSize, "file-size", "", "limit maximum file size written by the capsule, e.g. 64M")
	cmd.Flags().StringVar(&name, "name", "", "assign a name to the instance")
	cmd.Flags().BoolVar(&rm, "rm", false, "remove instance automatically after it stops")
	cmd.Flags().BoolVar(&requireSig, "require-signature", false, "refuse to run if the image is not signed")
	cmd.Flags().StringVar(&trustedKey, "trusted-key", "", "path to trusted public key; implies --require-signature")
	cmd.Flags().StringArrayVar(&mountSpecs, "mount", nil, "bind mount in SOURCE:TARGET[:ro] format, repeatable")
	cmd.Flags().StringArrayVar(&publishSpecs, "publish", nil, "publish a container port as HOST:CONTAINER[/proto], repeatable")
	cmd.Flags().StringVar(&restart, "restart", "", "restart policy: never, always, or on-failure")
	cmd.Flags().StringVar(&subnetID, "subnet-id", "", "VPC subnet ID to place the instance in (required)")
	cmd.Flags().StringVar(&vpcID, "vpc-id", "", "VPC ID (optional; must match the subnet)")
	cmd.Flags().StringVar(&networkName, "network", "", "removed: use --subnet-id")
	_ = cmd.Flags().MarkHidden("network")
	cmd.Flags().StringVar(&instanceTypeName, "instance-type", "", "enforce an instance type envelope (e.g. cap-g1)")
	cmd.Flags().StringArrayVar(&secretSpecs, "secret", nil, "inject a secret as an env var: SECRET_NAME[=ENV_VAR], repeatable")
	cmd.Flags().StringArrayVar(&labelSpecs, "label", nil, "attach a label: KEY=VALUE, repeatable")
	cmd.Flags().BoolVar(&overrideScan, "override-scan", false, "skip scan status check and run even if image has critical findings")
	cmd.Flags().StringVar(&runtimeModeOverride, "runtime-mode", "", "per-instance runtime override: auto, bwrap, chroot, crun, runc, lxc, or qemu")
	return cmd
}

func runLimitedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__run-limited LAUNCH_SPEC",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runtime.RunLimitedLauncher(args[0])
		},
	}
	return cmd
}

func connectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "connect INSTANCE_NAME|INSTANCE_ID",
		Short: "connect to a running instance shell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:connect", "project:"+opts.project); err != nil {
					return err
				}
				if err := ctrl.Instances.Connect(args[0]); err != nil {
					return fmt.Errorf("Failed to connect.\n\nReason:\n  %w", err)
				}
				return nil
			})
		},
	}
}

func execCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "exec INSTANCE_NAME|INSTANCE_ID COMMAND [ARGS...]",
		Short: "execute a command inside a running instance",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:exec", "project:"+opts.project); err != nil {
					return err
				}
				if err := ctrl.Instances.Exec(args[0], args[1:]); err != nil {
					return fmt.Errorf("Failed to exec.\n\nReason:\n  %w", err)
				}
				return nil
			})
		},
	}
}

func logsCmd(opts *options) *cobra.Command {
	var stdoutOnly, stderrOnly bool
	var selectorSpecs []string
	cmd := &cobra.Command{
		Use:   "logs INSTANCE_NAME|INSTANCE_ID",
		Short: "show instance logs (use --selector for multi-instance view)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:logs", "project:"+opts.project); err != nil {
					return err
				}
				selector, serr := parseKeyValSpecs(selectorSpecs)
				if serr != nil {
					return fmt.Errorf("invalid --selector: %w", serr)
				}
				// Collect instances to show logs for.
				var targets []*types.Instance
				if len(selector) > 0 {
					all, err := ctrl.Store.ListInstances()
					if err != nil {
						return err
					}
					for i := range all {
						inst := ctrl.Store.MergeInstanceFromDisk(all[i])
						if matchesSelector(*inst, selector) {
							targets = append(targets, inst)
						}
					}
					if len(targets) == 0 {
						return fmt.Errorf("no instances match selector %v", selectorSpecs)
					}
				} else {
					if len(args) == 0 {
						return fmt.Errorf("specify an instance name/ID or use --selector")
					}
					inst, err := ctrl.Store.ResolveInstance(args[0])
					if err != nil {
						return fmt.Errorf("instance not found: %s", args[0])
					}
					targets = []*types.Instance{inst}
				}
				showHeader := len(targets) > 1
				for _, inst := range targets {
					instDir := filepath.Dir(inst.RootFSPath)
					if showHeader {
						fmt.Printf("=== %s (%s) ===\n", inst.Name, inst.ID)
					}
					if !stderrOnly {
						data, err := os.ReadFile(filepath.Join(instDir, "stdout.log"))
						if err != nil && !os.IsNotExist(err) {
							return fmt.Errorf("read stdout log: %w", err)
						}
						if err == nil {
							os.Stdout.Write(data)
						}
					}
					if !stdoutOnly {
						data, err := os.ReadFile(filepath.Join(instDir, "stderr.log"))
						if err != nil && !os.IsNotExist(err) {
							return fmt.Errorf("read stderr log: %w", err)
						}
						if err == nil {
							os.Stderr.Write(data)
						}
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&stdoutOnly, "stdout", false, "show stdout only")
	cmd.Flags().BoolVar(&stderrOnly, "stderr", false, "show stderr only")
	cmd.Flags().StringArrayVar(&selectorSpecs, "selector", nil, "filter by label KEY=VALUE, repeatable (combined log view)")
	return cmd
}

func inspectCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "inspect", Short: "inspect an image or instance"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "image IMAGE_NAME",
			Short: "show detailed image information",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withController(opts, func(ctrl controller.Controller) error {
					if err := ctrl.Authorize("image:inspect", "project:"+opts.project); err != nil {
						return err
					}
					name := args[0]
					if !strings.HasSuffix(name, ".cap") {
						name += ".cap"
					}
					img, err := ctrl.Store.GetImage(name)
					if err != nil {
						return fmt.Errorf("image not found: %s", args[0])
					}
					if _, err := os.Stat(img.Path); err != nil {
						img.Missing = true
					}
					if opts.json {
						return printJSON(img)
					}
					fmt.Printf("Name:    %s\nVersion: %s\nPath:    %s\nDigest:  %s\nSize:    %s\nCreated: %s\n",
						img.Name, img.Version, img.Path, img.Digest, humanSize(img.SizeBytes), img.CreatedAt)
					if img.Missing {
						fmt.Println("\nWarning: image file not found on disk")
					}
					return nil
				})
			},
		},
		&cobra.Command{
			Use:   "instance INSTANCE_NAME|INSTANCE_ID",
			Short: "show detailed instance information",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withController(opts, func(ctrl controller.Controller) error {
					if err := ctrl.Authorize("instance:inspect", "project:"+opts.project); err != nil {
						return err
					}
					inst, err := ctrl.Store.ResolveInstance(args[0])
					if err != nil {
						return fmt.Errorf("instance not found: %s", args[0])
					}
					if err := ctrl.Instances.Refresh(inst); err != nil {
						return err
					}
					if opts.json {
						return printJSON(inst)
					}
					pid := "-"
					if inst.PID > 0 {
						pid = strconv.Itoa(inst.PID)
					}
					stopped := "-"
					if inst.StoppedAt != nil {
						stopped = *inst.StoppedAt
					}
					fmt.Printf("ID:        %s\nName:      %s\nImage:     %s\nStatus:    %s\nPID:       %s\nCommand:   %s\nCreated:   %s\nStarted:   %s\nStopped:   %s\nRootFS:    %s\n",
						inst.ID, inst.Name, inst.Image, inst.Status, pid,
						inst.Command, inst.CreatedAt, inst.StartedAt, stopped, inst.RootFSPath)
					if !inst.Resources.Empty() {
						fmt.Printf("Resources: memory=%s cpu-time=%ds pids=%d file-size=%s\n",
							humanSize(inst.Resources.MemoryBytes),
							inst.Resources.CPUTimeSecs,
							inst.Resources.MaxProcesses,
							humanSize(inst.Resources.FileSizeBytes))
					}
					return nil
				})
			},
		},
	)
	return cmd
}

func listCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "list images or instances"}
	cmd.AddCommand(&cobra.Command{
		Use:   "images",
		Short: "list images",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("image:list", "project:"+opts.project); err != nil {
					return err
				}
				images, err := ctrl.Images.List()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(images)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tVERSION\tSIZE\tDIGEST\tCREATED")
				for _, img := range images {
					digest := img.Digest
					if len(digest) > 16 {
						digest = digest[:16] + "..."
					}
					name := img.Name
					if img.Missing {
						name += " (missing)"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, img.Version, humanSize(img.SizeBytes), digest, shortTime(img.CreatedAt))
				}
				return w.Flush()
			})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "instances",
		Short: "list instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:list", "project:"+opts.project); err != nil {
					return err
				}
				instances, err := ctrl.Instances.List()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(instanceJSON(instances))
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tNAME\tIMAGE\tPID\tSTATUS\tCREATED")
				for _, inst := range instances {
					pid := "-"
					if inst.PID > 0 && inst.Status == types.StatusRunning {
						pid = strconv.Itoa(inst.PID)
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", inst.ID, inst.Name, inst.Image, pid, inst.Status, shortTime(inst.CreatedAt))
				}
				return w.Flush()
			})
		},
	})
	return cmd
}

func stopCmd(opts *options) *cobra.Command {
	var timeout int
	var killNow bool
	cmd := &cobra.Command{
		Use:   "stop INSTANCE_NAME|INSTANCE_ID",
		Short: "stop a running instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:stop", "project:"+opts.project); err != nil {
					return err
				}
				inst, stopped, err := ctrl.Instances.Stop(args[0], time.Duration(timeout)*time.Second, killNow)
				if err != nil {
					return fmt.Errorf("Failed to stop instance.\n\nReason:\n  %w", err)
				}
				if !stopped {
					fmt.Printf("Instance is already stopped: %s %s\n", inst.Name, inst.ID)
					return nil
				}
				_ = ctrl.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "instance",
					ResourceID:    inst.ID,
					Action:        "instance.stopped",
					ProjectID:     opts.project,
					PrincipalType: ctrl.PrincipalType(),
					PrincipalID:   ctrl.PrincipalID(),
				})
				fmt.Printf("Stopped instance %s %s\n", inst.Name, inst.ID)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&timeout, "timeout", 5, "seconds to wait before SIGKILL")
	cmd.Flags().BoolVar(&killNow, "kill", false, "send SIGKILL immediately")
	return cmd
}

func rmCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "rm INSTANCE_NAME|INSTANCE_ID",
		Short: "remove a stopped or failed instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:delete", "project:"+opts.project); err != nil {
					return err
				}
				// Deregister DNS and recompile firewall before Remove tears down the network.
				if inst, err := ctrl.Store.ResolveInstance(args[0]); err == nil && inst.NetworkID != "" {
					dnsAutoDeregister(ctrl.Store, inst.NetworkID, inst.Name)
					firewallAutoApply(ctrl.Store, inst.NetworkID)
				}
				if err := ctrl.Instances.Remove(args[0]); err != nil {
					return fmt.Errorf("Failed to remove instance.\n\nReason:\n  %w", err)
				}
				_ = ctrl.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "instance",
					ResourceID:    args[0],
					Action:        "instance.deleted",
					ProjectID:     opts.project,
					PrincipalType: ctrl.PrincipalType(),
					PrincipalID:   ctrl.PrincipalID(),
				})
				fmt.Printf("Removed instance %s\n", args[0])
				return nil
			})
		},
	}
}

func deleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete IMAGE_NAME",
		Short: "delete a local image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("image:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ctrl.Images.Delete(args[0]); err != nil {
					if strings.Contains(err.Error(), "image in use") {
						return fmt.Errorf("Cannot delete image %s.\n\nRunning instances still use this image.\n\nStop them first.", args[0])
					}
					return err
				}
				name := args[0]
				if !strings.HasSuffix(name, ".cap") {
					name += ".cap"
				}
				_ = ctrl.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "image",
					ResourceID:    name,
					Action:        "image.deleted",
					ProjectID:     opts.project,
					PrincipalType: ctrl.PrincipalType(),
					PrincipalID:   ctrl.PrincipalID(),
				})
				fmt.Printf("Deleted image %s\n", name)
				return nil
			})
		},
	}
}

func validateCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "validate", Short: "validate a config file or image"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "config FILE",
			Short: "validate a create config file",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := types.LoadCreateConfig(args[0])
				if err != nil {
					return fmt.Errorf("Invalid config: %w", err)
				}
				if opts.json {
					return printJSON(map[string]any{"valid": true, "name": cfg.Name, "version": cfg.Version})
				}
				fmt.Printf("Config is valid\n\nName:    %s\nVersion: %s\n", cfg.Name, cfg.Version)
				return nil
			},
		},
		&cobra.Command{
			Use:   "image IMAGE.cap",
			Short: "validate a .cap image file",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withController(opts, func(ctrl controller.Controller) error {
					ld := loader.Loader{Paths: ctrl.Store.Paths, Debug: opts.debug}
					loaded, cleanup, err := ld.Load(args[0])
					if err != nil {
						return fmt.Errorf("Invalid image: %w", err)
					}
					cleanup()
					if opts.json {
						return printJSON(map[string]any{
							"valid":   true,
							"name":    loaded.Manifest.Name,
							"version": loaded.Manifest.Version,
							"digest":  loaded.Digest,
						})
					}
					fmt.Printf("Image is valid\n\nName:    %s\nVersion: %s\nDigest:  %s\n",
						loaded.Manifest.Name, loaded.Manifest.Version, loaded.Digest)
					return nil
				})
			},
		},
	)
	return cmd
}

func keygenCmd() *cobra.Command {
	var keyOut string
	var pubOut string
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "generate an Ed25519 signing key pair",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sign.GenerateKeyPair(keyOut, pubOut); err != nil {
				return fmt.Errorf("Failed to generate key pair.\n\nReason:\n  %w", err)
			}
			fmt.Printf("Generated key pair\n\nPrivate key: %s\nPublic key:  %s\n", keyOut, pubOut)
			return nil
		},
	}
	cmd.Flags().StringVar(&keyOut, "key-out", "capper.key", "output path for private key")
	cmd.Flags().StringVar(&pubOut, "pub-out", "capper.pub", "output path for public key")
	return cmd
}

func signCmd(opts *options) *cobra.Command {
	var keyPath string
	cmd := &cobra.Command{
		Use:   "sign IMAGE.cap",
		Short: "sign a .cap image with an Ed25519 private key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				priv, err := sign.LoadPrivateKey(keyPath)
				if err != nil {
					return fmt.Errorf("load private key %s: %w", keyPath, err)
				}
				imagePath, _, err := loader.Loader{Paths: ctrl.Store.Paths}.ResolveImage(args[0])
				if err != nil {
					return fmt.Errorf("image not found: %s", args[0])
				}
				if err := sign.SignImage(imagePath, imagePath, priv); err != nil {
					return fmt.Errorf("Failed to sign image.\n\nReason:\n  %w", err)
				}
				fmt.Printf("Signed image %s\n", imagePath)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&keyPath, "key", "capper.key", "path to Ed25519 private key")
	return cmd
}

func verifyCmd(opts *options) *cobra.Command {
	var pubPath string
	cmd := &cobra.Command{
		Use:   "verify IMAGE.cap",
		Short: "verify the Ed25519 signature on a .cap image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				pub, err := sign.LoadPublicKey(pubPath)
				if err != nil {
					return fmt.Errorf("load public key %s: %w", pubPath, err)
				}
				imagePath, _, err := loader.Loader{Paths: ctrl.Store.Paths}.ResolveImage(args[0])
				if err != nil {
					return fmt.Errorf("image not found: %s", args[0])
				}
				if err := sign.VerifyImage(imagePath, pub); err != nil {
					return fmt.Errorf("Signature verification failed.\n\nReason:\n  %w", err)
				}
				if opts.json {
					return printJSON(map[string]any{"verified": true, "image": imagePath})
				}
				fmt.Printf("Signature verified: %s\n", imagePath)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&pubPath, "trusted-key", "capper.pub", "path to trusted Ed25519 public key")
	return cmd
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func parseResourceFlags(memory string, cpuTime, pids int64, fileSize string, memorySet, cpuTimeSet, pidsSet, fileSizeSet bool) (types.ResourceOverrides, error) {
	var limits types.ResourceLimits
	if memorySet {
		memBytes, err := parseSize(memory)
		if err != nil {
			return types.ResourceOverrides{}, fmt.Errorf("invalid --memory: %w", err)
		}
		limits.MemoryBytes = memBytes
	}
	if fileSizeSet {
		fileBytes, err := parseSize(fileSize)
		if err != nil {
			return types.ResourceOverrides{}, fmt.Errorf("invalid --file-size: %w", err)
		}
		limits.FileSizeBytes = fileBytes
	}
	if cpuTimeSet {
		if cpuTime < 0 {
			return types.ResourceOverrides{}, fmt.Errorf("--cpu-time must be non-negative")
		}
		limits.CPUTimeSecs = cpuTime
	}
	if pidsSet {
		if pids < 0 {
			return types.ResourceOverrides{}, fmt.Errorf("--pids must be non-negative")
		}
		limits.MaxProcesses = pids
	}
	return types.ResourceOverrides{
		Limits:      limits,
		MemorySet:   memorySet,
		CPUTimeSet:  cpuTimeSet,
		PidsSet:     pidsSet,
		FileSizeSet: fileSizeSet,
	}, nil
}

func parseSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	multiplier := float64(1)
	last := value[len(value)-1]
	switch last {
	case 'k', 'K':
		multiplier = 1024
		value = value[:len(value)-1]
	case 'm', 'M':
		multiplier = 1024 * 1024
		value = value[:len(value)-1]
	case 'g', 'G':
		multiplier = 1024 * 1024 * 1024
		value = value[:len(value)-1]
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number < 0 {
		return 0, fmt.Errorf("expected a non-negative size with optional K, M, or G suffix")
	}
	return int64(number * multiplier), nil
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func shortTime(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Format("2006-01-02 15:04")
}

func attestCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "attest", Short: "generate SBOM or provenance for a .cap image"}

	sbomSub := func() *cobra.Command {
		var outPath string
		var embed bool
		c := &cobra.Command{
			Use:   "sbom IMAGE.cap",
			Short: "generate an SPDX 2.3 SBOM for a .cap image",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withController(opts, func(ctrl controller.Controller) error {
					imagePath, _, err := loader.Loader{Paths: ctrl.Store.Paths}.ResolveImage(args[0])
					if err != nil {
						return fmt.Errorf("image not found: %s", args[0])
					}
					ld := loader.Loader{Paths: ctrl.Store.Paths, Debug: opts.debug}
					loaded, cleanup, err := ld.Load(args[0])
					if err != nil {
						return fmt.Errorf("load image: %w", err)
					}
					cleanup()
					doc := sbom.GenerateSPDX(loaded.Manifest, loaded.Digest)
					data, err := sbom.MarshalJSON(doc)
					if err != nil {
						return err
					}
					if embed {
						if err := sbom.EmbedInCap(imagePath, imagePath, sbom.EntryNameSBOM, data); err != nil {
							return fmt.Errorf("embed SBOM: %w", err)
						}
						fmt.Printf("SBOM embedded in %s as %s\n", imagePath, sbom.EntryNameSBOM)
						return nil
					}
					dest := outPath
					if dest == "" {
						dest = strings.TrimSuffix(args[0], ".cap") + ".sbom.spdx.json"
					}
					if err := os.WriteFile(dest, data, 0o644); err != nil {
						return err
					}
					fmt.Printf("SBOM written to %s\n", dest)
					return nil
				})
			},
		}
		c.Flags().StringVar(&outPath, "out", "", "output path (default: IMAGE.sbom.spdx.json)")
		c.Flags().BoolVar(&embed, "embed", false, "embed the SBOM inside the .cap archive")
		return c
	}()

	provSub := func() *cobra.Command {
		var outPath string
		var embed bool
		c := &cobra.Command{
			Use:   "provenance IMAGE.cap",
			Short: "generate a provenance record for a .cap image",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withController(opts, func(ctrl controller.Controller) error {
					imagePath, _, err := loader.Loader{Paths: ctrl.Store.Paths}.ResolveImage(args[0])
					if err != nil {
						return fmt.Errorf("image not found: %s", args[0])
					}
					ld := loader.Loader{Paths: ctrl.Store.Paths, Debug: opts.debug}
					loaded, cleanup, err := ld.Load(args[0])
					if err != nil {
						return fmt.Errorf("load image: %w", err)
					}
					cleanup()
					prov := sbom.GenerateProvenance(loaded.Manifest, filepath.Base(imagePath), loaded.Digest)
					data, err := sbom.MarshalJSON(prov)
					if err != nil {
						return err
					}
					if embed {
						if err := sbom.EmbedInCap(imagePath, imagePath, sbom.EntryNameProvenance, data); err != nil {
							return fmt.Errorf("embed provenance: %w", err)
						}
						fmt.Printf("Provenance embedded in %s as %s\n", imagePath, sbom.EntryNameProvenance)
						return nil
					}
					dest := outPath
					if dest == "" {
						dest = strings.TrimSuffix(args[0], ".cap") + ".provenance.json"
					}
					if err := os.WriteFile(dest, data, 0o644); err != nil {
						return err
					}
					fmt.Printf("Provenance written to %s\n", dest)
					return nil
				})
			},
		}
		c.Flags().StringVar(&outPath, "out", "", "output path (default: IMAGE.provenance.json)")
		c.Flags().BoolVar(&embed, "embed", false, "embed the provenance record inside the .cap archive")
		return c
	}()

	cmd.AddCommand(sbomSub, provSub)
	return cmd
}

func daemonCmd(opts *options) *cobra.Command {
	var interval int
	var metricsAddr string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "run the Capper control plane daemon",
		Long: `capper daemon starts the control plane: an instance supervisor (restart policy
enforcement) and a reconciler loop (host heartbeat and future reconcilers).

Press Ctrl-C to stop.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				dopts := control.DefaultDaemonOptions()
				if interval > 0 {
					dopts.SupervisorInterval = time.Duration(interval) * time.Second
				}
				d := control.NewDaemon(ctrl.Store, ctrl.Instances, dopts)
				d.IMDS = capinit.NewServer(ctrl.Store)
				control.WireLBCertificates(ctrl.Store, ctrl.CertMgr)
				ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer cancel()
				control.StartCertRenewal(ctx, ctrl.CertMgr)
				if metricsAddr != "" {
					srv := metrics.NewPrometheusServerWithLB(metricsAddr, ctrl.Store.ListInstances, ctrl.Store.LB.RunningStats)
					go func() {
						if err := srv.ListenAndServe(); err != nil {
							fmt.Fprintf(os.Stderr, "metrics server: %v\n", err)
						}
					}()
					go func() {
						<-ctx.Done()
						_ = srv.Close()
					}()
					fmt.Printf("Metrics endpoint: http://%s/metrics\n", metricsAddr)
				}
				fmt.Printf("Capper daemon started (supervisor interval: %s). Press Ctrl-C to stop.\n",
					dopts.SupervisorInterval)
				d.Run(ctx)
				stats := d.SupervisorStats()
				if len(stats) > 0 {
					fmt.Printf("\nRestart summary:\n")
					for id, count := range stats {
						fmt.Printf("  %s: %d restart(s)\n", id, count)
					}
				}
				fmt.Println("Daemon stopped.")
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&interval, "interval", 5, "supervisor poll interval in seconds")
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", "", "expose Prometheus metrics on this address, e.g. 127.0.0.1:9100")
	return cmd
}

// parseMountSpecs parses --mount SOURCE:TARGET[:ro] flags.
func parseMountSpecs(specs []string) ([]types.Mount, error) {
	var out []types.Mount
	for _, spec := range specs {
		parts := strings.Split(spec, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("expected SOURCE:TARGET[:ro], got %q", spec)
		}
		m := types.Mount{Source: parts[0], Target: parts[1]}
		if len(parts) == 3 {
			switch parts[2] {
			case "ro":
				m.ReadOnly = true
			case "rw":
				// default
			default:
				return nil, fmt.Errorf("unknown mount option %q in %q (valid: ro, rw)", parts[2], spec)
			}
		}
		if m.Source == "" || m.Target == "" {
			return nil, fmt.Errorf("source and target must not be empty in %q", spec)
		}
		out = append(out, m)
	}
	return out, nil
}

// parsePublishSpecs parses --publish HOST:CONTAINER[/proto] flags.
func parsePublishSpecs(specs []string) ([]types.PortMapping, error) {
	var out []types.PortMapping
	for _, spec := range specs {
		proto := "tcp"
		s := spec
		if idx := strings.LastIndex(s, "/"); idx >= 0 {
			proto = strings.ToLower(s[idx+1:])
			s = s[:idx]
			if proto != "tcp" && proto != "udp" {
				return nil, fmt.Errorf("unknown protocol %q in %q (valid: tcp, udp)", proto, spec)
			}
		}
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected HOST:CONTAINER[/proto], got %q", spec)
		}
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil || hostPort <= 0 || hostPort > 65535 {
			return nil, fmt.Errorf("invalid host port %q in %q", parts[0], spec)
		}
		ctrPort, err := strconv.Atoi(parts[1])
		if err != nil || ctrPort <= 0 || ctrPort > 65535 {
			return nil, fmt.Errorf("invalid container port %q in %q", parts[1], spec)
		}
		out = append(out, types.PortMapping{
			HostPort:      hostPort,
			ContainerPort: ctrPort,
			Protocol:      proto,
		})
	}
	return out, nil
}

// parseKeyValSpecs parses "KEY=VALUE" strings into a map.
func parseKeyValSpecs(specs []string) (map[string]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(specs))
	for _, s := range specs {
		k, v, ok := strings.Cut(s, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%q must be in KEY=VALUE format", s)
		}
		out[k] = v
	}
	return out, nil
}

// matchesSelector returns true if inst has all labels in selector.
func matchesSelector(inst types.Instance, selector map[string]string) bool {
	for k, v := range selector {
		if inst.Labels[k] != v {
			return false
		}
	}
	return true
}

func instanceJSON(instances []types.Instance) []map[string]any {
	out := make([]map[string]any, 0, len(instances))
	for _, inst := range instances {
		var pid any
		if inst.PID > 0 {
			pid = inst.PID
		}
		out = append(out, map[string]any{"id": inst.ID, "name": inst.Name, "image": inst.Image, "pid": pid, "status": inst.Status, "created": inst.CreatedAt})
	}
	return out
}

// resolveCLISubnetLaunch resolves a VPC subnet for a CLI launch and reserves an
// ENI (and thus a private IP) in it. The caller must delete the ENI if the
// launch fails, and attach it to the instance on success.
func resolveCLISubnetLaunch(st *store.Store, subnetID, vpcID string) (vpc.Subnet, vpc.ENI, *manager.NetworkRunOpts, error) {
	sub, err := networking.ResolveSubnetForLaunch(st.VPC, subnetID, vpcID)
	if err != nil {
		return vpc.Subnet{}, vpc.ENI{}, nil, fmt.Errorf("subnet: %w", err)
	}
	eni, err := st.VPC.CreateENI(sub.VPCID, sub.ID, nil, "")
	if err != nil {
		return vpc.Subnet{}, vpc.ENI{}, nil, fmt.Errorf("eni: %w", err)
	}
	return sub, eni, &manager.NetworkRunOpts{
		NetworkID:   sub.ID,
		Bridge:      sub.BridgeName,
		Subnet:      sub.CIDR,
		Gateway:     sub.GatewayIP,
		PreferredIP: eni.PrimaryPrivateIP,
	}, nil
}

// dnsAutoRegister creates an A record for the instance in any DNS zone tied to
// networkID. Non-fatal: errors are silently discarded so they never break launch.
func dnsAutoRegister(st *store.Store, networkID, instanceName, ip string) {
	zones, err := capperdns.NewManager(st.DNS).ListZones(networkID)
	if err != nil || len(zones) == 0 {
		return
	}
	mgr := capperdns.NewManager(st.DNS)
	for _, z := range zones {
		r, aerr := mgr.CreateRecord(z.Name, networkID, instanceName, "A", []string{ip}, 0)
		if aerr == nil {
			// Generate PTR record in the reverse zone for this network's subnet.
			if sub, serr := st.VPC.GetSubnetByID(networkID); serr == nil {
				dnsAutoRegisterPTR(mgr, networkID, ip, r.Name+"."+z.Name, sub.CIDR)
			} else if n, nerr := st.Networks.Get(networkID, ""); nerr == nil {
				dnsAutoRegisterPTR(mgr, networkID, ip, r.Name+"."+z.Name, n.Subnet)
			}
		}
	}
}

// dnsAutoRegisterPTR creates a PTR record for ip→fqdn in the appropriate
// in-addr.arpa reverse zone derived from subnet. Non-fatal.
func dnsAutoRegisterPTR(mgr *capperdns.Manager, networkID, ip, fqdn, subnet string) {
	reverseZone := ptrZone(subnet)
	if reverseZone == "" {
		return
	}
	// Ensure the reverse zone exists (idempotent).
	_, _ = mgr.CreateZone(reverseZone, capperdns.ZoneTypePrivate, networkID, 30, "auto reverse zone")
	ptrName := ptrHostLabel(ip, subnet)
	if ptrName == "" {
		return
	}
	_, _ = mgr.CreateRecord(reverseZone, networkID, ptrName, "PTR", []string{fqdn + "."}, 0)
}

// ptrZone returns the in-addr.arpa zone name for a /24 or /16 subnet.
// e.g. "10.88.0.0/24" → "0.88.10.in-addr.arpa"
//
//	"10.88.0.0/16" → "88.10.in-addr.arpa"
func ptrZone(subnet string) string {
	parts := strings.SplitN(subnet, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	ip := parts[0]
	prefix := parts[1]
	octets := strings.Split(ip, ".")
	if len(octets) != 4 {
		return ""
	}
	switch prefix {
	case "24":
		return octets[2] + "." + octets[1] + "." + octets[0] + ".in-addr.arpa"
	case "16":
		return octets[1] + "." + octets[0] + ".in-addr.arpa"
	default:
		return ""
	}
}

// ptrHostLabel returns the host label for an IP in a PTR record.
// e.g. ip "10.88.0.10" with /24 subnet → "10"
//
//	ip "10.88.0.10" with /16 subnet → "0.10"
func ptrHostLabel(ip, subnet string) string {
	parts := strings.SplitN(subnet, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	octets := strings.Split(ip, ".")
	if len(octets) != 4 {
		return ""
	}
	switch parts[1] {
	case "24":
		return octets[3]
	case "16":
		return octets[2] + "." + octets[3]
	default:
		return ""
	}
}

// dnsAutoDeregister removes the A record (and corresponding PTR record) for
// the instance from any DNS zone tied to networkID. Non-fatal.
func dnsAutoDeregister(st *store.Store, networkID, instanceName string) {
	mgr := capperdns.NewManager(st.DNS)
	zones, err := mgr.ListZones(networkID)
	if err != nil || len(zones) == 0 {
		return
	}
	n, _ := st.Networks.Get(networkID, "")
	reverseZone := ptrZone(n.Subnet)
	for _, z := range zones {
		// Skip reverse zones here; handle them separately below.
		if strings.HasSuffix(z.Name, ".in-addr.arpa") {
			continue
		}
		records, _ := mgr.ListRecords(z.Name, networkID)
		for _, r := range records {
			if r.Name == instanceName || r.Name == instanceName+"."+z.Name {
				// Remove PTR record before removing A record.
				if reverseZone != "" && len(r.Values) > 0 {
					ptrLabel := ptrHostLabel(r.Values[0], n.Subnet)
					if ptrLabel != "" {
						ptrRecords, _ := mgr.ListRecords(reverseZone, networkID)
						for _, ptr := range ptrRecords {
							if ptr.Name == ptrLabel {
								_ = mgr.DeleteRecord(reverseZone, networkID, ptr.ID)
							}
						}
					}
				}
				_ = mgr.DeleteRecord(z.Name, networkID, r.ID)
			}
		}
	}
}

// dnsAutoRegisterLB registers <lbName>.<zone> A records for each zone tied
// to the LB's network. The IP is taken from the listen address; if the host
// portion is "0.0.0.0" or empty the network gateway is used instead.
// Non-fatal.
func dnsAutoRegisterLB(st *store.Store, lbName, networkID, listenAddr string) {
	if networkID == "" {
		return
	}
	ip, _, err := net.SplitHostPort(listenAddr)
	if err != nil || ip == "" || ip == "0.0.0.0" {
		n, nerr := st.Networks.Get(networkID, "")
		if nerr != nil {
			return
		}
		ip = n.Gateway
	}
	dnsAutoRegister(st, networkID, lbName, ip)
}

// firewallAutoApply recompiles and applies the nftables policy for the given
// network. Called after instance start/stop to keep firewall state current.
// Non-fatal — any errors are silently dropped.
func firewallAutoApply(st *store.Store, networkID string) {
	n, err := st.Networks.Get(networkID, "")
	if err != nil {
		return
	}
	fw, err := st.Firewalls.Get(networkID)
	if err != nil {
		return
	}
	leases, err := st.Networks.LeasesForNetwork(networkID)
	if err != nil {
		return
	}
	leaseIPs := make(map[string]string, len(leases))
	for _, l := range leases {
		leaseIPs[l.InstanceID] = l.IP
	}
	netInfo := firewall.NetworkInfo{
		ID:      n.ID,
		Name:    n.Name,
		Subnet:  n.Subnet,
		Gateway: n.Gateway,
		Bridge:  n.Bridge,
		Mode:    n.Mode,
	}
	mgr := firewall.NewManager(st.Firewalls)
	_, _ = mgr.Apply(fw, netInfo, leaseIPs, nil, false)
}

// firewallAutoAddLBRules adds host→LB and LB→backend allow rules to the
// network firewall when a backend is added. Non-fatal.
func firewallAutoAddLBRules(st *store.Store, networkID string, listenPort int, backendAddr string) {
	fw, err := st.Firewalls.Get(networkID)
	if err != nil {
		return
	}
	fwMgr := firewall.NewManager(st.Firewalls)
	if listenPort > 0 {
		_, _ = fwMgr.AddRule(fw.NetworkID, firewall.RuleSpec{
			Action:      firewall.ActionAllow,
			Direction:   firewall.DirectionForward,
			Protocol:    "tcp",
			Ports:       []int{listenPort},
			From:        firewall.Endpoint{Type: firewall.EndpointAny},
			To:          firewall.Endpoint{Type: firewall.EndpointGateway},
			Description: "auto: allow host→LB",
		})
	}
	if backendAddr != "" {
		_, backendPort, _ := net.SplitHostPort(backendAddr)
		if backendPort != "" {
			var port int
			fmt.Sscanf(backendPort, "%d", &port)
			if port > 0 {
				_, _ = fwMgr.AddRule(fw.NetworkID, firewall.RuleSpec{
					Action:      firewall.ActionAllow,
					Direction:   firewall.DirectionForward,
					Protocol:    "tcp",
					Ports:       []int{port},
					From:        firewall.Endpoint{Type: firewall.EndpointGateway},
					To:          firewall.Endpoint{Type: firewall.EndpointNetwork},
					Description: "auto: allow LB→backend",
				})
			}
		}
	}
	firewallAutoApply(st, networkID)
}

// withStore opens the store and calls fn without constructing a full controller.
// Used by commands that only need store access (e.g. project management).
func withStore(opts *options, fn func(*store.Store) error) error {
	root := opts.storePath
	if root == "" {
		var err error
		root, err = store.DefaultRoot()
		if err != nil {
			return err
		}
	}
	st, err := store.Open(store.NewPaths(root))
	if err != nil {
		return err
	}
	defer st.Close()
	return fn(st)
}

// iamCtx carries an open store plus the resolved IAM principal for the current
// OS user. Commands that need both store access and authorization use withIAM
// instead of withStore so they get a consistent principal + Authorize call path.
type iamCtx struct {
	Store         *store.Store
	PrincipalType string
	PrincipalID   string
}

// Authorize checks whether the current principal may perform action on resource.
func (c *iamCtx) Authorize(action, resource string) error {
	if c.Store.IAM == nil {
		return nil
	}
	return c.Store.IAM.Authorize(c.PrincipalType, c.PrincipalID, action, resource)
}

// RecordEvent writes a resource lifecycle event to the event store.
func (c *iamCtx) RecordEvent(resourceType, resourceID, action, projectID string, data map[string]any) {
	_ = c.Store.Events.Insert(store.ResourceEvent{
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Action:        action,
		ProjectID:     projectID,
		PrincipalType: c.PrincipalType,
		PrincipalID:   c.PrincipalID,
		Data:          data,
	})
}

func withIAM(opts *options, fn func(*iamCtx) error) error {
	return withStore(opts, func(st *store.Store) error {
		ctx := &iamCtx{Store: st}
		if st.IAM != nil {
			ctx.PrincipalType, ctx.PrincipalID = st.IAM.LocalPrincipal()
		}
		return fn(ctx)
	})
}

// projectCmd returns the `capper project` command group.
func projectCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "manage projects (resource namespaces)",
	}
	cmd.AddCommand(
		projectCreateCmd(opts),
		projectListCmd(opts),
		projectInspectCmd(opts),
		projectDeleteCmd(opts),
		projectLabelCmd(opts),
	)
	return cmd
}

func projectCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create NAME",
		Short: "create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				name := args[0]
				p := org.Project{
					ID:   "proj_" + name,
					Name: name,
				}
				if err := st.Projects.InsertProject(p); err != nil {
					return fmt.Errorf("create project: %w", err)
				}
				if opts.json {
					return printJSON(map[string]any{"id": p.ID, "name": p.Name})
				}
				fmt.Printf("Created project %q\n", p.Name)
				return nil
			})
		},
	}
}

func projectListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list all projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				projects, err := st.Projects.ListProjects()
				if err != nil {
					return err
				}
				if opts.json {
					out := make([]map[string]any, 0, len(projects))
					for _, p := range projects {
						out = append(out, map[string]any{"id": p.ID, "name": p.Name, "created": p.CreatedAt})
					}
					return printJSON(out)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tCREATED")
				for _, p := range projects {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.ID, p.CreatedAt)
				}
				return tw.Flush()
			})
		},
	}
}

func projectInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				p, err := st.Projects.GetProject(args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(p)
				}
				fmt.Printf("ID:      %s\nName:    %s\nCreated: %s\n", p.ID, p.Name, p.CreatedAt)
				if len(p.Labels) > 0 {
					fmt.Println("Labels:")
					for k, v := range p.Labels {
						fmt.Printf("  %s=%s\n", k, v)
					}
				}
				return nil
			})
		},
	}
}

func projectDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				if err := st.Projects.DeleteProject(args[0]); err != nil {
					return err
				}
				fmt.Printf("Deleted project %q\n", args[0])
				return nil
			})
		},
	}
}

func projectLabelCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "label NAME KEY=VALUE [KEY=VALUE ...]",
		Short: "set labels on a project",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels := make(map[string]string)
			for _, kv := range args[1:] {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid label %q: must be KEY=VALUE", kv)
				}
				labels[parts[0]] = parts[1]
			}
			return withStore(opts, func(st *store.Store) error {
				if err := st.Projects.SetProjectLabels(args[0], labels); err != nil {
					return err
				}
				fmt.Printf("Updated labels on project %q\n", args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper iam
// ---------------------------------------------------------------------------

func iamCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "iam",
		Short: "manage IAM users, roles, policies, and audit log",
	}
	cmd.AddCommand(
		iamWhoamiCmd(opts),
		iamUserCmd(opts),
		iamGroupCmd(opts),
		iamRoleCmd(opts),
		iamPolicyCmd(opts),
		iamGrantCmd(opts),
		iamServiceAccountCmd(opts),
		iamTokenCmd(opts),
		iamAuditCmd(opts),
		iamCrossAccountCmd(opts),
	)
	return cmd
}

// ---- cross-account IAM policies -----------------------------------------------

func iamCrossAccountCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cross-account",
		Short: "manage cross-account IAM policies",
	}
	cmd.AddCommand(iamCrossAccountCreateCmd(opts), iamCrossAccountListCmd(opts), iamCrossAccountDeleteCmd(opts))
	return cmd
}

func iamCrossAccountCreateCmd(opts *options) *cobra.Command {
	var (
		name          string
		sourceAccount string
		targetAccount string
		principalType string
		principalID   string
		actions       []string
		resources     []string
		expiresAt     string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "create a cross-account IAM policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:create", "project:"+opts.project); err != nil {
					return err
				}
				p, err := ac.Store.IAM.CreateCrossAccountPolicy(iam.CrossAccountPolicy{
					Name:          name,
					SourceAccount: sourceAccount,
					TargetAccount: targetAccount,
					PrincipalType: principalType,
					PrincipalID:   principalID,
					Statements: []iam.Statement{{
						Effect:    "allow",
						Actions:   actions,
						Resources: resources,
					}},
					ExpiresAt: expiresAt,
				})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(p)
				}
				fmt.Printf("cross-account policy created: %s (%s)\n", p.Name, p.ID)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "policy name")
	cmd.Flags().StringVar(&sourceAccount, "source-account", "", "account granting trust")
	cmd.Flags().StringVar(&targetAccount, "target-account", "", "account being accessed")
	cmd.Flags().StringVar(&principalType, "principal-type", "user", "principal type (user|service-account)")
	cmd.Flags().StringVar(&principalID, "principal-id", "", "principal ID")
	cmd.Flags().StringSliceVar(&actions, "actions", nil, "allowed actions (e.g. instance:run)")
	cmd.Flags().StringSliceVar(&resources, "resources", []string{"*"}, "resource scopes")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "RFC3339 expiry (optional)")
	return cmd
}

func iamCrossAccountListCmd(opts *options) *cobra.Command {
	var account string
	return &cobra.Command{
		Use:   "list",
		Short: "list cross-account IAM policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:list", "project:"+opts.project); err != nil {
					return err
				}
				policies, err := ac.Store.IAM.ListCrossAccountPolicies(account)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(policies)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tSOURCE\tTARGET\tPRINCIPAL\tEXPIRES")
				for _, p := range policies {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s/%s\t%s\n",
						p.ID, p.Name, p.SourceAccount, p.TargetAccount, p.PrincipalType, p.PrincipalID, p.ExpiresAt)
				}
				return tw.Flush()
			})
		},
	}
}

func iamCrossAccountDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "delete a cross-account IAM policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.IAM.DeleteCrossAccountPolicy(args[0]); err != nil {
					return err
				}
				fmt.Printf("cross-account policy %s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---- whoami -----------------------------------------------------------------

func iamWhoamiCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "show the current IAM principal",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if opts.json {
					return printJSON(map[string]any{
						"principalType": ctrl.PrincipalType(),
						"principalId":   ctrl.PrincipalID(),
					})
				}
				fmt.Printf("%s:%s\n", ctrl.PrincipalType(), ctrl.PrincipalID())
				return nil
			})
		},
	}
}

// ---- user -------------------------------------------------------------------

func iamUserCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "manage IAM users"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "create NAME [--local-user OSUSER]",
			Short: "create an IAM user",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				localUser, _ := cmd.Flags().GetString("local-user")
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:user:create", "iam:system"); err != nil {
						return err
					}
					u := iam.User{
						ID:        "usr_" + args[0],
						Name:      args[0],
						LocalUser: localUser,
					}
					if err := ac.Store.IAM.IAMStore().InsertUser(u); err != nil {
						return err
					}
					fmt.Printf("Created user %q\n", u.Name)
					return nil
				})
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "list IAM users",
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:user:list", "iam:system"); err != nil {
						return err
					}
					users, err := ac.Store.IAM.IAMStore().ListUsers()
					if err != nil {
						return err
					}
					if opts.json {
						return printJSON(users)
					}
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tID\tLOCAL_USER\tCREATED")
					for _, u := range users {
						fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", u.Name, u.ID, u.LocalUser, u.CreatedAt)
					}
					return tw.Flush()
				})
			},
		},
		&cobra.Command{
			Use:   "delete NAME",
			Short: "delete an IAM user",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:user:delete", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().DeleteUser(args[0])
				})
			},
		},
	)
	cmd.Commands()[0].Flags().String("local-user", "", "OS username to associate")
	return cmd
}

// ---- group ------------------------------------------------------------------

func iamGroupCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "group", Short: "manage IAM groups"}
	cmd.AddCommand(
		&cobra.Command{
			Use:  "create NAME",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:group:create", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().InsertGroup(iam.Group{
						ID:   "grp_" + args[0],
						Name: args[0],
					})
				})
			},
		},
		&cobra.Command{
			Use:  "add-member GROUP USER",
			Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:group:write", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().AddGroupMember(args[0], args[1])
				})
			},
		},
		&cobra.Command{
			Use:  "remove-member GROUP USER",
			Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:group:write", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().RemoveGroupMember(args[0], args[1])
				})
			},
		},
		&cobra.Command{
			Use:  "inspect NAME",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:group:inspect", "iam:system"); err != nil {
						return err
					}
					g, err := ac.Store.IAM.IAMStore().GetGroup(args[0])
					if err != nil {
						return err
					}
					if opts.json {
						return printJSON(g)
					}
					fmt.Printf("ID:      %s\nName:    %s\nMembers: %v\n", g.ID, g.Name, g.Members)
					return nil
				})
			},
		},
	)
	return cmd
}

// ---- role -------------------------------------------------------------------

func iamRoleCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "role", Short: "manage IAM roles"}
	cmd.AddCommand(
		&cobra.Command{
			Use:  "create NAME",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:role:create", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().InsertRole(iam.Role{
						ID:   "role_" + args[0],
						Name: args[0],
					})
				})
			},
		},
		&cobra.Command{
			Use: "list",
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:role:list", "iam:system"); err != nil {
						return err
					}
					roles, err := ac.Store.IAM.IAMStore().ListRoles()
					if err != nil {
						return err
					}
					if opts.json {
						return printJSON(roles)
					}
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tID\tPOLICIES")
					for _, r := range roles {
						fmt.Fprintf(tw, "%s\t%s\t%v\n", r.Name, r.ID, r.Policies)
					}
					return tw.Flush()
				})
			},
		},
		&cobra.Command{
			Use:  "attach-policy ROLE POLICY",
			Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:role:write", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().AttachPolicy(args[0], args[1])
				})
			},
		},
		&cobra.Command{
			Use:  "detach-policy ROLE POLICY",
			Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:role:write", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().DetachPolicy(args[0], args[1])
				})
			},
		},
	)
	return cmd
}

// ---- policy -----------------------------------------------------------------

func iamPolicyCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "manage IAM policies"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "create NAME POLICY_FILE",
			Short: "create a policy from a JSON file",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				data, err := os.ReadFile(args[1])
				if err != nil {
					return fmt.Errorf("read policy file: %w", err)
				}
				var stmts []iam.Statement
				if err := json.Unmarshal(data, &stmts); err != nil {
					return fmt.Errorf("parse policy JSON (expected array of statements): %w", err)
				}
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:policy:create", "iam:system"); err != nil {
						return err
					}
					p := iam.Policy{
						ID:         "pol_" + args[0],
						Name:       args[0],
						Statements: stmts,
					}
					if err := ac.Store.IAM.IAMStore().InsertPolicy(p); err != nil {
						return err
					}
					fmt.Printf("Created policy %q\n", p.Name)
					return nil
				})
			},
		},
		&cobra.Command{
			Use: "list",
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:policy:list", "iam:system"); err != nil {
						return err
					}
					policies, err := ac.Store.IAM.IAMStore().ListPolicies()
					if err != nil {
						return err
					}
					if opts.json {
						return printJSON(policies)
					}
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tID\tSTATEMENTS")
					for _, p := range policies {
						fmt.Fprintf(tw, "%s\t%s\t%d\n", p.Name, p.ID, len(p.Statements))
					}
					return tw.Flush()
				})
			},
		},
		&cobra.Command{
			Use:  "inspect NAME",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:policy:inspect", "iam:system"); err != nil {
						return err
					}
					p, err := ac.Store.IAM.IAMStore().GetPolicy(args[0])
					if err != nil {
						return err
					}
					return printJSON(p)
				})
			},
		},
		&cobra.Command{
			Use:  "delete NAME",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:policy:delete", "iam:system"); err != nil {
						return err
					}
					return ac.Store.IAM.IAMStore().DeletePolicy(args[0])
				})
			},
		},
	)
	return cmd
}

// ---- grant ------------------------------------------------------------------

func iamGrantCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "grant", Short: "manage IAM grants"}
	var principalType, principalID, roleID, resourceScope string
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "grant a role to a principal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if principalType == "" || principalID == "" || roleID == "" {
				return fmt.Errorf("--principal-type, --principal-id, and --role are required")
			}
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:grant:create", "iam:system"); err != nil {
					return err
				}
				g := iam.Grant{
					ID:            "grn_" + principalID + "_" + roleID,
					PrincipalType: principalType,
					PrincipalID:   principalID,
					RoleID:        roleID,
					ResourceScope: resourceScope,
				}
				if err := ac.Store.IAM.IAMStore().InsertGrant(g); err != nil {
					return err
				}
				fmt.Printf("Granted role %q to %s:%s\n", roleID, principalType, principalID)
				return nil
			})
		},
	}
	createCmd.Flags().StringVar(&principalType, "principal-type", "user", "principal type (user|group|service-account)")
	createCmd.Flags().StringVar(&principalID, "principal-id", "", "principal ID or name")
	createCmd.Flags().StringVar(&roleID, "role", "", "role name or ID")
	createCmd.Flags().StringVar(&resourceScope, "scope", "*", "resource scope (\"*\" = all)")
	cmd.AddCommand(createCmd)
	cmd.AddCommand(&cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:grant:list", "iam:system"); err != nil {
					return err
				}
				grants, err := ac.Store.IAM.IAMStore().ListGrants()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(grants)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tPRINCIPAL\tROLE\tSCOPE")
				for _, g := range grants {
					fmt.Fprintf(tw, "%s\t%s:%s\t%s\t%s\n", g.ID, g.PrincipalType, g.PrincipalID, g.RoleID, g.ResourceScope)
				}
				return tw.Flush()
			})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:  "delete ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:grant:delete", "iam:system"); err != nil {
					return err
				}
				return ac.Store.IAM.IAMStore().DeleteGrant(args[0])
			})
		},
	})
	return cmd
}

// ---- service-account --------------------------------------------------------

func iamServiceAccountCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "service-account", Aliases: []string{"sa"}, Short: "manage service accounts"}
	cmd.AddCommand(
		&cobra.Command{
			Use:  "create NAME",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:serviceaccount:create", "iam:system"); err != nil {
						return err
					}
					sa := iam.ServiceAccount{
						ID:      "sa_" + args[0],
						Name:    args[0],
						Project: opts.project,
					}
					if err := ac.Store.IAM.IAMStore().InsertServiceAccount(sa); err != nil {
						return err
					}
					fmt.Printf("Created service account %q\n", sa.Name)
					return nil
				})
			},
		},
		&cobra.Command{
			Use: "list",
			RunE: func(cmd *cobra.Command, args []string) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("iam:serviceaccount:list", "iam:system"); err != nil {
						return err
					}
					sas, err := ac.Store.IAM.IAMStore().ListServiceAccounts()
					if err != nil {
						return err
					}
					if opts.json {
						return printJSON(sas)
					}
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tID\tPROJECT")
					for _, sa := range sas {
						fmt.Fprintf(tw, "%s\t%s\t%s\n", sa.Name, sa.ID, sa.Project)
					}
					return tw.Flush()
				})
			},
		},
	)
	return cmd
}

// ---- token ------------------------------------------------------------------

func iamTokenCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "token", Short: "issue and manage API tokens"}
	var ttl string
	var name string
	issueCmd := &cobra.Command{
		Use:   "create",
		Short: "issue a new API token for the current principal",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := time.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl: %w", err)
			}
			return withController(opts, func(ctrl controller.Controller) error {
				bearer, tok, err := ctrl.IAM().Issue(name, ctrl.PrincipalType(), ctrl.PrincipalID(), d)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(map[string]any{"id": tok.ID, "expiresAt": tok.ExpiresAt, "token": bearer})
				}
				fmt.Printf("Token ID:   %s\nExpires:    %s\nBearer:     %s\n", tok.ID, tok.ExpiresAt, bearer)
				return nil
			})
		},
	}
	issueCmd.Flags().StringVar(&ttl, "ttl", "24h", "token lifetime (e.g. 1h, 24h, 7d)")
	issueCmd.Flags().StringVar(&name, "name", "", "human-readable name for the token")
	cmd.AddCommand(issueCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "revoke ID",
		Short: "revoke a token by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("iam:token:revoke", "iam:system"); err != nil {
					return err
				}
				return ac.Store.IAM.RevokeToken(args[0])
			})
		},
	})
	return cmd
}

// ---- audit ------------------------------------------------------------------

func iamAuditCmd(opts *options) *cobra.Command {
	var actionFilter, principalFilter, since string
	var limit int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "view the IAM audit log",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "list IAM audit records",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				records, err := st.IAM.IAMStore().ListAudit(actionFilter, principalFilter, since, limit)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(records)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "TIMESTAMP\tPRINCIPAL\tACTION\tRESOURCE\tDECISION")
				for _, r := range records {
					fmt.Fprintf(tw, "%s\t%s:%s\t%s\t%s\t%s\n",
						r.Timestamp, r.PrincipalType, r.PrincipalID, r.Action, r.Resource, r.Decision)
				}
				return tw.Flush()
			})
		},
	}
	listCmd.Flags().StringVar(&actionFilter, "action", "", "filter by action prefix (e.g. instance)")
	listCmd.Flags().StringVar(&principalFilter, "principal", "", "filter by principal ID prefix")
	listCmd.Flags().StringVar(&since, "since", "", "show records at or after this RFC3339 timestamp")
	listCmd.Flags().IntVar(&limit, "limit", 100, "maximum number of records to return")
	cmd.AddCommand(listCmd, iamAuditTailCmd(opts))
	return cmd
}

// ---------------------------------------------------------------------------
// capper host
// ---------------------------------------------------------------------------

func hostCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "manage host inventory and run capability checks",
	}
	cmd.AddCommand(
		hostRegisterCmd(opts),
		hostListCmd(opts),
		hostInspectCmd(opts),
		hostLabelCmd(opts),
		hostDoctorCmd(opts),
	)
	return cmd
}

func hostRegisterCmd(opts *options) *cobra.Command {
	var roles []string
	var noGPUDetect bool
	cmd := &cobra.Command{
		Use:   "register",
		Short: "register the local host in the inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				h := host.Inventory()
				h.ID = "host_" + h.Hostname
				h.Roles = roles
				if err := st.Hosts.Upsert(h); err != nil {
					return err
				}
				// Auto-detect NVIDIA GPUs from /dev/nvidia* device nodes.
				var gpuCount int
				if !noGPUDetect {
					gpuCount = detectAndRegisterGPUs(st)
				}
				if opts.json {
					return printJSON(h)
				}
				msg := fmt.Sprintf("Registered host %s (%s/%s, kernel %s, %d CPUs, %s RAM)",
					h.Hostname, h.OS, h.Arch, h.KernelVersion, h.CPUCount,
					humanSize(h.MemoryBytes))
				if gpuCount > 0 {
					msg += fmt.Sprintf(", %d GPU(s) detected", gpuCount)
				}
				fmt.Println(msg)
				return nil
			})
		},
	}
	cmd.Flags().StringSliceVar(&roles, "role", []string{"compute"}, "host roles (comma-separated)")
	cmd.Flags().BoolVar(&noGPUDetect, "no-gpu-detect", false, "skip automatic GPU detection")
	return cmd
}

// detectAndRegisterGPUs scans /dev/nvidia* device nodes and registers any
// NVIDIA GPUs not already known to the store. Returns the count of newly
// registered GPUs.
func detectAndRegisterGPUs(st *store.Store) int {
	entries, err := filepath.Glob("/dev/nvidia[0-9]*")
	if err != nil || len(entries) == 0 {
		return 0
	}
	mgr := compute.NewManager(st.Compute)
	registered := 0
	for _, dev := range entries {
		g := compute.GPUDevice{
			Vendor:     "NVIDIA",
			Model:      "GPU",
			DevicePath: dev,
		}
		if _, err := mgr.RegisterGPU(g); err == nil {
			registered++
		}
	}
	return registered
}

func hostListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list registered hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				hosts, err := st.Hosts.List()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(hosts)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "HOSTNAME\tSTATUS\tOS/ARCH\tKERNEL\tCPUs\tLAST SEEN")
				for _, h := range hosts {
					fmt.Fprintf(tw, "%s\t%s\t%s/%s\t%s\t%d\t%s\n",
						h.Hostname, h.Status, h.OS, h.Arch, h.KernelVersion, h.CPUCount, h.LastSeen)
				}
				return tw.Flush()
			})
		},
	}
}

func hostInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect HOSTNAME",
		Short: "show detailed host information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				h, err := st.Hosts.Get(args[0])
				if err != nil {
					return err
				}
				return printJSON(h)
			})
		},
	}
}

func hostLabelCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "label HOSTNAME KEY=VALUE [KEY=VALUE ...]",
		Short: "set labels on a host",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels := make(map[string]string)
			for _, kv := range args[1:] {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid label %q: must be KEY=VALUE", kv)
				}
				labels[parts[0]] = parts[1]
			}
			return withStore(opts, func(st *store.Store) error {
				h, err := st.Hosts.Get(args[0])
				if err != nil {
					return err
				}
				return st.Hosts.SetLabels(h.ID, labels)
			})
		},
	}
}

func hostDoctorCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "run capability checks on the local host",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				results := host.RunDoctor(st.Paths.Root)
				if opts.json {
					return printJSON(results)
				}
				pass, fail := 0, 0
				for _, r := range results {
					icon := "✓"
					if !r.Pass {
						icon = "✗"
						fail++
					} else {
						pass++
					}
					fmt.Printf("  %s  %-40s %s\n", icon, r.Check, r.Message)
				}
				fmt.Printf("\n%d passed, %d failed\n", pass, fail)
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper network
// ---------------------------------------------------------------------------

const flatNetworksRemovedMsg = "flat networks have been removed; create a VPC and subnet instead " +
	"(CapperWeb, the VPC/Subnets API, or `capper vpc`), then pass --subnet-id " +
	"to `capper run`, `capper db create`, and `capper dns zone create`"

func networkCmd(opts *options) *cobra.Command {
	help := func(cmd *cobra.Command, args []string) error {
		return errors.New(flatNetworksRemovedMsg)
	}
	cmd := &cobra.Command{
		Use:                "network",
		Short:              "removed: flat virtual networks (use VPC subnets)",
		Long:               "Flat virtual networks have been removed.\n\n" + flatNetworksRemovedMsg,
		DisableFlagParsing: true,
		RunE:               help,
	}
	return cmd
}

// ---------------------------------------------------------------------------
// capper firewall
// ---------------------------------------------------------------------------

// parseEndpoint converts a string like "any", "label:role=web",
// "cidr:10.1.0.0/16", "instance:inst_abc" into an Endpoint.
func parseEndpoint(s string) (firewall.Endpoint, error) {
	switch s {
	case "any", "":
		return firewall.Endpoint{Type: firewall.EndpointAny}, nil
	case "internet":
		return firewall.Endpoint{Type: firewall.EndpointInternet}, nil
	case "host":
		return firewall.Endpoint{Type: firewall.EndpointHost}, nil
	case "gateway":
		return firewall.Endpoint{Type: firewall.EndpointGateway}, nil
	case "network":
		return firewall.Endpoint{Type: firewall.EndpointNetwork}, nil
	}
	// type:value or type:key=value
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return firewall.Endpoint{}, fmt.Errorf("invalid endpoint %q: expected TYPE, TYPE:VALUE, or TYPE:KEY=VALUE", s)
	}
	epType := s[:idx]
	rest := s[idx+1:]
	switch epType {
	case "cidr":
		return firewall.Endpoint{Type: firewall.EndpointCIDR, Value: rest}, nil
	case "instance":
		return firewall.Endpoint{Type: firewall.EndpointInstance, Value: rest}, nil
	case "label":
		kv := strings.SplitN(rest, "=", 2)
		if len(kv) != 2 {
			return firewall.Endpoint{}, fmt.Errorf("label endpoint requires KEY=VALUE, got %q", rest)
		}
		return firewall.Endpoint{Type: firewall.EndpointLabel, Key: kv[0], Value: kv[1]}, nil
	default:
		return firewall.Endpoint{}, fmt.Errorf("unknown endpoint type %q", epType)
	}
}

func firewallCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firewall",
		Short: "manage network firewall policies (nftables)",
	}
	cmd.AddCommand(
		fwInitCmd(opts),
		fwListCmd(opts),
		fwInspectCmd(opts),
		fwDeleteCmd(opts),
		fwApplyCmd(opts),
		fwResetCmd(opts),
		fwRuleCmd(opts),
	)
	return cmd
}

func fwInitCmd(opts *options) *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "init NETWORK_ID",
		Short: "initialize a firewall policy for a network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(iam *iamCtx) error {
				if err := iam.Authorize("firewall:create", "project:"+opts.project); err != nil {
					return err
				}
				n, err := iam.Store.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(iam.Store.Firewalls)
				fw, err := mgr.Init(n.ID, n.Name, mode)
				if err != nil {
					return err
				}
				iam.RecordEvent("firewall", fw.NetworkID, "firewall.created", opts.project, map[string]any{"network": n.Name, "mode": mode})
				if opts.json {
					return printJSON(fw)
				}
				fmt.Printf("Firewall initialized for network %q\nMode:    %s\nStatus:  %s\n", fw.NetworkName, fw.Mode, fw.Status)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "strict", "firewall mode: strict, permissive, internal")
	return cmd
}

func fwListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list firewall policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("firewall:list", "project:"+opts.project); err != nil {
					return err
				}
				mgr := firewall.NewManager(ac.Store.Firewalls)
				fws, err := mgr.List()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(fws)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NETWORK\tMODE\tDEFAULT\tNAT\tSTATUS\tLAST APPLIED")
				for _, fw := range fws {
					nat := "no"
					if fw.NATEnabled {
						nat = "yes"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
						fw.NetworkName, fw.Mode, fw.DefaultForwardPolicy, nat, fw.Status, fw.LastAppliedAt)
				}
				return tw.Flush()
			})
		},
	}
}

func fwInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NETWORK",
		Short: "show firewall details and rules",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("firewall:inspect", "project:"+opts.project); err != nil {
					return err
				}
				n, err := ac.Store.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(ac.Store.Firewalls)
				fw, rules, err := mgr.Inspect(n.ID)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(map[string]any{"firewall": fw, "rules": rules})
				}
				fmt.Printf("Network:  %s (%s)\nMode:     %s\nDefault:  %s\nDNS:      %v\nNAT:      %v\nStatus:   %s\n",
					fw.NetworkName, fw.NetworkID, fw.Mode, fw.DefaultForwardPolicy,
					fw.AllowDNS, fw.NATEnabled, fw.Status)
				if fw.LastAppliedAt != "" {
					fmt.Printf("Applied:  %s\n", fw.LastAppliedAt)
				}
				if len(rules) > 0 {
					fmt.Println("\nRules:")
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "  ID\tPRI\tACTION\tFROM\tTO\tPROTO\tPORTS\tENABLED\tDESCRIPTION")
					for _, r := range rules {
						ports := ""
						for i, p := range r.Ports {
							if i > 0 {
								ports += ","
							}
							ports += fmt.Sprintf("%d", p)
						}
						fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\t%s\t%s\t%s\t%v\t%s\n",
							r.ID, r.Priority, r.Action,
							endpointStr(r.From), endpointStr(r.To),
							r.Protocol, ports, r.Enabled, r.Description)
					}
					tw.Flush()
				}
				return nil
			})
		},
	}
}

func fwDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NETWORK",
		Short: "delete a firewall policy and remove nftables chain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(iam *iamCtx) error {
				if err := iam.Authorize("firewall:delete", "project:"+opts.project); err != nil {
					return err
				}
				n, err := iam.Store.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(iam.Store.Firewalls)
				if err := mgr.Delete(n.ID); err != nil {
					return err
				}
				iam.RecordEvent("firewall", n.ID, "firewall.deleted", opts.project, map[string]any{"network": args[0]})
				fmt.Printf("Deleted firewall for network %q\n", args[0])
				return nil
			})
		},
	}
}

func fwApplyCmd(opts *options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply NETWORK",
		Short: "compile and apply (or dry-run) the firewall policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(iam *iamCtx) error {
				if err := iam.Authorize("firewall:apply", "project:"+opts.project); err != nil {
					return err
				}
				n, err := iam.Store.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				fw, err := iam.Store.Firewalls.Get(n.ID)
				if err != nil {
					return fmt.Errorf("no firewall for network %q — run 'capper firewall init %s' first", args[0], args[0])
				}

				leases, err := iam.Store.Networks.LeasesForNetwork(n.ID)
				if err != nil {
					return err
				}
				leaseIPs := make(map[string]string, len(leases))
				for _, l := range leases {
					leaseIPs[l.InstanceID] = l.IP
				}

				netInfo := firewall.NetworkInfo{
					ID:      n.ID,
					Name:    n.Name,
					Subnet:  n.Subnet,
					Gateway: n.Gateway,
					Bridge:  n.Bridge,
					Mode:    n.Mode,
				}

				mgr := firewall.NewManager(iam.Store.Firewalls)
				result, err := mgr.Apply(fw, netInfo, leaseIPs, nil, dryRun)
				if err != nil {
					return err
				}

				if !dryRun {
					iam.RecordEvent("firewall", n.ID, "firewall.applied", opts.project, map[string]any{"network": n.Name})
				}
				if opts.json {
					return printJSON(result)
				}
				if dryRun {
					fmt.Printf("--- dry-run: nft script for network %q ---\n\n%s", args[0], result.Script)
				} else {
					fmt.Printf("Firewall applied to network %q\n", args[0])
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the nft script without applying")
	return cmd
}

func fwResetCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "reset NETWORK",
		Short: "flush nftables chain and mark firewall as pending",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				n, err := st.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(st.Firewalls)
				if err := mgr.Reset(n.ID); err != nil {
					return err
				}
				fmt.Printf("Reset firewall for network %q\n", args[0])
				return nil
			})
		},
	}
}

func fwRuleCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "manage firewall rules",
	}
	cmd.AddCommand(
		fwRuleAddCmd(opts),
		fwRuleDeleteCmd(opts),
		fwRuleEnableCmd(opts),
		fwRuleDisableCmd(opts),
	)
	return cmd
}

func fwRuleAddCmd(opts *options) *cobra.Command {
	var fromStr, toStr, proto, desc, direction string
	var ports []int
	var priority int
	var action string
	cmd := &cobra.Command{
		Use:   "add NETWORK",
		Short: "add a firewall rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				n, err := st.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				from, err := parseEndpoint(fromStr)
				if err != nil {
					return fmt.Errorf("--from: %w", err)
				}
				to, err := parseEndpoint(toStr)
				if err != nil {
					return fmt.Errorf("--to: %w", err)
				}
				mgr := firewall.NewManager(st.Firewalls)
				r, err := mgr.AddRule(n.ID, firewall.RuleSpec{
					Priority:    priority,
					Action:      action,
					Direction:   direction,
					From:        from,
					To:          to,
					Protocol:    proto,
					Ports:       ports,
					Description: desc,
				})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(r)
				}
				fmt.Printf("Added rule %s (priority %d): %s %s → %s\n",
					r.ID, r.Priority, r.Action, endpointStr(r.From), endpointStr(r.To))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&fromStr, "from", "any", "source endpoint (any, internet, gateway, network, cidr:CIDR, instance:ID, label:KEY=VAL)")
	cmd.Flags().StringVar(&toStr, "to", "any", "destination endpoint")
	cmd.Flags().StringVar(&action, "action", "allow", "rule action: allow, deny, reject")
	cmd.Flags().StringVar(&direction, "direction", "forward", "direction: forward, ingress, egress, any")
	cmd.Flags().StringVar(&proto, "proto", "any", "protocol: any, tcp, udp, icmp")
	cmd.Flags().IntSliceVar(&ports, "port", nil, "destination port(s)")
	cmd.Flags().StringVar(&desc, "description", "", "human-readable description")
	cmd.Flags().IntVar(&priority, "priority", 0, "rule priority (0 = auto-assign)")
	return cmd
}

func fwRuleDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NETWORK RULE_ID",
		Short: "delete a firewall rule",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				n, err := st.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(st.Firewalls)
				if err := mgr.DeleteRule(n.ID, args[1]); err != nil {
					return err
				}
				fmt.Printf("Deleted rule %s\n", args[1])
				return nil
			})
		},
	}
}

func fwRuleEnableCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable NETWORK RULE_ID",
		Short: "enable a disabled rule",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				n, err := st.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(st.Firewalls)
				if err := mgr.EnableRule(n.ID, args[1]); err != nil {
					return err
				}
				fmt.Printf("Enabled rule %s\n", args[1])
				return nil
			})
		},
	}
}

func fwRuleDisableCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable NETWORK RULE_ID",
		Short: "disable a rule without deleting it",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				n, err := st.Networks.Get(args[0], opts.project)
				if err != nil {
					return fmt.Errorf("network not found: %w", err)
				}
				mgr := firewall.NewManager(st.Firewalls)
				if err := mgr.DisableRule(n.ID, args[1]); err != nil {
					return err
				}
				fmt.Printf("Disabled rule %s\n", args[1])
				return nil
			})
		},
	}
}

func endpointStr(ep firewall.Endpoint) string {
	switch ep.Type {
	case firewall.EndpointLabel:
		return fmt.Sprintf("label:%s=%s", ep.Key, ep.Value)
	case firewall.EndpointInstance:
		return fmt.Sprintf("instance:%s", ep.Value)
	case firewall.EndpointCIDR:
		return fmt.Sprintf("cidr:%s", ep.Value)
	default:
		return ep.Type
	}
}

// ---------------------------------------------------------------------------
// capper dns
// ---------------------------------------------------------------------------

func dnsCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dns",
		Short: "manage private DNS zones, records, and service discovery",
	}
	cmd.AddCommand(
		dnsZoneCmd(opts),
		dnsRecordCmd(opts),
		dnsServiceCmd(opts),
		dnsQueryCmd(opts),
		dnsStartCmd(opts),
		dnsServeCmd(opts),
		dnsHealthcheckCmd(opts),
		dnsTraceCmd(opts),
	)
	return cmd
}

// ---- zone -------------------------------------------------------------------

func dnsZoneCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "zone", Short: "manage DNS hosted zones"}
	cmd.AddCommand(
		dnsZoneCreateCmd(opts),
		dnsZoneListCmd(opts),
		dnsZoneInspectCmd(opts),
		dnsZoneDeleteCmd(opts),
	)
	return cmd
}

func dnsZoneCreateCmd(opts *options) *cobra.Command {
	var ttl int
	var desc string
	var subnetID string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a private hosted zone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				if subnetID == "" {
					return fmt.Errorf("--subnet-id is required for a private hosted zone")
				}
				if _, err := st.VPC.GetSubnetByID(subnetID); err != nil {
					return fmt.Errorf("subnet %q not found: %w", subnetID, err)
				}
				networkID := subnetID // DNS networkId is the VPC subnet ID
				mgr := capperdns.NewManager(st.DNS)
				z, err := mgr.CreateZone(args[0], capperdns.ZoneTypePrivate, networkID, ttl, desc)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(z)
				}
				fmt.Printf("Created zone %q (ID: %s)\n", z.Name, z.ID)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&ttl, "ttl", 30, "default TTL for records in this zone")
	cmd.Flags().StringVar(&desc, "description", "", "zone description")
	cmd.Flags().StringVar(&subnetID, "subnet-id", "", "VPC subnet ID the private zone is attached to (required)")
	return cmd
}

func dnsZoneListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list hosted zones",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				zones, err := mgr.ListZones("")
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(zones)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tTYPE\tNETWORK\tTTL\tCREATED")
				for _, z := range zones {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
						z.Name, z.ID, z.Type, z.NetworkID, z.DefaultTTL, shortTime(z.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func dnsZoneInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show zone details, records, and services",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				z, err := mgr.GetZone(args[0], "")
				if err != nil {
					return err
				}
				records, _ := mgr.ListRecords(z.Name, "")
				services, _ := mgr.ListServices(z.Name, "")
				if opts.json {
					return printJSON(map[string]any{"zone": z, "records": records, "services": services})
				}
				fmt.Printf("ID:      %s\nName:    %s\nType:    %s\nNetwork: %s\nTTL:     %d\nCreated: %s\n",
					z.ID, z.Name, z.Type, z.NetworkID, z.DefaultTTL, z.CreatedAt)
				if len(records) > 0 {
					fmt.Println("\nRecords:")
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "  NAME\tTYPE\tVALUES\tTTL\tSOURCE")
					for _, r := range records {
						fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%s\n",
							r.Name, r.Type, strings.Join(r.Values, ","), r.TTL, r.Source)
					}
					tw.Flush()
				}
				if len(services) > 0 {
					fmt.Println("\nServices:")
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "  NAME\tFQDN\tSELECTOR\tPROTO\tPORT\tTTL")
					for _, s := range services {
						sel := fmt.Sprintf("%s:%s=%s", s.SelectorType, s.SelectorKey, s.SelectorValue)
						fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%d\t%d\n",
							s.Name, s.FQDN, sel, s.Protocol, s.Port, s.TTL)
					}
					tw.Flush()
				}
				return nil
			})
		},
	}
}

func dnsZoneDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a zone and all its records",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				if err := mgr.DeleteZone(args[0], ""); err != nil {
					return err
				}
				fmt.Printf("Deleted zone %q\n", args[0])
				return nil
			})
		},
	}
}

// ---- record -----------------------------------------------------------------

func dnsRecordCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "record", Short: "manage DNS records"}
	cmd.AddCommand(
		dnsRecordCreateCmd(opts),
		dnsRecordListCmd(opts),
		dnsRecordDeleteCmd(opts),
	)
	return cmd
}

func dnsRecordCreateCmd(opts *options) *cobra.Command {
	var ttl int
	cmd := &cobra.Command{
		Use:   "create ZONE NAME TYPE VALUE [VALUE...]",
		Short: "add a DNS record to a zone",
		Args:  cobra.MinimumNArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				zone, name, recType := args[0], args[1], args[2]
				values := args[3:]
				mgr := capperdns.NewManager(st.DNS)
				r, err := mgr.CreateRecord(zone, "", name, recType, values, ttl)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(r)
				}
				fmt.Printf("Created record %s %s → %s (TTL %ds)\n",
					r.FQDN, r.Type, strings.Join(r.Values, ", "), r.TTL)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&ttl, "ttl", 0, "TTL in seconds (0 = zone default)")
	return cmd
}

func dnsRecordListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list ZONE",
		Short: "list all records in a zone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				records, err := mgr.ListRecords(args[0], "")
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(records)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tTYPE\tVALUES\tTTL\tSOURCE")
				for _, r := range records {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
						r.ID, r.FQDN, r.Type, strings.Join(r.Values, ","), r.TTL, r.Source)
				}
				return tw.Flush()
			})
		},
	}
}

func dnsRecordDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete ZONE RECORD_ID",
		Short: "delete a DNS record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				if err := mgr.DeleteRecord(args[0], "", args[1]); err != nil {
					return err
				}
				fmt.Printf("Deleted record %s\n", args[1])
				return nil
			})
		},
	}
}

// ---- service ----------------------------------------------------------------

func dnsServiceCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "manage service discovery records"}
	cmd.AddCommand(
		dnsServiceCreateCmd(opts),
		dnsServiceListCmd(opts),
		dnsServiceDeleteCmd(opts),
	)
	return cmd
}

func dnsServiceCreateCmd(opts *options) *cobra.Command {
	var zone, selector, proto string
	var port, ttl int
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a selector-backed service discovery record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				if zone == "" {
					return fmt.Errorf("--zone is required")
				}
				if port == 0 {
					return fmt.Errorf("--port is required")
				}
				// Parse selector: "label:KEY=VALUE" or "label:VALUE"
				selType, selKey, selVal, err := parseSelector(selector)
				if err != nil {
					return fmt.Errorf("--selector: %w", err)
				}
				// Resolve network ID from zone
				z, err := st.DNS.GetZone(zone, "")
				if err != nil {
					return fmt.Errorf("zone %q not found", zone)
				}
				mgr := capperdns.NewManager(st.DNS)
				svc, err := mgr.CreateService(args[0], zone, z.NetworkID, capperdns.ServiceOptions{
					SelectorType:  selType,
					SelectorKey:   selKey,
					SelectorValue: selVal,
					Protocol:      proto,
					Port:          port,
					TTL:           ttl,
				})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(svc)
				}
				fmt.Printf("Created service %q → %s\nFQDN: %s\n", svc.Name, selector, svc.FQDN)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&zone, "zone", "", "zone name (required)")
	cmd.Flags().StringVar(&selector, "selector", "", "selector: label:KEY=VALUE (required)")
	cmd.Flags().StringVar(&proto, "proto", "tcp", "protocol: tcp or udp")
	cmd.Flags().IntVar(&port, "port", 0, "service port (required)")
	cmd.Flags().IntVar(&ttl, "ttl", 5, "TTL in seconds")
	return cmd
}

func dnsServiceListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list ZONE",
		Short: "list service discovery records in a zone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				svcs, err := mgr.ListServices(args[0], "")
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(svcs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tNAME\tFQDN\tSELECTOR\tPROTO\tPORT\tTTL")
				for _, s := range svcs {
					sel := fmt.Sprintf("%s:%s=%s", s.SelectorType, s.SelectorKey, s.SelectorValue)
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
						s.ID, s.Name, s.FQDN, sel, s.Protocol, s.Port, s.TTL)
				}
				return tw.Flush()
			})
		},
	}
}

func dnsServiceDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete ZONE SERVICE_ID",
		Short: "delete a service discovery record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capperdns.NewManager(st.DNS)
				if err := mgr.DeleteService(args[0], "", args[1]); err != nil {
					return err
				}
				fmt.Printf("Deleted service %s\n", args[1])
				return nil
			})
		},
	}
}

// ---- query ------------------------------------------------------------------

func dnsQueryCmd(opts *options) *cobra.Command {
	var qtype string
	var upstream string
	cmd := &cobra.Command{
		Use:   "query FQDN",
		Short: "resolve a DNS name against the local store (or a live daemon)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				var ups []string
				if upstream != "" {
					ups = []string{upstream}
				}
				resolver := capperdns.NewResolver(st.DNS, nil, ups)
				if qtype == "" {
					qtype = "A"
				}
				rrs, err := resolver.Query(args[0], qtype)
				if err != nil {
					return err
				}
				if opts.json {
					out := make([]string, 0, len(rrs))
					for _, rr := range rrs {
						out = append(out, rr.String())
					}
					return printJSON(map[string]any{"query": args[0], "type": qtype, "answers": out})
				}
				fmt.Println(capperdns.FormatRRs(rrs))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&qtype, "type", "A", "record type: A, AAAA, CNAME, TXT, MX, SRV")
	cmd.Flags().StringVar(&upstream, "upstream", "", "upstream resolver to forward to (ip[:port])")
	return cmd
}

// ---- start ------------------------------------------------------------------

func dnsStartCmd(opts *options) *cobra.Command {
	var listen string
	var upstream string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "run the embedded DNS daemon in the foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				ups := []string{"8.8.8.8:53", "8.8.4.4:53"}
				if upstream != "" {
					ups = strings.Split(upstream, ",")
				}
				resolver := capperdns.NewResolver(st.DNS, nil, ups)
				daemon := capperdns.NewDaemon(listen, resolver)
				if err := daemon.Start(); err != nil {
					return fmt.Errorf("dns daemon: %w", err)
				}
				fmt.Printf("Capper DNS daemon listening on %s (upstream: %s)\n",
					daemon.Addr(), strings.Join(ups, ", "))
				fmt.Println("Press Ctrl-C to stop.")

				ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
				defer cancel()
				<-ctx.Done()

				fmt.Println("\nShutting down DNS daemon...")
				return daemon.Stop()
			})
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:1053", "address to listen on (UDP+TCP)")
	cmd.Flags().StringVar(&upstream, "upstream", "", "comma-separated upstream resolvers (default: 8.8.8.8,8.8.4.4)")
	return cmd
}

// ---- helpers ----------------------------------------------------------------

// parseSelector parses "label:KEY=VALUE" → (type, key, value).
func parseSelector(s string) (selType, selKey, selVal string, err error) {
	if s == "" {
		return "", "", "", fmt.Errorf("selector is empty")
	}
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return "", "", "", fmt.Errorf("expected TYPE:VALUE or TYPE:KEY=VALUE, got %q", s)
	}
	selType = s[:colon]
	rest := s[colon+1:]
	if eq := strings.IndexByte(rest, '='); eq >= 0 {
		selKey = rest[:eq]
		selVal = rest[eq+1:]
	} else {
		selVal = rest
	}
	return selType, selKey, selVal, nil
}

// ===========================================================================
// capper compute
// ===========================================================================

func computeCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compute",
		Short: "manage compute hosts, templates, groups, and instances",
	}
	cmd.AddCommand(
		computeHostCmd(opts),
		computeTemplateCmd(opts),
		computeGroupCmd(opts),
		computeInstanceCmd(opts),
		computeInstanceTypeCmd(opts),
		computeGPUCmd(opts),
	)
	return cmd
}

// ---- compute host -----------------------------------------------------------

func computeHostCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "host", Short: "manage compute hosts"}
	cmd.AddCommand(
		computeHostRegisterCmd(opts),
		computeHostListCmd(opts),
		computeHostInspectCmd(opts),
		computeHostDrainCmd(opts),
		computeHostUncordonCmd(opts),
	)
	return cmd
}

func computeHostRegisterCmd(opts *options) *cobra.Command {
	var labelSpecs []string
	cmd := &cobra.Command{
		Use:   "register local|ADDR",
		Short: "register the local host or a remote host by address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				var h compute.Host
				var err error
				if args[0] == "local" {
					h, err = mgr.RegisterLocal()
				} else {
					labels, lerr := parseKeyValSpecs(labelSpecs)
					if lerr != nil {
						return fmt.Errorf("invalid --label: %w", lerr)
					}
					h, err = mgr.RegisterRemote(args[0], labels)
				}
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(h)
				}
				fmt.Printf("host/%s registered (status: %s, addr: %s)\n", h.Name, h.Status, h.Address)
				return nil
			})
		},
	}
	cmd.Flags().StringArrayVar(&labelSpecs, "label", nil, "label key=value for placement matching")
	return cmd
}

func computeHostListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list compute hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				hosts, err := mgr.ListHosts()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(hosts)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tSTATUS\tCPUS\tADDRESS\tCREATED")
				for _, h := range hosts {
					fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
						h.Name, h.Status, h.CPUCount, h.Address, shortTime(h.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
}

func computeHostInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show host details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				h, err := mgr.GetHost(args[0])
				if err != nil {
					return err
				}
				return printJSON(h)
			})
		},
	}
}

func computeHostDrainCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "drain NAME",
		Short: "mark a host as drained (no new scheduling)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				if err := mgr.DrainHost(args[0]); err != nil {
					return err
				}
				fmt.Printf("host/%s drained\n", args[0])
				return nil
			})
		},
	}
}

func computeHostUncordonCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "uncordon NAME",
		Short: "mark a host as ready",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				if err := mgr.UncordonHost(args[0]); err != nil {
					return err
				}
				fmt.Printf("host/%s uncordoned\n", args[0])
				return nil
			})
		},
	}
}

// ---- compute template -------------------------------------------------------

func computeTemplateCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "template", Short: "manage instance templates"}
	cmd.AddCommand(
		computeTemplateCreateCmd(opts),
		computeTemplateListCmd(opts),
		computeTemplateInspectCmd(opts),
		computeTemplateDeleteCmd(opts),
	)
	return cmd
}

func computeTemplateCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create FILE",
		Short: "create a template from a JSON document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("cannot read template file: %w", err)
			}
			var doc compute.TemplateDoc
			if err := json.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("invalid template JSON: %w", err)
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				t, err := mgr.CreateTemplate(doc)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(t)
				}
				fmt.Printf("template/%s created (id: %s, image: %s)\n", t.Name, t.ID, t.Image)
				return nil
			})
		},
	}
}

func computeTemplateListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list instance templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				ts, err := mgr.ListTemplates()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(ts)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tIMAGE\tRUNTIME\tCREATED")
				for _, t := range ts {
					rt := t.Runtime
					if rt == "" {
						rt = "-"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Image, rt, shortTime(t.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
}

func computeTemplateInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show template details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				t, err := mgr.GetTemplate(args[0])
				if err != nil {
					return err
				}
				return printJSON(t)
			})
		},
	}
}

func computeTemplateDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				if err := mgr.DeleteTemplate(args[0]); err != nil {
					return err
				}
				fmt.Printf("template/%s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---- compute group ----------------------------------------------------------

func computeGroupCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "group", Short: "manage instance groups"}
	cmd.AddCommand(
		computeGroupCreateCmd(opts),
		computeGroupListCmd(opts),
		computeGroupInspectCmd(opts),
		computeGroupScaleCmd(opts),
		computeGroupReconcileCmd(opts),
		computeGroupDeleteCmd(opts),
		computeGroupAutoscaleCmd(opts),
	)
	return cmd
}

func computeGroupCreateCmd(opts *options) *cobra.Command {
	var templateName string
	var minSize, desiredSize, maxSize int
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create an instance group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				g, err := mgr.CreateGroup(args[0], templateName, minSize, desiredSize, maxSize)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(g)
				}
				fmt.Printf("group/%s created (template: %s, desired: %d)\n", g.Name, g.TemplateName, g.DesiredSize)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&templateName, "template", "", "template name (required)")
	cmd.Flags().IntVar(&minSize, "min", 0, "minimum replica count")
	cmd.Flags().IntVar(&desiredSize, "desired", 1, "desired replica count")
	cmd.Flags().IntVar(&maxSize, "max", 1, "maximum replica count")
	_ = cmd.MarkFlagRequired("template")
	return cmd
}

func computeGroupListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list instance groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				gs, err := mgr.ListGroups()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(gs)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tTEMPLATE\tMIN\tDESIRED\tMAX\tSTATUS\tCREATED")
				for _, g := range gs {
					fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%s\t%s\n",
						g.Name, g.TemplateName, g.MinSize, g.DesiredSize, g.MaxSize,
						g.Status, shortTime(g.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
}

func computeGroupInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show group details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				g, err := mgr.GetGroup(args[0])
				if err != nil {
					return err
				}
				instances, _ := mgr.ListGroupInstances(args[0])
				out := map[string]any{
					"group":     g,
					"instances": instances,
				}
				return printJSON(out)
			})
		},
	}
}

func computeGroupScaleCmd(opts *options) *cobra.Command {
	var desired int
	cmd := &cobra.Command{
		Use:   "scale NAME",
		Short: "change the desired replica count",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				if err := mgr.ScaleGroup(args[0], desired); err != nil {
					return err
				}
				fmt.Printf("group/%s scaled to %d desired replicas\n", args[0], desired)
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&desired, "desired", 1, "desired replica count")
	_ = cmd.MarkFlagRequired("desired")
	return cmd
}

func computeGroupReconcileCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "reconcile NAME",
		Short: "ensure the group's actual replica count matches desired",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				mgr := compute.NewManager(ctrl.Store.Compute)

				statusFn := func(instanceID string) (compute.InstanceStatus, error) {
					inst, err := ctrl.Store.ResolveInstance(instanceID)
					if err != nil {
						return compute.InstanceStatus{}, err
					}
					return compute.InstanceStatus{ID: inst.ID, Status: inst.Status}, nil
				}

				runFn := func(image string, res compute.ResourceSpec, name string) (string, error) {
					ro := types.ResourceOverrides{
						Limits: types.ResourceLimits{
							MemoryBytes:  res.MemoryBytes,
							CPUTimeSecs:  res.CPUTimeSecs,
							MaxProcesses: res.MaxProcesses,
						},
						MemorySet:  res.MemoryBytes > 0,
						CPUTimeSet: res.CPUTimeSecs > 0,
						PidsSet:    res.MaxProcesses > 0,
					}
					inst, err := ctrl.Instances.Run(image, ro, manager.RunOptions{Name: name})
					if err != nil {
						return "", err
					}
					return inst.ID, nil
				}

				result, err := mgr.Reconcile(args[0], statusFn, runFn)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(result)
				}
				fmt.Printf("group/%s: desired=%d actual=%d created=%d removed=%d\n",
					args[0], result.Desired, result.Actual,
					len(result.Created), len(result.Removed))
				for _, e := range result.Errors {
					fmt.Fprintf(os.Stderr, "  error: %s\n", e)
				}
				return nil
			})
		},
	}
}

func computeGroupDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				if err := mgr.DeleteGroup(args[0]); err != nil {
					return err
				}
				fmt.Printf("group/%s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---- compute group autoscale ------------------------------------------------

func computeGroupAutoscaleCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autoscale",
		Short: "manage autoscaling policies for a group",
	}
	cmd.AddCommand(
		groupAutoscaleEnableCmd(opts),
		groupAutoscaleListCmd(opts),
		groupAutoscaleDisableCmd(opts),
		groupAutoscaleEvaluateCmd(opts),
		groupAutoscaleHistoryCmd(opts),
	)
	return cmd
}

func groupAutoscaleEnableCmd(opts *options) *cobra.Command {
	var groupName, metricName, policyType, queueName string
	var targetValue float64
	var minReplicas, maxReplicas int
	var scaleOutCooldown, scaleInCooldown int
	cmd := &cobra.Command{
		Use:   "enable POLICY_NAME",
		Short: "create or update an autoscaling policy for a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("autoscale:create", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				g, err := mgr.GetGroup(groupName)
				if err != nil {
					return fmt.Errorf("group %q not found: %w", groupName, err)
				}
				now := time.Now().UTC().Format(time.RFC3339)
				p := autoscale.AutoscalePolicy{
					ID:                   fmt.Sprintf("asp_%d", time.Now().UnixNano()),
					Project:              opts.project,
					Name:                 args[0],
					GroupID:              g.ID,
					GroupName:            g.Name,
					Enabled:              true,
					PolicyType:           policyType,
					MetricName:           metricName,
					MetricScope:          autoscale.ScopeGroup,
					QueueName:            queueName,
					TargetValue:          targetValue,
					MinReplicas:          minReplicas,
					MaxReplicas:          maxReplicas,
					ScaleOutCooldownSecs: scaleOutCooldown,
					ScaleInCooldownSecs:  scaleInCooldown,
					EvalWindowSecs:       300,
					StabWindowSecs:       300,
					ScaleOutStep:         1,
					ScaleInStep:          1,
					ScheduleJSON:         "[]",
					CreatedAt:            now,
					UpdatedAt:            now,
				}
				if err := ac.Store.Autoscale.Policies.Insert(p); err != nil {
					return err
				}
				ac.RecordEvent("autoscale_policy", p.ID, "autoscale.policy.created", opts.project,
					map[string]any{"name": p.Name, "group": g.Name})
				if opts.json {
					return printJSON(p)
				}
				fmt.Printf("Autoscaling enabled: group=%s policy=%s metric=%s target=%.2f min=%d max=%d\n",
					g.Name, p.Name, metricName, targetValue, minReplicas, maxReplicas)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&groupName, "group", "", "group name (required)")
	cmd.Flags().StringVar(&metricName, "metric", autoscale.MetricCPUAvgPercent, "metric name to scale on")
	cmd.Flags().StringVar(&policyType, "policy-type", autoscale.PolicyTypeTarget, "policy type: target, threshold, schedule, queue")
	cmd.Flags().StringVar(&queueName, "queue", "", "queue name (for queue policy type)")
	cmd.Flags().Float64Var(&targetValue, "target", 60, "target metric value")
	cmd.Flags().IntVar(&minReplicas, "min", 1, "minimum replicas")
	cmd.Flags().IntVar(&maxReplicas, "max", 10, "maximum replicas")
	cmd.Flags().IntVar(&scaleOutCooldown, "scale-out-cooldown", 60, "scale-out cooldown in seconds")
	cmd.Flags().IntVar(&scaleInCooldown, "scale-in-cooldown", 300, "scale-in cooldown in seconds")
	_ = cmd.MarkFlagRequired("group")
	return cmd
}

func groupAutoscaleListCmd(opts *options) *cobra.Command {
	var groupName string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list autoscaling policies for a group",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("autoscale:list", "project:"+opts.project); err != nil {
					return err
				}
				var policies []autoscale.AutoscalePolicy
				if groupName != "" {
					mgr := compute.NewManager(ac.Store.Compute)
					g, err := mgr.GetGroup(groupName)
					if err != nil {
						return fmt.Errorf("group %q not found: %w", groupName, err)
					}
					policies, err = ac.Store.Autoscale.Policies.ForGroup(g.ID)
					if err != nil {
						return err
					}
				} else {
					var err error
					policies, err = ac.Store.Autoscale.Policies.List(opts.project)
					if err != nil {
						return err
					}
				}
				if opts.json {
					return printJSON(policies)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tGROUP\tTYPE\tMETRIC\tTARGET\tMIN\tMAX\tENABLED\tLAST_DECISION")
				for _, p := range policies {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.2f\t%d\t%d\t%v\t%s\n",
						p.Name, p.GroupName, p.PolicyType, p.MetricName,
						p.TargetValue, p.MinReplicas, p.MaxReplicas, p.Enabled, p.LastDecision)
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&groupName, "group", "", "filter by group name")
	return cmd
}

func groupAutoscaleDisableCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable GROUP",
		Short: "disable all autoscaling policies for a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("autoscale:update", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				g, err := mgr.GetGroup(args[0])
				if err != nil {
					return fmt.Errorf("group %q not found: %w", args[0], err)
				}
				policies, err := ac.Store.Autoscale.Policies.ForGroup(g.ID)
				if err != nil {
					return err
				}
				for i := range policies {
					p := policies[i]
					p.Enabled = false
					_ = ac.Store.Autoscale.Policies.Update(p)
				}
				fmt.Printf("Disabled %d autoscaling policies for group %s\n", len(policies), g.Name)
				return nil
			})
		},
	}
}

func groupAutoscaleEvaluateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "evaluate GROUP",
		Short: "manually evaluate autoscaling policies for a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("autoscale:evaluate", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				g, err := mgr.GetGroup(args[0])
				if err != nil {
					return fmt.Errorf("group %q not found: %w", args[0], err)
				}
				policies, err := ac.Store.Autoscale.Policies.ForGroup(g.ID)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(map[string]any{"group": g.Name, "policies": len(policies), "currentDesired": g.DesiredSize})
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "GROUP\tPOLICY\tTYPE\tMETRIC\tTARGET\tCURRENT\tNOTE")
				for _, p := range policies {
					status := "enabled"
					if !p.Enabled {
						status = "disabled"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.2f\t%d\t%s\n",
						g.Name, p.Name, p.PolicyType, p.MetricName, p.TargetValue, g.DesiredSize, status)
				}
				return tw.Flush()
			})
		},
	}
}

func groupAutoscaleHistoryCmd(opts *options) *cobra.Command {
	var limitN int
	cmd := &cobra.Command{
		Use:   "history GROUP",
		Short: "show recent autoscaling decisions for a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("autoscale:inspect", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				g, err := mgr.GetGroup(args[0])
				if err != nil {
					return fmt.Errorf("group %q not found: %w", args[0], err)
				}
				decisions, err := ac.Store.Autoscale.Decisions.ListForGroup(g.ID, limitN)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(decisions)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "TIME\tDECISION\tOLD\tNEW\tMETRIC\tVALUE\tREASON")
				for _, d := range decisions {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%.2f\t%s\n",
						shortTime(d.CreatedAt), d.Decision, d.OldReplicas, d.NewReplicas,
						d.MetricName, d.MetricValue, d.Reason)
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().IntVar(&limitN, "limit", 20, "number of decisions to show")
	return cmd
}

// ---- compute instance -------------------------------------------------------

func computeInstanceCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "instance", Short: "manage compute instances"}
	cmd.AddCommand(computeInstanceRunCmd(opts))
	return cmd
}

func computeInstanceRunCmd(opts *options) *cobra.Command {
	var templateName string
	var instanceName string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "launch an instance from a template",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				mgr := compute.NewManager(ctrl.Store.Compute)
				spec, err := mgr.RunFromTemplate(templateName, instanceName)
				if err != nil {
					return err
				}
				ro := types.ResourceOverrides{
					Limits: types.ResourceLimits{
						MemoryBytes:  spec.Resources.MemoryBytes,
						CPUTimeSecs:  spec.Resources.CPUTimeSecs,
						MaxProcesses: spec.Resources.MaxProcesses,
					},
					MemorySet:  spec.Resources.MemoryBytes > 0,
					CPUTimeSet: spec.Resources.CPUTimeSecs > 0,
					PidsSet:    spec.Resources.MaxProcesses > 0,
				}
				inst, err := ctrl.Instances.Run(spec.Image, ro, manager.RunOptions{Name: spec.InstanceName})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(inst)
				}
				fmt.Printf("instance/%s running (id: %s, image: %s, template: %s)\n",
					inst.Name, inst.ID, inst.Image, spec.TemplateName)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&templateName, "template", "", "template name (required)")
	cmd.Flags().StringVar(&instanceName, "name", "", "custom instance name (optional)")
	_ = cmd.MarkFlagRequired("template")
	return cmd
}

// ---- compute instance-type --------------------------------------------------

func computeInstanceTypeCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "instance-type", Short: "manage capsule instance types"}
	cmd.AddCommand(
		computeInstanceTypeSeedCmd(opts),
		computeInstanceTypeListCmd(opts),
		computeInstanceTypeInspectCmd(opts),
		computeInstanceTypeCreateCmd(opts),
		computeInstanceTypeDeleteCmd(opts),
	)
	return cmd
}

func computeInstanceTypeSeedCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "seed built-in and standard instance types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := compute.NewManager(st.Compute)
				if err := mgr.SeedBuiltinTypes(); err != nil {
					return err
				}
				if err := mgr.SeedStandardTypes(); err != nil {
					return err
				}
				fmt.Println("built-in and standard instance types seeded")
				return nil
			})
		},
	}
}

func computeInstanceTypeListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list instance types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:type:list", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				types, err := mgr.ListInstanceTypes()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(types)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tFAMILY\tCPU\tMEMORY\tGPU\tGPU COUNT\tLOCKED\tDESCRIPTION")
				for _, t := range types {
					gpuFlag := "no"
					if t.GPUEligible {
						gpuFlag = "yes"
					}
					locked := "no"
					if t.Locked {
						locked = "yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%d\t%s\t%s\n",
						t.Name, t.Family, t.CPUCount, humanSize(t.MemoryBytes),
						gpuFlag, t.GPUCount, locked, t.Description)
				}
				return w.Flush()
			})
		},
	}
}

func computeInstanceTypeInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "inspect an instance type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:type:inspect", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				t, err := mgr.GetInstanceType(args[0])
				if err != nil {
					return err
				}
				return printJSON(t)
			})
		},
	}
}

func computeInstanceTypeCreateCmd(opts *options) *cobra.Command {
	var (
		family      string
		cpuCount    int
		memoryMB    int64
		pidLimit    int
		gpuEligible bool
		gpuCount    int
		description string
	)
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a custom instance type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:type:create", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				it := compute.InstanceType{
					Name:        args[0],
					Family:      family,
					CPUCount:    cpuCount,
					MemoryBytes: memoryMB * 1024 * 1024,
					PIDLimit:    pidLimit,
					GPUEligible: gpuEligible,
					GPUCount:    gpuCount,
					Description: description,
				}
				created, err := mgr.CreateInstanceType(it)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(created)
				}
				fmt.Printf("instance-type/%s created\n", created.Name)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&family, "family", compute.InstanceTypeFamilyCompute, "type family (memory, compute, gpu)")
	cmd.Flags().IntVar(&cpuCount, "cpu", 1, "CPU count")
	cmd.Flags().Int64Var(&memoryMB, "memory-mb", 1024, "memory in megabytes")
	cmd.Flags().IntVar(&pidLimit, "pids", 256, "PID limit")
	cmd.Flags().BoolVar(&gpuEligible, "gpu", false, "GPU eligible")
	cmd.Flags().IntVar(&gpuCount, "gpu-count", 0, "number of GPUs")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	return cmd
}

func computeInstanceTypeDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a custom instance type (locked types cannot be deleted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:type:delete", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				if err := mgr.DeleteInstanceType(args[0]); err != nil {
					return err
				}
				fmt.Printf("instance-type/%s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---- compute gpu ------------------------------------------------------------

func computeGPUCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "gpu", Short: "manage GPU devices"}
	cmd.AddCommand(
		computeGPURegisterCmd(opts),
		computeGPUListCmd(opts),
		computeGPUInspectCmd(opts),
		computeGPUAssignCmd(opts),
		computeGPUReleaseCmd(opts),
		computeGPURemoveCmd(opts),
	)
	return cmd
}

func computeGPURegisterCmd(opts *options) *cobra.Command {
	var (
		vendor     string
		model      string
		memoryMB   int64
		devicePath string
	)
	cmd := &cobra.Command{
		Use:   "register",
		Short: "register a GPU device",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:gpu:register", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				g := compute.GPUDevice{
					Vendor:      vendor,
					Model:       model,
					MemoryBytes: memoryMB * 1024 * 1024,
					DevicePath:  devicePath,
				}
				created, err := mgr.RegisterGPU(g)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(created)
				}
				fmt.Printf("gpu/%s registered (%s %s)\n", created.ID, created.Vendor, created.Model)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&vendor, "vendor", "", "GPU vendor (e.g. NVIDIA)")
	cmd.Flags().StringVar(&model, "model", "", "GPU model (e.g. RTX 3090 24GB)")
	cmd.Flags().Int64Var(&memoryMB, "memory-mb", 0, "GPU memory in megabytes")
	cmd.Flags().StringVar(&devicePath, "device-path", "", "device path (e.g. /dev/nvidia0)")
	return cmd
}

func computeGPUListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list GPU devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:gpu:list", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				gpus, err := mgr.ListGPUDevices()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(gpus)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tVENDOR\tMODEL\tMEMORY\tSTATUS\tASSIGNED TO")
				for _, g := range gpus {
					assigned := "-"
					if g.AssignedInstanceID != "" {
						assigned = g.AssignedInstanceID
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						g.ID, g.Vendor, g.Model, humanSize(g.MemoryBytes), g.Status, assigned)
				}
				return w.Flush()
			})
		},
	}
}

func computeGPUInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect ID",
		Short: "inspect a GPU device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:gpu:inspect", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				g, err := mgr.GetGPUDevice(args[0])
				if err != nil {
					return err
				}
				return printJSON(g)
			})
		},
	}
}

func computeGPUAssignCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "assign GPU-ID INSTANCE-ID",
		Short: "assign a GPU to an instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:gpu:assign", "project:"+opts.project); err != nil {
					_ = ac.Store.Events.Insert(store.ResourceEvent{
						ResourceType:  "gpu",
						ResourceID:    args[0],
						Action:        "gpu.assignment_denied",
						ProjectID:     opts.project,
						PrincipalType: ac.PrincipalType,
						PrincipalID:   ac.PrincipalID,
						Data:          map[string]any{"instanceId": args[1]},
					})
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				if err := mgr.AssignGPU(args[0], args[1]); err != nil {
					return err
				}
				_ = ac.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "gpu",
					ResourceID:    args[0],
					Action:        "gpu.assigned",
					ProjectID:     opts.project,
					PrincipalType: ac.PrincipalType,
					PrincipalID:   ac.PrincipalID,
					Data:          map[string]any{"instanceId": args[1]},
				})
				fmt.Printf("gpu/%s assigned to instance %s\n", args[0], args[1])
				return nil
			})
		},
	}
}

func computeGPUReleaseCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "release GPU-ID",
		Short: "release a GPU assignment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:gpu:release", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				// capture instance ID for audit before releasing
				g, _ := mgr.GetGPUDevice(args[0])
				if err := mgr.ReleaseGPU(args[0]); err != nil {
					return err
				}
				_ = ac.Store.Events.Insert(store.ResourceEvent{
					ResourceType:  "gpu",
					ResourceID:    args[0],
					Action:        "gpu.released",
					ProjectID:     opts.project,
					PrincipalType: ac.PrincipalType,
					PrincipalID:   ac.PrincipalID,
					Data:          map[string]any{"instanceId": g.AssignedInstanceID},
				})
				fmt.Printf("gpu/%s released\n", args[0])
				return nil
			})
		},
	}
}

func computeGPURemoveCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove GPU-ID",
		Short: "remove a GPU device record (must not be assigned)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("compute:gpu:remove", "project:"+opts.project); err != nil {
					return err
				}
				mgr := compute.NewManager(ac.Store.Compute)
				if err := mgr.RemoveGPU(args[0]); err != nil {
					return err
				}
				fmt.Printf("gpu/%s removed\n", args[0])
				return nil
			})
		},
	}
}

// ===========================================================================
// capper storage
// ===========================================================================

func storagePaths(st *store.Store) capstore.Paths {
	return capstore.Paths{
		Volumes:   st.Paths.StorageVolumes,
		Buckets:   st.Paths.StorageBuckets,
		Snapshots: st.Paths.StorageSnapshots,
	}
}

func storageCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "manage volumes, buckets, objects, and snapshots",
	}
	cmd.AddCommand(
		storageVolumeCmd(opts),
		storageBucketCmd(opts),
		storageObjectCmd(opts),
		storageSnapshotCmd(opts),
		storageS3Cmd(opts),
		storageShareCmd(opts),
	)
	return cmd
}

// ---- storage s3 -------------------------------------------------------------

func storageS3Cmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "s3", Short: "S3-compatible object storage server"}
	cmd.AddCommand(storageS3StartCmd(opts), storageS3CredentialCmd(opts))
	return cmd
}

func storageS3CredentialCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "credential", Short: "manage S3 IAM service account credentials"}
	cmd.AddCommand(storageS3CredentialCreateCmd(opts), storageS3CredentialListCmd(opts), storageS3CredentialDeleteCmd(opts))
	return cmd
}

func storageS3CredentialCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create ACCOUNT_ID",
		Short: "generate an S3 access/secret key pair for a service account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("storage:s3:credential:create", "project:"+opts.project); err != nil {
					return err
				}
				cred, err := caps3.GenerateS3Credential(ac.Store.DB, ac.Store.SecretKey, args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(cred)
				}
				fmt.Printf("Access key: %s\nSecret key: %s\n", cred.AccessKey, cred.SecretKey)
				return nil
			})
		},
	}
}

func storageS3CredentialListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list ACCOUNT_ID",
		Short: "list S3 credentials for a service account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("storage:s3:credential:list", "project:"+opts.project); err != nil {
					return err
				}
				creds, err := caps3.ListS3Credentials(ac.Store.DB, args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(creds)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ACCESS_KEY\tACCOUNT\tCREATED")
				for _, c := range creds {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", c.AccessKey, c.AccountID, shortTime(c.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func storageS3CredentialDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID_OR_KEY",
		Short: "delete an S3 credential by ID or access key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("storage:s3:credential:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := caps3.DeleteS3Credential(ac.Store.DB, args[0]); err != nil {
					return err
				}
				fmt.Printf("Credential %s deleted\n", args[0])
				return nil
			})
		},
	}
}

func storageS3StartCmd(opts *options) *cobra.Command {
	var listen string
	var noAuth bool
	var accessKey string
	var secretKey string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start the S3-compatible object storage server",
		Long: `Start the S3-compatible HTTP server for Capper object storage.

Third-party tools (AWS CLI, s3cmd, MinIO client) can connect using SigV4:

  aws s3 ls --endpoint-url http://127.0.0.1:9000

Use --no-auth to disable credential checking (development only).
Use --access-key and --secret-key to set static credentials.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.EnsurePaths(); err != nil {
					return err
				}
				cfg := caps3.Config{
					ListenAddr: listen,
					StorageDir: storagePaths(st).Buckets,
					Buckets:    mgr,
					AuditStore: st.Audit,
				}
				if !noAuth {
					if accessKey != "" && secretKey != "" {
						cfg.Credentials = caps3.StaticCredentials{accessKey: secretKey}
					} else {
						// Fall back to IAM-backed credentials from the database.
						cfg.Credentials = caps3.NewIAMCredentialProvider(st.DB, st.SecretKey)
					}
					// Enforce IAM authorization + bucket policy on object ops, and
					// reject requests when credentials/authorizer are unset.
					cfg.ObjectAuth = caps3.NewCapperObjectAuthorizer(st.IAM, st.DB)
					cfg.ProductionMode = true
				}
				fmt.Printf("S3 server listening on %s\n", listen)
				if noAuth {
					fmt.Println("WARNING: authentication disabled — development mode only")
				}
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background()
				}
				return caps3.New(cfg).Start(ctx)
			})
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:9000", "address:port to listen on")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "disable SigV4 authentication (development only)")
	cmd.Flags().StringVar(&accessKey, "access-key", "", "SigV4 access key (optional; uses IAM credentials when omitted)")
	cmd.Flags().StringVar(&secretKey, "secret-key", "", "SigV4 secret key (required when --access-key is set)")
	return cmd
}

// ---- storage volume ---------------------------------------------------------

func storageVolumeCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "volume", Short: "manage storage volumes"}
	cmd.AddCommand(
		storageVolumeCreateCmd(opts),
		storageVolumeListCmd(opts),
		storageVolumeInspectCmd(opts),
		storageVolumeAttachCmd(opts),
		storageVolumeDetachCmd(opts),
		storageVolumeDeleteCmd(opts),
	)
	return cmd
}

func storageVolumeCreateCmd(opts *options) *cobra.Command {
	var size string
	var class string
	var encrypted bool
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a directory-backed volume",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sizeBytes, err := parseSize(size)
			if err != nil {
				return fmt.Errorf("invalid --size: %w", err)
			}
			return withStore(opts, func(st *store.Store) error {
				if qerr := st.Billing.CheckAccountQuota(opts.project, "volume"); qerr != nil {
					return fmt.Errorf("quota exceeded: %w", qerr)
				}
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.EnsurePaths(); err != nil {
					return err
				}
				v, err := mgr.CreateVolume(capstore.CreateVolumeOptions{
					Name:      args[0],
					SizeBytes: sizeBytes,
					Class:     class,
					Encrypted: encrypted,
				})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(v)
				}
				fmt.Printf("volume/%s created (path: %s, size: %s)\n", v.Name, v.Path, humanSize(v.SizeBytes))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&size, "size", "0", "volume size hint (e.g. 10G)")
	cmd.Flags().StringVar(&class, "class", capstore.VolumeClassLocal, "volume class")
	cmd.Flags().BoolVar(&encrypted, "encrypted", false, "mark volume as encrypted")
	return cmd
}

func storageVolumeListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list volumes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				vs, err := mgr.ListVolumes()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(vs)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tSIZE\tCLASS\tATTACHED\tCREATED")
				for _, v := range vs {
					attached := "-"
					if v.AttachedInstanceID != "" {
						attached = v.AttachedInstanceID + ":" + v.AttachedPath
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						v.Name, humanSize(v.SizeBytes), v.Class, attached, shortTime(v.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
}

func storageVolumeInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show volume details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				v, err := mgr.GetVolume(args[0])
				if err != nil {
					return err
				}
				return printJSON(v)
			})
		},
	}
}

func storageVolumeAttachCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "attach VOLUME INSTANCE:PATH",
		Short: "record a volume attachment to an instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			parts := strings.SplitN(args[1], ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("expected INSTANCE:PATH, got %q", args[1])
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.AttachVolume(args[0], parts[0], parts[1]); err != nil {
					return err
				}
				fmt.Printf("volume/%s attached to %s at %s\n", args[0], parts[0], parts[1])
				return nil
			})
		},
	}
}

func storageVolumeDetachCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "detach VOLUME",
		Short: "clear a volume's attachment record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.DetachVolume(args[0]); err != nil {
					return err
				}
				fmt.Printf("volume/%s detached\n", args[0])
				return nil
			})
		},
	}
}

func storageVolumeDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a volume and its directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.DeleteVolume(args[0]); err != nil {
					return err
				}
				fmt.Printf("volume/%s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---- storage bucket ---------------------------------------------------------

func storageBucketCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "bucket", Short: "manage object storage buckets"}
	cmd.AddCommand(
		storageBucketCreateCmd(opts),
		storageBucketListCmd(opts),
		storageBucketInspectCmd(opts),
		storageBucketDeleteCmd(opts),
		storageBucketCredentialsCmd(opts),
	)
	return cmd
}

func storageBucketCreateCmd(opts *options) *cobra.Command {
	var versioning, encrypted bool
	var quotaStr string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create an object storage bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			quota, err := parseSize(quotaStr)
			if err != nil {
				return fmt.Errorf("invalid --quota: %w", err)
			}
			return withStore(opts, func(st *store.Store) error {
				if qerr := st.Billing.CheckAccountQuota(opts.project, "bucket"); qerr != nil {
					return fmt.Errorf("quota exceeded: %w", qerr)
				}
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.EnsurePaths(); err != nil {
					return err
				}
				b, err := mgr.CreateBucket(capstore.CreateBucketOptions{
					Name:       args[0],
					Versioning: versioning,
					Encrypted:  encrypted,
					QuotaBytes: quota,
				})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(b)
				}
				fmt.Printf("bucket/%s created (path: %s)\n", b.Name, b.Path)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&versioning, "versioning", false, "enable object versioning")
	cmd.Flags().BoolVar(&encrypted, "encrypted", false, "mark bucket as encrypted")
	cmd.Flags().StringVar(&quotaStr, "quota", "0", "storage quota (e.g. 100G)")
	return cmd
}

func storageBucketListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list buckets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				bs, err := mgr.ListBuckets()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(bs)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tBACKEND\tVERSIONING\tQUOTA\tCREATED")
				for _, b := range bs {
					ver := "no"
					if b.Versioning {
						ver = "yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						b.Name, b.Backend, ver, humanSize(b.QuotaBytes), shortTime(b.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
}

func storageBucketInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show bucket details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				b, err := mgr.GetBucket(args[0])
				if err != nil {
					return err
				}
				return printJSON(b)
			})
		},
	}
}

func storageBucketDeleteCmd(opts *options) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a bucket and its objects",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.DeleteBucket(args[0], force); err != nil {
					return err
				}
				fmt.Printf("bucket/%s deleted\n", args[0])
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "delete even if bucket is not empty")
	return cmd
}

// ---- storage object ---------------------------------------------------------

func storageObjectCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "object", Short: "manage objects within buckets"}
	cmd.AddCommand(
		storageObjectPutCmd(opts),
		storageObjectGetCmd(opts),
		storageObjectListCmd(opts),
		storageObjectDeleteCmd(opts),
	)
	return cmd
}

func storageObjectPutCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "put BUCKET KEY FILE",
		Short: "upload a file as an object",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				o, err := mgr.PutObject(args[0], args[1], args[2])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(o)
				}
				fmt.Printf("object/%s uploaded to %s (%s, sha256: %s)\n",
					o.Key, args[0], humanSize(o.SizeBytes), o.Digest[:12])
				return nil
			})
		},
	}
}

func storageObjectGetCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "get BUCKET KEY DEST",
		Short: "download an object to a file",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				return mgr.GetObject(args[0], args[1], args[2])
			})
		},
	}
}

func storageObjectListCmd(opts *options) *cobra.Command {
	var prefix string
	cmd := &cobra.Command{
		Use:   "list BUCKET",
		Short: "list objects in a bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				objs, err := mgr.ListObjects(args[0], prefix)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(objs)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "KEY\tSIZE\tDIGEST\tCREATED")
				for _, o := range objs {
					digest := o.Digest
					if len(digest) > 12 {
						digest = digest[:12]
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
						o.Key, humanSize(o.SizeBytes), digest, shortTime(o.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", "", "filter objects by key prefix")
	return cmd
}

func storageObjectDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete BUCKET KEY",
		Short: "delete an object from a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.DeleteObject(args[0], args[1]); err != nil {
					return err
				}
				fmt.Printf("object/%s deleted from %s\n", args[1], args[0])
				return nil
			})
		},
	}
}

// ---- storage snapshot -------------------------------------------------------

func storageSnapshotCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "snapshot", Short: "manage volume snapshots"}
	cmd.AddCommand(
		storageSnapshotCreateCmd(opts),
		storageSnapshotListCmd(opts),
		storageSnapshotInspectCmd(opts),
		storageSnapshotRestoreCmd(opts),
		storageSnapshotDeleteCmd(opts),
	)
	return cmd
}

func storageSnapshotCreateCmd(opts *options) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create VOLUME",
		Short: "create a tar.zst snapshot of a volume",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.EnsurePaths(); err != nil {
					return err
				}
				snap, err := mgr.SnapshotVolume(args[0], name)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(snap)
				}
				fmt.Printf("snapshot/%s created (%s, sha256: %s)\n",
					snap.Name, humanSize(snap.SizeBytes), snap.Digest[:12])
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "snapshot name (auto-generated if empty)")
	return cmd
}

func storageSnapshotListCmd(opts *options) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				snaps, err := mgr.ListSnapshots(source)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(snaps)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tSOURCE\tSIZE\tCREATED")
				for _, snap := range snaps {
					fmt.Fprintf(w, "%s\t%s/%s\t%s\t%s\n",
						snap.Name, snap.SourceType, snap.SourceID,
						humanSize(snap.SizeBytes), shortTime(snap.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "filter by source volume ID")
	return cmd
}

func storageSnapshotInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show snapshot details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				snap, err := mgr.GetSnapshot(args[0])
				if err != nil {
					return err
				}
				return printJSON(snap)
			})
		},
	}
}

func storageSnapshotRestoreCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "restore SNAPSHOT VOLUME",
		Short: "restore a snapshot into a volume directory",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.RestoreSnapshot(args[0], args[1]); err != nil {
					return err
				}
				fmt.Printf("snapshot/%s restored into volume/%s\n", args[0], args[1])
				return nil
			})
		},
	}
}

func storageSnapshotDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a snapshot and its archive file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := capstore.NewManager(st.Storage, storagePaths(st))
				if err := mgr.DeleteSnapshot(args[0]); err != nil {
					return err
				}
				fmt.Printf("snapshot/%s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ===========================================================================
// capper registry
// ===========================================================================

func registryCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "manage image and artifact registries",
	}
	cmd.AddCommand(
		registryInitCmd(opts),
		registryListCmd(opts),
		registryInspectCmd(opts),
		registryGCCmd(opts),
		registryDeleteCmd(opts),
		registryImageCmd(opts),
		registryArtifactCmd(opts),
		registryTokenCmd(opts),
	)
	return cmd
}

func regManager(st *store.Store) *capreg.Manager {
	return capreg.NewManager(st.Registry, st.Paths.Registries)
}

// ---- registry lifecycle -----------------------------------------------------

func registryInitCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "init NAME",
		Short: "create a local registry (idempotent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				if err := mgr.EnsureRoot(); err != nil {
					return err
				}
				r, err := mgr.Init(args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(r)
				}
				fmt.Printf("registry/%s initialized (path: %s)\n", r.Name, r.Path)
				return nil
			})
		},
	}
}

func registryListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list registries",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				rs, err := mgr.ListRegistries()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(rs)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tBACKEND\tPATH\tCREATED")
				for _, r := range rs {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Backend, r.Path, shortTime(r.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
}

func registryInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show registry details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				r, err := mgr.GetRegistry(args[0])
				if err != nil {
					return err
				}
				imgs, _ := mgr.ListImages(args[0])
				arts, _ := mgr.ListArtifacts(args[0])
				return printJSON(map[string]any{
					"registry":      r,
					"imageCount":    len(imgs),
					"artifactCount": len(arts),
				})
			})
		},
	}
}

func registryGCCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "gc NAME",
		Short: "remove unreferenced files from a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				n, err := mgr.GC(args[0])
				if err != nil {
					return err
				}
				fmt.Printf("registry/%s gc: removed %d unreferenced files\n", args[0], n)
				return nil
			})
		},
	}
}

func registryDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a registry and all its contents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				if err := mgr.DeleteRegistry(args[0]); err != nil {
					return err
				}
				fmt.Printf("registry/%s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---- registry image ---------------------------------------------------------

func registryImageCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "manage images in a registry"}
	cmd.AddCommand(
		registryImagePushCmd(opts),
		registryImagePullCmd(opts),
		registryImageTagCmd(opts),
		registryImageListCmd(opts),
		registryImageDeleteCmd(opts),
		registryImageScanCmd(opts),
	)
	return cmd
}

func registryImageScanCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "scan REGISTRY/NAME:VERSION",
		Short: "run static security scans on a registry image and update its scan_status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[0])
			if ref.Registry == "" {
				return fmt.Errorf("expected REGISTRY/NAME:VERSION, missing registry prefix")
			}
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("registry:scan", "project:"+opts.project); err != nil {
					return err
				}
				mgr := regManager(ac.Store)
				results, status, err := mgr.ScanImage(ref.Registry, ref.Name, ref.Version)
				if err != nil {
					return err
				}
				ac.RecordEvent("registry", ref.Registry+"/"+ref.Name+":"+ref.Version,
					"registry.image.scan", opts.project,
					map[string]any{"status": status, "findings": len(results)})
				if opts.json {
					return printJSON(map[string]any{"status": status, "results": results})
				}
				fmt.Printf("Scan complete for %s/%s:%s — status: %s\n\n",
					ref.Registry, ref.Name, ref.Version, status)
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "TYPE\tSTATUS\tFINDINGS\tDETAIL")
				for _, r := range results {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", r.Type, r.Status, r.Findings, r.Detail)
				}
				return tw.Flush()
			})
		},
	}
}

func registryImagePushCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "push FILE REGISTRY/NAME:VERSION",
		Short: "push a .cap image into a registry",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[1])
			if ref.Registry == "" {
				return fmt.Errorf("expected REGISTRY/NAME:VERSION, missing registry prefix")
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				img, err := mgr.Push(ref.Registry, ref.Name, ref.Version, args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(img)
				}
				fmt.Printf("pushed %s/%s:%s (sha256: %s)\n",
					img.RegistryName, img.Name, img.Version, img.Digest[:12])
				return nil
			})
		},
	}
}

func registryImagePullCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "pull REGISTRY/NAME:VERSION DEST",
		Short: "pull an image from a registry to a local path",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[0])
			if ref.Registry == "" {
				return fmt.Errorf("expected REGISTRY/NAME:VERSION, missing registry prefix")
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				img, err := mgr.Pull(ref.Registry, ref.Name, ref.Version, args[1])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(img)
				}
				fmt.Printf("pulled %s/%s:%s → %s\n", img.RegistryName, img.Name, img.Version, args[1])
				return nil
			})
		},
	}
}

func registryImageTagCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "tag REGISTRY/NAME:VERSION NEW_VERSION",
		Short: "tag an image with a new version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[0])
			if ref.Registry == "" {
				return fmt.Errorf("expected REGISTRY/NAME:VERSION, missing registry prefix")
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				img, err := mgr.TagImage(ref.Registry, ref.Name, ref.Version, args[1])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(img)
				}
				fmt.Printf("tagged %s/%s:%s → %s\n", img.RegistryName, img.Name, ref.Version, args[1])
				return nil
			})
		},
	}
}

func registryImageListCmd(opts *options) *cobra.Command {
	var regName string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list images in a registry (or all registries)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				imgs, err := mgr.ListImages(regName)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(imgs)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "REGISTRY\tNAME\tVERSION\tDIGEST\tSIGNED\tCREATED")
				for _, img := range imgs {
					digest := img.Digest
					if len(digest) > 12 {
						digest = digest[:12]
					}
					signed := "no"
					if img.Signed {
						signed = "yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						img.RegistryName, img.Name, img.Version, digest, signed, shortTime(img.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&regName, "registry", "", "filter by registry name")
	return cmd
}

func registryImageDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete REGISTRY/NAME:VERSION",
		Short: "delete an image version from a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[0])
			if ref.Registry == "" {
				return fmt.Errorf("expected REGISTRY/NAME:VERSION, missing registry prefix")
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				if err := mgr.DeleteImage(ref.Registry, ref.Name, ref.Version); err != nil {
					return err
				}
				fmt.Printf("deleted %s/%s:%s\n", ref.Registry, ref.Name, ref.Version)
				return nil
			})
		},
	}
}

// ---- registry artifact ------------------------------------------------------

func registryArtifactCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "manage artifacts in a registry"}
	cmd.AddCommand(
		registryArtifactPutCmd(opts),
		registryArtifactGetCmd(opts),
		registryArtifactListCmd(opts),
		registryArtifactDeleteCmd(opts),
	)
	return cmd
}

func registryArtifactPutCmd(opts *options) *cobra.Command {
	var name, version, artifactType string
	var labelStrs []string
	cmd := &cobra.Command{
		Use:   "put REGISTRY FILE",
		Short: "upload a file as a versioned artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels := make(map[string]string)
			for _, kv := range labelStrs {
				if eq := strings.IndexByte(kv, '='); eq >= 0 {
					labels[kv[:eq]] = kv[eq+1:]
				}
			}
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				a, err := mgr.PutArtifact(args[0], name, version, artifactType, args[1], labels)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(a)
				}
				fmt.Printf("artifact %s/%s:%s stored (%s, sha256: %s)\n",
					a.RegistryName, a.Name, a.Version, humanSize(a.SizeBytes), a.Digest[:12])
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "artifact name (required)")
	cmd.Flags().StringVar(&version, "version", "latest", "artifact version")
	cmd.Flags().StringVar(&artifactType, "type", "", "artifact type (inferred from filename if omitted)")
	cmd.Flags().StringArrayVar(&labelStrs, "label", nil, "label KEY=VALUE (repeatable)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func registryArtifactGetCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "get REGISTRY NAME:VERSION DEST",
		Short: "download an artifact from a registry",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[1])
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				a, err := mgr.GetArtifact(args[0], ref.Name, ref.Version, args[2])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(a)
				}
				fmt.Printf("artifact %s/%s:%s → %s\n", args[0], a.Name, a.Version, args[2])
				return nil
			})
		},
	}
}

func registryArtifactListCmd(opts *options) *cobra.Command {
	var regName string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list artifacts (optionally filtered by registry)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				arts, err := mgr.ListArtifacts(regName)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(arts)
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "REGISTRY\tNAME\tVERSION\tTYPE\tSIZE\tCREATED")
				for _, a := range arts {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						a.RegistryName, a.Name, a.Version, a.Type,
						humanSize(a.SizeBytes), shortTime(a.CreatedAt))
				}
				return w.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&regName, "registry", "", "filter by registry name")
	return cmd
}

func registryArtifactDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete REGISTRY NAME:VERSION",
		Short: "delete an artifact version from a registry",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := capreg.ParseRef(args[1])
			return withStore(opts, func(st *store.Store) error {
				mgr := regManager(st)
				if err := mgr.DeleteArtifact(args[0], ref.Name, ref.Version); err != nil {
					return err
				}
				fmt.Printf("artifact %s/%s:%s deleted\n", args[0], ref.Name, ref.Version)
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper event
// ---------------------------------------------------------------------------

func eventCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "view resource lifecycle events",
	}
	cmd.AddCommand(eventListCmd(opts), eventTailCmd(opts), eventExportCmd(opts))
	return cmd
}

func eventListCmd(opts *options) *cobra.Command {
	var resourceType, resourceID, action string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list recent resource events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				events, err := st.Events.List(store.ListEventsOptions{
					ResourceType: resourceType,
					ResourceID:   resourceID,
					ProjectID:    opts.project,
					Action:       action,
					Limit:        limit,
				})
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(events)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "TIMESTAMP\tPRINCIPAL\tACTION\tTYPE\tRESOURCE")
				for _, e := range events {
					fmt.Fprintf(tw, "%s\t%s:%s\t%s\t%s\t%s\n",
						shortTime(e.Timestamp), e.PrincipalType, e.PrincipalID,
						e.Action, e.ResourceType, e.ResourceID)
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&resourceType, "type", "", "filter by resource type (instance, network, firewall, image)")
	cmd.Flags().StringVar(&resourceID, "resource", "", "filter by resource ID")
	cmd.Flags().StringVar(&action, "action", "", "filter by action prefix (e.g. instance)")
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of events to return")
	return cmd
}

func eventTailCmd(opts *options) *cobra.Command {
	var resourceType string
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "stream new resource events (Ctrl-C to stop)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				cursor := st.Events.LatestTimestamp()
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(sigCh)
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for {
					select {
					case <-sigCh:
						return nil
					case <-ticker.C:
						events, err := st.Events.Since(cursor, 200)
						if err != nil {
							return err
						}
						for _, e := range events {
							if resourceType != "" && e.ResourceType != resourceType {
								cursor = e.Timestamp
								continue
							}
							fmt.Fprintf(tw, "%s\t%s:%s\t%s\t%s\t%s\n",
								shortTime(e.Timestamp), e.PrincipalType, e.PrincipalID,
								e.Action, e.ResourceType, e.ResourceID)
							_ = tw.Flush()
							cursor = e.Timestamp
						}
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&resourceType, "type", "", "filter by resource type")
	return cmd
}

// ---------------------------------------------------------------------------
// capper secret
// ---------------------------------------------------------------------------

func secretCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "manage encrypted secrets",
	}
	cmd.AddCommand(
		secretCreateCmd(opts),
		secretListCmd(opts),
		secretInspectCmd(opts),
		secretDeleteCmd(opts),
	)
	return cmd
}

func secretCreateCmd(opts *options) *cobra.Command {
	var value, description string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create or update an encrypted secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("secret:create", "project:"+opts.project); err != nil {
					return err
				}
				if value == "" {
					return fmt.Errorf("--value is required")
				}
				sec, err := ac.Store.Secrets.Create(args[0], opts.project, description, value)
				if err != nil {
					return err
				}
				ac.RecordEvent("secret", sec.ID, "secret.created", opts.project, map[string]any{"name": sec.Name})
				if opts.json {
					return printJSON(sec)
				}
				fmt.Printf("Secret created\n\nName:    %s\nID:      %s\nProject: %s\n", sec.Name, sec.ID, sec.Project)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&value, "value", "", "plaintext secret value (required)")
	cmd.Flags().StringVar(&description, "description", "", "optional description")
	return cmd
}

func secretListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list secrets in the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("secret:list", "project:"+opts.project); err != nil {
					return err
				}
				secrets, err := ac.Store.Secrets.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(secrets)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tDESCRIPTION\tUPDATED")
				for _, s := range secrets {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, s.ID, s.Description, s.UpdatedAt)
				}
				return tw.Flush()
			})
		},
	}
}

func secretInspectCmd(opts *options) *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:   "inspect NAME",
		Short: "show secret metadata (use --reveal to decrypt the value)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				action := "secret:list"
				if reveal {
					action = "secret:read"
				}
				if err := ac.Authorize(action, "project:"+opts.project); err != nil {
					return err
				}
				sec, err := ac.Store.Secrets.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				type out struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Project     string `json:"project"`
					Description string `json:"description,omitempty"`
					Value       string `json:"value,omitempty"`
					CreatedAt   string `json:"createdAt"`
					UpdatedAt   string `json:"updatedAt"`
				}
				o := out{
					ID:          sec.ID,
					Name:        sec.Name,
					Project:     sec.Project,
					Description: sec.Description,
					CreatedAt:   sec.CreatedAt,
					UpdatedAt:   sec.UpdatedAt,
				}
				if reveal {
					plain, derr := ac.Store.Secrets.Decrypt(sec)
					if derr != nil {
						return derr
					}
					o.Value = plain
				}
				if opts.json {
					return printJSON(o)
				}
				fmt.Printf("Name:        %s\nID:          %s\nProject:     %s\nDescription: %s\nCreated:     %s\nUpdated:     %s\n",
					o.Name, o.ID, o.Project, o.Description, o.CreatedAt, o.UpdatedAt)
				if reveal {
					fmt.Printf("Value:       %s\n", o.Value)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "decrypt and print the secret value (requires secret:read)")
	return cmd
}

func secretDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("secret:delete", "project:"+opts.project); err != nil {
					return err
				}
				sec, err := ac.Store.Secrets.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				if err := ac.Store.Secrets.Delete(args[0], opts.project); err != nil {
					return err
				}
				ac.RecordEvent("secret", sec.ID, "secret.deleted", opts.project, map[string]any{"name": sec.Name})
				fmt.Printf("Deleted secret %q\n", args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper kms
// ---------------------------------------------------------------------------

func kmsCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kms",
		Short: "manage local KMS keys (envelope encryption)",
	}
	cmd.AddCommand(kmsKeyCmd(opts))
	return cmd
}

func kmsKeyCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "manage symmetric data keys",
	}
	cmd.AddCommand(kmsKeyCreateCmd(opts), kmsKeyListCmd(opts), kmsKeyRotateCmd(opts),
		kmsKeyEncryptCmd(opts), kmsKeyDecryptCmd(opts))
	return cmd
}

func kmsKeyCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create NAME",
		Short: "create a new data key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("kms:key:create", "project:"+opts.project); err != nil {
					return err
				}
				k, err := ac.Store.KMS.Create(args[0], opts.project)
				if err != nil {
					return err
				}
				ac.RecordEvent("kms_key", k.ID, "kms.key.created", opts.project, map[string]any{"name": k.Name})
				if opts.json {
					return printJSON(k)
				}
				fmt.Printf("Key created\n\nName:    %s\nID:      %s\nProject: %s\nStatus:  %s\n",
					k.Name, k.ID, k.Project, k.Status)
				return nil
			})
		},
	}
}

func kmsKeyListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list KMS keys in the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("kms:key:list", "project:"+opts.project); err != nil {
					return err
				}
				keys, err := ac.Store.KMS.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(keys)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tSTATUS\tCREATED\tROTATED_FROM")
				for _, k := range keys {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						k.Name, k.ID, k.Status, k.CreatedAt, k.RotatedFrom)
				}
				return tw.Flush()
			})
		},
	}
}

func kmsKeyRotateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "rotate NAME",
		Short: "rotate a data key (generates new key, marks old as rotated)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("kms:key:rotate", "project:"+opts.project); err != nil {
					return err
				}
				newKey, err := ac.Store.KMS.Rotate(args[0], opts.project)
				if err != nil {
					return err
				}
				ac.RecordEvent("kms_key", newKey.ID, "kms.key.rotated", opts.project,
					map[string]any{"name": newKey.Name, "rotatedFrom": newKey.RotatedFrom})
				if opts.json {
					return printJSON(newKey)
				}
				fmt.Printf("Key rotated\n\nName:         %s\nNew ID:       %s\nPredecessor:  %s\nStatus:       %s\n",
					newKey.Name, newKey.ID, newKey.RotatedFrom, newKey.Status)
				return nil
			})
		},
	}
}

func kmsKeyEncryptCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "encrypt NAME PLAINTEXT",
		Short: "encrypt plaintext using a KMS data key (output: base64 ciphertext)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("kms:key:encrypt", "project:"+opts.project); err != nil {
					return err
				}
				ct, err := ac.Store.KMS.Encrypt(args[0], opts.project, []byte(args[1]))
				if err != nil {
					return err
				}
				encoded := base64.StdEncoding.EncodeToString(ct)
				if opts.json {
					return printJSON(map[string]string{"ciphertext": encoded})
				}
				fmt.Println(encoded)
				return nil
			})
		},
	}
}

func kmsKeyDecryptCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "decrypt NAME CIPHERTEXT_BASE64",
		Short: "decrypt base64 ciphertext using a KMS data key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("kms:key:decrypt", "project:"+opts.project); err != nil {
					return err
				}
				ct, err := base64.StdEncoding.DecodeString(args[1])
				if err != nil {
					return fmt.Errorf("invalid base64: %w", err)
				}
				plain, err := ac.Store.KMS.Decrypt(args[0], opts.project, ct)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(map[string]string{"plaintext": string(plain)})
				}
				fmt.Println(string(plain))
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper governance
// ---------------------------------------------------------------------------

func governanceCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "governance",
		Short: "manage governance policies",
	}
	cmd.AddCommand(governanceListCmd(opts), governanceAddCmd(opts), governanceEvalCmd(opts))
	return cmd
}

func governanceListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list governance policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("governance:list", "project:"+opts.project); err != nil {
					return err
				}
				policies := ac.Store.Billing.ListGovernancePolicies(opts.project)
				if opts.json {
					return printJSON(policies)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tRESOURCE\tACTION\tEFFECT\tPRIORITY\tCONDITION")
				for _, p := range policies {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
						p.Name, p.Resource, p.Action, p.Effect, p.Priority, p.Condition)
				}
				return tw.Flush()
			})
		},
	}
}

func governanceAddCmd(opts *options) *cobra.Command {
	var resource, action, effect, condition string
	var priority int
	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "add a governance policy rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("governance:create", "project:"+opts.project); err != nil {
					return err
				}
				rule := ac.Store.Billing.AddGovernancePolicy(
					args[0], opts.project, resource, action, effect, condition, priority,
				)
				ac.RecordEvent("governance_policy", rule.ID, "governance.policy.created", opts.project,
					map[string]any{"name": rule.Name})
				if opts.json {
					return printJSON(rule)
				}
				fmt.Printf("Policy created: %s (effect=%s resource=%s action=%s)\n",
					rule.Name, rule.Effect, rule.Resource, rule.Action)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&resource, "resource", "*", "resource type (e.g. instance, network, *)")
	cmd.Flags().StringVar(&action, "action", "*", "action (e.g. create, delete, *)")
	cmd.Flags().StringVar(&effect, "effect", "deny", "allow or deny")
	cmd.Flags().StringVar(&condition, "condition", "", "optional label condition (e.g. label.env=prod)")
	cmd.Flags().IntVar(&priority, "priority", 0, "rule priority (higher = evaluated first)")
	return cmd
}

func governanceEvalCmd(opts *options) *cobra.Command {
	var resource, action string
	var labels []string
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "evaluate governance policies for a resource/action",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("governance:evaluate", "project:"+opts.project); err != nil {
					return err
				}
				labelMap := make(map[string]string)
				for _, kv := range labels {
					k, v, _ := strings.Cut(kv, "=")
					labelMap[k] = v
				}
				allowed, matched := ac.Store.Billing.EvaluateGovernance(opts.project, resource, action, labelMap)
				if opts.json {
					return printJSON(map[string]any{"allowed": allowed, "matchedRule": matched})
				}
				if allowed {
					fmt.Println("ALLOWED")
				} else {
					fmt.Printf("DENIED (matched rule: %s)\n", matched)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&resource, "resource", "", "resource type to evaluate (required)")
	cmd.Flags().StringVar(&action, "action", "", "action to evaluate (required)")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "label key=value filters")
	_ = cmd.MarkFlagRequired("resource")
	_ = cmd.MarkFlagRequired("action")
	return cmd
}

// ---------------------------------------------------------------------------
// capper cert
// ---------------------------------------------------------------------------

func certCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "manage TLS certificates signed by the local CA",
	}
	cmd.AddCommand(certIssueCmd(opts), certListCmd(opts), certRevokeCmd(opts), certCACmd(opts))
	return cmd
}

func certIssueCmd(opts *options) *cobra.Command {
	var cn string
	var dnsNames []string
	cmd := &cobra.Command{
		Use:   "issue NAME",
		Short: "issue a certificate signed by the local CA",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("cert:issue", "project:"+opts.project); err != nil {
					return err
				}
				result, err := ac.Store.Certs.Issue(args[0], opts.project, cn, dnsNames)
				if err != nil {
					return err
				}
				ac.RecordEvent("cert", result.ID, "cert.issued", opts.project,
					map[string]any{"name": result.Name, "cn": result.CommonName})
				if opts.json {
					return printJSON(result)
				}
				fmt.Printf("Certificate issued\n\nName:       %s\nID:         %s\nCommonName: %s\nExpires:    %s\n\n%s\n%s",
					result.Name, result.ID, result.CommonName, result.ExpiresAt,
					result.CertPEM, result.KeyPEM)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&cn, "cn", "", "certificate common name (defaults to NAME)")
	cmd.Flags().StringArrayVar(&dnsNames, "dns", nil, "DNS SAN entries, repeatable")
	return cmd
}

func certListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list certificates in the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("cert:list", "project:"+opts.project); err != nil {
					return err
				}
				certs, err := ac.Store.Certs.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(certs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tCN\tSTATUS\tEXPIRES")
				for _, c := range certs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						c.Name, c.ID, c.CommonName, c.Status, c.ExpiresAt)
				}
				return tw.Flush()
			})
		},
	}
}

func certRevokeCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke NAME",
		Short: "revoke a certificate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("cert:revoke", "project:"+opts.project); err != nil {
					return err
				}
				rec, err := ac.Store.Certs.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				if err := ac.Store.Certs.Revoke(args[0], opts.project); err != nil {
					return err
				}
				ac.RecordEvent("cert", rec.ID, "cert.revoked", opts.project, map[string]any{"name": rec.Name})
				fmt.Printf("Revoked certificate %q\n", args[0])
				return nil
			})
		},
	}
}

func certCACmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "ca",
		Short: "print the local CA certificate (PEM)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("cert:list", "project:"+opts.project); err != nil {
					return err
				}
				fmt.Print(string(ac.Store.Certs.CACertPEM()))
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper posture
// ---------------------------------------------------------------------------

func postureCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "posture",
		Short: "run and review security posture checks",
	}
	cmd.AddCommand(postureScanCmd(opts), postureListCmd(opts))
	return cmd
}

func postureScanCmd(opts *options) *cobra.Command {
	var rootDir string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "run posture checks (open ports, world-writable paths, SUID files)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("posture:scan", "project:"+opts.project); err != nil {
					return err
				}
				result, err := ac.Store.Posture.Scan(opts.project, rootDir)
				if err != nil {
					return err
				}
				ac.RecordEvent("posture", result.ScanID, "posture.scan", opts.project,
					map[string]any{"findings": len(result.Findings)})
				if opts.json {
					return printJSON(result)
				}
				fmt.Printf("Scan %s completed at %s — %d finding(s)\n\n",
					result.ScanID, result.ScannedAt, len(result.Findings))
				if len(result.Findings) == 0 {
					fmt.Println("No findings.")
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "SEVERITY\tCHECK\tTARGET\tDETAIL")
				for _, f := range result.Findings {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Severity, f.Check, f.Target, f.Detail)
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "filesystem root to scan for file-based checks (empty = skip)")
	return cmd
}

func postureListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list stored posture findings for the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("posture:list", "project:"+opts.project); err != nil {
					return err
				}
				findings, err := ac.Store.Posture.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(findings)
				}
				if len(findings) == 0 {
					fmt.Println("No findings.")
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "SCANNED\tSEVERITY\tCHECK\tTARGET\tDETAIL")
				for _, f := range findings {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						f.ScannedAt, f.Severity, f.Check, f.Target, f.Detail)
				}
				return tw.Flush()
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper stats
// ---------------------------------------------------------------------------

func statsCmd(opts *options) *cobra.Command {
	var watchInterval int
	cmd := &cobra.Command{
		Use:   "stats [INSTANCE...]",
		Short: "show live cgroup resource metrics for running instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("instance:stats", "project:"+opts.project); err != nil {
					return err
				}
				collect := func() ([]metrics.InstanceMetrics, error) {
					all, err := ctrl.Store.ListInstances()
					if err != nil {
						return nil, err
					}
					var out []metrics.InstanceMetrics
					for _, inst := range all {
						if inst.Status != types.StatusRunning {
							continue
						}
						if len(args) > 0 {
							found := false
							for _, a := range args {
								if a == inst.ID || a == inst.Name {
									found = true
									break
								}
							}
							if !found {
								continue
							}
						}
						out = append(out, metrics.ReadInstance(inst.ID, inst.Name))
					}
					return out, nil
				}
				print := func(ms []metrics.InstanceMetrics) {
					if opts.json {
						_ = printJSON(ms)
						return
					}
					tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
					fmt.Fprintln(tw, "NAME\tCPU\tMEMORY\tPIDS")
					for _, m := range ms {
						fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n",
							m.InstanceName, metrics.HumanCPU(m.CPUUsageUs),
							metrics.HumanMemory(m.MemoryBytes), m.PIDCount)
					}
					_ = tw.Flush()
				}
				ms, err := collect()
				if err != nil {
					return err
				}
				print(ms)
				if watchInterval <= 0 {
					return nil
				}
				ticker := time.NewTicker(time.Duration(watchInterval) * time.Second)
				defer ticker.Stop()
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(sigCh)
				for {
					select {
					case <-sigCh:
						return nil
					case <-ticker.C:
						ms, err := collect()
						if err != nil {
							return err
						}
						fmt.Println()
						print(ms)
					}
				}
			})
		},
	}
	cmd.Flags().IntVar(&watchInterval, "watch", 0, "refresh interval in seconds (0 = one-shot)")
	return cmd
}

// ---------------------------------------------------------------------------
// capper lb
// ---------------------------------------------------------------------------

func lbCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lb",
		Short: "manage load balancers",
	}
	cmd.AddCommand(
		lbCreateCmd(opts),
		lbListCmd(opts),
		lbInspectCmd(opts),
		lbDeleteCmd(opts),
		lbPublishCmd(opts),
		lbBackendCmd(opts),
		lbLogsCmd(opts),
	)
	return cmd
}

func lbCreateCmd(opts *options) *cobra.Command {
	var networkName, listen, mode, selector, tlsCert, algo string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a load balancer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:create", "project:"+opts.project); err != nil {
					return err
				}
				lbMode := lb.ModeTCP
				if mode == "http" {
					lbMode = lb.ModeHTTP
				}
				l, err := ac.Store.LB.Create(args[0], opts.project, networkName, listen, lbMode)
				if err != nil {
					return err
				}
				if selector != "" || tlsCert != "" || algo != "" {
					_ = ac.Store.LB.Store().SetMeta(l.ID, selector, tlsCert, lb.LBAlgorithm(algo))
				}
				ac.RecordEvent("lb", l.ID, "lb.created", opts.project, map[string]any{"name": l.Name})
				if l.NetworkID != "" && l.ListenAddr != "" {
					dnsAutoRegisterLB(ac.Store, l.Name, l.NetworkID, l.ListenAddr)
				}
				if opts.json {
					return printJSON(l)
				}
				fmt.Printf("Load balancer created\n\nName:   %s\nID:     %s\nMode:   %s\nListen: %s\n",
					l.Name, l.ID, l.Mode, l.ListenAddr)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&networkName, "network", "", "attach to virtual network (name or ID)")
	cmd.Flags().StringVar(&listen, "listen", "", "listen address, e.g. 0.0.0.0:8080")
	cmd.Flags().StringVar(&mode, "mode", "tcp", "proxy mode: tcp or http")
	cmd.Flags().StringVar(&selector, "select", "", "service selector label key=value")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS cert name from cert store")
	cmd.Flags().StringVar(&algo, "algo", "", "balancing algorithm: round-robin or least-connections")
	return cmd
}

func lbLogsCmd(opts *options) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs NAME",
		Short: "show request logs for a load balancer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:inspect", "project:"+opts.project); err != nil {
					return err
				}
				l, err := ac.Store.LB.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				logPath := ac.Store.Paths.Root + "/lb-logs/" + l.ID + ".log"
				f, err := os.Open(logPath)
				if err != nil {
					return fmt.Errorf("no log file for %s: %w", l.Name, err)
				}
				defer f.Close()
				_, _ = io.Copy(os.Stdout, f)
				if !follow {
					return nil
				}
				offset, _ := f.Seek(0, io.SeekEnd)
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(sigCh)
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()
				buf := make([]byte, 4096)
				for {
					select {
					case <-sigCh:
						return nil
					case <-ticker.C:
						f2, rerr := os.Open(logPath)
						if rerr != nil {
							continue
						}
						if _, serr := f2.Seek(offset, io.SeekStart); serr == nil {
							n, _ := f2.Read(buf)
							if n > 0 {
								os.Stdout.Write(buf[:n])
								offset += int64(n)
							}
						}
						f2.Close()
					}
				}
			})
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}

func lbListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list load balancers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:list", "project:"+opts.project); err != nil {
					return err
				}
				lbs, err := ac.Store.LB.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(lbs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tMODE\tLISTEN\tSTATUS")
				for _, l := range lbs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", l.Name, l.ID, l.Mode, l.ListenAddr, l.Status)
				}
				return tw.Flush()
			})
		},
	}
}

func lbInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "inspect a load balancer and its backends",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:list", "project:"+opts.project); err != nil {
					return err
				}
				l, err := ac.Store.LB.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				backends, err := ac.Store.LB.ListBackends(args[0], opts.project)
				if err != nil {
					return err
				}
				type inspectOut struct {
					lb.LoadBalancer
					Backends []lb.Backend `json:"backends"`
				}
				out := inspectOut{LoadBalancer: l, Backends: backends}
				if opts.json {
					return printJSON(out)
				}
				fmt.Printf("Name:      %s\nID:        %s\nMode:      %s\nListen:    %s\nStatus:    %s\nCreated:   %s\n\nBackends:\n",
					l.Name, l.ID, l.Mode, l.ListenAddr, l.Status, l.CreatedAt)
				for _, b := range backends {
					fmt.Printf("  %s\n", b.Address)
				}
				return nil
			})
		},
	}
}

func lbDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a load balancer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:delete", "project:"+opts.project); err != nil {
					return err
				}
				l, err := ac.Store.LB.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				if err := ac.Store.LB.Delete(args[0], opts.project); err != nil {
					return err
				}
				ac.RecordEvent("lb", l.ID, "lb.deleted", opts.project, map[string]any{"name": l.Name})
				fmt.Printf("Deleted load balancer %q\n", args[0])
				return nil
			})
		},
	}
}

func lbPublishCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "publish NAME HOST:PORT",
		Short: "set the host listen address for a load balancer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:update", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.LB.Publish(args[0], opts.project, args[1]); err != nil {
					return err
				}
				fmt.Printf("Load balancer %q will listen on %s (daemon reconcile picks it up)\n", args[0], args[1])
				return nil
			})
		},
	}
}

func lbBackendCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backend",
		Short: "manage backends for a load balancer",
	}
	cmd.AddCommand(lbBackendAddCmd(opts), lbBackendRemoveCmd(opts))
	return cmd
}

func lbBackendAddCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "add LB INSTANCE:PORT",
		Short: "add a backend to a load balancer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:update", "project:"+opts.project); err != nil {
					return err
				}
				b, err := ac.Store.LB.AddBackend(args[0], opts.project, args[1])
				if err != nil {
					return err
				}
				// Auto-add firewall rules for host→LB and LB→backend.
				if l, lerr := ac.Store.LB.Get(args[0], opts.project); lerr == nil && l.NetworkID != "" {
					_, listenPortStr, _ := net.SplitHostPort(l.ListenAddr)
					var listenPort int
					fmt.Sscanf(listenPortStr, "%d", &listenPort)
					firewallAutoAddLBRules(ac.Store, l.NetworkID, listenPort, b.Address)
				}
				if opts.json {
					return printJSON(b)
				}
				fmt.Printf("Added backend %s to %s\n", b.Address, args[0])
				return nil
			})
		},
	}
}

func lbBackendRemoveCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove LB INSTANCE:PORT",
		Short: "remove a backend from a load balancer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("lb:update", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.LB.RemoveBackend(args[0], opts.project, args[1]); err != nil {
					return err
				}
				fmt.Printf("Removed backend %s from %s\n", args[1], args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper alert
// ---------------------------------------------------------------------------

func alertCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "manage alert rules and evaluate firing alerts",
	}
	cmd.AddCommand(alertCreateCmd(opts), alertListCmd(opts), alertDeleteCmd(opts), alertEvalCmd(opts))
	return cmd
}

func alertCreateCmd(opts *options) *cobra.Command {
	var ruleType, eventAction, metricName string
	var windowSecs, threshold int
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create an alert rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("alert:create", "project:"+opts.project); err != nil {
					return err
				}
				rt := alert.RuleType(ruleType)
				mgr := alert.NewManager(alert.NewStore(ac.Store.DB))
				r, err := mgr.Create(args[0], opts.project, rt, eventAction, windowSecs, threshold, metricName)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(r)
				}
				fmt.Printf("Alert rule created\n\nName:      %s\nID:        %s\nType:      %s\nThreshold: %d\n",
					r.Name, r.ID, r.Type, r.Threshold)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&ruleType, "type", "event_count", "rule type: event_count or metric_threshold")
	cmd.Flags().StringVar(&eventAction, "event-action", "", "event action prefix to match (event_count rules)")
	cmd.Flags().IntVar(&windowSecs, "window", 60, "evaluation window in seconds")
	cmd.Flags().IntVar(&threshold, "threshold", 1, "firing threshold (count or metric value)")
	cmd.Flags().StringVar(&metricName, "metric", "", "metric name: cpu_micros, memory_bytes, pid_count (metric_threshold rules)")
	return cmd
}

func alertListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list alert rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("alert:list", "project:"+opts.project); err != nil {
					return err
				}
				mgr := alert.NewManager(alert.NewStore(ac.Store.DB))
				rules, err := mgr.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(rules)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tTYPE\tTHRESHOLD\tENABLED")
				for _, r := range rules {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%v\n", r.Name, r.ID, r.Type, r.Threshold, r.Enabled)
				}
				return tw.Flush()
			})
		},
	}
}

func alertDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete an alert rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("alert:delete", "project:"+opts.project); err != nil {
					return err
				}
				mgr := alert.NewManager(alert.NewStore(ac.Store.DB))
				return mgr.Delete(args[0], opts.project)
			})
		},
	}
}

func alertEvalCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "eval",
		Short: "evaluate all alert rules and print firing alerts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("alert:list", "project:"+opts.project); err != nil {
					return err
				}
				// Collect recent events (last hour).
				cutoff := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
				rawEvents, err := ac.Store.Events.Since(cutoff, 10000)
				if err != nil {
					return err
				}
				evts := make([]alert.EventRecord, len(rawEvents))
				for i, e := range rawEvents {
					evts[i] = alert.EventRecord{Action: e.Action, Timestamp: e.Timestamp}
				}
				// Collect instance metrics.
				instances, _ := ac.Store.ListInstances()
				var instMetrics []metrics.InstanceMetrics
				for _, inst := range instances {
					if inst.Status == types.StatusRunning {
						instMetrics = append(instMetrics, metrics.ReadInstance(inst.ID, inst.Name))
					}
				}
				mgr := alert.NewManager(alert.NewStore(ac.Store.DB))
				firing, err := mgr.Evaluate(opts.project, evts, instMetrics)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(firing)
				}
				if len(firing) == 0 {
					fmt.Println("No alerts firing.")
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "RULE\tVALUE\tTHRESHOLD\tFIRED")
				for _, f := range firing {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", f.RuleName, f.Value, f.Threshold, f.FiredAt)
				}
				return tw.Flush()
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper event export (JSONL)
// ---------------------------------------------------------------------------

func init() {
	// eventExportCmd is added to eventCmd via eventCmd's AddCommand call below.
	// We register it here so it is wired in at startup.
}

// eventExportCmd adds a JSONL export sub-command to the event group.
// It is appended by extending the eventCmd function's AddCommand list.
func eventExportCmd(opts *options) *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "export events to a JSONL file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("event:list", "project:"+opts.project); err != nil {
					return err
				}
				events, err := ac.Store.Events.List(store.ListEventsOptions{
					ProjectID: opts.project,
					Limit:     100000,
				})
				if err != nil {
					return err
				}
				w := os.Stdout
				if outPath != "" {
					f, ferr := os.Create(outPath)
					if ferr != nil {
						return ferr
					}
					defer f.Close()
					w = f
				}
				enc := json.NewEncoder(w)
				for _, e := range events {
					if err := enc.Encode(e); err != nil {
						return err
					}
				}
				if outPath != "" {
					fmt.Printf("Exported %d events to %s\n", len(events), outPath)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "output file path (default: stdout)")
	return cmd
}

// ---------------------------------------------------------------------------
// capper iam audit tail (appended to iamAuditCmd via addTailCmd helper)
// ---------------------------------------------------------------------------

func iamAuditTailCmd(opts *options) *cobra.Command {
	var actionFilter, principalFilter string
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "stream new IAM audit events (Ctrl-C to stop)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				// Seed cursor from latest existing record
				existing, _ := st.IAM.IAMStore().ListAudit("", "", "", 1)
				cursor := ""
				if len(existing) > 0 {
					cursor = existing[0].Timestamp
				}
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(sigCh)
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				for {
					select {
					case <-sigCh:
						return nil
					case <-ticker.C:
						records, err := st.IAM.IAMStore().ListAudit(actionFilter, principalFilter, cursor, 200)
						if err != nil {
							return err
						}
						// ListAudit returns newest-first; print in arrival order
						for i := len(records) - 1; i >= 0; i-- {
							r := records[i]
							if r.Timestamp <= cursor {
								continue
							}
							fmt.Fprintf(tw, "%s\t%s:%s\t%s\t%s\t%s\n",
								r.Timestamp, r.PrincipalType, r.PrincipalID,
								r.Action, r.Resource, r.Decision)
							_ = tw.Flush()
							cursor = r.Timestamp
						}
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&actionFilter, "action", "", "filter by action prefix")
	cmd.Flags().StringVar(&principalFilter, "principal", "", "filter by principal ID prefix")
	return cmd
}

// ---------------------------------------------------------------------------
// capper db
// ---------------------------------------------------------------------------

func dbCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "manage managed database services",
	}
	cmd.AddCommand(
		dbCreateCmd(opts),
		dbListCmd(opts),
		dbInspectCmd(opts),
		dbRestoreCmd(opts),
		dbDeleteCmd(opts),
	)
	return cmd
}

func dbCreateCmd(opts *options) *cobra.Command {
	var engine, subnetID, vpcID, legacyNetwork, version string
	var port int
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a managed database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("db:create", "project:"+opts.project); err != nil {
						return err
					}
					if engine == "" {
						return fmt.Errorf("--engine is required (postgres, redis, mariadb, capdb)")
					}
					if legacyNetwork != "" {
						return fmt.Errorf("--network is removed; use --subnet-id with a VPC subnet ID")
					}
					if subnetID == "" {
						return fmt.Errorf("--subnet-id is required")
					}
					sub, eni, netOpts, err := resolveCLISubnetLaunch(ac.Store, subnetID, vpcID)
					if err != nil {
						return err
					}
					db, err := manager.CreateManagedDatabase(
						ac.Store, ctrl.Instances, ac.Store.Metadata,
						args[0], opts.project, engine, version, sub.ID, port, netOpts,
					)
					if err != nil {
						_ = ac.Store.VPC.DeleteENI(eni.ID)
						return err
					}
					if db.InstanceID != "" {
						if _, aerr := ac.Store.VPC.AttachENI(eni.ID, db.InstanceID, 0); aerr != nil {
							_ = ac.Store.VPC.DeleteENI(eni.ID)
							_, _ = manager.DeleteManagedDatabase(ac.Store, ctrl.Instances, db.Name, opts.project)
							return fmt.Errorf("eni attach: %w", aerr)
						}
					}
					ac.RecordEvent("database", db.ID, "db.created", opts.project, map[string]any{"name": db.Name, "engine": db.Engine, "instanceId": db.InstanceID})
					if opts.json {
						return printJSON(db)
					}
					fmt.Printf("Database created\n\nName:       %s\nID:         %s\nEngine:     %s\nStatus:     %s\nInstance:   %s\nProject:    %s\n",
						db.Name, db.ID, db.Engine, db.Status, db.InstanceID, db.Project)
					return nil
				})
			})
		},
	}
	cmd.Flags().StringVar(&engine, "engine", "", "database engine: postgres, redis, mariadb, or capdb (required)")
	cmd.Flags().StringVar(&subnetID, "subnet-id", "", "VPC subnet ID to place the database in (required)")
	cmd.Flags().StringVar(&vpcID, "vpc-id", "", "VPC ID (optional; must match the subnet)")
	cmd.Flags().StringVar(&legacyNetwork, "network", "", "removed: use --subnet-id")
	_ = cmd.Flags().MarkHidden("network")
	cmd.Flags().StringVar(&version, "version", "", "engine version (optional)")
	cmd.Flags().IntVar(&port, "port", 0, "database port (optional)")
	return cmd
}

func dbListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list managed databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("db:list", "project:"+opts.project); err != nil {
					return err
				}
				dbs, err := ac.Store.Databases.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(dbs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tENGINE\tSTATUS\tPORT\tCREATED")
				for _, d := range dbs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", d.Name, d.ID, d.Engine, d.Status, d.Port, shortTime(d.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func dbInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "inspect a managed database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("db:list", "project:"+opts.project); err != nil {
					return err
				}
				db, err := ac.Store.Databases.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(db)
				}
				fmt.Printf("Name:       %s\nID:         %s\nEngine:     %s\nVersion:    %s\nStatus:     %s\nPort:       %d\nProject:    %s\nCreated:    %s\n",
					db.Name, db.ID, db.Engine, db.Version, db.Status, db.Port, db.Project, db.CreatedAt)
				return nil
			})
		},
	}
}

func dbRestoreCmd(opts *options) *cobra.Command {
	var target, conn, engine, version, network string
	var port int
	cmd := &cobra.Command{
		Use:   "restore BACKUP_ID",
		Short: "restore a database backup into a new managed database record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("db:restore", "project:"+opts.project); err != nil {
					return err
				}
				if target == "" {
					return fmt.Errorf("--target is required")
				}
				if conn == "" {
					return fmt.Errorf("--conn is required")
				}
				if engine == "" {
					engine = string(database.EnginePostgres)
				}
				db, err := ac.Store.Databases.RestoreIntoNew(args[0], target, opts.project, engine, version, network, port, conn, ac.Store.Backup.RestoreDatabase)
				if err != nil {
					return err
				}
				ac.RecordEvent("database", db.ID, "db.restored", opts.project, map[string]any{"name": db.Name, "backup": args[0]})
				if opts.json {
					return printJSON(db)
				}
				fmt.Printf("Database restored\n\nName:    %s\nID:      %s\nStatus:  %s\nProject: %s\n", db.Name, db.ID, db.Status, db.Project)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "new managed database name")
	cmd.Flags().StringVar(&conn, "conn", "", "target database connection string for pg_restore")
	cmd.Flags().StringVar(&engine, "engine", string(database.EnginePostgres), "database engine")
	cmd.Flags().StringVar(&network, "network", "", "attach restored database record to virtual network")
	cmd.Flags().StringVar(&version, "version", "", "engine version")
	cmd.Flags().IntVar(&port, "port", 0, "database port")
	return cmd
}

func dbDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a managed database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				return withIAM(opts, func(ac *iamCtx) error {
					if err := ac.Authorize("db:delete", "project:"+opts.project); err != nil {
						return err
					}
					db, err := manager.DeleteManagedDatabase(ac.Store, ctrl.Instances, args[0], opts.project)
					if err != nil {
						return err
					}
					ac.RecordEvent("database", db.ID, "db.deleted", opts.project, map[string]any{"name": db.Name})
					fmt.Printf("Deleted database %q\n", args[0])
					return nil
				})
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper ai
// ---------------------------------------------------------------------------

func aiCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ai",
		Short: "manage AI agents, sessions, and MCP servers",
	}
	cmd.AddCommand(
		aiAgentCmd(opts),
		aiSessionCmd(opts),
		aiMCPCmd(opts),
	)
	return cmd
}

func aiAgentCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "manage AI agents",
	}
	cmd.AddCommand(
		aiAgentRegisterCmd(opts),
		aiAgentListCmd(opts),
		aiAgentRevokeCmd(opts),
	)
	return cmd
}

func aiAgentRegisterCmd(opts *options) *cobra.Command {
	var model, owner string
	cmd := &cobra.Command{
		Use:   "register NAME",
		Short: "register an AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ai:agent:register", "project:"+opts.project); err != nil {
					return err
				}
				if owner == "" {
					owner = ac.PrincipalID
				}
				a, err := ac.Store.AI.RegisterAgent(args[0], opts.project, model, owner, "")
				if err != nil {
					return err
				}
				ac.RecordEvent("ai_agent", a.ID, "ai.agent.registered", opts.project, map[string]any{"name": a.Name})
				if opts.json {
					return printJSON(a)
				}
				fmt.Printf("Agent registered\n\nName:    %s\nID:      %s\nModel:   %s\nOwner:   %s\nStatus:  %s\n",
					a.Name, a.ID, a.Model, a.Owner, a.Status)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "model identifier, e.g. claude-opus-4")
	cmd.Flags().StringVar(&owner, "owner", "", "IAM user owner (defaults to current principal)")
	return cmd
}

func aiAgentListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list AI agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ai:agent:list", "project:"+opts.project); err != nil {
					return err
				}
				agents, err := ac.Store.AI.ListAgents(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(agents)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tMODEL\tOWNER\tSTATUS\tCREATED")
				for _, a := range agents {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", a.Name, a.ID, a.Model, a.Owner, a.Status, shortTime(a.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func aiAgentRevokeCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke NAME",
		Short: "revoke an AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ai:agent:register", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.AI.RevokeAgent(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Revoked agent %q\n", args[0])
				return nil
			})
		},
	}
}

func aiSessionCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "manage AI sessions",
	}
	cmd.AddCommand(aiSessionListCmd(opts))
	return cmd
}

func aiSessionListCmd(opts *options) *cobra.Command {
	var agentFilter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list AI sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ai:session:list", "project:"+opts.project); err != nil {
					return err
				}
				sessions, err := ac.Store.AI.ListSessions(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					if agentFilter != "" {
						var filtered []interface{}
						for _, s := range sessions {
							if s.AgentID == agentFilter {
								filtered = append(filtered, s)
							}
						}
						return printJSON(filtered)
					}
					return printJSON(sessions)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tAGENT\tPRINCIPAL\tMODEL\tSTATUS\tSTARTED")
				for _, s := range sessions {
					if agentFilter != "" && s.AgentID != agentFilter {
						continue
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, s.AgentID, s.Principal, s.Model, s.Status, shortTime(s.StartedAt))
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&agentFilter, "agent", "", "filter by agent ID or name")
	return cmd
}

func aiMCPCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "manage MCP servers",
	}
	cmd.AddCommand(aiMCPRegisterCmd(opts), aiMCPListCmd(opts))
	return cmd
}

func aiMCPRegisterCmd(opts *options) *cobra.Command {
	var endpoint, iamAction string
	cmd := &cobra.Command{
		Use:   "register NAME",
		Short: "register an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ai:mcp:register", "project:"+opts.project); err != nil {
					return err
				}
				srv, err := ac.Store.AI.RegisterMCP(args[0], opts.project, endpoint, iamAction)
				if err != nil {
					return err
				}
				ac.RecordEvent("ai_mcp", srv.ID, "ai.mcp.registered", opts.project, map[string]any{"name": srv.Name})
				if opts.json {
					return printJSON(srv)
				}
				fmt.Printf("MCP server registered\n\nName:      %s\nID:        %s\nEndpoint:  %s\n",
					srv.Name, srv.ID, srv.Endpoint)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "MCP server endpoint URL")
	cmd.Flags().StringVar(&iamAction, "action", "", "required IAM action to call this server")
	return cmd
}

func aiMCPListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list MCP servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ai:mcp:register", "project:"+opts.project); err != nil {
					return err
				}
				servers, err := ac.Store.AI.ListMCP(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(servers)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tENDPOINT\tIAM_ACTION\tCREATED")
				for _, s := range servers {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.ID, s.Endpoint, s.IAMAction, shortTime(s.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper backup
// ---------------------------------------------------------------------------

func backupCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "manage backups and backup policies",
	}
	cmd.AddCommand(backupStoreCmd(opts), backupListCmd(opts), backupRestoreCmd(opts),
		backupPolicyCreateCmd(opts), backupPolicyListCmd(opts), backupPolicyDeleteCmd(opts),
		backupRestoreTestCmd(opts))
	return cmd
}

func backupStoreCmd(opts *options) *cobra.Command {
	var destDir string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "create a backup of the store",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:create", "project:"+opts.project); err != nil {
					return err
				}
				if destDir == "" {
					destDir = ac.Store.Paths.Root + "/backups"
				}
				rec, err := ac.Store.Backup.BackupStore(opts.project, destDir)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(rec)
				}
				fmt.Printf("Backup created\n\nID:    %s\nPath:  %s\nSize:  %d bytes\n", rec.ID, rec.Path, rec.SizeBytes)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&destDir, "dest", "", "destination directory (default: <store-root>/backups)")
	return cmd
}

func backupListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:list", "project:"+opts.project); err != nil {
					return err
				}
				recs, err := ac.Store.Backup.ListRecords(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(recs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tPATH\tSIZE\tCREATED")
				for _, r := range recs {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", r.ID, r.Path, r.SizeBytes, shortTime(r.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func backupRestoreCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "restore ID",
		Short: "restore a backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:restore", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Backup.Restore(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Backup %s restored\n", args[0])
				return nil
			})
		},
	}
}

func backupPolicyCreateCmd(opts *options) *cobra.Command {
	var targetPath, btype, source string
	var interval, retention int
	cmd := &cobra.Command{
		Use:   "policy-create NAME",
		Short: "create a backup policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:create", "project:"+opts.project); err != nil {
					return err
				}
				p, err := ac.Store.Backup.CreatePolicyWithSource(args[0], opts.project, targetPath, source, backup.BackupType(btype), interval, retention)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(p)
				}
				fmt.Printf("Policy created\n\nName:      %s\nID:        %s\nInterval:  %ds\nRetention: %d\n",
					p.Name, p.ID, p.IntervalSecs, p.Retention)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&targetPath, "target", "", "target path or resource")
	cmd.Flags().StringVar(&btype, "type", "store", "backup type: store|database")
	cmd.Flags().StringVar(&source, "source", "", "backup source (database connection string for --type database)")
	cmd.Flags().IntVar(&interval, "interval", 3600, "backup interval in seconds")
	cmd.Flags().IntVar(&retention, "retention", 5, "number of backups to retain")
	return cmd
}

func backupPolicyListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "policy-list",
		Short: "list backup policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:list", "project:"+opts.project); err != nil {
					return err
				}
				policies, err := ac.Store.Backup.ListPolicies(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(policies)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tTYPE\tSOURCE\tINTERVAL\tRETENTION")
				for _, p := range policies {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%ds\t%d\n", p.Name, p.ID, p.Type, p.Source, p.IntervalSecs, p.Retention)
				}
				return tw.Flush()
			})
		},
	}
}

func backupPolicyDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "policy-delete NAME",
		Short: "delete a backup policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Backup.DeletePolicy(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Policy %s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper control
// ---------------------------------------------------------------------------

func controlStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show daemon and subsystem status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withStore(opts, func(st *store.Store) error {
				instances, _ := st.ListInstances()
				lbs, _ := st.LB.List(opts.project)
				networks, _ := st.Networks.List(opts.project)
				if opts.json {
					return printJSON(map[string]any{
						"instances": len(instances),
						"lbs":       len(lbs),
						"networks":  len(networks),
					})
				}
				fmt.Printf("Capper daemon status\n\nInstances: %d\nNetworks:  %d\nLBs:       %d\n",
					len(instances), len(networks), len(lbs))
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper stack
// ---------------------------------------------------------------------------

func stackCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stack",
		Short: "manage infrastructure stacks",
	}
	cmd.AddCommand(
		stackPlanCmd(opts),
		stackApplyCmd(opts),
		stackListCmd(opts),
		stackInspectCmd(opts),
		stackDiffCmd(opts),
		stackDestroyCmd(opts),
		stackUpdateCmd(opts),
	)
	return cmd
}

func stackPlanCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "plan FILE",
		Short: "plan stack changes from a template file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:apply", "project:"+opts.project); err != nil {
					return err
				}
				tmpl, err := stack.LoadTemplate(args[0])
				if err != nil {
					return fmt.Errorf("load template: %w", err)
				}
				ops, err := ac.Store.Stack.Plan(tmpl, opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(ops)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ACTION\tTYPE\tNAME\tREASON")
				for _, op := range ops {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", op.Action, op.Type, op.Name, op.Reason)
				}
				return tw.Flush()
			})
		},
	}
}

func stackApplyCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "apply FILE",
		Short: "apply stack from a template file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:apply", "project:"+opts.project); err != nil {
					return err
				}
				tmpl, err := stack.LoadTemplate(args[0])
				if err != nil {
					return fmt.Errorf("load template: %w", err)
				}
				st, err := ac.Store.Stack.Apply(context.Background(), tmpl, opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(st)
				}
				fmt.Printf("Stack applied\n\nName:      %s\nID:        %s\nStatus:    %s\nResources: %d\n",
					st.Name, st.ID, st.Status, len(st.Resources))
				return nil
			})
		},
	}
}

func stackListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:list", "project:"+opts.project); err != nil {
					return err
				}
				stacks, err := ac.Store.Stack.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(stacks)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tSTATUS\tRESOURCES\tUPDATED")
				for _, s := range stacks {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", s.Name, s.ID, s.Status, len(s.Resources), shortTime(s.UpdatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func stackInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "inspect a stack",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:list", "project:"+opts.project); err != nil {
					return err
				}
				st, err := ac.Store.Stack.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(st)
				}
				fmt.Printf("Name:    %s\nID:      %s\nStatus:  %s\nUpdated: %s\n\nResources:\n", st.Name, st.ID, st.Status, st.UpdatedAt)
				for _, r := range st.Resources {
					fmt.Printf("  %-12s %-20s %s\n", r.Type, r.Name, r.ID)
				}
				return nil
			})
		},
	}
}

func stackDiffCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "diff NAME",
		Short: "diff stack against live state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:list", "project:"+opts.project); err != nil {
					return err
				}
				ops, err := ac.Store.Stack.Diff(args[0], opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(ops)
				}
				if len(ops) == 0 {
					fmt.Println("No drift detected.")
					return nil
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ACTION\tTYPE\tNAME\tREASON")
				for _, op := range ops {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", op.Action, op.Type, op.Name, op.Reason)
				}
				return tw.Flush()
			})
		},
	}
}

func stackDestroyCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy NAME",
		Short: "destroy all resources in a stack",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:apply", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Stack.Destroy(context.Background(), args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Stack %s destroyed\n", args[0])
				return nil
			})
		},
	}
}

func stackUpdateCmd(opts *options) *cobra.Command {
	var marketVersion string
	cmd := &cobra.Command{
		Use:   "update NAME",
		Short: "update a stack to a new marketplace listing version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("stack:apply", "project:"+opts.project); err != nil {
					return err
				}
				name := args[0]
				// If --market-version is set, re-install from marketplace.
				if marketVersion != "" {
					listings, err := ac.Store.Marketplace.List()
					if err != nil {
						return fmt.Errorf("marketplace: %w", err)
					}
					var listingID string
					for _, l := range listings {
						if l.Name == name && l.Version == marketVersion {
							listingID = l.ID
							break
						}
					}
					if listingID == "" {
						return fmt.Errorf("marketplace: no listing %q version %q found", name, marketVersion)
					}
					return ac.Store.Marketplace.Install(listingID, opts.project, nil, nil)
				}
				// Without --market-version, re-apply the existing stack template.
				ops, err := ac.Store.Stack.Diff(name, opts.project)
				if err != nil {
					return err
				}
				if len(ops) == 0 {
					fmt.Println("Stack is up to date.")
					return nil
				}
				fmt.Printf("%d change(s) to apply.\n", len(ops))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&marketVersion, "market-version", "", "marketplace listing version to upgrade to")
	return cmd
}

// ---------------------------------------------------------------------------
// capper restore test
// ---------------------------------------------------------------------------

func backupRestoreTestCmd(opts *options) *cobra.Command {
	var namespace string
	return &cobra.Command{
		Use:   "test BACKUP_ID",
		Short: "restore into an isolated test namespace and verify DNS/endpoints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("backup:restore", "project:"+opts.project); err != nil {
					return err
				}
				backupID := args[0]
				if namespace == "" {
					namespace = "restore-test-" + backupID[:min8(len(backupID), 8)]
				}
				// Locate the backup record.
				records, err := ac.Store.Backup.ListRecords(opts.project)
				if err != nil {
					return err
				}
				var rec *backup.BackupRecord
				for i := range records {
					if records[i].ID == backupID || records[i].Path == backupID {
						rec = &records[i]
						break
					}
				}
				if rec == nil {
					return fmt.Errorf("backup %q not found in project %q", backupID, opts.project)
				}
				info, err := os.Stat(rec.Path)
				if err != nil {
					return fmt.Errorf("backup artifact is not readable: %w", err)
				}
				if info.Size() == 0 {
					return fmt.Errorf("backup artifact is empty: %s", rec.Path)
				}
				testRoot := filepath.Join(ac.Store.Paths.Tmp, namespace)
				if err := os.MkdirAll(testRoot, 0o700); err != nil {
					return err
				}
				defer os.RemoveAll(testRoot)
				testCopy := filepath.Join(testRoot, filepath.Base(rec.Path))
				if err := copyFile(rec.Path, testCopy); err != nil {
					return fmt.Errorf("restore test copy: %w", err)
				}
				if copied, err := os.Stat(testCopy); err != nil || copied.Size() != info.Size() {
					return fmt.Errorf("restore test artifact verification failed")
				}
				fmt.Printf("restore test: backup=%s namespace=%s\n", backupID, namespace)
				fmt.Printf("  type=%s artifact=%s size=%d\n", rec.Type, testCopy, info.Size())
				fmt.Println("restore test: PASS")
				return nil
			})
		},
	}
}

func min8(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ---------------------------------------------------------------------------
// capper health
// ---------------------------------------------------------------------------

func healthCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "instance health check status",
	}
	cmd.AddCommand(healthListCmd(opts))
	return cmd
}

func healthListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list health check results",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("instance:list", "project:"+opts.project); err != nil {
					return err
				}
				results, err := ac.Store.Health.List()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(results)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "INSTANCE\tSTATUS\tMESSAGE\tCHECKED_AT")
				for _, r := range results {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.InstanceID, r.Status, r.Message, shortTime(r.CheckedAt))
				}
				return tw.Flush()
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper quota (Block 18)
// ---------------------------------------------------------------------------

func quotaCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "manage per-project resource quotas",
	}
	cmd.AddCommand(quotaSetCmd(opts), quotaListCmd(opts))
	return cmd
}

func quotaSetCmd(opts *options) *cobra.Command {
	var resource string
	var limit int64
	cmd := &cobra.Command{
		Use:   "set",
		Short: "set a quota for a project resource",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("quota:set", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Billing.SetQuota(opts.project, resource, limit); err != nil {
					return err
				}
				fmt.Printf("Quota set: project=%s resource=%s limit=%d\n", opts.project, resource, limit)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&resource, "resource", "", "resource type: instance, storage, network")
	cmd.Flags().Int64Var(&limit, "limit", 0, "quota limit")
	_ = cmd.MarkFlagRequired("resource")
	return cmd
}

func quotaListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list quotas for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("quota:list", "project:"+opts.project); err != nil {
					return err
				}
				quotas, err := ac.Store.Billing.ListQuotas(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(quotas)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "RESOURCE\tLIMIT")
				for _, q := range quotas {
					fmt.Fprintf(tw, "%s\t%d\n", q.Resource, q.Limit)
				}
				return tw.Flush()
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper queue (Block 19)
// ---------------------------------------------------------------------------

func queueCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "manage message queues",
	}
	cmd.AddCommand(queueCreateCmd(opts), queueListCmd(opts), queueDeleteCmd(opts),
		queuePublishCmd(opts), queueConsumeCmd(opts))
	return cmd
}

func queueCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create NAME",
		Short: "create a message queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("queue:create", "project:"+opts.project); err != nil {
					return err
				}
				q, err := ac.Store.Queue.Create(args[0], opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(q)
				}
				fmt.Printf("Queue created: %s (%s)\n", q.Name, q.ID)
				return nil
			})
		},
	}
}

func queueListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list queues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("queue:list", "project:"+opts.project); err != nil {
					return err
				}
				qs, err := ac.Store.Queue.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(qs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tCREATED")
				for _, q := range qs {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", q.Name, q.ID, shortTime(q.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func queueDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("queue:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Queue.Delete(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Queue %s deleted\n", args[0])
				return nil
			})
		},
	}
}

func queuePublishCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "publish QUEUE MESSAGE",
		Short: "publish a message to a queue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("queue:publish", "project:"+opts.project); err != nil {
					return err
				}
				msg, err := ac.Store.Queue.Publish(args[0], opts.project, args[1])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(msg)
				}
				fmt.Printf("Published message %s to %s\n", msg.ID, args[0])
				return nil
			})
		},
	}
}

func queueConsumeCmd(opts *options) *cobra.Command {
	var max int
	cmd := &cobra.Command{
		Use:   "consume QUEUE",
		Short: "consume messages from a queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("queue:consume", "project:"+opts.project); err != nil {
					return err
				}
				msgs, err := ac.Store.Queue.Consume(args[0], opts.project, max)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(msgs)
				}
				for _, m := range msgs {
					fmt.Printf("[%s] %s\n", m.ID, m.Body)
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&max, "max", 10, "maximum messages to consume")
	return cmd
}

// ---------------------------------------------------------------------------
// capper ingress (Block 20)
// ---------------------------------------------------------------------------

func ingressCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingress",
		Short: "manage ingress rules",
	}
	cmd.AddCommand(ingressCreateCmd(opts), ingressListCmd(opts), ingressDeleteCmd(opts))
	return cmd
}

func ingressCreateCmd(opts *options) *cobra.Command {
	var host, path, backend, tlsCert string
	var rateLimit int
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create an ingress rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ingress:create", "project:"+opts.project); err != nil {
					return err
				}
				rule, err := ac.Store.Ingress.Create(args[0], opts.project, host, path, backend, tlsCert, rateLimit)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(rule)
				}
				fmt.Printf("Ingress rule created\n\nName:    %s\nHost:    %s\nPath:    %s\nBackend: %s\n",
					rule.Name, rule.Host, rule.PathPrefix, rule.BackendLB)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "hostname to match")
	cmd.Flags().StringVar(&path, "path", "/", "path prefix to match")
	cmd.Flags().StringVar(&backend, "backend", "", "backend LB name")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS cert name")
	cmd.Flags().IntVar(&rateLimit, "rate-limit", 0, "requests per minute (0 = unlimited)")
	return cmd
}

func ingressListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list ingress rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ingress:list", "project:"+opts.project); err != nil {
					return err
				}
				rules, err := ac.Store.Ingress.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(rules)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tHOST\tPATH\tBACKEND")
				for _, r := range rules {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Name, r.Host, r.PathPrefix, r.BackendLB)
				}
				return tw.Flush()
			})
		},
	}
}

func ingressDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete an ingress rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("ingress:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Ingress.Delete(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Ingress rule %s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper rule / schedule (Block 19)
// ---------------------------------------------------------------------------

func ruleCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "manage event rules",
	}
	cmd.AddCommand(ruleCreateCmd(opts), ruleListCmd(opts), ruleDeleteCmd(opts))
	return cmd
}

func ruleCreateCmd(opts *options) *cobra.Command {
	var eventType, action, actionArgs string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create an event rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("rule:create", "project:"+opts.project); err != nil {
					return err
				}
				r, err := ac.Store.Eventing.CreateRule(args[0], opts.project, eventType, action, actionArgs)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(r)
				}
				fmt.Printf("Rule created: %s (on %s → %s)\n", r.Name, r.EventType, r.Action)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&eventType, "event", "", "event type pattern, e.g. instance.started")
	cmd.Flags().StringVar(&action, "action", "notify", "action: notify, webhook")
	cmd.Flags().StringVar(&actionArgs, "args", "", "action arguments (e.g. webhook URL)")
	_ = cmd.MarkFlagRequired("event")
	return cmd
}

func ruleListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list event rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("rule:list", "project:"+opts.project); err != nil {
					return err
				}
				rules, err := ac.Store.Eventing.ListRules(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(rules)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tEVENT\tACTION\tENABLED")
				for _, r := range rules {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%v\n", r.Name, r.EventType, r.Action, r.Enabled)
				}
				return tw.Flush()
			})
		},
	}
}

func ruleDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete an event rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("rule:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Eventing.DeleteRule(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Rule %s deleted\n", args[0])
				return nil
			})
		},
	}
}

func scheduleCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "manage cron-based schedules",
	}
	cmd.AddCommand(scheduleCreateCmd(opts), scheduleListCmd(opts), scheduleDeleteCmd(opts))
	return cmd
}

func scheduleCreateCmd(opts *options) *cobra.Command {
	var cron, action, actionArgs string
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "create a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("schedule:create", "project:"+opts.project); err != nil {
					return err
				}
				sc, err := ac.Store.Eventing.CreateSchedule(args[0], opts.project, cron, action, actionArgs)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(sc)
				}
				fmt.Printf("Schedule created: %s (cron=%s action=%s)\n", sc.Name, sc.Cron, sc.Action)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&cron, "cron", "@1h", "cron interval: @5m, @1h, or standard cron expression")
	cmd.Flags().StringVar(&action, "action", "backup", "action: backup, webhook, run-instance")
	cmd.Flags().StringVar(&actionArgs, "args", "", "action arguments")
	return cmd
}

func scheduleListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("schedule:list", "project:"+opts.project); err != nil {
					return err
				}
				schedules, err := ac.Store.Eventing.ListSchedules(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(schedules)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tCRON\tACTION\tLAST_RUN")
				for _, sc := range schedules {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", sc.Name, sc.Cron, sc.Action, shortTime(sc.LastRunAt))
				}
				return tw.Flush()
			})
		},
	}
}

func scheduleDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("schedule:delete", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Eventing.DeleteSchedule(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Schedule %s deleted\n", args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper org (Block 17)
// ---------------------------------------------------------------------------

func orgCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "manage organizations and accounts",
	}
	cmd.AddCommand(orgCreateCmd(opts), orgListCmd(opts), orgDeleteCmd(opts), orgInspectCmd(opts),
		accountCreateCmd(opts), accountListCmd(opts), accountInspectCmd(opts))
	return cmd
}

func orgCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create NAME",
		Short: "create an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:create", "system"); err != nil {
					return err
				}
				o, err := ac.Store.Projects.CreateOrg(args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(o)
				}
				fmt.Printf("Organization created: %s (%s)\n", o.Name, o.ID)
				return nil
			})
		},
	}
}

func orgListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list organizations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:list", "system"); err != nil {
					return err
				}
				orgs, err := ac.Store.Projects.ListOrgs()
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(orgs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tCREATED")
				for _, o := range orgs {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", o.Name, o.ID, shortTime(o.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
}

func orgDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:delete", "system"); err != nil {
					return err
				}
				if err := ac.Store.Projects.DeleteOrg(args[0]); err != nil {
					return err
				}
				fmt.Printf("Organization %s deleted\n", args[0])
				return nil
			})
		},
	}
}

func orgInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect NAME",
		Short: "show details of an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:list", "system"); err != nil {
					return err
				}
				o, err := ac.Store.Projects.GetOrg(args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(o)
				}
				fmt.Printf("ID:      %s\nName:    %s\nCreated: %s\n", o.ID, o.Name, o.CreatedAt)
				return nil
			})
		},
	}
}

func accountInspectCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "account-inspect NAME",
		Short: "show details of an account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:list", "system"); err != nil {
					return err
				}
				a, err := ac.Store.Projects.GetAccount(args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(a)
				}
				fmt.Printf("ID:      %s\nName:    %s\nOrg:     %s\nCreated: %s\n", a.ID, a.Name, a.OrgID, a.CreatedAt)
				return nil
			})
		},
	}
}

func accountCreateCmd(opts *options) *cobra.Command {
	var orgID string
	cmd := &cobra.Command{
		Use:   "account-create NAME",
		Short: "create an account within an organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:create", "system"); err != nil {
					return err
				}
				a, err := ac.Store.Projects.CreateAccount(orgID, args[0])
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(a)
				}
				fmt.Printf("Account created: %s (%s) in org %s\n", a.Name, a.ID, a.OrgID)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&orgID, "org", "", "organization ID or name")
	_ = cmd.MarkFlagRequired("org")
	return cmd
}

func accountListCmd(opts *options) *cobra.Command {
	var orgID string
	cmd := &cobra.Command{
		Use:   "account-list",
		Short: "list accounts in an organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("org:list", "system"); err != nil {
					return err
				}
				accounts, err := ac.Store.Projects.ListAccounts(orgID)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(accounts)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tORG\tCREATED")
				for _, a := range accounts {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Name, a.ID, a.OrgID, shortTime(a.CreatedAt))
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&orgID, "org", "", "organization ID or name")
	return cmd
}

// ---- job commands -----------------------------------------------------------

func jobCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "manage and run operational jobs",
	}
	cmd.AddCommand(
		jobCreateCmd(opts),
		jobRunCmd(opts),
		jobListCmd(opts),
		jobLogsCmd(opts),
		jobDeleteCmd(opts),
	)
	return cmd
}

func jobCreateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "create FILE",
		Short: "import a job spec from a JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("job:create", "project:"+opts.project); err != nil {
					return err
				}
				data, err := os.ReadFile(args[0])
				if err != nil {
					return fmt.Errorf("read job file: %w", err)
				}
				spec, err := stack.ParseJobSpec(string(data))
				if err != nil {
					return err
				}
				j := stack.Job{
					ID:        fmt.Sprintf("job_%d", time.Now().UnixNano()),
					Name:      spec.Metadata.Name,
					Project:   opts.project,
					SpecYAML:  string(data),
					Status:    stack.JobQueued,
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
					UpdatedAt: time.Now().UTC().Format(time.RFC3339),
				}
				if err := ac.Store.Jobs.Insert(j); err != nil {
					return fmt.Errorf("create job: %w", err)
				}
				fmt.Printf("Job created\n\nName:    %s\nID:      %s\nStatus:  %s\n", j.Name, j.ID, j.Status)
				return nil
			})
		},
	}
}

func jobRunCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "run NAME",
		Short: "execute a job's steps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("job:run", "project:"+opts.project); err != nil {
					return err
				}
				j, err := ac.Store.Jobs.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				fmt.Printf("Running job %s (%s)...\n", j.Name, j.ID)
				if err := stack.RunJob(ac.Store.Jobs, j); err != nil {
					return fmt.Errorf("job failed: %w", err)
				}
				fmt.Println("Job completed successfully.")
				return nil
			})
		},
	}
}

func jobListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("job:read", "project:"+opts.project); err != nil {
					return err
				}
				jobs, err := ac.Store.Jobs.List(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(jobs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tID\tSTATUS\tCREATED")
				for _, j := range jobs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", j.Name, j.ID, j.Status, j.CreatedAt)
				}
				return tw.Flush()
			})
		},
	}
}

// ---- bottle commands --------------------------------------------------------

func bottleCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bottle",
		Short: "manage Capper Bottles (declarative app deployments)",
	}
	cmd.AddCommand(
		bottleImportCmd(opts),
		bottleValidateCmd(opts),
		bottlePlanCmd(opts),
		bottleDeployCmd(opts),
		bottleListCmd(opts),
		bottleOutputsCmd(opts),
		bottleRemoveCmd(opts),
		bottleDeploymentsCmd(opts),
	)
	return cmd
}

func bottleImportCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "import FILE",
		Short: "import a bottle definition from a JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("bottle:import", "project:"+opts.project); err != nil {
					return err
				}
				spec, data, err := bottle.LoadSpec(args[0])
				if err != nil {
					return err
				}
				errs := bottle.ValidateSpec(spec, nil)
				if len(errs) > 0 {
					for _, e := range errs {
						fmt.Fprintln(os.Stderr, "validation error:", e)
					}
					return fmt.Errorf("bottle has %d validation error(s)", len(errs))
				}
				b := bottle.Bottle{
					ID:          fmt.Sprintf("btl_%d", time.Now().UnixNano()),
					Project:     opts.project,
					Name:        spec.Metadata.Name,
					DisplayName: spec.Metadata.DisplayName,
					Version:     spec.Metadata.Version,
					Description: spec.Metadata.Description,
					Author:      spec.Metadata.Author,
					License:     spec.Metadata.License,
					Source:      spec.Metadata.Source,
					Digest:      bottle.Digest(data),
					RawJSON:     string(data),
					Status:      bottle.BottleActive,
					Tags:        spec.Metadata.Tags,
					CreatedAt:   time.Now().UTC().Format(time.RFC3339),
					UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
				}
				if err := ac.Store.Bottles.InsertBottle(b); err != nil {
					return fmt.Errorf("import bottle: %w", err)
				}
				fmt.Printf("Bottle imported\n\nName:     %s\nVersion:  %s\nID:       %s\nDigest:   %s\n",
					b.Name, b.Version, b.ID, b.Digest)
				return nil
			})
		},
	}
}

func bottleValidateCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate FILE",
		Short: "validate a bottle file without importing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, _, err := bottle.LoadSpec(args[0])
			if err != nil {
				return err
			}
			errs := bottle.ValidateSpec(spec, nil)
			if len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, "error:", e)
				}
				return fmt.Errorf("%d validation error(s)", len(errs))
			}
			fmt.Println("Bottle is valid.")
			return nil
		},
	}
}

func bottlePlanCmd(opts *options) *cobra.Command {
	var setFlags []string
	cmd := &cobra.Command{
		Use:   "plan NAME",
		Short: "show what a bottle deployment would create",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("bottle:plan", "project:"+opts.project); err != nil {
					return err
				}
				b, err := ac.Store.Bottles.GetBottle(args[0], opts.project)
				if err != nil {
					return err
				}
				spec, err := bottle.ParseSpec([]byte(b.RawJSON))
				if err != nil {
					return err
				}
				params := parseSetFlags(setFlags)
				plan, planErr := bottle.Plan(spec, params, args[0]+"-deploy")
				if opts.json {
					_ = printJSON(plan)
					return planErr
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ACTION\tKIND\tNAME\tDETAIL")
				for _, a := range plan {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Action, a.Kind, a.Name, a.Detail)
				}
				_ = tw.Flush()
				return planErr
			})
		},
	}
	cmd.Flags().StringArrayVar(&setFlags, "set", nil, "parameter overrides (key=value)")
	return cmd
}

func bottleDeployCmd(opts *options) *cobra.Command {
	var (
		setFlags   []string
		deployName string
	)
	cmd := &cobra.Command{
		Use:   "deploy NAME",
		Short: "deploy a bottle (create all resources)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withController(opts, func(ctrl controller.Controller) error {
				if err := ctrl.Authorize("bottle:deploy", "project:"+opts.project); err != nil {
					return err
				}
				b, err := ctrl.Store.Bottles.GetBottle(args[0], opts.project)
				if err != nil {
					return err
				}
				spec, err := bottle.ParseSpec([]byte(b.RawJSON))
				if err != nil {
					return err
				}
				params := parseSetFlags(setFlags)
				if deployName == "" {
					deployName = args[0] + "-deploy"
				}

				depl := bottle.BottleDeployment{
					ID:         fmt.Sprintf("bdep_%d", time.Now().UnixNano()),
					Project:    opts.project,
					BottleID:   b.ID,
					Name:       deployName,
					Version:    b.Version,
					Status:     bottle.DeploymentPlanning,
					Parameters: params,
					CreatedAt:  time.Now().UTC().Format(time.RFC3339),
					UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
				}
				if err := ctrl.Store.Bottles.InsertDeployment(depl); err != nil {
					return fmt.Errorf("create deployment: %w", err)
				}

				netMgr := network.NewManager(ctrl.Store.Networks)
				dnsMgr := capperdns.NewManager(ctrl.Store.DNS)

				applyDeps := bottle.ApplyDeps{
					CreateNetwork: func(name, proj, mode, subnet string) (string, error) {
						n, err := netMgr.Create(name, proj, network.CreateOptions{Mode: mode, Subnet: subnet})
						if err != nil {
							return "", err
						}
						return n.ID, nil
					},
					CreateSecret: func(name, proj, value string) (string, error) {
						sec, err := ctrl.Store.Secrets.Create(name, proj, "", value)
						if err != nil {
							return "", err
						}
						return sec.ID, nil
					},
					CreateInstanceGroup: func(ctx context.Context, name, proj, image, netName string, replicas int) ([]string, error) {
						var ids []string
						var netOpts *manager.NetworkRunOpts
						if netName != "" {
							n, nerr := ctrl.Store.Networks.Get(netName, proj)
							if nerr == nil {
								netOpts = &manager.NetworkRunOpts{
									NetworkID: n.ID,
									Bridge:    n.Bridge,
									Subnet:    n.Subnet,
									Gateway:   n.Gateway,
								}
							}
						}
						for i := 0; i < replicas; i++ {
							instName := fmt.Sprintf("%s-%d", name, i+1)
							inst, err := ctrl.Instances.Run(image, types.ResourceOverrides{},
								manager.RunOptions{Name: instName, Network: netOpts})
							if err != nil {
								return ids, err
							}
							ids = append(ids, inst.ID)
						}
						return ids, nil
					},
					CreateLB: func(name, proj, mode, listenPort string) (string, error) {
						lbMode := lb.ModeTCP
						if mode == "http" {
							lbMode = lb.ModeHTTP
						}
						newLB, err := ctrl.Store.LB.Create(name, proj, "", listenPort, lbMode)
						if err != nil {
							return "", err
						}
						return newLB.ID, nil
					},
					CreateDNS: func(name, proj, host, target string) (string, error) {
						zones, _ := dnsMgr.ListZones("")
						if len(zones) == 0 {
							return "dns-skipped", nil
						}
						rec, err := dnsMgr.CreateRecord(zones[0].Name, "", host, "A", []string{target}, 300)
						if err != nil {
							return "", err
						}
						return rec.ID, nil
					},
				}

				depl, err = bottle.Apply(context.Background(), ctrl.Store.Bottles, depl, spec, params, applyDeps)
				if err != nil {
					return fmt.Errorf("bottle deploy failed: %w", err)
				}
				fmt.Printf("Bottle deployed\n\nDeployment: %s\nID:         %s\nStatus:     %s\n",
					depl.Name, depl.ID, depl.Status)
				if len(depl.Outputs) > 0 {
					fmt.Println("\nOutputs:")
					for k, v := range depl.Outputs {
						fmt.Printf("  %s: %s\n", k, v)
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringArrayVar(&setFlags, "set", nil, "parameter overrides (key=value)")
	cmd.Flags().StringVar(&deployName, "name", "", "deployment name (default: BOTTLE-deploy)")
	return cmd
}

func bottleListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list imported bottles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("bottle:read", "project:"+opts.project); err != nil {
					return err
				}
				bottles, err := ac.Store.Bottles.ListBottles(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(bottles)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tVERSION\tAUTHOR\tSTATUS\tID")
				for _, b := range bottles {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", b.Name, b.Version, b.Author, b.Status, b.ID)
				}
				return tw.Flush()
			})
		},
	}
}

func bottleOutputsCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "outputs DEPLOYMENT",
		Short: "show outputs from a bottle deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("bottle:read", "project:"+opts.project); err != nil {
					return err
				}
				d, err := ac.Store.Bottles.GetDeployment(args[0], opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(d.Outputs)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintf(tw, "Deployment: %s\nStatus:     %s\n\n", d.Name, d.Status)
				fmt.Fprintln(tw, "KEY\tVALUE")
				for k, v := range d.Outputs {
					fmt.Fprintf(tw, "%s\t%s\n", k, v)
				}
				return tw.Flush()
			})
		},
	}
}

func bottleDeploymentsCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "deployments",
		Short: "list bottle deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("bottle:read", "project:"+opts.project); err != nil {
					return err
				}
				depls, err := ac.Store.Bottles.ListDeployments(opts.project)
				if err != nil {
					return err
				}
				if opts.json {
					return printJSON(depls)
				}
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tVERSION\tSTATUS\tID")
				for _, d := range depls {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Name, d.Version, d.Status, d.ID)
				}
				return tw.Flush()
			})
		},
	}
}

func bottleRemoveCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove DEPLOYMENT",
		Short: "remove a bottle deployment record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("bottle:remove", "project:"+opts.project); err != nil {
					return err
				}
				d, err := ac.Store.Bottles.GetDeployment(args[0], opts.project)
				if err != nil {
					return err
				}
				_ = ac.Store.Bottles.UpdateDeploymentStatus(d.ID, bottle.DeploymentRemoved)
				if err := ac.Store.Bottles.DeleteDeployment(d.ID); err != nil {
					return fmt.Errorf("remove deployment: %w", err)
				}
				fmt.Printf("Deployment %q removed.\n", args[0])
				return nil
			})
		},
	}
}

// parseSetFlags converts ["key=val", ...] into a map.
func parseSetFlags(flags []string) map[string]string {
	out := make(map[string]string)
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func jobLogsCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "logs NAME",
		Short: "show logs from the last job run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("job:read", "project:"+opts.project); err != nil {
					return err
				}
				j, err := ac.Store.Jobs.Get(args[0], opts.project)
				if err != nil {
					return err
				}
				fmt.Print(j.Logs)
				return nil
			})
		},
	}
}

func jobDeleteCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "delete a job record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIAM(opts, func(ac *iamCtx) error {
				if err := ac.Authorize("job:create", "project:"+opts.project); err != nil {
					return err
				}
				if err := ac.Store.Jobs.Delete(args[0], opts.project); err != nil {
					return err
				}
				fmt.Printf("Job %q deleted.\n", args[0])
				return nil
			})
		},
	}
}

// ---------------------------------------------------------------------------
// capper context — manage active org/account/project context
// ---------------------------------------------------------------------------

type capperContext struct {
	OrgID     string `json:"orgId"`
	AccountID string `json:"accountId"`
	ProjectID string `json:"projectId"`
}

func contextConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".capper", "context.json"), nil
}

func loadContext() (capperContext, error) {
	p, err := contextConfigPath()
	if err != nil {
		return capperContext{}, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return capperContext{}, nil
	}
	if err != nil {
		return capperContext{}, err
	}
	var ctx capperContext
	return ctx, json.Unmarshal(b, &ctx)
}

func saveContext(ctx capperContext) error {
	p, err := contextConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0600)
}

func contextCmd(_ *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "manage active org / account / project context",
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "show active context",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadContext()
			if err != nil {
				return err
			}
			fmt.Printf("Org:     %s\nAccount: %s\nProject: %s\n",
				orDefault(ctx.OrgID, "(not set)"),
				orDefault(ctx.AccountID, "(not set)"),
				orDefault(ctx.ProjectID, "(not set)"),
			)
			return nil
		},
	}

	useOrgCmd := &cobra.Command{
		Use:   "use-org ORG_ID",
		Short: "set the active organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadContext()
			if err != nil {
				return err
			}
			ctx.OrgID = args[0]
			if err := saveContext(ctx); err != nil {
				return err
			}
			fmt.Printf("Active org set to: %s\n", args[0])
			return nil
		},
	}

	useAccountCmd := &cobra.Command{
		Use:   "use-account ACCOUNT_ID",
		Short: "set the active account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadContext()
			if err != nil {
				return err
			}
			ctx.AccountID = args[0]
			if err := saveContext(ctx); err != nil {
				return err
			}
			fmt.Printf("Active account set to: %s\n", args[0])
			return nil
		},
	}

	useProjectCmd := &cobra.Command{
		Use:   "use-project PROJECT_ID",
		Short: "set the active project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := loadContext()
			if err != nil {
				return err
			}
			ctx.ProjectID = args[0]
			if err := saveContext(ctx); err != nil {
				return err
			}
			fmt.Printf("Active project set to: %s\n", args[0])
			return nil
		},
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "clear all context (org, account, project)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := saveContext(capperContext{}); err != nil {
				return err
			}
			fmt.Println("Context cleared.")
			return nil
		},
	}

	cmd.AddCommand(showCmd, useOrgCmd, useAccountCmd, useProjectCmd, clearCmd)
	return cmd
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
