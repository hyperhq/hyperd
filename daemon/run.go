package daemon

import (
	"fmt"
	"io"

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

	if _, ok := daemon.PodList.Get(podSpec.Id); ok {
		return nil, fmt.Errorf("pod %s already exist", podSpec.Id)
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

//TODO: remove the tty stream in StartPod API, now we could support attach after created
func (daemon *Daemon) StartPod(stdin io.ReadCloser, stdout io.WriteCloser, podId string, attach bool) (int, string, error) {
	p, ok := daemon.PodList.Get(podId)
	if !ok {
		return -1, "", fmt.Errorf("The pod(%s) can not be found, please create it first", podId)
	}

	var waitTty chan error

	if attach {
		glog.V(1).Info("Run pod with tty attached")

		ids := p.ContainerIdsOf(apitypes.UserContainer_REGULAR)
		for _, id := range ids {
			waitTty = make(chan error, 1)
			p.Attach(id, stdin, stdout, waitTty)
			break
		}
	}

	glog.Infof("pod:%s, vm:%s", podId)

	err := p.Start()
	if err != nil {
		glog.Infof("failed to  start pod %s: %v", p.Id(), err)
		return -1, err.Error(), err
	}

	if waitTty != nil {
		<-waitTty
	}

	return 0, "", err
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
