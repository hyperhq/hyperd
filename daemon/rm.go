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

	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodList.Unlock()

	os.RemoveAll(path.Join(utils.HYPER_ROOT, "services", podId))
	os.RemoveAll(path.Join(utils.HYPER_ROOT, "hosts", podId))
	pod, ok := daemon.PodList.Get(podId)
	if !ok {
		return -1, "", fmt.Errorf("Can not find that Pod(%s)", podId)
	}
	if pod.status.Status != types.S_POD_RUNNING {
		// If the pod type is kubernetes, we just remove the pod from the pod list.
		// The persistent data has been removed since we got the E_VM_SHUTDOWN event.
		if pod.status.Type == "kubernetes" {
			daemon.RemovePod(podId)
			code = types.E_OK
		} else {
			daemon.DeletePodFromDB(podId)
			for _, c := range pod.status.Containers {
				glog.V(1).Infof("Ready to rm container: %s", c.Id)
				if err = daemon.Daemon.ContainerRm(c.Id, &dockertypes.ContainerRmConfig{}); err != nil {
					glog.Warningf("Error to rm container: %s", err.Error())
				}
			}
			daemon.RemovePod(podId)
			daemon.DeletePodContainerFromDB(podId)
			daemon.DeleteVolumeId(podId)
			code = types.E_OK
		}
	} else {
		code, cause, err = daemon.StopPodWithLock(podId, "yes")
		if err != nil {
			return code, cause, err
		}
		if code == types.E_VM_SHUTDOWN {
			daemon.DeletePodFromDB(podId)
			for _, c := range pod.status.Containers {
				glog.V(1).Infof("Ready to rm container: %s", c.Id)
				if err = daemon.Daemon.ContainerRm(c.Id, &dockertypes.ContainerRmConfig{}); err != nil {
					glog.V(1).Infof("Error to rm container: %s", err.Error())
				}
			}
			daemon.RemovePod(podId)
			daemon.DeletePodContainerFromDB(podId)
			daemon.DeleteVolumeId(podId)
		}
		code = types.E_OK
	}

	return code, cause, nil
}
