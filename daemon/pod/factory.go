package pod

import (
	"io"

	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/pkg/version"
	dockertypes "github.com/docker/engine-api/types"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/daemon/daemondb"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	runv "github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/factory"
)

type ContainerEngine interface {
	ContainerCreate(params dockertypes.ContainerCreateConfig) (dockertypes.ContainerCreateResponse, error)
	ContainerInspect(id string, size bool, version version.Version) (interface{}, error)
	ContainerRm(name string, config *dockertypes.ContainerRmConfig) error
	ContainerRename(oldName, newName string) error
}

type PodStorage interface {
	Type() string

	PrepareContainer(mountId, sharedDir string) (*runv.VolumeDescription, error)
	CleanupContainer(id, sharedDir string) error
	InjectFile(src io.Reader, containerId, target, baseDir string, perm, uid, gid int) error
	CreateVolume(podId string, spec *apitypes.UserVolume) error
	RemoveVolume(podId string, record []byte) error
}

type GlobalLogConfig struct {
	*apitypes.PodLogConfig
	PathPrefix  string
	PodIdInPath bool
}

type PodFactory struct {
	sd         PodStorage
	registry   *PodList
	db         *daemondb.DaemonDB
	engine     ContainerEngine
	vmFactory  factory.Factory
	hosts      *utils.Initializer
	logCfg     *GlobalLogConfig
	logCreator logger.Creator
}

type LogStatus struct {
	Copier  *logger.Copier
	Driver  logger.Logger
	LogPath string
}

func NewPodFactory(vmFactory factory.Factory, registry *PodList, db *daemondb.DaemonDB, sd PodStorage, eng ContainerEngine, logCfg *GlobalLogConfig) *PodFactory {
	return &PodFactory{
		sd:        sd,
		registry:  registry,
		engine:    eng,
		vmFactory: vmFactory,
		hosts:     nil,
		logCfg:    logCfg,
	}
}

func initLogCreator(factory *PodFactory, spec *apitypes.UserPod) logger.Creator {
	if spec.Log.Type == "" {
		spec.Log.Type = factory.logCfg.Type
		spec.Log.Config = factory.logCfg.Config
	}
	factory.logCfg.Config = spec.Log.Config

	if spec.Log.Type == "none" {
		return nil
	}

	var (
		creator logger.Creator
		err     error
	)

	if err = logger.ValidateLogOpts(spec.Log.Type, spec.Log.Config); err != nil {
		hlog.Log(ERROR, "invalid log options for pod %s. type: %s; options: %#v", spec.Id, spec.Log.Type, spec.Log.Config)
		return nil
	}
	creator, err = logger.GetLogDriver(spec.Log.Type)
	if err != nil {
		hlog.Log(ERROR, "cannot create logCreator for pod %s. type: %s; err: %v", spec.Id, spec.Log.Type, err)
		return nil
	}
	hlog.Log(DEBUG, "configuring log driver [%s] for %s", spec.Log.Type, spec.Id)

	return creator
}
