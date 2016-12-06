package network

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network/iptables"
	"github.com/vishvananda/netlink"
)

const (
	IFNAMSIZ       = 16
	DEFAULT_CHANGE = 0xFFFFFFFF
	SIOC_BRADDBR   = 0x89a0
	SIOC_BRDELBR   = 0x89a1
	SIOC_BRADDIF   = 0x89a2
	CIFF_TAP       = 0x0002
	CIFF_NO_PI     = 0x1000
	CIFF_ONE_QUEUE = 0x2000
)

const (
	ipv4ForwardConf     = "/proc/sys/net/ipv4/ip_forward"
	ipv4ForwardConfPerm = 0644
)

var (
	native          binary.ByteOrder
	nextSeqNr       uint32
	disableIptables bool
)

type ifReq struct {
	Name  [IFNAMSIZ]byte
	Flags uint16
	pad   [0x28 - 0x10 - 2]byte
}

type IfInfomsg struct {
	syscall.IfInfomsg
}

type IfAddrmsg struct {
	syscall.IfAddrmsg
}

type ifreqIndex struct {
	IfrnName  [IFNAMSIZ]byte
	IfruIndex int32
}

type NetlinkRequestData interface {
	Len() int
	ToWireFormat() []byte
}

type IfAddr struct {
	iface *net.Interface
	ip    net.IP
	ipNet *net.IPNet
}

type RtAttr struct {
	syscall.RtAttr
	Data     []byte
	children []NetlinkRequestData
}

type NetlinkSocket struct {
	fd  int
	lsa syscall.SockaddrNetlink
}

type NetlinkRequest struct {
	syscall.NlMsghdr
	Data []NetlinkRequestData
}

// Network interface represents the networking stack of a container
type networkInterface struct {
	IP           net.IP
	PortMappings []net.Addr // There are mappings to the host interfaces
}

