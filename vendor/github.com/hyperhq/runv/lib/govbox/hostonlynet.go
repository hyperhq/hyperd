package virtualbox

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	reHostonlyInterfaceCreated = regexp.MustCompile(`Interface '(.+)' was successfully created`)
)

var (
	ErrHostonlyInterfaceCreation = errors.New("failed to create hostonly interface")
)

// Host-only network.
type HostonlyNet struct {
	Name        string
	GUID        string
	DHCP        bool
	IPv4        net.IPNet
	IPv6        net.IPNet
	HwAddr      net.HardwareAddr
	Medium      string
	Status      string
	NetworkName string // referenced in DHCP.NetworkName
}

// CreateHostonlyNet creates a new host-only network.
func CreateHostonlyNet() (*HostonlyNet, error) {
	out, err := vbmOut("hostonlyif", "create")
	if err != nil {
		return nil, err
	}
	res := reHostonlyInterfaceCreated.FindStringSubmatch(string(out))
	if res == nil {
		return nil, ErrHostonlyInterfaceCreation
	}
	return &HostonlyNet{Name: res[1]}, nil
}

// Config changes the configuration of the host-only network.
func (n *HostonlyNet) Config() error {
	if n.IPv4.IP != nil && n.IPv4.Mask != nil {
		if err := vbm("hostonlyif", "ipconfig", n.Name, "--ip", n.IPv4.IP.String(), "--netmask", net.IP(n.IPv4.Mask).String()); err != nil {
			return err
		}
	}

	if n.IPv6.IP != nil && n.IPv6.Mask != nil {
		prefixLen, _ := n.IPv6.Mask.Size()
		if err := vbm("hostonlyif", "ipconfig", n.Name, "--ipv6", n.IPv6.IP.String(), "--netmasklengthv6", fmt.Sprintf("%d", prefixLen)); err != nil {
			return err
		}
	}

	if n.DHCP {
		vbm("hostonlyif", "ipconfig", n.Name, "--dhcp") // not implemented as of VirtualBox 4.3
	}

	return nil
}

// HostonlyNets gets all host-only networks in a  map keyed by HostonlyNet.NetworkName.
func HostonlyNets() (map[string]*HostonlyNet, error) {
	out, err := vbmOut("list", "hostonlyifs")
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(strings.NewReader(out))
	m := map[string]*HostonlyNet{}
	n := &HostonlyNet{}
	for s.Scan() {
		line := s.Text()
		if line == "" {
			m[n.NetworkName] = n
			n = &HostonlyNet{}
			continue
		}
		res := reColonLine.FindStringSubmatch(line)
		if res == nil {
			continue
		}
		switch key, val := res[1], res[2]; key {
		case "Name":
			n.Name = val
		case "GUID":
			n.GUID = val
		case "DHCP":
			n.DHCP = (val != "Disabled")
		case "IPAddress":
			n.IPv4.IP = net.ParseIP(val)
		case "NetworkMask":
			n.IPv4.Mask = ParseIPv4Mask(val)
		case "IPV6Address":
			n.IPv6.IP = net.ParseIP(val)
		case "IPV6NetworkMaskPrefixLength":
			l, err := strconv.ParseUint(val, 10, 7)
			if err != nil {
				return nil, err
			}
			n.IPv6.Mask = net.CIDRMask(int(l), net.IPv6len*8)
		case "HardwareAddress":
			mac, err := net.ParseMAC(val)
			if err != nil {
				return nil, err
			}
			n.HwAddr = mac
		case "MediumType":
			n.Medium = val
		case "Status":
			n.Status = val
		case "VBoxNetworkName":
			n.NetworkName = val
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return m, nil
}
