package pod

import (
	"fmt"
	"strings"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	apitypes "github.com/hyperhq/hyperd/types"
)

const (
	// set default mode to masquerading
	// Others are `i` tunneling and `g` gatewaying
	DEFAULT_MODE   string = "m"
	DEFAULT_WEITHT int    = 1
	// Others are wrr|lc|wlc|lblc|lblcr|dh|sh|sed|nq
	DEFAULT_SCHEDULER = "rr"
)

type Services struct {
	p *XPod

	spec []*apitypes.UserService
}

func newServices(p *XPod, spec []*apitypes.UserService) *Services {
	return &Services{
		p:    p,
		spec: spec,
	}
}

func (s *Services) LogPrefix() string {
	return fmt.Sprintf("%s[Serv] ", s.p.LogPrefix())
}

func (s *Services) Log(level hlog.LogLevel, args ...interface{}) {
	hlog.HLog(level, s, 1, args...)
}

type serviceKey struct {
	IP       string
	Port     int32
	Protocol string
}

func generateIPVSCmd(service *apitypes.UserService, op string) ([]byte, error) {
	if service == nil {
		return nil, nil
	}

	var (
		cmd       string
		protoFlag string
	)
	cmds := []byte{}
	DEFAULT_POSTFIX := fmt.Sprintf("-%s -w %d", DEFAULT_MODE, DEFAULT_WEITHT)

	if strings.ToLower(service.Protocol) == "tcp" {
		protoFlag = "-t"
	} else if strings.ToLower(service.Protocol) == "udp" {
		protoFlag = "-u"
	} else {
		return nil, fmt.Errorf("unsupported service protocol type: %s", service.Protocol)
	}
	sConf := fmt.Sprintf("%s %s:%d", protoFlag, service.ServiceIP, service.ServicePort)
	switch op {
	case "add":
		if service.ServiceIP == "" || service.ServicePort == 0 {
			return nil, fmt.Errorf("invlide service format, missing service IP or Port")
		}
		cmd = fmt.Sprintf("-A %s -s %s\n", sConf, DEFAULT_SCHEDULER)
		cmds = append(cmds, cmd...)
		for _, b := range service.Hosts {
			cmd = fmt.Sprintf("-a %s -r %s:%d %s\n", sConf, b.HostIP, b.HostPort, DEFAULT_POSTFIX)
			cmds = append(cmds, cmd...)
		}
	case "del":
		cmd = fmt.Sprintf("-D %s\n", sConf)
		cmds = append(cmds, cmd...)
	default:
		return nil, fmt.Errorf("undefined operation type: %s", op)
	}
	return cmds, nil
}

// add will only add services in list that don't exist, else failed
func (s *Services) add(newServs []*apitypes.UserService) error {
	var err error
	// check if adding service conflict with existing ones
	exist := make(map[serviceKey]bool, s.size())
	for _, srv := range s.spec {
		exist[serviceKey{srv.ServiceIP, srv.ServicePort, srv.Protocol}] = true
	}
	for _, srv := range newServs {
		if exist[serviceKey{srv.ServiceIP, srv.ServicePort, srv.Protocol}] {
			err = fmt.Errorf("service %v conflicts with existing ones", newServs)
			s.Log(ERROR, err)
			return err
		}
		exist[serviceKey{srv.ServiceIP, srv.ServicePort, srv.Protocol}] = true
	}

	// if pod is running, convert service to patch and send to vm
	if s.p.IsRunning() {
		if err = s.commit(newServs, "add"); err != nil {
			return err
		}
	}
	s.spec = append(s.spec, newServs...)

	return nil
}

func (s *Services) del(srvs []*apitypes.UserService) error {
	var err error
	tbd := make(map[serviceKey]bool, len(srvs))
	for _, srv := range srvs {
		tbd[serviceKey{srv.ServiceIP, srv.ServicePort, srv.Protocol}] = true
	}
	target := make([]*apitypes.UserService, 0, len(srvs))
	remain := make([]*apitypes.UserService, 0, s.size())
	for _, srv := range s.spec {
		if tbd[serviceKey{srv.ServiceIP, srv.ServicePort, srv.Protocol}] {
			s.Log(TRACE, "remove serivce %#v", srv)
			target = append(target, srv)
		} else {
			remain = append(remain, srv)
		}
	}

	if s.p.IsRunning() {
		if err = s.commit(target, "del"); err != nil {
			return err
		}
	}
	s.spec = remain

	return nil
}

