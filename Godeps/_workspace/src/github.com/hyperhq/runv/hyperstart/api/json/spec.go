package json

type VolumeDescriptor struct {
	Device       string `json:"device"`
	Addr         string `json:"addr,omitempty"`
	Mount        string `json:"mount"`
	Fstype       string `json:"fstype,omitempty"`
	ReadOnly     bool   `json:"readOnly"`
	DockerVolume bool   `json:"dockerVolume"`
}

type FsmapDescriptor struct {
	Source       string `json:"source"`
	Path         string `json:"path"`
	ReadOnly     bool   `json:"readOnly"`
	DockerVolume bool   `json:"dockerVolume"`
}

type EnvironmentVar struct {
	Env   string `json:"env"`
	Value string `json:"value"`
}

// Rlimit type and restrictions
type Rlimit struct {
	// Type of the rlimit to set
	Type string `json:"type"`
	// Hard is the hard limit for the specified type
	Hard uint64 `json:"hard"`
	// Soft is the soft limit for the specified type
	Soft uint64 `json:"soft"`
}

type Process struct {
	// User, Group, AdditionalGroups specify the user information
	User             string   `json:"user,omitempty"`
	Group            string   `json:"group,omitempty"`
	AdditionalGroups []string `json:"additionalGroups,omitempty"`
	// Terminal creates an interactive terminal for the process.
	Terminal bool `json:"terminal"`
	// Sequeue number for stdin and stdout
	Stdio uint64 `json:"stdio,omitempty"`
	// sequeue number for stderr if it is not shared with stdout
	Stderr uint64 `json:"stderr,omitempty"`
	// Args specifies the binary and arguments for the application to execute.
	Args []string `json:"args"`
	// Envs populates the process environment for the process.
	Envs []EnvironmentVar `json:"envs,omitempty"`
	// Workdir is the current working directory for the process and must be
	// relative to the container's root.
	Workdir string `json:"workdir"`
	// Rlimits specifies rlimit options to apply to the process.
	Rlimits []Rlimit `json:"rlimits,omitempty"`
}

type Port struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type Container struct {
	Id            string              `json:"id"`
	Rootfs        string              `json:"rootfs"`
	Fstype        string              `json:"fstype,omitempty"`
	Image         string              `json:"image"`
	Addr          string              `json:"addr,omitempty"`
	Volumes       []*VolumeDescriptor `json:"volumes,omitempty"`
	Fsmap         []*FsmapDescriptor  `json:"fsmap,omitempty"`
	Sysctl        map[string]string   `json:"sysctl,omitempty"`
	Process       *Process            `json:"process"`
	RestartPolicy string              `json:"restartPolicy"`
	Initialize    bool                `json:"initialize"`
	Ports         []Port              `json:"ports,omitempty"` //deprecated
}

type NetworkInf struct {
	Device    string `json:"device"`
	IpAddress string `json:"ipAddress"`
	NetMask   string `json:"netMask"`
}

type Route struct {
	Dest    string `json:"dest"`
	Gateway string `json:"gateway,omitempty"`
	Device  string `json:"device,omitempty"`
}

type PortmappingWhiteList struct {
	InternalNetworks []string `json:"internalNetworks,omitempty"`
	ExternalNetworks []string `json:"externalNetworks,omitempty"`
}

type Pod struct {
	Hostname              string                `json:"hostname"`
	DeprecatedContainers  []Container           `json:"containers,omitempty"`
	DeprecatedInterfaces  []NetworkInf          `json:"interfaces,omitempty"`
	Dns                   []string              `json:"dns,omitempty"`
	DeprecatedRoutes      []Route               `json:"routes,omitempty"`
	ShareDir              string                `json:"shareDir"`
	PortmappingWhiteLists *PortmappingWhiteList `json:"portmappingWhiteLists,omitempty"`
}

func (cr *Container) RoLookup(mpoint string) bool {
	if v := cr.volLookup(mpoint); v != nil {
		return v.ReadOnly
	} else if m := cr.mapLookup(mpoint); m != nil {
		return m.ReadOnly
	}

	return false
}

func (cr *Container) mapLookup(mpoint string) *FsmapDescriptor {
	for _, fs := range cr.Fsmap {
		if fs.Path == mpoint {
			return fs
		}
	}
	return nil
}

func (cr *Container) volLookup(mpoint string) *VolumeDescriptor {
	for _, vol := range cr.Volumes {
		if vol.Mount == mpoint {
			return vol
		}
	}
	return nil
}
