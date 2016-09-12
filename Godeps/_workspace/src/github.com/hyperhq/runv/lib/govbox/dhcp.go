package virtualbox

import (
	"bufio"
	"net"
	"strings"
)

// DHCP server info.
type DHCP struct {
	NetworkName string
	IPv4        net.IPNet
	LowerIP     net.IP
	UpperIP     net.IP
	Enabled     bool
}

func addDHCP(kind, name string, d DHCP) error {
	args := []string{"dhcpserver", "add",
		kind, name,
		"--ip", d.IPv4.IP.String(),
		"--netmask", net.IP(d.IPv4.Mask).String(),
		"--lowerip", d.LowerIP.String(),
		"--upperip", d.UpperIP.String(),
	}
	if d.Enabled {
		args = append(args, "--enable")
	} else {
		args = append(args, "--disable")
	}
	return vbm(args...)
}

// AddInternalDHCP adds a DHCP server to an internal network.
func AddInternalDHCP(netname string, d DHCP) error {
	return addDHCP("--netname", netname, d)
}

// AddHostonlyDHCP adds a DHCP server to a host-only network.
func AddHostonlyDHCP(ifname string, d DHCP) error {
	return addDHCP("--ifname", ifname, d)
}

// DHCPs gets all DHCP server settings in a map keyed by DHCP.NetworkName.
func DHCPs() (map[string]*DHCP, error) {
	out, err := vbmOut("list", "dhcpservers")
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(strings.NewReader(out))
	m := map[string]*DHCP{}
	dhcp := &DHCP{}
	for s.Scan() {
		line := s.Text()
		if line == "" {
			m[dhcp.NetworkName] = dhcp
			dhcp = &DHCP{}
			continue
		}
		res := reColonLine.FindStringSubmatch(line)
		if res == nil {
			continue
		}
		switch key, val := res[1], res[2]; key {
		case "NetworkName":
			dhcp.NetworkName = val
		case "IP":
			dhcp.IPv4.IP = net.ParseIP(val)
		case "upperIPAddress":
			dhcp.UpperIP = net.ParseIP(val)
		case "lowerIPAddress":
			dhcp.LowerIP = net.ParseIP(val)
		case "NetworkMask":
			dhcp.IPv4.Mask = ParseIPv4Mask(val)
		case "Enabled":
			dhcp.Enabled = (val == "Yes")
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return m, nil
}
