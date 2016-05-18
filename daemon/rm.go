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

func (daemon *Daemon) CleanPod(podId string) (int, string, error) {
	var (
		code  = 0
		cause = ""
		err   error
	)

	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return -1, "", fmt.Errorf("Can not find that Pod(%s)", podId)
	}

	if !pod.TransitionLock("rm") {
		glog.Errorf("Pod %s is under other operation", podId)
		return -1, "", fmt.Errorf("Pod %s is under other operation", podId)
	}
	defer pod.TransitionUnlock("rm")

	if pod.status.Status == types.S_POD_RUNNING {
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
	return p.vm != nil
}

func (daemon *Daemon) RemovePodResource(p *Pod) {
	if p.ShouldWaitCleanUp() {
		glog.V(3).Infof("pod %s should wait clean up before being purged", p.Id)
		p.status.Status = types.S_POD_NONE
		return
	}
	glog.V(3).Infof("pod %s is being purged", p.Id)

	os.RemoveAll(path.Join(utils.HYPER_ROOT, "services", p.Id))
	os.RemoveAll(path.Join(utils.HYPER_ROOT, "hosts", p.Id))

	if p.status.Type != "kubernetes" {
		daemon.RemovePodContainer(p)
	}
	daemon.DeleteVolumeId(p.Id)
	daemon.db.DeletePod(p.Id)
	daemon.RemovePod(p.Id)
}

func (daemon *Daemon) RemovePodContainer(p *Pod) {
	for _, c := range p.status.Containers {
		glog.V(1).Infof("Ready to rm container: %s", c.Id)
		if err := daemon.Daemon.ContainerRm(c.Id, &dockertypes.ContainerRmConfig{}); err != nil {
			glog.Warningf("Error to rm container: %s", err.Error())
		}
	}
	daemon.db.DeleteP2C(p.Id)
}
