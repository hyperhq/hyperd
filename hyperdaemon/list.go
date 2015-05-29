package daemon

import (
	"fmt"
	"hyper/engine"
	"hyper/types"
)

func (daemon *Daemon) CmdList(job *engine.Job) error {
	var item string
	if len(job.Args) == 0 {
		item = "pod"
	} else {
		item = job.Args[0]
	}
	if item != "pod" && item != "container" && item != "vm" {
		return fmt.Errorf("Can not support %s list!", item)
	}

	var (
		vmJsonResponse = []string{}
		podJsonResponse = []string{}
		containerJsonResponse = []string{}
		status string
		podId string
	)

	// Prepare the qemu status to client
	v := &engine.Env{}
	v.Set("item", item)
	if item == "vm" {
		for vm, v := range daemon.vmList {
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
			if v.Pod != nil {
				podId = v.Pod.Id
			}
			vmJsonResponse = append(vmJsonResponse, vm+":"+podId+":"+status)
		}
		v.SetList("vmData", vmJsonResponse)
	}

	if item == "pod" {
		for p, v := range daemon.podList {
			switch v.Status {
			case types.S_POD_RUNNING:
				status = "running"
				break
			case types.S_POD_CREATED:
				status = "pending"
				break
			case types.S_POD_FAILED:
				status = "failed"
				if v.Type == "kubernetes" {
					status = "failed(kubernetes)"
				}
				break
			case types.S_POD_SUCCEEDED:
				status = "succeeded"
				if v.Type == "kubernetes" {
					status = "succeeded(kubernetes)"
				}
				break
			default:
				status = ""
				break
			}
			podJsonResponse = append(podJsonResponse, p+":"+v.Name+":"+v.Vm+":"+status)
		}
		v.SetList("podData", podJsonResponse)
	}

	if item == "container" {
		for _, c := range daemon.containerList {
			switch c.Status {
			case types.S_POD_RUNNING:
				status = "running"
				break
			case types.S_POD_CREATED:
				status = "pending"
				break
			case types.S_POD_FAILED:
				status = "failed"
				break
			case types.S_POD_SUCCEEDED:
				status = "succeeded"
				break
			default:
				status = ""
			}
			containerJsonResponse = append(containerJsonResponse, c.Id+":"+c.PodId+":"+status)
		}
		v.SetList("cData", containerJsonResponse)
	}

	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
