package servicediscovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/linuxsignal"
)

var (
	ServiceVolume string = "/usr/local/etc/haproxy/"
	ServiceImage  string = "haproxy:1.5"
	ServiceConfig string = "haproxy.cfg"
)

func UpdateLoopbackAddress(vm *hypervisor.Vm, container string, oldServices, newServices []pod.UserService) error {
	addedIPs := make([]string, 0, 1)
	deletedIPs := make([]string, 0, 1)

	for _, n := range newServices {
		found := 0
		for _, o := range oldServices {
			if n.ServiceIP == o.ServiceIP {
				found = 1
			}
		}
		if found == 0 {
			addedIPs = append(addedIPs, n.ServiceIP)
		}
	}

	for _, o := range oldServices {
		found := 0
		for _, n := range newServices {
			if n.ServiceIP == o.ServiceIP {
				found = 1
			}
		}
		if found == 0 {
			deletedIPs = append(deletedIPs, o.ServiceIP)
		}
	}

	for _, ip := range addedIPs {
		err := SetupLoopbackAddress(vm, container, ip, "add")
		if err != nil {
			return err
		}
	}

	for _, ip := range deletedIPs {
		err := SetupLoopbackAddress(vm, container, ip, "del")
		if err != nil {
			return err
		}
	}

	return nil
}

// Setup lo ip address
// options for operation: add or del
func SetupLoopbackAddress(vm *hypervisor.Vm, container, ip, operation string) error {
	execId := fmt.Sprintf("exec-%s", utils.RandStr(10, "alpha"))
	command := "ip addr " + operation + " dev lo " + ip + "/32"
	execcmd, err := json.Marshal(strings.Split(command, " "))
	if err != nil {
		return err
	}

	tty := &hypervisor.TtyIO{
		Callback: make(chan *types.VmResponse, 1),
	}

	vm.Pod.AddExec(container, execId, command, false)
	defer vm.Pod.DeleteExec(execId)

	if err := vm.Exec(container, execId, string(execcmd), false, tty); err != nil {
		return err
	}

	es := vm.Pod.GetExec(execId)
	if es == nil {
		return fmt.Errorf("cannot find exec status for %s: %s", command, execId)
	}
	if es.ExitCode != 0 {
		return fmt.Errorf("exec %s on container %s failed with exit code %d", command, container, es.ExitCode)
	}

	return nil
}

func ApplyServices(vm *hypervisor.Vm, container string, services []pod.UserService) error {
	// Update lo ip addresses
	oldServices, err := GetServices(vm, container)
	if err != nil {
		return err
	}
	err = UpdateLoopbackAddress(vm, container, oldServices, services)
	if err != nil {
		return err
	}

	// Update haproxy config
	config := path.Join(ServiceVolume, ServiceConfig)
	vm.WriteFile(container, config, GenerateServiceConfig(services))

	return vm.KillContainer(container, linuxsignal.SIGHUP)
}

func GetServices(vm *hypervisor.Vm, container string) ([]pod.UserService, error) {
	var services []pod.UserService
	config := path.Join(ServiceVolume, ServiceConfig)

	data, err := vm.ReadFile(container, config)
	if err != nil {
		return nil, err
	}

	// if there's no data read, token will be empty and this method will return an empty service list
	token := bytes.Split(data, []byte("\n"))

	for _, tok := range token {
		first := bytes.Split(tok, []byte(" "))
		reader := bytes.NewReader(tok)
		if len(first) > 0 {
			var t1, t2, t3, t4 string
			if string(first[0][:]) == "frontend" {
				s := pod.UserService{
					Protocol: "TCP",
				}

				_, err := fmt.Fscanf(reader, "%s %s %s", &t1, &t2, &t3)
				if err != nil {
					return nil, err
				}

				hostport := strings.Split(t3, ":")
				s.ServiceIP = hostport[0]
				port, err := strconv.ParseInt(hostport[1], 10, 32)
				if err != nil {
					return nil, err
				}
				s.ServicePort = int(port)

				services = append(services, s)
			} else if string(first[0][:]) == "\tserver" {
				var idx int
				var h pod.UserServiceBackend
				_, err := fmt.Fscanf(reader, "%s %s %s %s", &t1, &t2, &t3, &t4)
				if err != nil {
					return nil, err
				}

				hostport := strings.Split(t3, ":")
				h.HostIP = hostport[0]
				port, err := strconv.ParseInt(hostport[1], 10, 32)
				if err != nil {
					return nil, err
				}
				h.HostPort = int(port)

				idxs := strings.Split(t2, "-")
				idxLong, err := strconv.ParseInt(idxs[1], 10, 32)
				if err != nil {
					return nil, err
				}
				idx = int(idxLong)

				services[idx].Hosts = append(services[idx].Hosts, h)
			}
		}
	}
	return services, nil
}

func GenerateServiceConfig(services []pod.UserService) []byte {
	data := []byte{}

	globalConfig := fmt.Sprintf("global\n\t#chroot\t/var/lib/haproxy\n\tpidfile\t/var/run/haproxy.pid\n\tmaxconn\t4000\n\t#user\thaproxy\n\t#group\thaproxy\n\tdaemon\ndefaults\n\tmode\ttcp\n\tretries\t3\n\ttimeout queue\t1m\n\ttimeout connect\t10s\n\ttimeout client\t1m\n\ttimeout server\t1m\n\ttimeout check\t10s\n\tmaxconn\t3000\n")

	data = append(data, globalConfig...)
	for idx, srv := range services {
		front := fmt.Sprintf("frontend front%d %s:%d\n\tdefault_backend\tback%d\n",
			idx, srv.ServiceIP, srv.ServicePort, idx)
		data = append(data, front...)
		back := fmt.Sprintf("backend back%d\n\tbalance\troundrobin\n", idx)
		data = append(data, back...)
		for hostid, host := range srv.Hosts {
			back := fmt.Sprintf("\tserver back-%d-%d %s:%d check\n",
				idx, hostid, host.HostIP, host.HostPort)
			data = append(data, back...)
		}
	}

	glog.V(1).Infof("haproxy config: %s", data[:])
	return data
}

func checkHaproxyConfig(services []pod.UserService, config string) error {
	var err error
	glog.V(1).Infof("haproxy config: %s\n", config)
	if _, err = os.Stat(config); err != nil && os.IsNotExist(err) {
		/* Generate haproxy config from service and write to config */
		return ioutil.WriteFile(config, GenerateServiceConfig(services), 0644)
	}
	return err
}

func PrepareServices(userPod *pod.UserPod, podId string) error {
	var serviceDir string = path.Join(utils.HYPER_ROOT, "services", podId)
	var config string = path.Join(serviceDir, ServiceConfig)
	var err error

	if len(userPod.Services) == 0 {
		return nil
	}

	if err = os.MkdirAll(serviceDir, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	return checkHaproxyConfig(userPod.Services, config)
}