type ifaces struct {
	c map[string]*networkInterface
	sync.Mutex
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

func setupIPTables(addr net.Addr) error {
	if disableIptables {
		return nil
	}

	// Enable NAT
	natArgs := []string{"-s", addr.String(), "!", "-o", BridgeIface, "-j", "MASQUERADE"}

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
	gotoArgs := []string{"-o", BridgeIface, "-j", "HYPER"}
	if !iptables.Exists(iptables.Filter, "FORWARD", gotoArgs...) {
		if output, err := iptables.Raw(append([]string{"-I", "FORWARD"}, gotoArgs...)...); err != nil {
			return fmt.Errorf("Unable to setup goto HYPER rule %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD goto HYPER", Output: output}
		}
	}

	// Accept all outgoing packets
	outgoingArgs := []string{"-i", BridgeIface, "-j", "ACCEPT"}
	if !iptables.Exists(iptables.Filter, "FORWARD", outgoingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I", "FORWARD"}, outgoingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow outgoing packets: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD outgoing", Output: output}
		}
	}

	// Accept incoming packets for existing connections
	existingArgs := []string{"-o", BridgeIface, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}

	if !iptables.Exists(iptables.Filter, "FORWARD", existingArgs...) {
		if output, err := iptables.Raw(append([]string{"-I", "FORWARD"}, existingArgs...)...); err != nil {
			return fmt.Errorf("Unable to allow incoming packets: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "FORWARD incoming", Output: output}
		}
	}

	err := Modprobe("br_netfilter")
	if err != nil {
		glog.V(1).Infof("modprobe br_netfilter failed %s", err)
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

func init() {
	var x uint32 = 0x01020304
	if *(*byte)(unsafe.Pointer(&x)) == 0x01 {
		native = binary.BigEndian
	} else {
		native = binary.LittleEndian
	}
}

func InitNetwork(bIface, bIP string, disable bool) error {
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

	disableIptables = disable
	if disableIptables {
		glog.V(1).Info("Iptables is disabled")
	}

	addr, err := GetIfaceAddr(BridgeIface)
	if err != nil {
		glog.V(1).Infof("create bridge %s, ip %s", BridgeIface, BridgeIP)
		// No Bridge existent, create one

		// If the iface is not found, try to create it
		if err := configureBridge(BridgeIP, BridgeIface); err != nil {
			glog.Error("create bridge failed")
			return err
		}

		addr, err = GetIfaceAddr(BridgeIface)
		if err != nil {
			glog.Error("get iface addr failed")
			return err
		}

		BridgeIPv4Net = addr.(*net.IPNet)
	} else {
		glog.V(1).Info("bridge exist")
		// Validate that the bridge ip matches the ip specified by BridgeIP
		BridgeIPv4Net = addr.(*net.IPNet)

		if BridgeIP != "" {
			bip, ipnet, err := net.ParseCIDR(BridgeIP)
			if err != nil {
				return err
			}
			if !BridgeIPv4Net.Contains(bip) {
				return fmt.Errorf("Bridge ip (%s) does not match existing bridge configuration %s", addr, BridgeIP)
			}

			mask1, _ := ipnet.Mask.Size()
			mask2, _ := BridgeIPv4Net.Mask.Size()

			if mask1 != mask2 {
				return fmt.Errorf("Bridge netmask (%d) does not match existing bridge netmask %d", mask1, mask2)
			}
		}
	}

	err = setupIPTables(addr)
	if err != nil {
		return err
	}

	err = setupIPForwarding()
	if err != nil {
		return err
	}

	IpAllocator.RequestIP(BridgeIPv4Net, BridgeIPv4Net.IP)
	return nil
}

// Return the first IPv4 address for the specified network interface
func GetIfaceAddr(name string) (net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	var addr4 []net.Addr
	for _, addr := range addrs {
		ip := (addr.(*net.IPNet)).IP
		if ip4 := ip.To4(); ip4 != nil {
			addr4 = append(addr4, addr)
		}
	}

	if len(addr4) == 0 {
		return nil, fmt.Errorf("Interface %v has no IPv4 addresses", name)
	}
	return addr4[0], nil
}

// create and setup network bridge
func configureBridge(bridgeIP, bridgeIface string) error {
	var ifaceAddr string
	if len(bridgeIP) != 0 {
		_, _, err := net.ParseCIDR(bridgeIP)
		if err != nil {
			glog.Errorf("%s parsecidr failed", bridgeIP)
			return err
		}
		ifaceAddr = bridgeIP
	}

	if ifaceAddr == "" {
		return fmt.Errorf("Could not find a free IP address range for interface '%s'. Please configure its address manually", bridgeIface)
	}

	if err := CreateBridgeIface(bridgeIface); err != nil {
		// The bridge may already exist, therefore we can ignore an "exists" error
		if !os.IsExist(err) {
			glog.Errorf("CreateBridgeIface failed %s %s", bridgeIface, ifaceAddr)
			return err
		}
	}

	iface, err := net.InterfaceByName(bridgeIface)
	if err != nil {
		return err
	}

	ipAddr, ipNet, err := net.ParseCIDR(ifaceAddr)
	if err != nil {
		return err
	}

	if ipAddr.Equal(ipNet.IP) {
		ipAddr, err = IpAllocator.RequestIP(ipNet, nil)
	} else {
		ipAddr, err = IpAllocator.RequestIP(ipNet, ipAddr)
	}

	if err != nil {
		return err
	}

	glog.V(3).Infof("Allocate IP Address %s for bridge %s", ipAddr, bridgeIface)

	if err := NetworkLinkAddIp(iface, ipAddr, ipNet); err != nil {
		return fmt.Errorf("Unable to add private network: %s", err)
	}

	if err := NetworkLinkUp(iface); err != nil {
		return fmt.Errorf("Unable to start network bridge: %s", err)
	}
	return nil
}

func getNetlinkSocket() (*NetlinkSocket, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	s := &NetlinkSocket{
		fd: fd,
	}
	s.lsa.Family = syscall.AF_NETLINK
	if err := syscall.Bind(fd, &s.lsa); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	return s, nil
}

func (s *NetlinkSocket) Close() {
	syscall.Close(s.fd)
}

func (s *NetlinkSocket) Send(request *NetlinkRequest) error {
	if err := syscall.Sendto(s.fd, request.ToWireFormat(), 0, &s.lsa); err != nil {
		return err
	}
	return nil
}

func (s *NetlinkSocket) Receive() ([]syscall.NetlinkMessage, error) {
	rb := make([]byte, syscall.Getpagesize())
	nr, _, err := syscall.Recvfrom(s.fd, rb, 0)
	if err != nil {
		return nil, err
	}
	if nr < syscall.NLMSG_HDRLEN {
		return nil, fmt.Errorf("Got short response fromnetlink")
	}
	rb = rb[:nr]
	return syscall.ParseNetlinkMessage(rb)
}

func (s *NetlinkSocket) CheckMessage(m syscall.NetlinkMessage, seq, pid uint32) error {
	if m.Header.Seq != seq {
		return fmt.Errorf("netlink: invalid seq %d, expected %d", m.Header.Seq, seq)
	}
	if m.Header.Pid != pid {
		return fmt.Errorf("netlink: wrong pid %d, expected %d", m.Header.Pid, pid)
	}
	if m.Header.Type == syscall.NLMSG_DONE {
		return io.EOF
	}
	if m.Header.Type == syscall.NLMSG_ERROR {
		e := int32(native.Uint32(m.Data[0:4]))
		if e == 0 {
			return io.EOF
		}
		return syscall.Errno(-e)
	}
	return nil
}

func (s *NetlinkSocket) GetPid() (uint32, error) {
	lsa, err := syscall.Getsockname(s.fd)
	if err != nil {
		return 0, err
	}
	switch v := lsa.(type) {
	case *syscall.SockaddrNetlink:
		return v.Pid, nil
	}
	return 0, fmt.Errorf("Wrong socket type")
}

func (s *NetlinkSocket) HandleAck(seq uint32) error {
	pid, err := s.GetPid()
	if err != nil {
		return err
	}

outer:
	for {
		msgs, err := s.Receive()
		if err != nil {
			return err
		}
		for _, m := range msgs {
			if err := s.CheckMessage(m, seq, pid); err != nil {
				if err == io.EOF {
					break outer
				}
				return err
			}
		}
	}

	return nil
}

func newIfInfomsg(family int) *IfInfomsg {
	return &IfInfomsg{
		IfInfomsg: syscall.IfInfomsg{
			Family: uint8(family),
		},
	}
}

func newIfInfomsgChild(parent *RtAttr, family int) *IfInfomsg {
	msg := newIfInfomsg(family)
	parent.children = append(parent.children, msg)
	return msg
}

func (msg *IfInfomsg) ToWireFormat() []byte {
	length := syscall.SizeofIfInfomsg
	b := make([]byte, length)
	b[0] = msg.Family
	b[1] = 0
	native.PutUint16(b[2:4], msg.Type)
	native.PutUint32(b[4:8], uint32(msg.Index))
	native.PutUint32(b[8:12], msg.Flags)
	native.PutUint32(b[12:16], msg.Change)
	return b
}

func (msg *IfInfomsg) Len() int {
	return syscall.SizeofIfInfomsg
}

func newIfAddrmsg(family int) *IfAddrmsg {
	return &IfAddrmsg{
		IfAddrmsg: syscall.IfAddrmsg{
			Family: uint8(family),
		},
	}
}

func (msg *IfAddrmsg) ToWireFormat() []byte {

	length := syscall.SizeofIfAddrmsg
	glog.V(1).Infof("ifaddmsg length %d", length)
	b := make([]byte, length)
	b[0] = msg.Family
	b[1] = msg.Prefixlen
	b[2] = msg.Flags
	b[3] = msg.Scope
	native.PutUint32(b[4:8], uint32(msg.Index))
	return b
}

func (msg *IfAddrmsg) Len() int {
	return syscall.SizeofIfAddrmsg
}

func newRtAttr(attrType int, data []byte) *RtAttr {
	return &RtAttr{
		RtAttr: syscall.RtAttr{
			Type: uint16(attrType),
		},
		children: []NetlinkRequestData{},
		Data:     data,
	}
}

func rtaAlignOf(attrlen int) int {
	return (attrlen + syscall.RTA_ALIGNTO - 1) & ^(syscall.RTA_ALIGNTO - 1)
}

func (a *RtAttr) Len() int {
	if len(a.children) == 0 {
		return (syscall.SizeofRtAttr + len(a.Data))
	}

	l := 0
	for _, child := range a.children {
		l += child.Len()
	}
	l += syscall.SizeofRtAttr
	return rtaAlignOf(l + len(a.Data))
}

func (a *RtAttr) ToWireFormat() []byte {
	length := a.Len()
	buf := make([]byte, rtaAlignOf(length))

	if a.Data != nil {
		copy(buf[4:], a.Data)
	} else {
		next := 4
		for _, child := range a.children {
			childBuf := child.ToWireFormat()
			copy(buf[next:], childBuf)
			next += rtaAlignOf(len(childBuf))
		}
	}

	if l := uint16(length); l != 0 {
		native.PutUint16(buf[0:2], l)
	}
	native.PutUint16(buf[2:4], a.Type)
	return buf
}

func (rr *NetlinkRequest) ToWireFormat() []byte {
	length := rr.Len
	dataBytes := make([][]byte, len(rr.Data))
	for i, data := range rr.Data {
		dataBytes[i] = data.ToWireFormat()
		length += uint32(len(dataBytes[i]))
	}
	b := make([]byte, length)
	native.PutUint32(b[0:4], length)
	native.PutUint16(b[4:6], rr.Type)
	native.PutUint16(b[6:8], rr.Flags)
	native.PutUint32(b[8:12], rr.Seq)
	native.PutUint32(b[12:16], rr.Pid)

	next := 16
	for _, data := range dataBytes {
		copy(b[next:], data)
		next += len(data)
	}
	return b
}

func (rr *NetlinkRequest) AddData(data NetlinkRequestData) {
	if data != nil {
		rr.Data = append(rr.Data, data)
	}
}

func newNetlinkRequest(proto, flags int) *NetlinkRequest {
	return &NetlinkRequest{
		NlMsghdr: syscall.NlMsghdr{
			Len:   uint32(syscall.NLMSG_HDRLEN),
			Type:  uint16(proto),
			Flags: syscall.NLM_F_REQUEST | uint16(flags),
			Seq:   atomic.AddUint32(&nextSeqNr, 1),
		},
	}
}

func getIpFamily(ip net.IP) int {
	if len(ip) <= net.IPv4len {
		return syscall.AF_INET
	}
	if ip.To4() != nil {
		return syscall.AF_INET
	}
	return syscall.AF_INET6
}

func networkLinkIpAction(action, flags int, ifa IfAddr) error {
	s, err := getNetlinkSocket()
	if err != nil {
		return err
	}
	defer s.Close()

	family := getIpFamily(ifa.ip)

	nlreq := newNetlinkRequest(action, flags)

	msg := newIfAddrmsg(family)
	msg.Index = uint32(ifa.iface.Index)
	prefixLen, _ := ifa.ipNet.Mask.Size()
	msg.Prefixlen = uint8(prefixLen)
	nlreq.AddData(msg)

	var ipData []byte
	ipData = ifa.ip.To4()

	localData := newRtAttr(syscall.IFA_LOCAL, ipData)
	nlreq.AddData(localData)

	if err := s.Send(nlreq); err != nil {
		return err
	}

	return s.HandleAck(nlreq.Seq)
}

// Delete an IP address from an interface. This is identical to:
// ip addr del $ip/$ipNet dev $iface
func NetworkLinkDelIp(iface *net.Interface, ip net.IP, ipNet *net.IPNet) error {
	return networkLinkIpAction(
		syscall.RTM_DELADDR,
		syscall.NLM_F_ACK,
		IfAddr{iface, ip, ipNet},
	)
}

func NetworkLinkAddIp(iface *net.Interface, ip net.IP, ipNet *net.IPNet) error {
	return networkLinkIpAction(
		syscall.RTM_NEWADDR,
		syscall.NLM_F_CREATE|syscall.NLM_F_EXCL|syscall.NLM_F_ACK,
		IfAddr{iface, ip, ipNet},
	)
}

// Bring up a particular network interface.
// This is identical to running: ip link set dev $name up
func NetworkLinkUp(iface *net.Interface) error {
	s, err := getNetlinkSocket()
	if err != nil {
		return err
	}
	defer s.Close()

	nlreq := newNetlinkRequest(syscall.RTM_NEWLINK, syscall.NLM_F_ACK)

	msg := newIfInfomsg(syscall.AF_UNSPEC)
	msg.Index = int32(iface.Index)
	msg.Flags = syscall.IFF_UP
	msg.Change = syscall.IFF_UP
	nlreq.AddData(msg)

	if err := s.Send(nlreq); err != nil {
		return err
	}

	return s.HandleAck(nlreq.Seq)
}

// Bring down a particular network interface.
// This is identical to running: ip link set $name down
func NetworkLinkDown(iface *net.Interface) error {
	s, err := getNetlinkSocket()
	if err != nil {
		return err
	}
	defer s.Close()

	wb := newNetlinkRequest(syscall.RTM_NEWLINK, syscall.NLM_F_ACK)

	msg := newIfInfomsg(syscall.AF_UNSPEC)
	msg.Index = int32(iface.Index)
	msg.Flags = 0 & ^syscall.IFF_UP
	msg.Change = DEFAULT_CHANGE
	wb.AddData(msg)

	if err := s.Send(wb); err != nil {
		return err
	}

	return s.HandleAck(wb.Seq)
}

// THIS CODE DOES NOT COMMUNICATE WITH KERNEL VIA RTNETLINK INTERFACE
// IT IS HERE FOR BACKWARDS COMPATIBILITY WITH OLDER LINUX KERNELS
// WHICH SHIP WITH OLDER NOT ENTIRELY FUNCTIONAL VERSION OF NETLINK
func getIfSocket() (fd int, err error) {
	for _, socket := range []int{
		syscall.AF_INET,
		syscall.AF_PACKET,
		syscall.AF_INET6,
	} {
		if fd, err = syscall.Socket(socket, syscall.SOCK_DGRAM, 0); err == nil {
			break
		}
	}
	if err == nil {
		return fd, nil
	}
	return -1, err
}

// Create the actual bridge device.  This is more backward-compatible than
// netlink and works on RHEL 6.
func CreateBridgeIface(name string) error {
	if len(name) >= IFNAMSIZ {
		return fmt.Errorf("Interface name %s too long", name)
	}

	s, err := getIfSocket()
	if err != nil {
		return err
	}
	defer syscall.Close(s)

	nameBytePtr, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(s), SIOC_BRADDBR, uintptr(unsafe.Pointer(nameBytePtr))); err != 0 {
		return err
	}
	return nil
}

func DeleteBridge(name string) error {
	s, err := getIfSocket()
	if err != nil {
		return err
	}
	defer syscall.Close(s)

	nameBytePtr, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}

	var ifr ifReq
	copy(ifr.Name[:len(ifr.Name)-1], []byte(name))
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(s),
		syscall.SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifr))); err != 0 {
		return err
	}

	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(s),
		SIOC_BRDELBR, uintptr(unsafe.Pointer(nameBytePtr))); err != 0 {
		return err
	}
	return nil
}

