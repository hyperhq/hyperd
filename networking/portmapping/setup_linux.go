package portmapping

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/networking/portmapping/iptables"
)

var (
	disableIptables bool
	bridgeIface     string
)

//setup environment for iptables and IP forwarding
func Setup(bIface, addr string, disable bool) error {
	var err error

	disableIptables = disable
	bridgeIface = bIface

	if disableIptables {
		hlog.Log(hlog.DEBUG, "Iptables is disabled")
		return nil
	}

	hlog.Log(hlog.TRACE, "setting up iptables")
	err = setupIPTables(addr)
	if err != nil {
		hlog.Log(hlog.ERROR, "failed to setup iptables: %v", err)
		return err
	}

	return nil
}

func setupIPTables(addr string) error {
	if disableIptables {
		return nil
	}

	// Enable NAT
	natArgs := []string{"-s", addr, "!", "-o", bridgeIface, "-j", "MASQUERADE"}

	if !iptables.Exists(iptables.Nat, "POSTROUTING", natArgs...) {
		if output, err := iptables.Raw(append([]string{
			"-t", string(iptables.Nat), "-I", "POSTROUTING"}, natArgs...)...); err != nil {
			return fmt.Errorf("Unable to enable network bridge NAT: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "POSTROUTING", Output: output}
		}
	}

	// Create HYPER iptables Chain
	iptables.Raw("-N", "HYPER")

	// Goto HYPER chain
	gotoArgs := []string{"-o", bridgeIface, "-j", "HYPER"}
	if !iptables.Exists(iptables.Filter, "FORWARD", gotoArgs...) {
		if output, err := iptables.Raw(append([]string{"-I", "FORWARD"}, gotoArgs...)...); err != nil {
			return fmt.Errorf("Unable to setup goto HYPER rule %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD goto HYPER", Output: output}
		}
	}

	// Accept all outgoing packets
	outgoingArgs := []string{"-i", bridgeIface, "-j", "ACCEPT"}
	if !iptables.Exists(iptables.Filter, "FORWARD", outgoingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I", "FORWARD"}, outgoingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow outgoing packets: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD outgoing", Output: output}
		}
	}

	// Accept incoming packets for existing connections
	existingArgs := []string{"-o", bridgeIface, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}

	if !iptables.Exists(iptables.Filter, "FORWARD", existingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I", "FORWARD"}, existingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow incoming packets: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD incoming", Output: output}
		}
	}

	err := Modprobe("br_netfilter")
	if err != nil {
		hlog.Log(hlog.DEBUG, "modprobe br_netfilter failed %s", err)
	}

	file, err := os.OpenFile("/proc/sys/net/bridge/bridge-nf-call-iptables",
		os.O_RDWR, 0)
	if err != nil {
		return err
	}

	_, err = file.WriteString("1")
	if err != nil {
		return err
	}

	// Create HYPER iptables Chain
	iptables.Raw("-t", string(iptables.Nat), "-N", "HYPER")
	// Goto HYPER chain
	gotoArgs = []string{"-m", "addrtype", "--dst-type", "LOCAL", "!",
		"-d", "127.0.0.1/8", "-j", "HYPER"}
	if !iptables.Exists(iptables.Nat, "OUTPUT", gotoArgs...) {
		if output, err := iptables.Raw(append([]string{"-t", string(iptables.Nat),
			"-I", "OUTPUT"}, gotoArgs...)...); err != nil {
			return fmt.Errorf("Unable to setup goto HYPER rule %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "OUTPUT goto HYPER", Output: output}
		}
	}

	gotoArgs = []string{"-m", "addrtype", "--dst-type", "LOCAL",
		"-j", "HYPER"}
	if !iptables.Exists(iptables.Nat, "PREROUTING", gotoArgs...) {
		if output, err := iptables.Raw(append([]string{"-t", string(iptables.Nat),
			"-I", "PREROUTING"}, gotoArgs...)...); err != nil {
			return fmt.Errorf("Unable to setup goto HYPER rule %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "PREROUTING goto HYPER", Output: output}
		}
	}

	return nil
}

func Modprobe(module string) error {
	modprobePath, err := exec.LookPath("modprobe")
	if err != nil {
		return fmt.Errorf("modprobe not found")
	}

	_, err = exec.Command(modprobePath, module).CombinedOutput()
	if err != nil {
		return fmt.Errorf("modprobe %s failed", module)
	}

	return nil
}
