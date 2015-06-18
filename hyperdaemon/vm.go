package daemon

import (
	"fmt"
	"strconv"

	"hyper/engine"
	"hyper/lib/glog"
	"hyper/pod"
	"hyper/hypervisor"
	"hyper/types"
)

func (daemon *Daemon) CmdVmCreate(job *engine.Job) (err error) {
	var (
		vmId          = fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))
		qemuPodEvent  = make(chan hypervisor.QemuEvent, 128)
		qemuStatus    = make(chan *types.QemuResponse, 128)
		subQemuStatus = make(chan *types.QemuResponse, 128)
		cpu           = 1
		mem           = 128
	)
	if job.Args[0] != "" {
		cpu, err = strconv.Atoi(job.Args[0])
		if err != nil {
			return err
		}
	}
	if job.Args[1] != "" {
		mem, err = strconv.Atoi(job.Args[1])
		if err != nil {
			return err
		}
	}
	b := &hypervisor.BootConfig{
		CPU:    cpu,
		Memory: mem,
		Kernel: daemon.kernel,
		Initrd: daemon.initrd,
		Bios:   daemon.bios,
		Cbfs:   daemon.cbfs,
	}
	go hypervisor.QemuLoop(vmId, qemuPodEvent, qemuStatus, b)
	if err := daemon.SetQemuChan(vmId, qemuPodEvent, qemuStatus, subQemuStatus); err != nil {
		glog.V(1).Infof("SetQemuChan error: %s", err.Error())
		return err
	}

	vm := &Vm{
		Id:     vmId,
		Pod:    nil,
		Status: types.S_VM_IDLE,
		Cpu:    cpu,
		Mem:    mem,
	}
	daemon.AddVm(vm)

	// Prepare the qemu status to client
	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", 0)
	v.Set("Cause", "")
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CmdVmKill(job *engine.Job) error {
	vmId := job.Args[0]
	if _, ok := daemon.vmList[vmId]; !ok {
		return fmt.Errorf("Can not find the VM(%s)", vmId)
	}
	code, cause, err := daemon.KillVm(vmId)
	if err != nil {
		return err
	}

	// Prepare the qemu status to client
	v := &engine.Env{}
	v.Set("ID", vmId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) KillVm(vmId string) (int, string, error) {
	qemuPodEvent, qemuStatus, subQemuStatus, err := daemon.GetQemuChan(vmId)
	if err != nil {
		return -1, "", err
	}
	var qemuResponse *types.QemuResponse
	shutdownPodEvent := &hypervisor.ShutdownCommand{Wait: false}
	qemuPodEvent.(chan hypervisor.QemuEvent) <- shutdownPodEvent
	// wait for the qemu response
	for {
		stop := 0
		select {
		case qemuResponse = <-qemuStatus.(chan *types.QemuResponse):
			glog.V(1).Infof("Got response: %d: %s", qemuResponse.Code, qemuResponse.Cause)
			if qemuResponse.Code == types.E_VM_SHUTDOWN {
				stop = 1
			}
		case qemuResponse = <-subQemuStatus.(chan *types.QemuResponse):
			glog.V(1).Infof("Got response: %d: %s", qemuResponse.Code, qemuResponse.Cause)
			if qemuResponse.Code == types.E_VM_SHUTDOWN {
				stop = 1
			}
		}
		if stop == 1 {
			break
		}
	}
	close(qemuStatus.(chan *types.QemuResponse))
	close(subQemuStatus.(chan *types.QemuResponse))
	daemon.RemoveVm(vmId)
	daemon.DeleteQemuChan(vmId)

	return qemuResponse.Code, qemuResponse.Cause, nil
}

// This function will only be invoked during daemon start
func (daemon *Daemon) AssociateAllVms() error {
	for _, mypod := range daemon.podList {
		if mypod.Vm == "" {
			continue
		}
		data, err := daemon.GetPodByName(mypod.Id)
		if err != nil {
			continue
		}
		userPod, err := pod.ProcessPodBytes(data)
		if err != nil {
			continue
		}
		glog.V(1).Infof("Associate the POD(%s) with VM(%s)", mypod.Id, mypod.Vm)
		var (
			qemuPodEvent  = make(chan hypervisor.QemuEvent, 128)
			qemuStatus    = make(chan *types.QemuResponse, 128)
			subQemuStatus = make(chan *types.QemuResponse, 128)
		)
		data, err = daemon.GetVmData(mypod.Vm)
		if err != nil {
			continue
		}
		glog.V(1).Infof("The data for vm(%s) is %v", mypod.Vm, data)
		go hypervisor.QemuAssociate(mypod.Vm, qemuPodEvent, qemuStatus, mypod.Wg, data)
		if err := daemon.SetQemuChan(mypod.Vm, qemuPodEvent, qemuStatus, subQemuStatus); err != nil {
			glog.V(1).Infof("SetQemuChan error: %s", err.Error())
			return err
		}
		vm := &Vm{
			Id:     mypod.Vm,
			Pod:    mypod,
			Status: types.S_VM_ASSOCIATED,
			Cpu:    userPod.Resource.Vcpu,
			Mem:    userPod.Resource.Memory,
		}
		daemon.AddVm(vm)
		daemon.SetContainerStatus(mypod.Id, types.S_POD_RUNNING)
		mypod.Status = types.S_POD_RUNNING
		go func(interface{}) {
			for {
				podId := mypod.Id
				qemuResponse := <-qemuStatus
				subQemuStatus <- qemuResponse
				if qemuResponse.Code == types.E_POD_FINISHED {
					data := qemuResponse.Data.([]uint32)
					daemon.SetPodContainerStatus(podId, data)
				} else if qemuResponse.Code == types.E_VM_SHUTDOWN {
					if daemon.podList[mypod.Id].Status == types.S_POD_RUNNING {
						daemon.podList[mypod.Id].Status = types.S_POD_SUCCEEDED
						daemon.SetContainerStatus(podId, types.S_POD_SUCCEEDED)
					}
					daemon.podList[mypod.Id].Vm = ""
					daemon.RemoveVm(mypod.Vm)
					daemon.DeleteQemuChan(mypod.Vm)
					if mypod.Type == "kubernetes" {
						switch mypod.Status {
						case types.S_POD_SUCCEEDED:
							if mypod.RestartPolicy == "always" {
								daemon.RestartPod(mypod)
							} else {
								daemon.DeletePodFromDB(podId)
								for _, c := range mypod.Containers {
									glog.V(1).Infof("Ready to rm container: %s", c.Id)
									if _, _, err = daemon.dockerCli.SendCmdDelete(c.Id); err != nil {
										glog.V(1).Infof("Error to rm container: %s", err.Error())
									}
								}
								//								daemon.RemovePod(podId)
								daemon.DeletePodContainerFromDB(podId)
								daemon.DeleteVolumeId(podId)
							}
							break
						case types.S_POD_FAILED:
							if mypod.RestartPolicy != "never" {
								daemon.RestartPod(mypod)
							} else {
								daemon.DeletePodFromDB(podId)
								for _, c := range mypod.Containers {
									glog.V(1).Infof("Ready to rm container: %s", c.Id)
									if _, _, err = daemon.dockerCli.SendCmdDelete(c.Id); err != nil {
										glog.V(1).Infof("Error to rm container: %s", err.Error())
									}
								}
								//								daemon.RemovePod(podId)
								daemon.DeletePodContainerFromDB(podId)
								daemon.DeleteVolumeId(podId)
							}
							break
						default:
							break
						}
					}
					break
				}
			}
		}(subQemuStatus)
	}
	return nil
}

func (daemon *Daemon) ReleaseAllVms() (int, error) {
	var qemuResponse *types.QemuResponse
	for vmId, vm := range daemon.vmList {
		qemuPodEvent, _, qemuStatus, err := daemon.GetQemuChan(vmId)
		if err != nil {
			return -1, err
		}
		if vm.Status == types.S_VM_IDLE {
			shutdownPodEvent := &hypervisor.ShutdownCommand{Wait: false}
			qemuPodEvent.(chan hypervisor.QemuEvent) <- shutdownPodEvent
			for {
				qemuResponse = <-qemuStatus.(chan *types.QemuResponse)
				if qemuResponse.Code == types.E_VM_SHUTDOWN {
					break
				}
			}
			close(qemuStatus.(chan *types.QemuResponse))
		} else {
			releasePodEvent := &hypervisor.ReleaseVMCommand{}
			qemuPodEvent.(chan hypervisor.QemuEvent) <- releasePodEvent
			for {
				qemuResponse = <-qemuStatus.(chan *types.QemuResponse)
				if qemuResponse.Code == types.E_VM_SHUTDOWN ||
					qemuResponse.Code == types.E_OK {
					break
				}
				if qemuResponse.Code == types.E_BUSY {
					return types.E_BUSY, fmt.Errorf("VM busy")
				}
			}
		}
	}
	return types.E_OK, nil
}
