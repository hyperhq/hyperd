package daemon

import (
	"fmt"
	"os"
	"path"

	dockertypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor/types"
)

const (
	E_NOT_FOUND       = -2
	E_UNDER_OPERATION = -1
	E_OK              = 0
)

func (daemon *Daemon) CleanPod(podId string) (int, string, error) {
	var (
		code  = E_OK
		cause = ""
		err   error
	)

	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return E_NOT_FOUND, "", fmt.Errorf("Can not find that Pod(%s)", podId)
	}

	if !pod.TransitionLock("rm") {
		glog.Errorf("Pod %s is under other operation", podId)
		return E_UNDER_OPERATION, "", fmt.Errorf("Pod %s is under other operation", podId)
	}
	defer pod.TransitionUnlock("rm")

	if pod.PodStatus.Status == types.S_POD_RUNNING {
		code, cause, err = daemon.StopPodWithinLock(pod)
		if err != nil {
			glog.Errorf("failed to stop pod %s", podId)
		}
	}

	pod.Lock()
	defer pod.Unlock()
	daemon.RemovePodResource(pod)
	return code, cause, err
}

func (p *Pod) ShouldWaitCleanUp() bool {
	return p.VM != nil
}

func (daemon *Daemon) RemovePodResource(p *Pod) {
	if p.ShouldWaitCleanUp() {
		glog.V(3).Infof("pod %s should wait clean up before being purged", p.Id)
		p.PodStatus.Status = types.S_POD_NONE
		return
	}
	glog.V(3).Infof("pod %s is being purged", p.Id)

	os.RemoveAll(path.Join(utils.HYPER_ROOT, "services", p.Id))
	os.RemoveAll(path.Join(utils.HYPER_ROOT, "hosts", p.Id))

	if p.PodStatus.Type != "kubernetes" {
		daemon.RemovePodContainer(p)
	}
	daemon.DeleteVolumeId(p.Id)
	daemon.db.DeletePod(p.Id)
	daemon.RemovePod(p.Id)
}

func (daemon *Daemon) RemovePodContainer(p *Pod) {
	for _, c := range p.PodStatus.Containers {
		glog.V(1).Infof("Ready to rm container: %s", c.Id)
		if err := daemon.Daemon.ContainerRm(c.Id, &dockertypes.ContainerRmConfig{}); err != nil {
			glog.Warningf("Error to rm container: %s", err.Error())
		}
	}
	daemon.db.DeleteP2C(p.Id)
}
