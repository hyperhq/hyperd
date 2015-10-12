package daemon

import (
	"fmt"
	"os"
	"path"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdPodRm(job *engine.Job) (err error) {
	var (
		podId = job.Args[0]
		code  = 0
		cause = ""
	)

	daemon.PodList.Lock()
	glog.V(2).Infof("lock PodList")
	defer glog.V(2).Infof("unlock PodList")
	defer daemon.PodList.Unlock()
	code, cause, err = daemon.CleanPod(podId)
	if err != nil {
		return err
	}

	// Prepare the vm status to client
	v := &engine.Env{}
	v.Set("ID", podId)
	v.SetInt("Code", code)
	v.Set("Cause", cause)
	if _, err = v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) CleanPod(podId string) (int, string, error) {
	var (
		code  = 0
		cause = ""
		err   error
	)
	os.RemoveAll(path.Join(utils.HYPER_ROOT, "services", podId))
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
				if _, _, err = daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
					glog.Warningf("Error to rm container: %s", err.Error())
				}
			}
			daemon.RemovePod(podId)
			daemon.DeletePodContainerFromDB(podId)
			daemon.DeleteVolumeId(podId)
			code = types.E_OK
		}
	} else {
		code, cause, err = daemon.StopPod(podId, "yes")
		if err != nil {
			return -1, "", err
		}
		if code == types.E_VM_SHUTDOWN {
			daemon.DeletePodFromDB(podId)
			for _, c := range pod.status.Containers {
				glog.V(1).Infof("Ready to rm container: %s", c.Id)
				if _, _, err = daemon.DockerCli.SendCmdDelete(c.Id); err != nil {
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
