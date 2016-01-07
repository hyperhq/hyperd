package types

// Pod JSON Data Structure
type Container struct {
	Name            string           `json:"name"`
	ContainerID     string           `json:"containerID"`
	Image           string           `json:"image"`
	ImageID         string           `json:"imageID"`
	Commands        []string         `json:"commands"`
	Args            []string         `json:"args"`
	Workdir         string           `json:"workingDir"`
	Ports           []ContainerPort  `json:"ports"`
	Environment     []EnvironmentVar `json:"env"`
	Volume          []VolumeMount    `json:"volumeMounts"`
	ImagePullPolicy string           `json:"imagePullPolicy"`
}

type RBDVolumeSource struct {
	Monitors []string `json:"monitors"`
	Image    string   `json:"image"`
	FsType   string   `json:"fsType"`
	Pool     string   `json:"pool"`
	User     string   `json:"user"`
	Keyring  string   `json:"keyring"`
	ReadOnly bool     `json:"readOnly"`
}

type PodVolume struct {
	Name     string          `json:"name"`
	HostPath string          `json:"source"`
	Driver   string          `json:"driver"`
	Rbd      RBDVolumeSource `json:"rbd"`
}

type PodSpec struct {
	Volumes    []PodVolume       `json:"volumes"`
	Containers []Container       `json:"containers"`
	Labels     map[string]string `json:"labels"`
	Vcpu       int               `json:"vcpu"`
	Memory     int               `json:"memory"`
}

type PodStatus struct {
	Phase     string            `json:"phase"`
	Message   string            `json:"message"`
	Reason    string            `json:"reason"`
	HostIP    string            `json:"hostIP"`
	PodIP     []string          `json:"podIP"`
	StartTime string            `json:"startTime"`
	Status    []ContainerStatus `json:"containerStatus"`
}

type PodInfo struct {
	Kind       string    `json:"kind"`
	ApiVersion string    `json:"apiVersion"`
	Vm         string    `json:"vm"`
	Spec       PodSpec   `json:"spec"`
	Status     PodStatus `json:"status"`
}
