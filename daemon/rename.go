package daemon

func (daemon *Daemon) ContainerRename(oldname, newname string) error {
	if err := daemon.Daemon.ContainerRename(oldname, newname); err != nil {
		return err
	}

	daemon.PodList.Find(func(p *Pod) bool {
		for _, c := range p.PodStatus.Containers {
			if c.Name == "/"+oldname {
				c.Name = "/" + newname
				return true
			}
		}
		return false
	})

	return nil
}
