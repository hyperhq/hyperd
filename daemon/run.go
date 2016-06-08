package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/golang/glog"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (daemon *Daemon) CreatePod(podId string, podSpec *apitypes.UserPod) (*Pod, error) {
	// we can only support 1024 Pods
	if daemon.GetRunningPodNum() > 1024 {
		return nil, fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}

	if podId == "" {
		podId = fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	}

	p, err := daemon.createPodInternal(podId, podSpec, false)
	if err != nil {
		return nil, err
	}

	/* Create pod may change the pod spec */
	spec, err := json.Marshal(p.spec)
	if err != nil {
		return nil, err
	}

	if err = daemon.AddPod(p, string(spec)); err != nil {
		return nil, err
	}

	return p, nil
}

func (daemon *Daemon) createPodInternal(podId string, podSpec *apitypes.UserPod, withinLock bool) (*Pod, error) {
	glog.V(2).Infof("podArgs: %s", podSpec.String())

	pod, err := NewPod(podSpec, podId, daemon)
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
	if daemon.GetRunningPodNum() > 1024 {
		return -1, "", fmt.Errorf("Pod full, the maximum Pod is 1024!")
	}

	var ttys []*hypervisor.TtyIO = []*hypervisor.TtyIO{}

	if tag != "" {
		// tag is only used to identify if attach to the container
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

	code, cause, err := daemon.StartInternal(p, vmId, nil, lazy, ttys, tag)
	if err != nil {
		glog.Error(err.Error())
		return -1, "", err
	}

	if err := p.InitializeFinished(daemon); err != nil {
		glog.Error(err.Error())
		return -1, "", err
	}

	if len(ttys) > 0 {
		ttys[0].WaitForFinish()

		p.RLock()
		ttyContainers := p.ctnInfo
		if p.spec.Type == "service-discovery" {
			ttyContainers = p.ctnInfo[1:]
		}

		if len(ttyContainers) > 0 {
			ttyContainers[0].ExitCode = ttys[0].ExitCode
		}
		p.RUnlock()
	}

	return code, cause, nil
}

func (daemon *Daemon) StartInternal(p *Pod, vmId string, config interface{}, lazy bool, streams []*hypervisor.TtyIO, tag string) (int, string, error) {
	if !p.TransitionLock("start") {
		return -1, "", fmt.Errorf("The pod(%s) is operting by others, please retry later", p.Id)
	}
	defer p.TransitionUnlock("start")

	if p.vm != nil {
		return -1, "", fmt.Errorf("pod %s is already running", p.Id)
	}

	vmResponse, err := p.Start(daemon, vmId, lazy, streams, tag)
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

	var podSpec apitypes.UserPod
	err = json.Unmarshal(podData, &podSpec)
	if err != nil {
		return err
	}

	// Start the pod
	pnew, err := daemon.CreatePod(pod.Id, &podSpec)
	if err != nil {
		glog.Errorf(err.Error())
		return err
	}
	_, _, err = daemon.StartInternal(pnew, "", nil, lazy, []*hypervisor.TtyIO{}, "")
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

	pod.Lock()
	defer pod.Unlock()

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

	if err := daemon.db.UpdatePod(pod.Id, spec); err != nil {
		return err
	}

	return nil
}
