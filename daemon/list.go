package daemon

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) List(item, podId, vmId string, auxiliary bool) (map[string][]string, error) {
	var (
		pod                   *Pod           = nil
		vm                    *hypervisor.Vm = nil
		list                                 = make(map[string][]string)
		vmJsonResponse                       = []string{}
		podJsonResponse                      = []string{}
		containerJsonResponse                = []string{}
	)
	if item != "pod" && item != "container" && item != "vm" {
		return list, fmt.Errorf("Can not support %s list!", item)
	}

	daemon.PodList.RLock()
	glog.Infof("lock read of PodList")
	defer glog.Infof("unlock read of PodList")
	defer daemon.PodList.RUnlock()

	if podId != "" {
		var ok bool
		pod, ok = daemon.PodList.Get(podId)
		if !ok || (pod == nil) {
			return list, fmt.Errorf("Cannot find specified pod %s", podId)
		}
	}

	if vmId != "" {
		var ok bool
		vm, ok = daemon.VmList[vmId]
		if !ok || (vm == nil) {
			return list, fmt.Errorf("Cannot find specified vm %s", vmId)
		}
	}

	if item == "vm" {
		if podId == "" && vmId == "" {
			for v, info := range daemon.VmList {
				vmJsonResponse = append(vmJsonResponse, v+":"+showVM(info))
			}
		} else if podId != "" && vmId == "" {
			if v, ok := daemon.VmList[pod.status.Vm]; ok {
				vmJsonResponse = append(vmJsonResponse, pod.status.Vm+":"+showVM(v))
			}
		} else if podId == "" && vmId != "" {
			vmJsonResponse = append(vmJsonResponse, vmId+":"+showVM(vm))
		} else {
			if pod.status.Vm == vmId {
				vmJsonResponse = append(vmJsonResponse, vmId+":"+showVM(vm))
			}
		}

		list["vmData"] = vmJsonResponse
	}

	if item == "pod" {
		if podId == "" && vmId == "" {
			daemon.PodList.Foreach(func(p *Pod) error {
				podJsonResponse = append(podJsonResponse, p.id+":"+showPod(p.status))
				return nil
			})
		} else if podId != "" && vmId == "" {
			podJsonResponse = append(podJsonResponse, pod.id+":"+showPod(pod.status))
		} else if podId == "" && vmId != "" {
			daemon.PodList.Foreach(func(p *Pod) error {
				if p.status.Vm == vmId {
					podJsonResponse = append(podJsonResponse, p.id+":"+showPod(p.status))
				}
				return nil
			})
		} else {
			if pod.status.Vm == vmId {
				podJsonResponse = append(podJsonResponse, pod.id+":"+showPod(pod.status))
			}
		}
		list["podData"] = podJsonResponse
	}

	if item == "container" {
		if podId == "" && vmId == "" {
			daemon.PodList.Foreach(func(p *Pod) error {
				containerJsonResponse = append(containerJsonResponse, showPodContainers(p.status, auxiliary)...)
				return nil
			})
		} else if podId != "" && vmId == "" {
			containerJsonResponse = append(containerJsonResponse, showPodContainers(pod.status, auxiliary)...)
		} else if podId == "" && vmId != "" {
			daemon.PodList.Foreach(func(p *Pod) error {
				if p.status.Vm == vmId {
					containerJsonResponse = append(containerJsonResponse, showPodContainers(p.status, auxiliary)...)
				}
				return nil
			})
		} else {
			if pod.status.Vm == vmId {
				containerJsonResponse = append(containerJsonResponse, showPodContainers(pod.status, auxiliary)...)
			}
		}
		list["cData"] = containerJsonResponse
	}

	return list, nil
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

func showPod(pod *hypervisor.PodStatus) string {
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

func showPodContainers(pod *hypervisor.PodStatus, aux bool) []string {
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
