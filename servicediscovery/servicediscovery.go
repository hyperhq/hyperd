package servicediscovery

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
)

func generateHaproxyConfig(services []pod.UserService, config string) error {
	data := []byte{}

	globalConfig := fmt.Sprintf("global\n\t#chroot\t/var/lib/haproxy\n\tpidfile\t/var/run/haproxy.pid\n\tmaxconn\t4000\n\t#user\thaproxy\n\t#group\thaproxy\n\tdaemon\ndefaults\n\tmode\ttcp\n\tretries\t3\n\ttimeout queue\t1m\n\ttimeout connect\t10s\n\ttimeout client\t1m\n\ttimeout server\t1m\n\ttimeout check\t10s\n\tmaxconn\t3000\n")

	data = append(data, globalConfig...)
	for idx, srv := range services {
		front := fmt.Sprintf("frontend front%d %s:%d\n\tuse_backend\tback%d\n",
			idx, srv.ServiceIP, srv.ServicePort, idx)
		data = append(data, front...)
		back := fmt.Sprintf("backend back%d\n\tbalance\troundrobin\n", idx)
		data = append(data, back...)
		for hostid, host := range srv.Hosts {
			back := fmt.Sprintf("\tserver\tback%d%d %s:%d check\n",
				idx, hostid, host.HostIP, host.HostPort)
			data = append(data, back...)
		}
	}

	glog.V(1).Infof("haproxy config: %s\n%s", config, data[:])
	return ioutil.WriteFile(config, data, 0644)
}

func checkHaproxyConfig(services []pod.UserService, config string) error {
	var err error
	glog.V(1).Infof("haproxy config: %s\n", config)
	if _, err = os.Stat(config); err != nil && os.IsNotExist(err) {
		/* Generate haproxy config from service */
		return generateHaproxyConfig(services, config)
	}
	return err
}

func PrepareServices(userPod *pod.UserPod, podId string) error {
	var haproxyDir string = path.Join(utils.HYPER_ROOT, "services", podId, "haproxy")
	var config string = path.Join(haproxyDir, "haproxy.cfg")
	var err error

	if len(userPod.Services) == 0 {
		return nil
	}

	if err = os.MkdirAll(haproxyDir, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	return checkHaproxyConfig(userPod.Services, config)
}
