package network

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vishvananda/netlink"
)

// TAPName derives a short TAP interface name for an instance.
func TAPName(instanceID string) string {
	short := instanceID
	if idx := strings.Index(instanceID, "_"); idx >= 0 {
		short = instanceID[idx+1:]
	}
	if len(short) > 8 {
		short = short[:8]
	}
	return "ctap" + short
}

// CreateTAPOnBridge creates a TAP device attached to bridgeName and brings it up.
// The TAP stays in the host network namespace so QEMU can open it.
func CreateTAPOnBridge(bridgeName, tapName string) error {
	la := netlink.NewLinkAttrs()
	la.Name = tapName
	tuntap := &netlink.Tuntap{
		LinkAttrs: la,
		Mode:      netlink.TUNTAP_MODE_TAP,
	}
	if err := netlink.LinkAdd(tuntap); err != nil && !isExist(err) {
		return fmt.Errorf("network: create tap %s: %w", tapName, err)
	}
	tap, err := netlink.LinkByName(tapName)
	if err != nil {
		return fmt.Errorf("network: lookup tap %s: %w", tapName, err)
	}
	bridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		_ = netlink.LinkDel(tap)
		return fmt.Errorf("network: lookup bridge %s: %w", bridgeName, err)
	}
	if err := netlink.LinkSetMaster(tap, bridge); err != nil {
		_ = netlink.LinkDel(tap)
		return fmt.Errorf("network: attach %s to %s: %w", tapName, bridgeName, err)
	}
	if err := netlink.LinkSetUp(tap); err != nil {
		_ = netlink.LinkDel(tap)
		return fmt.Errorf("network: bring up tap %s: %w", tapName, err)
	}
	return nil
}

// DeleteTAP removes a TAP device if it exists.
func DeleteTAP(tapName string) error {
	if tapName == "" {
		return nil
	}
	link, err := netlink.LinkByName(tapName)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return fmt.Errorf("network: lookup tap %s: %w", tapName, err)
	}
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("network: delete tap %s: %w", tapName, err)
	}
	return nil
}

// WriteTAPName persists the TAP interface name for the QEMU runtime.
func WriteTAPName(instDir, tapName string) error {
	return os.WriteFile(filepath.Join(instDir, "tap.name"), []byte(tapName+"\n"), 0o644)
}
