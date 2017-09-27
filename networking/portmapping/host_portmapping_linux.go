package portmapping

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/networking/portmapping/iptables"
	"github.com/hyperhq/hyperd/networking/portmapping/portmapper"
)

var (
	PortMapper = portmapper.New()
)

type PortRange struct {
	Begin int
	End   int
}

type PortMapping struct {
	Protocol  string
	ToPorts   *PortRange
	FromPorts *PortRange
}

// NewPortRange generate a port range from string r. the r should be a decimal number or
// in format begin-end, where begin and end are both decimal number. And the port range should
// be 0-65535, i.e. 16-bit unsigned int
// It returns PortRange pointer for valid input, otherwise return error
func NewPortRange(r string) (*PortRange, error) {
	segs := strings.SplitN(r, "-", 2)
	b, err := strconv.ParseUint(segs[0], 10, 16)
	if err != nil {
		return nil, err
	}
	e := b
	if len(segs) > 1 {
		e, err = strconv.ParseUint(segs[1], 10, 16)
		if err != nil {
			return nil, err
		}
	}
	return &PortRange{
		Begin: int(b),
		End:   int(e),
	}, nil
}

// NewPortMapping generate a PortMapping from three strings: proto (tcp or udp, default is tcp),
// and from/to port (single port or a range, see NewPortRange)
func NewPortMapping(proto, from, to string) (*PortMapping, error) {
	if proto == "" {
		proto = "tcp"
	}
	if proto != "tcp" && proto != "udp" {
		return nil, fmt.Errorf("unsupported protocol %s", proto)
	}
	f, err := NewPortRange(from)
	if err != nil {
		return nil, err
	}
	t, err := NewPortRange(to)
	if err != nil {
		return nil, err
	}
	return &PortMapping{
		Protocol:  proto,
		ToPorts:   t,
		FromPorts: f,
	}, nil
}

func generateIptablesArgs(containerip string, m *PortMapping) ([]string, []string, error) {
	var (
		proto string
		from  string
		to    string
		dport string
	)

	if strings.EqualFold(m.Protocol, "udp") {
		proto = "udp"
	} else {
		proto = "tcp"
	}

	if m.FromPorts.End == 0 || m.FromPorts.End == m.FromPorts.Begin {
		from = strconv.Itoa(m.FromPorts.Begin)
		m.FromPorts.End = m.FromPorts.Begin
	} else if m.FromPorts.End > m.FromPorts.Begin {
		from = fmt.Sprintf("%d:%d", m.FromPorts.Begin, m.FromPorts.End)
	} else {
		return []string{}, []string{}, fmt.Errorf("invalid from port range %d-%d", m.FromPorts.Begin, m.FromPorts.End)
	}

	if m.ToPorts.End == 0 || m.ToPorts.End == m.ToPorts.Begin {
		dport = strconv.Itoa(m.ToPorts.Begin)
		to = net.JoinHostPort(containerip, dport)
		m.ToPorts.End = m.ToPorts.Begin
	} else if m.ToPorts.End > m.ToPorts.Begin {
		dport = fmt.Sprintf("%d:%d", m.ToPorts.Begin, m.ToPorts.End)
		to = net.JoinHostPort(containerip, fmt.Sprintf("%d-%d", m.ToPorts.Begin, m.ToPorts.End))
	} else {
		return []string{}, []string{}, fmt.Errorf("invalid to port range %d-%d", m.ToPorts.Begin, m.ToPorts.End)
	}

	//we may map ports 1:N or N:N, but not M:N (M!=1, M!=N)
	hostRange := m.FromPorts.End - m.FromPorts.Begin
	containerRange := m.ToPorts.End - m.ToPorts.Begin
	if hostRange != 0 && hostRange != containerRange {
		return []string{}, []string{}, fmt.Errorf("range mismatch, cannot map ports %s to %s", from, to)
	}

	natArgs := []string{"-p", proto, "-m", proto, "--dport", from, "-j", "DNAT", "--to-destination", to}
	filterArgs := []string{"-d", containerip, "-p", proto, "-m", proto, "--dport", dport, "-j", "ACCEPT"}

	return natArgs, filterArgs, nil
}

