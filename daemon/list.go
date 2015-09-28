package daemon

import (
	"fmt"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdList(job *engine.Job) error {
	var (
		item                  string
		dedicade              bool            = false
		podId                 string          = ""
		auxiliary             bool            = false
		pod                   *hypervisor.Pod = nil
		vmJsonResponse                        = []string{}
		podJsonResponse                       = []string{}
		containerJsonResponse                 = []string{}
	)
	if len(job.Args) == 0 {
		item = "pod"
	} else {
		item = job.Args[0]
	}
	if item != "pod" && item != "container" && item != "vm" {
		return fmt.Errorf("Can not support %s list!", item)
	}

	if len(job.Args) > 1 && (job.Args[1] != "") {
		dedicade = true
		podId = job.Args[1]
	}

	if len(job.Args) > 2 && (job.Args[2] == "yes" || job.Args[2] == "true") {
		auxiliary = true
	}

	daemon.PodsMutex.RLock()
	glog.Infof("lock read of PodList")
	defer glog.Infof("unlock read of PodList")
	defer daemon.PodsMutex.RUnlock()

	if dedicade {
		var ok bool
		pod, ok = daemon.PodList[podId]
		if !ok || (pod == nil) {
			return fmt.Errorf("Cannot find specified pod %s", podId)
		}
	}

	// Prepare the VM status to client
	v := &engine.Env{}
	v.Set("item", item)
	if item == "vm" {
		if !dedicade {
			for vm, v := range daemon.VmList {
				vmJsonResponse = append(vmJsonResponse, vm+":"+showVM(v))
			}
		} else {
			if v, ok := daemon.VmList[pod.Vm]; ok {
				vmJsonResponse = append(vmJsonResponse, pod.Vm+":"+showVM(v))
			}
		}
		v.SetList("vmData", vmJsonResponse)
	}

	if item == "pod" {
		if !dedicade {
			for p, v := range daemon.PodList {
				podJsonResponse = append(podJsonResponse, p+":"+showPod(v))
			}
		} else {
			podJsonResponse = append(podJsonResponse, pod.Id+":"+showPod(pod))
		}
		v.SetList("podData", podJsonResponse)
	}

	if item == "container" {
		if !dedicade {
			for _, p := range daemon.PodList {
				containerJsonResponse = append(containerJsonResponse, showPodContainers(p, auxiliary)...)
			}
		} else {
			containerJsonResponse = append(containerJsonResponse, showPodContainers(pod, auxiliary)...)
		}
		v.SetList("cData", containerJsonResponse)
	}

	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func showVM(v *hypervisor.Vm) string {
	var status string
	switch v.Status {
	case types.S_VM_ASSOCIATED:
		status = "associated"
		break
	case types.S_VM_IDLE:
		status = "idle"
		break
	default:
		status = ""
		break
	}
	p := ""
	if v.Pod != nil {
		p = v.Pod.Id
	}

	return p + ":" + status
}

func showPod(pod *hypervisor.Pod) string {
	var status string

	switch pod.Status {
	case types.S_POD_RUNNING:
		status = "running"
	case types.S_POD_CREATED:
		status = "pending"
	case types.S_POD_FAILED:
		status = "failed"
		if pod.Type == "kubernetes" {
			status = "failed(kubernetes)"
		}
	case types.S_POD_SUCCEEDED:
		status = "succeeded"
		if pod.Type == "kubernetes" {
			status = "succeeded(kubernetes)"
		}
	default:
		status = ""
	}

	return pod.Name + ":" + pod.Vm + ":" + status
}

func showPodContainers(pod *hypervisor.Pod, aux bool) []string {

	rsp := []string{}
	filterServiceDiscovery := !aux && (pod.Type == "service-discovery")
	proxyName := ServiceDiscoveryContainerName(pod.Name)

	for _, c := range pod.Containers {
		if filterServiceDiscovery && c.Name == proxyName {
			continue
		}
		rsp = append(rsp, showContainer(c))
	}
	return rsp
}

func showContainer(c *hypervisor.Container) string {
	var status string

	switch c.Status {
	case types.S_POD_RUNNING:
		status = "running"
	case types.S_POD_CREATED:
		status = "pending"
	case types.S_POD_FAILED:
		status = "failed"
	case types.S_POD_SUCCEEDED:
		status = "succeeded"
	default:
		status = ""
	}

	return c.Id + ":" + c.Name + ":" + c.PodId + ":" + status
}
