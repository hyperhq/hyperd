package daemon

import (
	"fmt"

	"github.com/golang/glog"

	"github.com/hyperhq/hyperd/daemon/pod"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
)

func (daemon *Daemon) CreatePod(podId string, podSpec *apitypes.UserPod) (*pod.XPod, error) {
	//FIXME: why restrict to 1024
	if daemon.PodList.CountRunning() >= 1024 {
		return nil, fmt.Errorf("There have already been %d running Pods", 1024)
	}
	if podId == "" {
		podId = fmt.Sprintf("pod-%s", utils.RandStr(10, "alpha"))
	}

	if podSpec.Id == "" {
		podSpec.Id = podId
	}

	if err := podSpec.Validate(); err != nil {
		return nil, err
	}

	factory := pod.NewPodFactory(daemon.Factory, daemon.PodList, daemon.db, daemon.Storage, daemon.Daemon, daemon.DefaultLog)

	p, err := pod.CreateXPod(factory, podSpec)
	if err != nil {
		glog.Errorf("%s: failed to add pod: %v", podSpec.Id, err)
		return nil, err
	}

	return p, nil
}

func (daemon *Daemon) StartPod(podId string) error {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}

	glog.Infof("Starting pod %q in vm: %q", podId, p.SandboxName())

	err := p.Start()
	if err != nil {
		glog.Infof("failed to  start pod %s: %v", p.Id(), err)
		return err
	}

	return err
}

func (daemon *Daemon) WaitContainer(cid string, second int) (int, error) {
	p, id, ok := daemon.PodList.GetByContainerIdOrName(cid)
	if !ok {
		return -1, fmt.Errorf("can not find container %s", cid)
	}

	return p.WaitContainer(id, second)

}

func (daemon *Daemon) SetPodLabels(pn string, override bool, labels map[string]string) error {

	p, ok := daemon.PodList.Get(pn)
	if !ok {
		return fmt.Errorf("Can not get Pod %s info", pn)
	}

	err := p.SetLabel(labels, override)
	if err != nil {
		return err
	}

	// TODO update the persisit info

	//spec, err := json.Marshal(p.spec)
	//if err != nil {
	//	return err
	//}
	//
	//if err := daemon.db.UpdatePod(p.name, spec); err != nil {
	//	return err
	//}

	return nil
}