// AddToBridge attch interface to the bridge,
// we only support ovs bridge and linux bridge at present.
func AddToBridge(iface, master *net.Interface) error {
	link, err := netlink.LinkByName(master.Name)
	if err != nil {
		return err
	}

	switch link.Type() {
	case "openvswitch":
		return AddToOpenvswitchBridge(iface, master)
	case "bridge":
		return AddToLinuxBridge(iface, master)
	default:
		return fmt.Errorf("unknown link type:%+v", link.Type())
	}
}

func AddToOpenvswitchBridge(iface, master *net.Interface) error {
	glog.V(1).Infof("Found ovs bridge %s, attaching tap %s to it\n", master.Name, iface.Name)

	// Check whether there is already a device with the same name has already been attached
	// to the ovs bridge or not. If so, skip the follow attaching operation.
	out, err := exec.Command("ovs-vsctl", "list-ports", master.Name).CombinedOutput()
	if err != nil {
		return err
	}
	ports := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, port := range ports {
		if port == iface.Name {
			glog.V(1).Infof("A port named %s already exists on bridge %s, using it.\n", iface.Name, master.Name)
			return nil
		}
	}

	// ovs command "ovs-vsctl add-port BRIDGE PORT" add netwok device PORT to BRIDGE,
	// PORT and BRIDGE here indicate the device name respectively.
	out, err = exec.Command("ovs-vsctl", "add-port", master.Name, iface.Name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Ovs failed to add port: %s, error :%v", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func AddToLinuxBridge(iface, master *net.Interface) error {
	if len(master.Name) >= IFNAMSIZ {
		return fmt.Errorf("Interface name %s too long", master.Name)
	}

	s, err := getIfSocket()
	if err != nil {
		return err
	}
	defer syscall.Close(s)

	ifr := ifreqIndex{}
	copy(ifr.IfrnName[:len(ifr.IfrnName)-1], master.Name)
	ifr.IfruIndex = int32(iface.Index)

	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(s), SIOC_BRADDIF, uintptr(unsafe.Pointer(&ifr))); err != 0 {
		return err
	}

	return nil
}

