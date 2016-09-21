package daemon

import (
	"fmt"
	"io"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/golang/glog"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) Attach(stdin io.ReadCloser, stdout io.WriteCloser, container string) error {
	var (
		vmId string
		err  error
	)

	tty := &hypervisor.TtyIO{
		Stdin:    stdin,
		Stdout:   stdout,
		Callback: make(chan *types.VmResponse, 1),
	}

	pod, idx, err := daemon.GetPodByContainerIdOrName(container)
	if err != nil {
		return err
	}

	vmId, err = daemon.GetVmByPodId(pod.Id)
	if err != nil {
		return err
	}

	vm, ok := daemon.VmList.Get(vmId)
	if !ok {
		err = fmt.Errorf("Can find VM whose Id is %s!", vmId)
		return err
	}

	if !pod.Spec.Containers[idx].Tty {
		tty.Stderr = stdcopy.NewStdWriter(stdout, stdcopy.Stderr)
		tty.Stdout = stdcopy.NewStdWriter(stdout, stdcopy.Stdout)
		tty.OutCloser = stdout
	}

	err = vm.Attach(tty, container, nil)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(2).Info("Defer function for attach!")
	}()

	err = tty.WaitForFinish()

	return err
}
