package portmapper

import (
	"fmt"
	"sync"
	"strings"
	"hyper/lib/glog"
)

type PortMap struct {
	containerIP	string
	containerPort	int
}

func newPortMap(containerip string, containerport int) *PortMap {
	return &PortMap {
		containerIP:	containerip,
		containerPort:	containerport,
	}
}

type PortSet map[int]*PortMap

type PortMapper struct {
	tcpMap		PortSet
	udpMap		PortSet
	mutex		sync.Mutex
}

func New() *PortMapper {
	return &PortMapper{PortSet{}, PortSet{}, sync.Mutex{}}
}

func (p *PortMapper) AllocateMap(protocol string, hostPort int,
				 containerIP string, ContainerPort int) error {
	var portset PortSet

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if strings.EqualFold(protocol, "udp") {
		portset = p.udpMap
	} else {
		portset = p.tcpMap
	}

	_, ok := portset[hostPort]
	if ok {
		return fmt.Errorf("Host port %d had already been used", hostPort)
	}

	allocated := newPortMap(containerIP, ContainerPort)
	portset[hostPort] = allocated

	return nil
}

func (p *PortMapper) ReleaseMap(protocol string, hostPort int) error {
	var portset PortSet

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if strings.EqualFold(protocol, "udp") {
		portset = p.udpMap
	} else {
		portset = p.tcpMap
	}

	_, ok := portset[hostPort]
	if !ok {
		glog.Errorf("Host port %d has not been used", hostPort)
	}

	portset[hostPort] = nil
	return nil
}