func GenRandomMac() (string, error) {
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

func SetupPortMaps(containerip string, maps []*api.PortDescription) error {
	if disableIptables || len(maps) == 0 {
		return nil
	}

	for _, m := range maps {
		var proto string

		if strings.EqualFold(m.Protocol, "udp") {
			proto = "udp"
		} else {
			proto = "tcp"
		}

		natArgs := []string{"-p", proto, "-m", proto, "--dport",
			strconv.Itoa(int(m.HostPort)), "-j", "DNAT", "--to-destination",
			net.JoinHostPort(containerip, strconv.Itoa(int(m.ContainerPort)))}

		if iptables.PortMapExists("HYPER", natArgs) {
			return nil
		}

		if iptables.PortMapUsed("HYPER", natArgs) {
			return fmt.Errorf("Host port %d has aleady been used", m.HostPort)
		}

		err := iptables.OperatePortMap(iptables.Insert, "HYPER", natArgs)
		if err != nil {
			return err
		}

		err = PortMapper.AllocateMap(m.Protocol, int(m.HostPort), containerip, int(m.ContainerPort))
		if err != nil {
			return err
		}

		filterArgs := []string{"-d", containerip, "-p", proto, "-m", proto,
			"--dport", strconv.Itoa(int(m.ContainerPort)), "-j", "ACCEPT"}
		if output, err := iptables.Raw(append([]string{"-I", "HYPER"}, filterArgs...)...); err != nil {
			return fmt.Errorf("Unable to setup forward rule in HYPER chain: %s", err)
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: "HYPER", Output: output}
		}
	}
	/* forbid to map ports twice */
	return nil
}

