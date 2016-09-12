package pod

import (
	"strings"

	"github.com/golang/glog"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func ConvertOCF2UserContainer(s *specs.Spec) *UserContainer {
	container := &UserContainer{
		Command:       s.Process.Args,
		Workdir:       s.Process.Cwd,
		Tty:           s.Process.Terminal,
		Image:         s.Root.Path,
		RestartPolicy: "never",
	}

	if s.Linux != nil {
		container.Sysctl = s.Linux.Sysctl
	}

	for _, value := range s.Process.Env {
		glog.V(1).Infof("env: %s\n", value)
		values := strings.Split(value, "=")
		tmp := UserEnvironmentVar{
			Env:   values[0],
			Value: values[1],
		}
		container.Envs = append(container.Envs, tmp)
	}

	return container
}

func ConvertOCF2PureUserPod(s *specs.Spec) *UserPod {
	mem := 0
	if s.Linux != nil && s.Linux.Resources != nil && s.Linux.Resources.Memory != nil && s.Linux.Resources.Memory.Limit != nil {
		mem = int(*s.Linux.Resources.Memory.Limit >> 20)
	}
	return &UserPod{
		Name: s.Hostname,
		Resource: UserResource{
			Memory: mem,
			Vcpu:   0,
		},
		Tty:           s.Process.Terminal,
		Type:          "OCF",
		RestartPolicy: "never",
	}
}
