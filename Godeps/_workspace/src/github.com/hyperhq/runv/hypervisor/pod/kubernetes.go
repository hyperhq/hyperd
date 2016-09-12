package pod

import "fmt"

type KPod struct {
	Kind string `json:"kind"`
	Meta *KMeta `json:"metadata"`
	Spec *KSpec `json:"spec"`
}

type KSpec struct {
	Containers    []*KContainer `json:"containers"`
	Volumes       []*KVolume    `json:"volumes"`
	RestartPolicy string        `json:"restartPolicy"`
	DNSPolicy     []string
}

type KMeta struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

type KContainer struct {
	Name       string                 `json:"name"`
	Image      string                 `json:"image"`
	Command    []string               `json:"command"`
	Args       []string               `json:"args"`
	WorkingDir string                 `json:"workingDir"`
	Resources  map[string]interface{} `json:"resources"`
	CPU        int
	Memory     int64
	Volumes    []*KVolumeReference `json:"volumeMounts"`
	Ports      []*KPort            `json:"ports"`
	Env        []*KEnv             `json:"env"`
}

type KVolumeReference struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

type KPort struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
}

type KEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type KVolume struct {
	Name   string         `json:"name"`
	Source *KVolumeSource `json:"source"`
}

type KVolumeSource struct {
	EmptyDir          *KEmptyDir `json:"emptyDir"`
	HostDir           *KHostDir  `json:"hostDir"`
	GCEPersistentDisk *KGCEPersistentDisk
}

type KEmptyDir struct{}

type KHostDir struct {
	Path string `json:"path"`
}

type KGCEPersistentDisk struct{}

func (kp *KPod) Convert() (*UserPod, error) {

	name := "default"
	if kp.Meta != nil && kp.Meta.Name != "" {
		name = kp.Meta.Name
	}

	if kp.Kind != "Pod" {
		return nil, fmt.Errorf("kind of the json is not Pod: %s", kp.Kind)
	}

	if kp.Spec == nil {
		return nil, fmt.Errorf("No spec in the file")
	}

	rpolicy := "never"
	switch kp.Spec.RestartPolicy {
	case "Never":
		rpolicy = "never"
	case "Always":
		rpolicy = "always"
	case "OnFailure":
		rpolicy = "onFailure"
	default:
	}
	var memory int64 = 0
	containers := make([]UserContainer, len(kp.Spec.Containers))
	for i, kc := range kp.Spec.Containers {
		memory += kc.Memory

		ports := make([]UserContainerPort, len(kc.Ports))
		for j, p := range kc.Ports {
			ports[j] = UserContainerPort{
				HostPort:      p.HostPort,
				ContainerPort: p.ContainerPort,
				Protocol:      p.Protocol,
			}
		}

		envs := make([]UserEnvironmentVar, len(kc.Env))
		for j, e := range kc.Env {
			envs[j] = UserEnvironmentVar{
				Env:   e.Name,
				Value: e.Value,
			}
		}

		vols := make([]UserVolumeReference, len(kc.Volumes))
		for j, v := range kc.Volumes {
			vols[j] = UserVolumeReference{
				Path:     v.MountPath,
				Volume:   v.Name,
				ReadOnly: v.ReadOnly,
			}
		}

		containers[i] = UserContainer{
			Name:          kc.Name,
			Image:         kc.Image,
			Entrypoint:    kc.Command,
			Command:       kc.Args,
			Workdir:       kc.WorkingDir,
			Ports:         ports,
			Envs:          envs,
			Volumes:       vols,
			Files:         []UserFileReference{},
			RestartPolicy: "never",
		}
	}

	volumes := make([]UserVolume, len(kp.Spec.Volumes))
	for i, vol := range kp.Spec.Volumes {
		volumes[i].Name = vol.Name
		if vol.Source.HostDir != nil && vol.Source.HostDir.Path != "" {
			volumes[i].Source = vol.Source.HostDir.Path
			volumes[i].Driver = "vfs"
		} else {
			volumes[i].Source = ""
			volumes[i].Driver = ""
		}
	}

	return &UserPod{
		Name:       name,
		Containers: containers,
		Labels:     kp.Meta.Labels,
		Resource: UserResource{
			Vcpu:   1,
			Memory: int(memory / 1024 / 1024),
		},
		Volumes:       volumes,
		Tty:           true,
		Type:          "kubernetes",
		RestartPolicy: rpolicy,
	}, nil
}