func ReleasePortMaps(containerip string, maps []*api.PortDescription) error {
	if disableIptables || len(maps) == 0 {
		return nil
	}

	for _, m := range maps {
		glog.V(1).Infof("release port map %d", m.HostPort)
		err := PortMapper.ReleaseMap(m.Protocol, int(m.HostPort))
		if err != nil {
			continue
		}

		var proto string

		if strings.EqualFold(m.Protocol, "udp") {
			proto = "udp"
		} else {
			proto = "tcp"
		}

		natArgs := []string{"-p", proto, "-m", proto, "--dport",
			strconv.Itoa(int(m.HostPort)), "-j", "DNAT", "--to-destination",
			net.JoinHostPort(containerip, strconv.Itoa(int(m.ContainerPort)))}

		iptables.OperatePortMap(iptables.Delete, "HYPER", natArgs)

		filterArgs := []string{"-d", containerip, "-p", proto, "-m", proto,
			"--dport", strconv.Itoa(int(m.ContainerPort)), "-j", "ACCEPT"}
		iptables.Raw(append([]string{"-D", "HYPER"}, filterArgs...)...)
	}
	/* forbid to map ports twice */
	return nil
}

func UpAndAddToBridge(name string) error {
	inf, err := net.InterfaceByName(name)
	if err != nil {
		glog.Error("cannot find network interface ", name)
		return err
	}
	brg, err := net.InterfaceByName(BridgeIface)
	if err != nil {
		glog.Error("cannot find bridge interface ", BridgeIface)
		return err
	}
	err = AddToBridge(inf, brg)
	if err != nil {
		glog.Errorf("cannot add %s to %s ", name, BridgeIface)
		return err
	}
	err = NetworkLinkUp(inf)
	if err != nil {
		glog.Error("cannot up interface ", name)
		return err
	}

	return nil
}

