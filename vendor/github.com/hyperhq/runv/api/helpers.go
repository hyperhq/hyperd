package api

import (
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func (v *VolumeDescription) IsDir() bool {
	return v.Format == "vfs"
}

func (v *VolumeDescription) IsNas() bool {
	return v.Format == "nas"
}

func SandboxInfoFromOCF(s *specs.Spec) *SandboxConfig {
	return &SandboxConfig{
		Hostname: s.Hostname,
	}
}

func ContainerDescriptionFromOCF(id string, s *specs.Spec) *ContainerDescription {
	container := &ContainerDescription{
		Id:         id,
		Name:       s.Hostname,
		Image:      "",
		Labels:     make(map[string]string),
		Tty:        s.Process.Terminal,
		RootVolume: nil,
		MountId:    "",
		RootPath:   "rootfs",
		UGI:        UGIFromOCF(&s.Process.User),
		Envs:       make(map[string]string),
		Workdir:    s.Process.Cwd,
		Path:       s.Process.Args[0],
		Args:       s.Process.Args[1:],
		Rlimits:    []*Rlimit{},
		Sysctl:     s.Linux.Sysctl,
	}

	for _, value := range s.Process.Env {
		values := strings.SplitN(value, "=", 2)
		container.Envs[values[0]] = values[1]
	}

	for idx := range s.Process.Rlimits {
		container.Rlimits = append(container.Rlimits, &Rlimit{
			Type: s.Process.Rlimits[idx].Type,
			Hard: s.Process.Rlimits[idx].Hard,
			Soft: s.Process.Rlimits[idx].Soft,
		})
	}
	// TODO handle Rlimits in hyperstart
	container.Rlimits = []*Rlimit{}

	if container.Sysctl == nil {
		container.Sysctl = map[string]string{}
	}
	container.Sysctl["vm.overcommit_memory"] = "1"

	rootfs := &VolumeDescription{
		Name:   id,
		Source: id,
		Fstype: "dir",
		Format: "vfs",
	}
	container.RootVolume = rootfs

	return container
}

func UGIFromOCF(u *specs.User) *UserGroupInfo {

	if u == nil || (u.UID == 0 && u.GID == 0 && len(u.AdditionalGids) == 0) {
		return nil
	}

	ugi := &UserGroupInfo{}
	if u.UID != 0 {
		ugi.User = strconv.FormatUint(uint64(u.UID), 10)
	}
	if u.GID != 0 {
		ugi.Group = strconv.FormatUint(uint64(u.GID), 10)
	}
	if len(u.AdditionalGids) > 0 {
		ugi.AdditionalGroups = []string{}
		for _, gid := range u.AdditionalGids {
			ugi.AdditionalGroups = append(ugi.AdditionalGroups, strconv.FormatUint(uint64(gid), 10))
		}
	}

	return ugi
}
