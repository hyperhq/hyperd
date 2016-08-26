package daemon

import (
	"fmt"
	"strings"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) ListVMs(podId, vmId string) ([]*hypervisor.Vm, error) {
	var (
		pod *Pod           = nil
		vm  *hypervisor.Vm = nil
	)

	if podId != "" {
		var ok bool
		pod, ok = daemon.PodList.Get(podId)
		if !ok || (pod == nil) {
			return nil, fmt.Errorf("Cannot find specified pod %s", podId)
		}
	}

	if vmId != "" {
		var ok bool
		vm, ok = daemon.VmList.Get(vmId)
		if !ok || (vm == nil) {
			return nil, fmt.Errorf("Cannot find specified vm %s", vmId)
		}
	}

	results := make([]*hypervisor.Vm, 0)
	if pod == nil && vm == nil {
		daemon.VmList.Foreach(func(vm *hypervisor.Vm) error {
			results = append(results, vm)
			return nil
		})
	} else if pod != nil && vm == nil {
		if pod.VM != nil {
			results = append(results, pod.VM)
		}
	} else if pod == nil && vm != nil {
		results = append(results, vm)
	} else {
		if pod.PodStatus.Vm == vm.Id {
			results = append(results, vm)
		}
	}

	return results, nil
}

func (daemon *Daemon) ListPods(podId, vmId string) ([]*Pod, error) {
	var (
		pod *Pod           = nil
		vm  *hypervisor.Vm = nil
	)

	if podId != "" {
		var ok bool
		pod, ok = daemon.PodList.Get(podId)
		if !ok || (pod == nil) {
			return nil, fmt.Errorf("Cannot find specified pod %s", podId)
		}
	}

	if vmId != "" {
		var ok bool
		vm, ok = daemon.VmList.Get(vmId)
		if !ok || (vm == nil) {
			return nil, fmt.Errorf("Cannot find specified vm %s", vmId)
		}
	}

	results := make([]*Pod, 0)
	if pod == nil && vm == nil {
		daemon.PodList.Foreach(func(p *Pod) error {
			if p.PodStatus.Status != types.S_POD_NONE {
				results = append(results, p)
			}
			return nil
		})
	} else if pod != nil && vm == nil {
		if pod.PodStatus.Status != types.S_POD_NONE {
			results = append(results, pod)
		}
	} else if pod == nil && vm != nil {
		daemon.PodList.Foreach(func(p *Pod) error {
			if p.PodStatus.Vm == vmId && p.PodStatus.Status != types.S_POD_NONE {
				results = append(results, p)
			}
			return nil
		})
	} else {
		if pod.PodStatus.Vm == vmId && pod.PodStatus.Status != types.S_POD_NONE {
			results = append(results, pod)
		}
	}

	return results, nil
}

func filterPodContainers(pod *hypervisor.PodStatus, aux bool) []*hypervisor.ContainerStatus {
	results := make([]*hypervisor.ContainerStatus, 0)

	filterServiceDiscovery := !aux && (pod.Type == "service-discovery")

	for _, c := range pod.Containers {
		// NOTE(harry) filter out containers ended with "service-discovery" which is for internal usage
		if filterServiceDiscovery && strings.HasSuffix(c.Name, "service-discovery") {
			continue
		}
		results = append(results, c)
	}
	return results
}

func (daemon *Daemon) ListContainers(podId, vmId string, auxiliary bool) ([]*hypervisor.ContainerStatus, error) {
	var (
		pod *Pod           = nil
		vm  *hypervisor.Vm = nil
	)

	if podId != "" {
		var ok bool
		pod, ok = daemon.PodList.Get(podId)
		if !ok || (pod == nil) {
			return nil, fmt.Errorf("Cannot find specified pod %s", podId)
		}
	}

	if vmId != "" {
		var ok bool
		vm, ok = daemon.VmList.Get(vmId)
		if !ok || (vm == nil) {
			return nil, fmt.Errorf("Cannot find specified vm %s", vmId)
		}
	}

	results := make([]*hypervisor.ContainerStatus, 0)
	if pod == nil && vm == nil {
		daemon.PodList.Foreach(func(p *Pod) error {
			filteredContainers := filterPodContainers(p.PodStatus, auxiliary)
			results = append(results, filteredContainers...)
			return nil
		})
	} else if pod != nil && vm == nil {
		filteredContainers := filterPodContainers(pod.PodStatus, auxiliary)
		results = append(results, filteredContainers...)
	} else if pod == nil && vm != nil {
		daemon.PodList.Foreach(func(p *Pod) error {
			if p.PodStatus.Vm == vmId {
				filteredContainers := filterPodContainers(p.PodStatus, auxiliary)
				results = append(results, filteredContainers...)
			}
			return nil
		})
	} else {
		if pod.PodStatus.Vm == vmId {
			filteredContainers := filterPodContainers(pod.PodStatus, auxiliary)
			results = append(results, filteredContainers...)
		}
	}

	return results, nil
}