func GetTapFd(tapname, bridge string) (device string, tapFile *os.File, err error) {
	var (
		req   ifReq
		errno syscall.Errno
	)

	tapFile, err = os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return "", nil, err
	}

	req.Flags = CIFF_TAP | CIFF_NO_PI | CIFF_ONE_QUEUE
	if tapname != "" {
		copy(req.Name[:len(req.Name)-1], []byte(tapname))
	}
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, tapFile.Fd(),
		uintptr(syscall.TUNSETIFF),
		uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		err = fmt.Errorf("create tap device failed\n")
		tapFile.Close()
		return "", nil, err
	}

	device = strings.Trim(string(req.Name[:]), "\x00")

	tapIface, err := net.InterfaceByName(device)
	if err != nil {
		glog.Errorf("get interface by name %s failed %s", device, err)
		tapFile.Close()
		return "", nil, err
	}

	bIface, err := net.InterfaceByName(bridge)
	if err != nil {
		glog.Errorf("get interface by name %s failed", bridge)
		tapFile.Close()
		return "", nil, err
	}

	err = AddToBridge(tapIface, bIface)
	if err != nil {
		glog.Errorf("Add to bridge failed %s %s", bridge, device)
		tapFile.Close()
		return "", nil, err
	}

	err = NetworkLinkUp(tapIface)
	if err != nil {
		glog.Errorf("Link up device %s failed", device)
		tapFile.Close()
		return "", nil, err
	}

	return device, tapFile, nil

}

