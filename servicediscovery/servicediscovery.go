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

	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
)

var (
	ServiceVolume string = "/usr/local/etc/haproxy/"
	ServiceImage  string = "haproxy:latest"
	ServiceConfig string = "haproxy.cfg"
)

func ApplyServices(vm *hypervisor.Vm, container string, services []pod.UserService) error {
	config := path.Join(ServiceVolume, ServiceConfig)

	vm.WriteFile(container, config, GenerateServiceConfig(services))

	command := "haproxy -D -f /usr/local/etc/haproxy/haproxy.cfg -p /var/run/haproxy.pid -sf `cat /var/run/haproxy.pid`"
	execcmd, err := json.Marshal(command)
	if err != nil {
		return err
	}

	return vm.Exec(nil, nil, string(execcmd), "", container)
}

func GetServices(vm *hypervisor.Vm, container string) ([]pod.UserService, error) {
	var services []pod.UserService
	config := path.Join(ServiceVolume, ServiceConfig)

	data, err := vm.ReadFile(container, config)
	if err != nil {
		return nil, err
	}

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