func (daemon *Daemon) List(item, podId, vmId string, auxiliary bool) (map[string][]string, error) {
	var (
		list                  = make(map[string][]string)
		vmJsonResponse        = []string{}
		podJsonResponse       = []string{}
		containerJsonResponse = []string{}
	)
	if item != "pod" && item != "container" && item != "vm" {
		return list, fmt.Errorf("Can not support %s list!", item)
	}

	if item == "vm" {
		VMs, err := daemon.ListVMs(podId, vmId)
		if err != nil {
			return list, err
		}

		for _, vm := range VMs {
			vmJsonResponse = append(vmJsonResponse, vm.Id+":"+daemon.showVM(vm))
		}

		list["vmData"] = vmJsonResponse
	}

	if item == "pod" {
		pods, err := daemon.ListPods(podId, vmId)
		if err != nil {
			return list, err
		}

		for _, p := range pods {
			podJsonResponse = append(podJsonResponse, p.Id+":"+daemon.showPod(p.PodStatus))
		}

		list["podData"] = podJsonResponse
	}

	if item == "container" {
		containers, err := daemon.ListContainers(podId, vmId, auxiliary)
		if err != nil {
			return list, err
		}

		for _, c := range containers {
			containerJsonResponse = append(containerJsonResponse, daemon.showContainer(c))
		}

		list["cData"] = containerJsonResponse
	}

	return list, nil
}

func (daemon *Daemon) GetVMStatus(state uint) string {
	var status string

	switch state {
	case types.S_VM_ASSOCIATED:
		status = "associated"
		break
	case types.S_VM_IDLE:
		status = "idle"
		break
	case types.S_VM_PAUSED:
		status = "pasued"
		break
	default:
		status = ""
		break
	}

	return status
}

func (daemon *Daemon) showVM(v *hypervisor.Vm) string {

	p := ""
	if v.Pod != nil {
		p = v.Pod.Id
	}

	return p + ":" + daemon.GetVMStatus(v.Status)
}

func (daemon *Daemon) GetPodStatus(state uint, podType string) string {
	var status string

	switch state {
	case types.S_POD_RUNNING:
		status = "running"
	case types.S_POD_CREATED:
		status = "pending"
	case types.S_POD_FAILED:
		status = "failed"
		if podType == "kubernetes" {
			status = "failed(kubernetes)"
		}
	case types.S_POD_PAUSED:
		status = "paused"
	case types.S_POD_SUCCEEDED:
		status = "succeeded"
		if podType == "kubernetes" {
			status = "succeeded(kubernetes)"
		}
	default:
		status = ""
	}

	return status
}

func (daemon *Daemon) showPod(pod *hypervisor.PodStatus) string {
	return pod.Name + ":" + pod.Vm + ":" + daemon.GetPodStatus(pod.Status, pod.Type)
}

func (daemon *Daemon) GetContainerStatus(state uint32) string {
	var status string

	switch state {
	case types.S_POD_RUNNING:
		status = "running"
	case types.S_POD_CREATED:
		status = "pending"
	case types.S_POD_FAILED:
		status = "failed"
	case types.S_POD_SUCCEEDED:
		status = "succeeded"
	case types.S_POD_PAUSED:
		status = "paused"
	default:
		status = ""
	}

	return status
}

func (daemon *Daemon) showContainer(c *hypervisor.ContainerStatus) string {
	return c.Id + ":" + c.Name + ":" + c.PodId + ":" + daemon.GetContainerStatus(c.Status)
}
