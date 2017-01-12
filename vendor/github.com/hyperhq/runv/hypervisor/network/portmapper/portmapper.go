package portmapper

import (
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
)

type PortMap struct {
	containerIP   string
	containerPort int
}

func newPortMap(containerip string, containerport int) *PortMap {
	return &PortMap{
		containerIP:   containerip,
		containerPort: containerport,
	}
}

type PortSet map[int]*PortMap

type PortMapper struct {
	tcpMap PortSet
	udpMap PortSet
	mutex  sync.Mutex
}

func New() *PortMapper {
	return &PortMapper{PortSet{}, PortSet{}, sync.Mutex{}}
}

func (p *PortMapper) AllocateMap(protocol string, hostPort int,
	containerIP string, ContainerPort int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var pset PortSet

	if strings.EqualFold(protocol, "udp") {
		pset = p.udpMap
	} else {
		pset = p.tcpMap
	}

	e, ok := pset[hostPort]
	if ok {
		return fmt.Errorf("Host port %d had already been used, %s %d",
			hostPort, e.containerIP, e.containerPort)
	}

	allocated := newPortMap(containerIP, ContainerPort)
	pset[hostPort] = allocated

	return nil
}

func (p *PortMapper) ReleaseMap(protocol string, hostPort int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var pset PortSet

	if strings.EqualFold(protocol, "udp") {
		pset = p.udpMap
	} else {
		pset = p.tcpMap
	}

	_, ok := pset[hostPort]
	if !ok {
		glog.Errorf("Host port %d has not been used", hostPort)
	}

	delete(pset, hostPort)
	return nil
}
