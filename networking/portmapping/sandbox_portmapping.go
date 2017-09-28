package portmapping

import (
	"fmt"
	"strconv"
	"strings"
)

/*  Inside hyperstart, there are iptables init job should be done before configure in sandbox rules.

	// iptables -t filter -N hyperstart-INPUT
	// iptables -t nat -N hyperstart-PREROUTING
	// iptables -t filter -I INPUT -j hyperstart-INPUT
	// iptables -t nat -I PREROUTING -j hyperstart-PREROUTING
	// iptables -t filter -A hyperstart-INPUT -m state --state RELATED,ESTABLISHED -j ACCEPT
	// iptables -t filter -A hyperstart-INPUT -p icmp -j ACCEPT
	// iptables -t filter -A hyperstart-INPUT -i lo -j ACCEPT
	// iptables -t filter -A hyperstart-INPUT -j DROP
	// iptables -t nat -A hyperstart-PREROUTING -j RETURN
	// sh -c "echo 10485760 > /proc/sys/net/nf_conntrack_max"
	// sh -c "echo 300 > /proc/sys/net/netfilter/nf_conntrack_tcp_timeout_established"

	// lan
	// iptables -t filter -I hyperstart-INPUT -s %s -j ACCEPT

    These job has been done by hyperstart during initSandbox.
*/

func generateRedirectArgs(prefix string, m *PortMapping, insert bool) ([]string, []string, error) {
	// outside
	//iptables -t nat -I hyperstart-PREROUTING -s %s -p %s -m %s --dport %d -j REDIRECT --to-ports %d"
	//iptables -t filter -I hyperstart-INPUT -s %s -p %s -m %s --dport %d -j ACCEPT
	var (
		action = "-I"
		proto  string
		dest   string
		to     string
	)

	if !insert {
		action = "-D"
	}

	if strings.EqualFold(m.Protocol, "udp") {
		proto = "udp"
	} else {
		proto = "tcp"
	}

	if m.FromPorts.End == 0 || m.FromPorts.End == m.FromPorts.Begin {
		dest = strconv.Itoa(m.FromPorts.Begin)
		m.FromPorts.End = m.FromPorts.Begin
	} else if m.FromPorts.End > m.FromPorts.Begin {
		dest = fmt.Sprintf("%d:%d", m.FromPorts.Begin, m.FromPorts.End)
	} else {
		return []string{}, []string{}, fmt.Errorf("invalid from port range %d-%d", m.FromPorts.Begin, m.FromPorts.End)
	}

	if m.ToPorts.End == 0 || m.ToPorts.End == m.ToPorts.Begin {
		to = strconv.Itoa(m.ToPorts.Begin)
		m.ToPorts.End = m.ToPorts.Begin
	} else if m.ToPorts.End > m.ToPorts.Begin {
		to = fmt.Sprintf("%d-%d", m.ToPorts.Begin, m.ToPorts.End)
	} else {
		return []string{}, []string{}, fmt.Errorf("invalid to port range %d-%d", m.ToPorts.Begin, m.ToPorts.End)
	}

	//we may map ports 1:N or N:N, but not M:N (M!=1, M!=N)
	hostRange := m.FromPorts.End - m.FromPorts.Begin
	containerRange := m.ToPorts.End - m.ToPorts.Begin
	if hostRange != 0 && hostRange != containerRange {
		return []string{}, []string{}, fmt.Errorf("range mismatch, cannot map ports %s to %s", dest, to)
	}

	filterArgs := []string{"iptables", "-t", "filter", action, "hyperstart-INPUT", "-s", prefix, "-p", proto, "-m", proto, "--dport", dest, "-j", "ACCEPT"}
	redirectArgs := []string{"iptables", "-t", "nat", action, "hyperstart-PREROUTING", "-s", prefix, "-p", proto, "-m", proto, "--dport", dest, "-j", "REDIRECT", "--to-port", to}

	return redirectArgs, filterArgs, nil
}

func setupInSandboxMappings(extPrefix []string, maps []*PortMapping) ([][]string, error) {
	res := [][]string{}
	for _, prefix := range extPrefix {
		for _, m := range maps {
			redirect, filter, err := generateRedirectArgs(prefix, m, true)
			if err != nil {
				return nil, err
			}
			res = append(res, redirect, filter)
		}
	}
	return res, nil
}

func releaseInSandboxMappings(extPrefix []string, maps []*PortMapping) ([][]string, error) {
	res := [][]string{}
	for _, prefix := range extPrefix {
		for _, m := range maps {
			redirect, filter, err := generateRedirectArgs(prefix, m, false)
			if err != nil {
				return nil, err
			}
			res = append(res, redirect, filter)
		}
	}
	return res, nil
}
