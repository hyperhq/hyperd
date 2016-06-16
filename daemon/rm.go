package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/docker/docker/pkg/version"
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

func (daemon *Daemon) DeleteContainer(containerId string) error {
	pod, idx, ok := daemon.PodList.GetByContainerIdOrName(containerId)
	if !ok {
		return fmt.Errorf("can not find container %s", containerId)
	}

	if !pod.TransitionLock("rm") {
		glog.Errorf("Pod %s is under other operation", pod.Id)
		return fmt.Errorf("Pod %s is under other operation", pod.Id)
	}
	defer pod.TransitionUnlock("rm")

	if pod.status.Status == types.S_POD_RUNNING && pod.status.Containers[idx].Status == types.S_POD_RUNNING {
		err := daemon.StopContainerWithinLock(pod, containerId)
		if err != nil {
			return fmt.Errorf("failed to stop container %s", containerId)
		}
	}

	pod.Lock()
	defer pod.Unlock()

	pod.Status().DeleteContainer(containerId)

	daemon.PodList.Put(pod)
	if err := daemon.WritePodAndContainers(pod.Id); err != nil {
		glog.Errorf("Found an error while saving the Containers info: %v", err)
		return err
	}

	r, err := daemon.ContainerInspect(containerId, false, version.Version("1.21"))
	if err != nil {
		return err
	}

	if err := daemon.Daemon.ContainerRm(containerId, &dockertypes.ContainerRmConfig{}); err != nil {
		return err
	}

	rsp, ok := r.(*dockertypes.ContainerJSON)
	if !ok {
		return fmt.Errorf("fail to unpack container json response for %s of %s", containerId, pod.Id)
	}
	name := strings.TrimLeft(rsp.Name, "/")
	for i, c := range pod.spec.Containers {
		if name == c.Name {
			pod.spec.Containers = append(pod.spec.Containers[:i], pod.spec.Containers[i+1:]...)
			break
		}
	}
	podSpec, err := json.Marshal(pod.spec)
	if err != nil {
		glog.Errorf("Marshal podspec %v failed: %v", pod.spec, err)
		return err
	}
	if err = daemon.db.UpdatePod(pod.Id, podSpec); err != nil {
		glog.Errorf("Found an error while saving the POD file: %v", err)
		return err
	}

	jsons, err := pod.TryLoadContainers(daemon)
	if err != nil {
		return err
	}
	if err = pod.ParseContainerJsons(daemon, jsons); err != nil {
		glog.Errorf("Found an error while parsing the Containers json: %v", err)
		return err
	}

	daemon.PodList.Put(pod)

	return nil
}
