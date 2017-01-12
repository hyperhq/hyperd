package virtualbox

import (
	"fmt"
	"net"
)

// PFRule represents a port forwarding rule.
type PFRule struct {
	Proto     PFProto
	HostIP    net.IP // can be nil to match any host interface
	HostPort  uint16
	GuestIP   net.IP // can be nil if guest IP is leased from built-in DHCP
	GuestPort uint16
}

// PFProto represents the protocol of a port forwarding rule.
type PFProto string

const (
	PFTCP = PFProto("tcp")
	PFUDP = PFProto("udp")
)

// String returns a human-friendly representation of the port forwarding rule.
func (r PFRule) String() string {
	hostip := ""
	if r.HostIP != nil {
		hostip = r.HostIP.String()
	}
	guestip := ""
	if r.GuestIP != nil {
		guestip = r.GuestIP.String()
	}
	return fmt.Sprintf("%s://%s:%d --> %s:%d",
		r.Proto, hostip, r.HostPort,
		guestip, r.GuestPort)
}

// Format returns the string needed as a command-line argument to VBoxManage.
func (r PFRule) Format() string {
	hostip := ""
	if r.HostIP != nil {
		hostip = r.HostIP.String()
	}
	guestip := ""
	if r.GuestIP != nil {
		guestip = r.GuestIP.String()
	}
	return fmt.Sprintf("%s,%s,%d,%s,%d", r.Proto, hostip, r.HostPort, guestip, r.GuestPort)
}
