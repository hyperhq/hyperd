package daemon

import (
	"fmt"
	"os"
	"path"

	dockertypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/utils"
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
		code, cause, err = daemon.StopPodWithinLock(pod, "yes")
		if err != nil {
			glog.Errorf("failed to stop pod %s", podId)
		}
	}

	os.RemoveAll(path.Join(utils.HYPER_ROOT, "services", podId))
	os.RemoveAll(path.Join(utils.HYPER_ROOT, "hosts", podId))

	daemon.db.DeletePod(podId)
	daemon.RemovePod(podId)
	if pod.status.Type != "kubernetes" {
		daemon.RemovePodContainer(pod)
	}
	daemon.DeleteVolumeId(podId)
	code = types.E_OK

	return code, cause, nil
}

func (daemon *Daemon) RemovePodContainer(p *Pod) {
	for _, c := range p.status.Containers {
		glog.V(1).Infof("Ready to rm container: %s", c.Id)
		if err := daemon.Daemon.ContainerRm(c.Id, &dockertypes.ContainerRmConfig{}); err != nil {
			glog.Warningf("Error to rm container: %s", err.Error())
		}
	}
	daemon.db.DeleteP2C(p.id)
}
