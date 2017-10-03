package network

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	"github.com/vishvananda/netlink"
)

const (
	ipv4ForwardConf     = "/proc/sys/net/ipv4/ip_forward"
	ipv4ForwardConfPerm = 0644
)

func InitNetwork(bIface, bIP string, disable bool) error {
	if err := ensureBridge(bIface, bIP); err != nil {
		return err
	}

	if err := setupIPForwarding(); err != nil {
		return err
	}

	return nil
}

func setupIPForwarding() error {
	// Get current IPv4 forward setup
	ipv4ForwardData, err := ioutil.ReadFile(ipv4ForwardConf)
	if err != nil {
		return fmt.Errorf("Cannot read IP forwarding setup: %v", err)
	}

	// Enable IPv4 forwarding only if it is not already enabled
	if ipv4ForwardData[0] != '1' {
		// Enable IPv4 forwarding
		if err := ioutil.WriteFile(ipv4ForwardConf, []byte{'1', '\n'}, ipv4ForwardConfPerm); err != nil {
			return fmt.Errorf("Setup IP forwarding failed: %v", err)
		}
	}

	return nil
}

func ensureBridge(bIface, bIP string) error {
	if bIface == "" {
		BridgeIface = DefaultBridgeIface
	} else {
		BridgeIface = bIface
	}

	if bIP == "" {
		BridgeIP = DefaultBridgeIP
	} else {
		BridgeIP = bIP
	}

	ipAddr, ipNet, err := net.ParseCIDR(BridgeIP)
	if err != nil {
		glog.Errorf("%s parsecidr failed", BridgeIP)
		return err
	}

	if brlink, err := netlink.LinkByName(BridgeIface); err != nil {
		glog.V(1).Infof("create bridge %s, ip %s", BridgeIface, BridgeIP)
		// No Bridge existent, create one
		if ipAddr.Equal(ipNet.IP) {
			ipAddr, err = IpAllocator.RequestIP(ipNet, nil)
		} else {
			ipAddr, err = IpAllocator.RequestIP(ipNet, ipAddr)
		}

		if err != nil {
			return err
		}

		glog.V(3).Infof("Allocate IP Address %s for bridge %s", ipAddr, BridgeIface)

		BridgeIPv4Net = &net.IPNet{IP: ipAddr, Mask: ipNet.Mask}
		if err := createBridgeIface(BridgeIface, BridgeIPv4Net); err != nil {
			// The bridge may already exist, therefore we can ignore an "exists" error
			if !os.IsExist(err) {
				glog.Errorf("CreateBridgeIface failed %s %s", BridgeIface, ipAddr)
				return err
			}
			// should not reach here
		}
	} else {
		glog.V(1).Info("bridge exist")
		// Validate that the bridge ip matches the ip specified by BridgeIP

		addrs, err := netlink.AddrList(brlink, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if len(addrs) == 0 {
			return fmt.Errorf("Interface %v has no IPv4 addresses", BridgeIface)
		}

		BridgeIPv4Net = addrs[0].IPNet

		if !BridgeIPv4Net.Contains(ipAddr) {
			return fmt.Errorf("Bridge ip (%s) does not match existing bridge configuration %s", BridgeIPv4Net, BridgeIP)
		}

		mask1, _ := ipNet.Mask.Size()
		mask2, _ := BridgeIPv4Net.Mask.Size()

		if mask1 != mask2 {
			return fmt.Errorf("Bridge netmask (%d) does not match existing bridge netmask %d", mask1, mask2)
		}
	}

	IpAllocator.RequestIP(BridgeIPv4Net, BridgeIPv4Net.IP)

	return nil
}

func createBridgeIface(name string, addr *net.IPNet) error {
	la := netlink.NewLinkAttrs()
	la.Name = name
	bridge := &netlink.Bridge{LinkAttrs: la}
	if err := netlink.LinkAdd(bridge); err != nil {
		return err
	}
	if err := netlink.AddrAdd(bridge, &netlink.Addr{IPNet: addr}); err != nil {
		return err
	}
	return netlink.LinkSetUp(bridge)
}

func DeleteBridge(name string) error {
	bridge, err := netlink.LinkByName(BridgeIface)
	if err != nil {
		glog.Errorf("cannot find bridge %v: %v", name, err)
		return err
	}

	netlink.LinkDel(bridge)
	return nil
}

// addToBridge attch interface to the bridge,
// we only support ovs bridge and linux bridge at present.
func addToBridge(iface, master netlink.Link, options string) error {
	switch master.Type() {
	case "openvswitch":
		return addToOpenvswitchBridge(iface, master, options)
	case "bridge":
		return netlink.LinkSetMaster(iface, master.(*netlink.Bridge))
	default:
		return fmt.Errorf("unknown link type:%+v", master.Type())
	}
}

func addToOpenvswitchBridge(iface, master netlink.Link, options string) error {
	masterName := master.Attrs().Name
	ifaceName := iface.Attrs().Name
	glog.V(3).Infof("Found ovs bridge %s, attaching tap %s to it\n", masterName, ifaceName)

	// ovs command "ovs-vsctl add-port BRIDGE PORT" add netwok device PORT to BRIDGE,
	// PORT and BRIDGE here indicate the device name respectively.
	out, err := exec.Command("ovs-vsctl", "--may-exist", "add-port", masterName, ifaceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Ovs failed to add port: %s, error :%v", strings.TrimSpace(string(out)), err)
	}

	out, err = exec.Command("ovs-vsctl", "set", "port", ifaceName, options).CombinedOutput()
	return nil
}

func genRandomMac() (string, error) {
	const alphanum = "0123456789abcdef"
	var bytes = make([]byte, 8)
	_, err := rand.Read(bytes)

	if err != nil {
		glog.Errorf("get random number faild")
		return "", err
	}

	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}

	tmp := []string{"52:54", string(bytes[0:2]), string(bytes[2:4]), string(bytes[4:6]), string(bytes[6:8])}
	return strings.Join(tmp, ":"), nil
}

