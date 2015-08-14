package daemon

import (
	"github.com/hyperhq/hyper/engine"
)

func (daemon *Daemon) CmdImagesRemove(job *engine.Job) error {
	imagesList := []string{}
	name := job.Args[0]
	force := job.Args[1]
	noprune := job.Args[2]
	list, err := daemon.DockerCli.SendImageDelete(name, force, noprune)
	if err != nil {
		return err
	}
	// FIXME
	_ = list
	v := &engine.Env{}
	v.SetList("imagesList", imagesList)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
