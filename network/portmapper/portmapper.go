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
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if strings.EqualFold(protocol, "udp") {
		e, ok := p.udpMap[hostPort]
		if ok {
			return fmt.Errorf("Host port %d had already been used, %s %d",
					  hostPort, e.containerIP, e.containerPort)
		}

		allocated := newPortMap(containerIP, ContainerPort)
		p.udpMap[hostPort] = allocated
		return nil;
	}


	e, ok := p.tcpMap[hostPort]
	if ok {
		return fmt.Errorf("Host port %d had already been used, %s %d",
				  hostPort, e.containerIP, e.containerPort)
	}

	allocated := newPortMap(containerIP, ContainerPort)
	p.tcpMap[hostPort] = allocated

	return nil
}

func (p *PortMapper) ReleaseMap(protocol string, hostPort int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if strings.EqualFold(protocol, "udp") {
		_, ok := p.udpMap[hostPort]
		if !ok {
			glog.Errorf("Host port %d has not been used", hostPort)
		}

		delete(p.udpMap, hostPort)

	} else {
		_, ok := p.tcpMap[hostPort]
		if !ok {
			glog.Errorf("Host port %d has not been used", hostPort)
		}

		delete(p.tcpMap, hostPort)
	}

	return nil
}