// update removes services in list that already exist, and add with new ones
// or just add new ones if they are not exist already
func (s *Services) update(srvs []*apitypes.UserService) error {
	var err error
	// check if update service list conflicts
	tbd := make(map[serviceKey]bool, len(srvs))
	for _, srv := range srvs {
		key := serviceKey{srv.ServiceIP, srv.ServicePort, srv.Protocol}
		if tbd[key] {
			err = fmt.Errorf("given service list conflict: %v", srv)
			s.Log(ERROR, err)
			return err
		}
		tbd[key] = true
	}

	if s.p.IsRunning() {
		if err = s.commit(srvs, "update"); err != nil {
			return err
		}
	}
	s.spec = srvs

	return nil
}

func (s *Services) apply() error {
	return s.commit(s.spec, "add")
}

func (s *Services) commit(srvs []*apitypes.UserService, operation string) error {
	var (
		err   error
		patch []byte
	)
	if operation == "update" {
		// clear all rules first
		patch = append(patch, []byte("-C\n")...)
		operation = "add"
	}
	// generate patch
	for _, srv := range srvs {
		cmd, err := generateIPVSCmd(srv, operation)
		if err != nil {
			s.Log(ERROR, "faild to generate IPVS command: %v", err)
			return err
		}
		patch = append(patch, cmd...)
	}
	// send to vm
	if err = s.commitToVm(patch); err != nil {
		s.Log(ERROR, "faild to apply IPVS service patch: %v", err)
		return err
	}

	return nil
}

func (s *Services) commitToVm(patch []byte) error {
	s.Log(TRACE, "commit IPVS service patch: \n%s", string(patch))

	saveData, err := s.getFromVm()
	if err != nil {
		return err
	}

	clear := func() error {
		cmd := []string{"ipvsadm", "-C"}
		_, stderr, err := s.p.sandbox.HyperstartExecSync(cmd, nil)
		if err != nil {
			s.Log(ERROR, "clear ipvs rules failed: %v, %s", err, stderr)
			return err
		}

		return nil
	}

	apply := func(rules []byte) error {
		cmd := []string{"ipvsadm", "-R"}
		_, stderr, err := s.p.sandbox.HyperstartExecSync(cmd, rules)
		if err != nil {
			s.Log(ERROR, "apply ipvs rules failed: %v, %s", err, stderr)
			return err
		}

		return nil
	}

	if err = apply(patch); err != nil {
		// restore original ipvs services
		err1 := clear()
		if err1 != nil {
			s.Log(ERROR, "restore original ipvs services failed in clear stage: %v", err1)
			return err
		}
		err1 = apply(saveData)
		if err1 != nil {
			s.Log(ERROR, "restore original ipvs services failed in apply stage: %v", err1)
		}
		return err
	}

	return nil
}

func (s *Services) getFromVm() ([]byte, error) {
	cmd := []string{"ipvsadm", "-Ln"}
	stdout, stderr, err := s.p.sandbox.HyperstartExecSync(cmd, nil)
	if err != nil {
		s.Log(ERROR, "get ipvs service from vm failed: %v, %s", err, stderr)
		return nil, err
	}

	return stdout, nil
}

func (s *Services) size() int {
	if s.spec == nil {
		return 0
	}
	return len(s.spec)
}

func (s *Services) get() []*apitypes.UserService {
	return s.spec
}

func (p *XPod) GetServices() ([]*apitypes.UserService, error) {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	return p.services.get(), nil
}

func (p *XPod) UpdateService(srvs []*apitypes.UserService) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if err := p.services.update(srvs); err != nil {
		p.Log(ERROR, "failed to update services: %v", err)
		return err
	}

	return p.savePodMeta()
}

func (p *XPod) AddService(srvs []*apitypes.UserService) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if err := p.services.add(srvs); err != nil {
		p.Log(ERROR, "failed to add services: %v", err)
		return err
	}

	return p.savePodMeta()
}

func (p *XPod) DeleteService(srvs []*apitypes.UserService) error {
	p.resourceLock.Lock()
	defer p.resourceLock.Unlock()

	if err := p.services.del(srvs); err != nil {
		p.Log(ERROR, "failed to delete service: %v", err)
		return err
	}

	return p.savePodMeta()
}
