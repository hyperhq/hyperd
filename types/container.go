package types

import ()

// Container JSON Data Structure
type ContainerPort struct {
	Name          string `json:"name"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIP"`
}

type EnvironmentVar struct {
	Env   string `json:"env"`
	Value string `json:"value"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	ReadOnly  bool   `json:"readOnly"`
	MountPath string `json:"mountPath"`
}

type WaitingStatus struct {
	Reason string `json:"reason"`
}

type RunningStatus struct {
	StartedAt string `json:"startedAt"`
}

type TermStatus struct {
	ExitCode   int    `json:"exitCode"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

type ContainerStatus struct {
	Name        string        `json:"name"`
	ContainerID string        `json:"containerID"`
	Phase       string        `json:"phase"`
	Waiting     WaitingStatus `json:"waiting"`
	Running     RunningStatus `json:"running"`
	Terminated  TermStatus    `json:"terminated"`
}

type ContainerInfo struct {
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
	Status          ContainerStatus  `json:"status"`
}