func AllocateAddr(requestedIP string) (*Settings, error) {

	ip, err := IpAllocator.RequestIP(BridgeIPv4Net, net.ParseIP(requestedIP))
	if err != nil {
		return nil, err
	}

	maskSize, _ := BridgeIPv4Net.Mask.Size()

	mac, err := GenRandomMac()
	if err != nil {
		glog.Errorf("Generate Random Mac address failed")
		return nil, err
	}

	return &Settings{
		Mac:         mac,
		IPAddress:   ip.String(),
		Gateway:     BridgeIPv4Net.IP.String(),
		Bridge:      BridgeIface,
		IPPrefixLen: maskSize,
		Device:      "",
		File:        nil,
		Automatic:   true,
	}, nil
}

func Allocate(vmId, requestedIP string, addrOnly bool) (*Settings, error) {

	setting, err := AllocateAddr(requestedIP)
	if err != nil {
		return nil, err
	}

	//TODO: will move to a dedicate method
	//err = SetupPortMaps(ip.String(), maps)
	//if err != nil {
	//	glog.Errorf("Setup Port Map failed %s", err)
	//	return nil, err
	//}

	device, tapFile, err := GetTapFd("", BridgeIface)
	if err != nil {
		IpAllocator.ReleaseIP(BridgeIPv4Net, net.ParseIP(setting.IPAddress))
		return nil, err
	}

	setting.Device = device
	setting.File = tapFile
	return setting, nil
}

func Configure(vmId, requestedIP string, addrOnly bool, inf *api.InterfaceDescription) (*Settings, error) {

	ip, mask, err := ipParser(inf.Ip)
	if err != nil {
		glog.Errorf("Parse config IP failed %s", err)
		return nil, err
	}

	maskSize, _ := mask.Size()

	/* TODO: Move port maps out of the plugging procedure
	err = SetupPortMaps(ip.String(), maps)
	if err != nil {
		glog.Errorf("Setup Port Map failed %s", err)
		return nil, err
	}
	*/

	mac := inf.Mac
	if mac == "" {
		mac, err = GenRandomMac()
		if err != nil {
			glog.Errorf("Generate Random Mac address failed")
			return nil, err
		}
	}

	if addrOnly {
		return &Settings{
			Mac:         mac,
			IPAddress:   ip.String(),
			Gateway:     inf.Gw,
			Bridge:      inf.Bridge,
			IPPrefixLen: maskSize,
			Device:      inf.TapName,
			File:        nil,
			Automatic:   false,
		}, nil
	}

	device, tapFile, err := GetTapFd(inf.TapName, inf.Bridge)
	if err != nil {
		return nil, err
	}

	return &Settings{
		Mac:         mac,
		IPAddress:   ip.String(),
		Gateway:     inf.Gw,
		Bridge:      inf.Bridge,
		IPPrefixLen: maskSize,
		Device:      device,
		File:        tapFile,
		Automatic:   false,
	}, nil
}

func Close(file *os.File) error {
	if file != nil {
		file.Close()
	}
	return nil
}

func ReleaseAddr(releasedIP string) error {
	if err := IpAllocator.ReleaseIP(BridgeIPv4Net, net.ParseIP(releasedIP)); err != nil {
		return err
	}
	return nil
}

// Release an interface for a select ip
func Release(vmId, releasedIP string) error {

	if err := ReleaseAddr(releasedIP); err != nil {
		return err
	}

	/* TODO: call this after release networks
	if err := ReleasePortMaps(releasedIP, maps); err != nil {
		glog.Errorf("fail to release port map %s", err)
		return err
	}
	*/
	return nil
}

func ipParser(ipstr string) (net.IP, net.IPMask, error) {
	glog.V(1).Info("parse IP addr ", ipstr)
	ip, ipnet, err := net.ParseCIDR(ipstr)
	if err == nil {
		return ip, ipnet.Mask, nil
	}

	ip = net.ParseIP(ipstr)
	if ip != nil {
		return ip, ip.DefaultMask(), nil
	}

	return nil, nil, err
}
