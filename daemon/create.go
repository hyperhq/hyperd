package daemon

import (
	"fmt"
	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) CmdCreate(job *engine.Job) error {
	imgName := job.Args[0]
	cli := daemon.DockerCli
	body, _, err := cli.SendCmdCreate(imgName, []string{}, nil)
	if err != nil {
		return err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		return fmt.Errorf("Error while reading remote info!\n")
	}
	out.Close()

	v := &engine.Env{}
	v.SetJson("ID", daemon.ID)
	containerId := remoteInfo.Get("Id")
	if containerId != "" {
		v.Set("ContainerID", containerId)
		glog.V(3).Infof("The ContainerID is %s\n", containerId)
	} else {
		return fmt.Errorf("Hyper ERROR: AN error encountered during creating container!\n")
	}

	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
