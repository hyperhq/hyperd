package daemon

import (
	"fmt"

	"github.com/hyperhq/hyper/engine"
)

func (daemon *Daemon) CmdImages(job *engine.Job) error {
	var (
		imagesList = []string{}
	)
	images, err := daemon.DockerCli.SendCmdImages(job.Args[0])
	if err != nil {
		return err
	}
	for _, i := range images {
		id := i.ID
		created := fmt.Sprintf("%d", i.Created)
		size := fmt.Sprintf("%d", i.VirtualSize)
		for _, r := range i.RepoTags {
			imagesList = append(imagesList, r+":"+id+":"+created+":"+size)
		}
	}
	v := &engine.Env{}
	v.SetList("imagesList", imagesList)

	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}