func UpAndAddToBridge(name, bridge, options string) error {
	iface, err := netlink.LinkByName(name)
	if err != nil {
		glog.Error("cannot find network interface ", name)
		return err
	}
	if bridge == "" {
		bridge = BridgeIface
	}
	if bridge != "" {
		master, err := netlink.LinkByName(bridge)
		if err != nil {
			glog.Error("cannot find bridge interface ", bridge)
			return err
		}
		if err = addToBridge(iface, master, options); err != nil {
			glog.Errorf("cannot add %s to %s ", name, bridge)
			return err
		}
	}
	if err = netlink.LinkSetUp(iface); err != nil {
		glog.Error("cannot up interface ", name)
		return err
	}

	return nil
}

func AllocateAddr(requestedIP string) (*Settings, error) {
	ip, err := IpAllocator.RequestIP(BridgeIPv4Net, net.ParseIP(requestedIP))
	if err != nil {
		return nil, err
	}

	maskSize, _ := BridgeIPv4Net.Mask.Size()

	mac, err := genRandomMac()
	if err != nil {
		glog.Errorf("Generate Random Mac address failed")
		return nil, err
	}

	return &Settings{
		Mac:       mac,
		IPAddress: fmt.Sprintf("%s/%d", ip.String(), maskSize),
		Gateway:   BridgeIPv4Net.IP.String(),
		Bridge:    BridgeIface,
		Device:    "",
		Automatic: true,
	}, nil
}

func Configure(inf *api.InterfaceDescription) (*Settings, error) {
	var err error
	mac := inf.Mac
	if mac == "" {
		if mac, err = genRandomMac(); err != nil {
			glog.Errorf("Generate Random Mac address failed")
			return nil, err
		}
	}

	return &Settings{
		Mac:       mac,
		IPAddress: inf.Ip,
		Gateway:   inf.Gw,
		Bridge:    inf.Bridge,
		Device:    inf.TapName,
		Mtu:       inf.Mtu,
		Automatic: false,
	}, nil
}

func ReleaseAddr(releasedIP string) error {
	if err := IpAllocator.ReleaseIP(BridgeIPv4Net, net.ParseIP(releasedIP)); err != nil {
		return err
	}
	return nil
}
