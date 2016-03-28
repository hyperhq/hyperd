package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"

	"github.com/golang/glog"
)

func (daemon *Daemon) CreatePod(podId, podArgs string) (*Pod, error) {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return nil, fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}

	if podId == "" {
		podId = fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	}

	p, err := daemon.createPodInternal(podId, podArgs, false)
	if err != nil {
		return nil, err
	}

	if err = daemon.AddPod(p, podArgs); err != nil {
		return nil, err
	}

	return p, nil
}

func (daemon *Daemon) createPodInternal(podId, podArgs string, withinLock bool) (*Pod, error) {
	glog.V(2).Infof("podArgs: %s", podArgs)

	pod, err := NewPod([]byte(podArgs), podId, daemon)
	if err != nil {
		return nil, err
	}

	// Creation
	if err = pod.DoCreate(daemon); err != nil {
		return nil, err
	}

	return pod, nil
}

func (daemon *Daemon) StartPod(stdin io.ReadCloser, stdout io.WriteCloser, podId, vmId, tag string) (int, string, error) {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() >= 1024 {
		return -1, "", fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}

	var ttys []*hypervisor.TtyIO = []*hypervisor.TtyIO{}

	if tag != "" {
		glog.V(1).Info("Pod Run with client terminal tag: ", tag)
		ttys = append(ttys, &hypervisor.TtyIO{
			Stdin:     stdin,
			Stdout:    stdout,
			ClientTag: tag,
			Callback:  make(chan *types.VmResponse, 1),
		})
	}

	glog.Infof("pod:%s, vm:%s", podId, vmId)

	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return -1, "", fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode() && vmId == ""

	code, cause, err := daemon.StartInternal(p, vmId, nil, lazy, types.VM_KEEP_NONE, ttys)
	if err != nil {
		glog.Error(err.Error())
		return -1, "", err
	}

	if len(ttys) > 0 {
		p.RLock()
		tty, ok := p.ttyList[tag]
		p.RUnlock()

		if ok {
			tty.WaitForFinish()
		}
	}

	return code, cause, nil
}

func (daemon *Daemon) StartInternal(p *Pod, vmId string, config interface{}, lazy bool, keep int, streams []*hypervisor.TtyIO) (int, string, error) {
	if p.vm != nil {
		return -1, "", fmt.Errorf("pod %s is already running", p.id)
	}

	vmResponse, err := p.Start(daemon, vmId, lazy, keep, streams)
	if err != nil {
		return -1, "", err
	}

	return vmResponse.Code, vmResponse.Cause, nil
}

// The caller must make sure that the restart policy and the status is right to restart
func (daemon *Daemon) RestartPod(mypod *hypervisor.PodStatus) error {
	// Remove the pod
	// The pod is stopped, the vm is gone
	pod, ok := daemon.PodList.Get(mypod.Id)
	if ok {
		daemon.RemovePodContainer(pod)
	}
	daemon.RemovePod(mypod.Id)
	daemon.DeleteVolumeId(mypod.Id)

	podData, err := daemon.db.GetPod(mypod.Id)
	if err != nil {
		return err
	}
	var lazy bool = hypervisor.HDriver.SupportLazyMode()

	// Start the pod
	pnew, err := daemon.CreatePod(pod.id, string(podData))
	if err != nil {
		glog.Errorf(err.Error())
		return err
	}
	_, _, err = daemon.StartInternal(pnew, "", nil, lazy, types.VM_KEEP_NONE, []*hypervisor.TtyIO{})
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	return nil
}

func (daemon *Daemon) SetPodLabels(podId string, override bool, labels map[string]string) error {

	var pod *Pod
	if strings.Contains(podId, "pod-") {
		var ok bool
		pod, ok = daemon.PodList.Get(podId)
		if !ok {
			return fmt.Errorf("Can not get Pod info with pod ID(%s)", podId)
		}
	} else {
		pod = daemon.PodList.GetByName(podId)
		if pod == nil {
			return fmt.Errorf("Can not get Pod info with pod name(%s)", podId)
		}
	}

	if pod.spec.Labels == nil {
		pod.spec.Labels = make(map[string]string)
	}

	for k := range labels {
		if _, ok := pod.spec.Labels[k]; ok && !override {
			return fmt.Errorf("Can't update label %s without override", k)
		}
	}

	for k, v := range labels {
		pod.spec.Labels[k] = v
	}

	spec, err := json.Marshal(pod.spec)
	if err != nil {
		return err
	}

	if err := daemon.db.UpdatePod(pod.id, spec); err != nil {
		return err
	}

	return nil
}