func parseRawResultOnHyper(output []byte, err error) error {
	if err != nil {
		return err
	} else if len(output) != 0 {
		return &iptables.ChainError{Chain: "HYPER", Output: output}

	}
	return nil
}

func setupIptablesPortMaps(containerip string, maps []*PortMapping) error {
	var (
		revert      bool
		revertRules = [][]string{}
	)
	defer func() {
		if revert {
			hlog.Log(hlog.WARNING, "revert portmapping rules...")
			for _, r := range revertRules {
				hlog.Log(hlog.INFO, "revert rule: %v", r)
				err := parseRawResultOnHyper(iptables.Raw(r...))
				if err != nil {
					hlog.Log(hlog.ERROR, "failed to revert rule: %v", err)
					err = nil //just ignore
				}
			}
		}
	}()

	for _, m := range maps {

		natArgs, filterArgs, err := generateIptablesArgs(containerip, m)
		if err != nil {
			revert = true
			return err
		}

		//check if this rule has already existed
		if iptables.PortMapExists("HYPER", natArgs) {
			continue
		}

		if iptables.PortMapUsed("HYPER", m.Protocol, m.FromPorts.Begin, m.FromPorts.End) {
			revert = true
			return fmt.Errorf("Host port %v has aleady been used", m.FromPorts)
		}

		err = parseRawResultOnHyper(iptables.Raw(append([]string{"-t", "nat", "-I", "HYPER"}, natArgs...)...))
		if err != nil {
			revert = true
			return fmt.Errorf("Unable to setup NAT rule in HYPER chain: %s", err)
		}
		revertRules = append(revertRules, append([]string{"-t", "nat", "-D", "HYPER"}, natArgs...))

		if err = parseRawResultOnHyper(iptables.Raw(append([]string{"-I", "HYPER"}, filterArgs...)...)); err != nil {
			revert = true
			return fmt.Errorf("Unable to setup FILTER rule in HYPER chain: %s", err)
		}
		revertRules = append(revertRules, append([]string{"-D", "HYPER"}, filterArgs...))

		i := m.FromPorts.Begin
		j := m.ToPorts.Begin
		defer func() {
			if err != nil {
				i--
				for i >= m.FromPorts.Begin {
					PortMapper.ReleaseMap(m.Protocol, i)
					i--
				}
			}
		}()

		for i <= m.FromPorts.End {
			if err = PortMapper.AllocateMap(m.Protocol, i, containerip, j); err != nil {
				revert = true
				return err
			}
			i++
			j++
		}
	}
	/* forbid to map ports twice */
	return nil
}

func releaseIptablesPortMaps(containerip string, maps []*PortMapping) error {

release_loop:
	for _, m := range maps {
		if !strings.EqualFold(m.Protocol, "udp") {
			m.Protocol = "tcp"
		}

		if m.FromPorts.End == 0 {
			m.FromPorts.End = m.FromPorts.Begin
		}

		hlog.Log(hlog.DEBUG, "release port map %d-%d/%s", m.FromPorts.Begin, m.FromPorts.End, m.Protocol)
		for i := m.FromPorts.Begin; i <= m.FromPorts.End; i++ {
			err := PortMapper.ReleaseMap(m.Protocol, i)
			if err != nil {
				continue release_loop
			}
		}

		natArgs, filterArgs, err := generateIptablesArgs(containerip, m)
		if err != nil {
			continue
		}

		iptables.OperatePortMap(iptables.Delete, "HYPER", natArgs)

		iptables.Raw(append([]string{"-D", "HYPER"}, filterArgs...)...)
	}
	/* forbid to map ports twice */
	return nil
}
