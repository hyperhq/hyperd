// Package daemon exposes the functions that occur on the host server
// that the Docker daemon is running.
//
// In implementing the various functions of the daemon, there is often
// a method-specific struct for configuring the runtime behavior.
package daemon

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/daemon/events"
	"github.com/docker/docker/daemon/execdriver"
	"github.com/docker/docker/daemon/network"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/broadcaster"
	"github.com/docker/docker/pkg/discovery"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/docker/docker/pkg/graphdb"
	"github.com/docker/docker/pkg/idtools"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/namesgenerator"
	"github.com/docker/docker/pkg/nat"
	"github.com/docker/docker/pkg/parsers/filters"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/docker/pkg/stringutils"
	"github.com/docker/docker/pkg/sysinfo"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/docker/docker/registry"
	"github.com/docker/docker/runconfig"
	"github.com/docker/docker/utils"
	"github.com/docker/docker/volume/drivers"
	"github.com/docker/docker/volume/local"
	"github.com/docker/docker/volume/store"
	"github.com/docker/libnetwork"
	lntypes "github.com/docker/libnetwork/types"
	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/daemon/graphdriver"
	_ "github.com/hyperhq/hyper/lib/docker/daemon/graphdriver/vfs"
	"github.com/opencontainers/runc/libcontainer"
)

var (
	validContainerNameChars   = `[a-zA-Z0-9][a-zA-Z0-9_.-]`
	validContainerNamePattern = regexp.MustCompile(`^/?` + validContainerNameChars + `+$`)
)

type contStore struct {
	s map[string]*Container
	sync.Mutex
}

func (c *contStore) Add(id string, cont *Container) {
	c.Lock()
	c.s[id] = cont
	c.Unlock()
}

func (c *contStore) Get(id string) *Container {
	c.Lock()
	res := c.s[id]
	c.Unlock()
	return res
}

func (c *contStore) Delete(id string) {
	c.Lock()
	delete(c.s, id)
	c.Unlock()
}

func (c *contStore) List() []*Container {
	containers := new(History)
	c.Lock()
	for _, cont := range c.s {
		containers.Add(cont)
	}
	c.Unlock()
	containers.Sort()
	return *containers
}

// Daemon holds information about the Docker daemon.
type Daemon struct {
	ID               string
	repository       string
	sysInitPath      string
	containers       *contStore
	graph            *graph.Graph
	repositories     *graph.TagStore
	idIndex          *truncindex.TruncIndex
	configStore      *Config
	sysInfo          *sysinfo.SysInfo
	containerGraphDB *graphdb.Database
	driver           graphdriver.Driver
	execDriver       execdriver.Driver
	defaultLogConfig runconfig.LogConfig
	RegistryService  *registry.Service
	EventsService    *events.Events
	netController    libnetwork.NetworkController
	volumes          *store.VolumeStore
	discoveryWatcher discovery.Watcher
	root             string
	shutdown         bool
	uidMaps          []idtools.IDMap
	gidMaps          []idtools.IDMap
}

// Get looks for a container using the provided information, which could be
// one of the following inputs from the caller:
//  - A full container ID, which will exact match a container in daemon's list
//  - A container name, which will only exact match via the GetByName() function
//  - A partial container ID prefix (e.g. short ID) of any length that is
//    unique enough to only return a single container object
//  If none of these searches succeed, an error is returned
func (daemon *Daemon) Get(prefixOrName string) (*Container, error) {
	if containerByID := daemon.containers.Get(prefixOrName); containerByID != nil {
		// prefix is an exact match to a full container ID
		return containerByID, nil
	}

	// GetByName will match only an exact name provided; we ignore errors
	containerByName, _ := daemon.GetByName(prefixOrName)
	containerId, indexError := daemon.idIndex.Get(prefixOrName)

	if containerByName != nil {
		// prefix is an exact match to a full container Name
		return containerByName, nil
	}

	if containerId != "" {
		// prefix is a fuzzy match to a container ID
		return daemon.containers.Get(containerId), nil
	}
	return nil, indexError
}

// Exists returns a true if a container of the specified ID or name exists,
// false otherwise.
func (daemon *Daemon) Exists(id string) bool {
	c, _ := daemon.Get(id)
	return c != nil
}

func (daemon *Daemon) containerRoot(id string) string {
	return path.Join(daemon.repository, id)
}

// Load reads the contents of a container from disk
// This is typically done at startup.
func (daemon *Daemon) load(id string) (*Container, error) {
	container := &Container{
		CommonContainer: CommonContainer{
			State: NewState(),
			root:  daemon.containerRoot(id),
		},
	}

	if err := container.fromDisk(); err != nil {
		return nil, err
	}

	if container.ID != id {
		return container, fmt.Errorf("Container %s is stored at %s", container.ID, id)
	}

	return container, nil
}

// Register makes a container object usable by the daemon as <container.ID>
// This is a wrapper for register
func (daemon *Daemon) Register(container *Container) error {
	if container.daemon != nil || daemon.Exists(container.ID) {
		return fmt.Errorf("Container is already loaded")
	}
	if err := validateID(container.ID); err != nil {
		return err
	}
	if err := daemon.ensureName(container); err != nil {
		return err
	}
	if daemon == nil {
		glog.Error("daemon can not be nil")
		return fmt.Errorf("daemon can not be nil")
	}

	container.daemon = daemon

	// Attach to stdout and stderr
	container.stderr = new(broadcaster.Unbuffered)
	container.stdout = new(broadcaster.Unbuffered)
	// Attach to stdin
	if container.Config.OpenStdin {
		container.stdin, container.stdinPipe = io.Pipe()
	} else {
		container.stdinPipe = ioutils.NopWriteCloser(ioutil.Discard) // Silently drop stdin
	}
	// done
	daemon.containers.Add(container.ID, container)

	// don't update the Suffixarray if we're starting up
	// we'll waste time if we update it for every container
	daemon.idIndex.Add(container.ID)

	if container.IsRunning() {
		glog.Infof("killing old running container %s", container.ID)
		// Set exit code to 128 + SIGKILL (9) to properly represent unsuccessful exit
		container.setStoppedLocking(&execdriver.ExitStatus{ExitCode: 137})

		if daemon.execDriver != nil {
			// use the current driver and ensure that the container is dead x.x
			cmd := &execdriver.Command{
				CommonCommand: execdriver.CommonCommand{
					ID: container.ID,
				},
			}
			daemon.execDriver.Terminate(cmd)
		}

		container.unmountIpcMounts(mount.Unmount)

		if err := daemon.Unmount(container); err != nil {
			logrus.Debugf("unmount error %s", err)
		}
		if err := container.toDiskLocking(); err != nil {
			logrus.Errorf("Error saving stopped state to disk: %v", err)
		}
	}

	if err := daemon.prepareMountPoints(container); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) ensureName(container *Container) error {
	if container.Name == "" {
		name, err := daemon.generateNewName(container.ID)
		if err != nil {
			return err
		}
		container.Name = name

		if err := container.toDisk(); err != nil {
			glog.V(1).Infof("Error saving container name %s", err)
		}
	}
	return nil
}

func (daemon *Daemon) Restore() error {
	return daemon.restore()
}

func (daemon *Daemon) restore() error {
	type cr struct {
		container  *Container
		registered bool
	}

	var (
		currentDriver = daemon.driver.String()
		containers    = make(map[string]*cr)
	)

	dir, err := ioutil.ReadDir(daemon.repository)
	if err != nil {
		return err
	}

	for _, v := range dir {
		id := v.Name()
		container, err := daemon.load(id)
		if err != nil {
			glog.Errorf("Failed to load container %v: %v", id, err)
			continue
		}

		// Ignore the container if it does not support the current driver being used by the graph
		if (container.Driver == "" && currentDriver == "aufs") || container.Driver == currentDriver {
			glog.V(1).Infof("Loaded container %v", container.ID)

			containers[container.ID] = &cr{container: container}
		} else {
			glog.V(1).Infof("Cannot load container %s because it was created with another graph driver.", container.ID)
		}
	}

	if entities := daemon.containerGraph().List("/", -1); entities != nil {
		for _, p := range entities.Paths() {
			e := entities[p]

			if c, ok := containers[e.ID()]; ok {
				c.registered = true
			}
		}
	}

	group := sync.WaitGroup{}
	for _, c := range containers {
		group.Add(1)

		go func(container *Container, registered bool) {
			defer group.Done()

			if !registered {
				// Try to set the default name for a container if it exists prior to links
				container.Name, err = daemon.generateNewName(container.ID)
				if err != nil {
					glog.V(1).Infof("Setting default id - %s", err)
				}
			}

			if err := daemon.Register(container); err != nil {
				glog.V(1).Infof("Failed to register container %s: %s", container.ID, err)
			}

			// check the restart policy on the containers and restart any container with
			// the restart policy of "always"
			if daemon.configStore.AutoRestart && container.shouldRestart() {
				glog.V(1).Infof("Starting container %s", container.ID)

				if err := daemon.containerStart(container); err != nil {
					glog.V(1).Infof("Failed to start container %s: %s", container.ID, err)
				}
			}
		}(c.container, c.registered)
	}
	group.Wait()

	glog.Info("Loading containers: done.")

	return nil
}

func (daemon *Daemon) mergeAndVerifyConfig(config *runconfig.Config, img *image.Image) error {
	if img != nil && img.Config != nil {
		if err := runconfig.Merge(config, img.Config); err != nil {
			return err
		}
	}
	if config.Entrypoint.Len() == 0 && config.Cmd.Len() == 0 {
		return fmt.Errorf("No command specified")
	}
	return nil
}

func (daemon *Daemon) generateIdAndName(name string) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateRandomID()
	)

	if name == "" {
		if name, err = daemon.generateNewName(id); err != nil {
			return "", "", err
		}
		return id, name, nil
	}

	if name, err = daemon.reserveName(id, name); err != nil {
		return "", "", err
	}

	return id, name, nil
}

func (daemon *Daemon) reserveName(id, name string) (string, error) {
	if !validContainerNamePattern.MatchString(name) {
		return "", fmt.Errorf("Invalid container name (%s), only %s are allowed", name, validContainerNameChars)
	}

	if name[0] != '/' {
		name = "/" + name
	}

	if _, err := daemon.containerGraph().Set(name, id); err != nil {
		if !graphdb.IsNonUniqueNameError(err) {
			return "", err
		}

		conflictingContainer, err := daemon.GetByName(name)
		if err != nil {
			if strings.Contains(err.Error(), "Could not find entity") {
				return "", err
			}

			// Remove name and continue starting the container
			if err := daemon.containerGraph().Delete(name); err != nil {
				return "", err
			}
		} else {
			nameAsKnownByUser := strings.TrimPrefix(name, "/")
			return "", fmt.Errorf(
				"Conflict. The name %q is already in use by container %s. You have to delete (or rename) that container to be able to reuse that name.", nameAsKnownByUser,
				stringid.TruncateID(conflictingContainer.ID))
		}
	}
	return name, nil
}

func (daemon *Daemon) generateNewName(id string) (string, error) {
	var name string
	for i := 0; i < 6; i++ {
		name = namesgenerator.GetRandomName(i)
		if name[0] != '/' {
			name = "/" + name
		}

		if _, err := daemon.containerGraph().Set(name, id); err != nil {
			if !graphdb.IsNonUniqueNameError(err) {
				return "", err
			}
			continue
		}
		return name, nil
	}

	name = "/" + stringid.TruncateID(id)
	if _, err := daemon.containerGraph().Set(name, id); err != nil {
		return "", err
	}
	return name, nil
}

func (daemon *Daemon) generateHostname(id string, config *runconfig.Config) {
	// Generate default hostname
	// FIXME: the lxc template no longer needs to set a default hostname
	if config.Hostname == "" {
		config.Hostname = id[:12]
	}
}

func (daemon *Daemon) getEntrypointAndArgs(configEntrypoint *stringutils.StrSlice, configCmd *stringutils.StrSlice) (string, []string) {
	var (
		entrypoint string
		args       []string
	)

	cmdSlice := configCmd.Slice()
	if configEntrypoint.Len() != 0 {
		eSlice := configEntrypoint.Slice()
		entrypoint = eSlice[0]
		args = append(eSlice[1:], cmdSlice...)
	} else {
		entrypoint = cmdSlice[0]
		args = cmdSlice[1:]
	}
	return entrypoint, args
}

func (daemon *Daemon) newContainer(name string, config *runconfig.Config, imgID string) (*Container, error) {
	var (
		id  string
		err error
	)
	id, name, err = daemon.generateIdAndName(name)
	if err != nil {
		return nil, err
	}

	daemon.generateHostname(id, config)
	entrypoint, args := daemon.getEntrypointAndArgs(config.Entrypoint, config.Cmd)

	container := &Container{
		CommonContainer: CommonContainer{
			ID:              id, // FIXME: we should generate the ID here instead of receiving it as an argument
			Created:         time.Now().UTC(),
			Path:            entrypoint,
			Args:            args, //FIXME: de-duplicate from config
			Config:          config,
			hostConfig:      &runconfig.HostConfig{},
			ImageID:         imgID,
			NetworkSettings: &network.Settings{},
			Name:            name,
			Driver:          daemon.driver.String(),
			State:           NewState(),
		},
	}
	container.root = daemon.containerRoot(container.ID)

	return container, err
}

func (daemon *Daemon) createRootfs(container *Container) error {
	// Step 1: create the container directory.
	// This doubles as a barrier to avoid race conditions.
	// Step 1: create the container directory.
	// This doubles as a barrier to avoid race conditions.
	rootUID, rootGID, err := idtools.GetRootUIDGID(daemon.uidMaps, daemon.gidMaps)
	if err != nil {
		return err
	}
	if err := idtools.MkdirAs(container.root, 0700, rootUID, rootGID); err != nil {
		return err
	}
	initID := fmt.Sprintf("%s-init", container.ID)

	if err := daemon.driver.Create(initID, container.ImageID, container.getMountLabel()); err != nil {
		return err
	}
	initPath, err := daemon.driver.Get(initID, "")
	if err != nil {
		return err
	}

	if err := setupInitLayer(initPath, rootUID, rootGID); err != nil {
		if err := daemon.driver.Put(initID); err != nil {
			logrus.Errorf("Failed to Put init layer: %v", err)
		}
		return err
	}

	// We want to unmount init layer before we take snapshot of it
	// for the actual container.
	if err := daemon.driver.Put(initID); err != nil {
		return err
	}

	if err := daemon.driver.Create(container.ID, initID, ""); err != nil {
		return err
	}
	return nil
}

func GetFullContainerName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("Container name cannot be empty")
	}
	if name[0] != '/' {
		name = "/" + name
	}
	return name, nil
}

func (daemon *Daemon) GetByName(name string) (*Container, error) {
	fullName, err := GetFullContainerName(name)
	if err != nil {
		return nil, err
	}
	entity := daemon.containerGraph().Get(fullName)
	if entity == nil {
		return nil, fmt.Errorf("Could not find entity for %s", name)
	}
	e := daemon.containers.Get(entity.ID())
	if e == nil {
		return nil, fmt.Errorf("Could not find container for entity id %s", entity.ID())
	}
	return e, nil
}

// GetEventFilter returns a filters.Filter for a set of filters
func (daemon *Daemon) GetEventFilter(filter filters.Args) *events.Filter {
	// incoming container filter can be name, id or partial id, convert to
	// a full container id
	for i, cn := range filter["container"] {
		c, err := daemon.Get(cn)
		if err != nil {
			filter["container"][i] = ""
		} else {
			filter["container"][i] = c.ID
		}
	}
	return events.NewFilter(filter, daemon.GetLabels)
}

// SubscribeToEvents returns the currently record of events, a channel to stream new events from, and a function to cancel the stream of events.
func (daemon *Daemon) SubscribeToEvents() ([]*jsonmessage.JSONMessage, chan interface{}, func()) {
	return daemon.EventsService.Subscribe()
}

// GetLabels for a container or image id
func (daemon *Daemon) GetLabels(id string) map[string]string {
	// TODO: TestCase
	container := daemon.containers.Get(id)
	if container != nil {
		return container.Config.Labels
	}

	img, err := daemon.repositories.LookupImage(id)
	if err == nil {
		return img.ContainerConfig.Labels
	}
	return nil
}

func (daemon *Daemon) children(name string) (map[string]*Container, error) {
	name, err := GetFullContainerName(name)
	if err != nil {
		return nil, err
	}
	children := make(map[string]*Container)

	err = daemon.containerGraph().Walk(name, func(p string, e *graphdb.Entity) error {
		c, err := daemon.Get(e.ID())
		if err != nil {
			return err
		}
		children[p] = c
		return nil
	}, 0)

	if err != nil {
		return nil, err
	}
	return children, nil
}

func (daemon *Daemon) parents(name string) ([]string, error) {
	name, err := GetFullContainerName(name)
	if err != nil {
		return nil, err
	}

	return daemon.containerGraph().Parents(name)
}

func (daemon *Daemon) registerLink(parent, child *Container, alias string) error {
	fullName := path.Join(parent.Name, alias)
	if !daemon.containerGraph().Exists(fullName) {
		_, err := daemon.containerGraph().Set(fullName, child.ID)
		return err
	}
	return nil
}

func NewDaemon(config *Config, registryService *registry.Service) (daemon *Daemon, err error) {
	uidMaps, gidMaps, err := setupRemappedRoot(config)
	if err != nil {
		return nil, err
	}
	rootUID, rootGID, err := idtools.GetRootUIDGID(uidMaps, gidMaps)
	if err != nil {
		return nil, err
	}

	// get the canonical path to the Docker root directory
	var realRoot string
	if _, err := os.Stat(config.Root); err != nil && os.IsNotExist(err) {
		realRoot = config.Root
	} else {
		realRoot, err = fileutils.ReadSymlinkedDirectory(config.Root)
		if err != nil {
			return nil, fmt.Errorf("Unable to get the full path to root (%s): %s", config.Root, err)
		}
	}

	if err = setupDaemonRoot(config, realRoot, rootUID, rootGID); err != nil {
		return nil, err
	}

	// set up the tmpDir to use a canonical path
	tmp, err := tempDir(config.Root, rootUID, rootGID)
	if err != nil {
		return nil, fmt.Errorf("Unable to get the TempDir under %s: %s", config.Root, err)
	}
	realTmp, err := fileutils.ReadSymlinkedDirectory(tmp)
	if err != nil {
		return nil, fmt.Errorf("Unable to get the full path to the TempDir (%s): %s", tmp, err)
	}
	os.Setenv("TMPDIR", realTmp)

	// Load storage driver
	driver, err := graphdriver.New(config.Root, config.GraphOptions, uidMaps, gidMaps)
	if err != nil {
		return nil, fmt.Errorf("error initializing graphdriver: %v", err)
	}
	glog.V(1).Infof("Using graph driver %s", driver)

	d := &Daemon{}
	d.driver = driver

	defer func() {
		if err != nil {
			if err := d.Shutdown(); err != nil {
				glog.Error(err)
			}
		}
	}()

	daemonRepo := path.Join(config.Root, "containers")

	if err := os.MkdirAll(daemonRepo, 0700); err != nil && !os.IsExist(err) {
		glog.Error(err.Error())
		return nil, err
	}

	glog.Info("Creating images graph")
	g, err := graph.NewGraph(filepath.Join(config.Root, "graph"), d.driver, uidMaps, gidMaps)
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}

	// Configure the volumes driver
	volStore, err := configureVolumes(config, rootUID, rootGID)
	if err != nil {
		return nil, err
	}

	trustKey, err := api.LoadOrCreateTrustKey(config.TrustKeyPath)
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}

	trustDir := path.Join(config.Root, "trust")
	if err := os.MkdirAll(trustDir, 0700); err != nil && !os.IsExist(err) {
		glog.Error(err.Error())
		return nil, err
	}

	eventsService := events.New()
	glog.Info("Creating repository list")
	tagCfg := &graph.TagStoreConfig{
		Graph:    g,
		Key:      trustKey,
		Registry: registryService,
		Events:   eventsService,
	}
	repositories, err := graph.NewTagStore(path.Join(config.Root, "repositories-"+d.driver.String()), tagCfg)
	if err != nil {
		glog.Error(err.Error())
		return nil, fmt.Errorf("Couldn't create Tag store: %s", err)
	}

	graphdbPath := path.Join(config.Root, "linkgraph.db")
	graph, err := graphdb.NewSqliteConn(graphdbPath)
	if err != nil {
		glog.Error(err.Error())
		return nil, err
	}

	d.containerGraphDB = graph

	sysInfo := sysinfo.New(false)
	d.ID = trustKey.PublicKey().KeyID()
	d.repository = daemonRepo
	d.containers = &contStore{s: make(map[string]*Container)}
	d.graph = g
	d.repositories = repositories
	d.idIndex = truncindex.NewTruncIndex([]string{})
	d.sysInfo = sysInfo
	d.configStore = config
	d.sysInitPath = ""
	d.defaultLogConfig = config.LogConfig
	d.RegistryService = registryService
	d.EventsService = eventsService
	d.root = config.Root
	d.volumes = volStore

	if err := d.restore(); err != nil {
		return nil, err
	}
	return d, nil
}

func (daemon *Daemon) shutdownContainer(c *Container) error {
	// TODO(windows): Handle docker restart with paused containers
	if c.isPaused() {
		// To terminate a process in freezer cgroup, we should send
		// SIGTERM to this process then unfreeze it, and the process will
		// force to terminate immediately.
		logrus.Debugf("Found container %s is paused, sending SIGTERM before unpause it", c.ID)
		sig, ok := signal.SignalMap["TERM"]
		if !ok {
			return fmt.Errorf("System doesn not support SIGTERM")
		}
		if err := daemon.kill(c, int(sig)); err != nil {
			return fmt.Errorf("sending SIGTERM to container %s with error: %v", c.ID, err)
		}
		if err := daemon.containerUnpause(c); err != nil {
			return fmt.Errorf("Failed to unpause container %s with error: %v", c.ID, err)
		}
		if _, err := c.WaitStop(10 * time.Second); err != nil {
			logrus.Debugf("container %s failed to exit in 10 second of SIGTERM, sending SIGKILL to force", c.ID)
			sig, ok := signal.SignalMap["KILL"]
			if !ok {
				return fmt.Errorf("System does not support SIGKILL")
			}
			if err := daemon.kill(c, int(sig)); err != nil {
				logrus.Errorf("Failed to SIGKILL container %s", c.ID)
			}
			c.WaitStop(-1 * time.Second)
			return err
		}
	}
	// If container failed to exit in 10 seconds of SIGTERM, then using the force
	if err := daemon.containerStop(c, 10); err != nil {
		return fmt.Errorf("Stop container %s with error: %v", c.ID, err)
	}

	c.WaitStop(-1 * time.Second)
	return nil
}

func (daemon *Daemon) Shutdown() error {
	if daemon.containerGraphDB != nil {
		if err := daemon.containerGraph().Close(); err != nil {
			glog.Errorf("Error during container graph.Close(): %v", err)
		}
	}
	if daemon.driver != nil {
		if err := daemon.driver.Cleanup(); err != nil {
			glog.Errorf("Error during graph storage driver.Cleanup(): %v", err)
		}
	}
	if daemon.containers != nil {
		group := sync.WaitGroup{}
		glog.V(1).Info("starting clean shutdown of all containers...")
		for _, container := range daemon.List() {
			c := container
			if c.IsRunning() {
				glog.V(1).Infof("stopping %s", c.ID)
				group.Add(1)

				go func() {
					defer group.Done()
					// If container failed to exit in 10 seconds of SIGTERM, then using the force
					if err := c.Stop(10); err != nil {
						glog.Errorf("Stop container %s with error: %v", c.ID, err)
					}
					c.WaitStop(-1 * time.Second)
					glog.V(1).Infof("container stopped %s", c.ID)
				}()
			}
		}
		group.Wait()
	}

	return nil
}

func (daemon *Daemon) Mount(container *Container) error {
	dir, err := daemon.driver.Get(container.ID, container.getMountLabel())
	if err != nil {
		return fmt.Errorf("Error getting container %s from driver %s: %s", container.ID, daemon.driver, err)
	}
	if container.basefs == "" {
		container.basefs = dir
	} else if container.basefs != dir {
		daemon.driver.Put(container.ID)
		return fmt.Errorf("Error: driver %s is returning inconsistent paths for container %s ('%s' then '%s')",
			daemon.driver, container.ID, container.basefs, dir)
	}
	return nil
}

func (daemon *Daemon) Unmount(container *Container) error {
	daemon.driver.Put(container.ID)
	return nil
}

func (daemon *Daemon) changes(container *Container) ([]archive.Change, error) {
	initID := fmt.Sprintf("%s-init", container.ID)
	return daemon.driver.Changes(container.ID, initID)
}

func (daemon *Daemon) diff(container *Container) (archive.Archive, error) {
	glog.V(2).Infof("container address %p, daemon address %p", container, daemon)
	initID := fmt.Sprintf("%s-init", container.ID)
	driver := daemon.driver
	return driver.Diff(container.ID, initID)
}

// FIXME: this is a convenience function for integration tests
// which need direct access to daemon.graph.
// Once the tests switch to using engine and jobs, this method
// can go away.
func (daemon *Daemon) Graph() *graph.Graph {
	return daemon.graph
}

// TagImage creates a tag in the repository reponame, pointing to the image named
// imageName. If force is true, an existing tag with the same name may be
// overwritten.
func (daemon *Daemon) TagImage(repoName, tag, imageName string, force bool) error {
	if err := daemon.repositories.Tag(repoName, tag, imageName, force); err != nil {
		return err
	}
	daemon.EventsService.Log("tag", utils.ImageReference(repoName, tag), "")
	return nil
}

// PullImage initiates a pull operation. image is the repository name to pull, and
// tag may be either empty, or indicate a specific tag to pull.
func (daemon *Daemon) PullImage(image string, tag string, imagePullConfig *graph.ImagePullConfig) error {
	return daemon.repositories.Pull(image, tag, imagePullConfig)
}

// ImportImage imports an image, getting the archived layer data either from
// inConfig (if src is "-"), or from a URI specified in src. Progress output is
// written to outStream. Repository and tag names can optionally be given in
// the repo and tag arguments, respectively.
func (daemon *Daemon) ImportImage(src, repo, tag, msg string, inConfig io.ReadCloser, outStream io.Writer, containerConfig *runconfig.Config) error {
	return daemon.repositories.Import(src, repo, tag, msg, inConfig, outStream, containerConfig)
}

// ExportImage exports a list of images to the given output stream. The
// exported images are archived into a tar when written to the output
// stream. All images with the given tag and all versions containing
// the same tag are exported. names is the set of tags to export, and
// outStream is the writer which the images are written to.
func (daemon *Daemon) ExportImage(names []string, outStream io.Writer) error {
	return daemon.repositories.ImageExport(names, outStream)
}

// PushImage initiates a push operation on the repository named localName.
func (daemon *Daemon) PushImage(localName string, imagePushConfig *graph.ImagePushConfig) error {
	return daemon.repositories.Push(localName, imagePushConfig)
}

// LookupImage looks up an image by name and returns it as an ImageInspect
// structure.
func (daemon *Daemon) LookupImage(name string) (*types.ImageInspect, error) {
	return daemon.repositories.Lookup(name)
}

// LoadImage uploads a set of images into the repository. This is the
// complement of ImageExport.  The input stream is an uncompressed tar
// ball containing images and metadata.
func (daemon *Daemon) LoadImage(inTar io.ReadCloser, outStream io.Writer) error {
	return daemon.repositories.Load(inTar, outStream)
}

// ListImages returns a filtered list of images. filterArgs is a JSON-encoded set
// of filter arguments which will be interpreted by pkg/parsers/filters.
// filter is a shell glob string applied to repository names. The argument
// named all controls whether all images in the graph are filtered, or just
// the heads.
func (daemon *Daemon) ListImages(filterArgs, filter string, all bool) ([]*types.Image, error) {
	return daemon.repositories.Images(filterArgs, filter, all)
}

// ImageHistory returns a slice of ImageHistory structures for the specified image
// name by walking the image lineage.
func (daemon *Daemon) ImageHistory(name string) ([]*types.ImageHistory, error) {
	return daemon.repositories.History(name)
}

// GetImage returns pointer to an Image struct corresponding to the given
// name. The name can include an optional tag; otherwise the default tag will
// be used.
func (daemon *Daemon) GetImage(name string) (*image.Image, error) {
	return daemon.repositories.LookupImage(name)
}

func (daemon *Daemon) Repositories() *graph.TagStore {
	return daemon.repositories
}

func (daemon *Daemon) Config() *Config {
	return daemon.configStore
}

func (daemon *Daemon) SystemConfig() *sysinfo.SysInfo {
	return daemon.sysInfo
}

func (daemon *Daemon) SystemInitPath() string {
	return daemon.sysInitPath
}

func (daemon *Daemon) GraphDriver() graphdriver.Driver {
	return daemon.driver
}

func (daemon *Daemon) ContainerGraph() *graphdb.Database {
	return daemon.containerGraphDB
}

// ExecutionDriver returns the currently used driver for creating and
// starting execs in a container.
func (daemon *Daemon) ExecutionDriver() execdriver.Driver {
	return daemon.execDriver
}

func (daemon *Daemon) containerGraph() *graphdb.Database {
	return daemon.containerGraphDB
}

// GetUIDGIDMaps returns the current daemon's user namespace settings
// for the full uid and gid maps which will be applied to containers
// started in this instance.
func (daemon *Daemon) GetUIDGIDMaps() ([]idtools.IDMap, []idtools.IDMap) {
	return daemon.uidMaps, daemon.gidMaps
}

// GetRemappedUIDGID returns the current daemon's uid and gid values
// if user namespaces are in use for this daemon instance.  If not
// this function will return "real" root values of 0, 0.
func (daemon *Daemon) GetRemappedUIDGID() (int, int) {
	uid, gid, _ := idtools.GetRootUIDGID(daemon.uidMaps, daemon.gidMaps)
	return uid, gid
}

func (daemon *Daemon) ImageGetCached(imgID string, config *runconfig.Config) (*image.Image, error) {
	// for now just exit if imgID has no children.
	// maybe parentRefs in graph could be used to store
	// the Image obj children for faster lookup below but this can
	// be quite memory hungry.
	if !daemon.Graph().HasChildren(imgID) {
		return nil, nil
	}

	// Retrieve all images
	images := daemon.Graph().Map()

	// Store the tree in a map of map (map[parentId][childId])
	imageMap := make(map[string]map[string]struct{})
	for _, img := range images {
		if _, exists := imageMap[img.Parent]; !exists {
			imageMap[img.Parent] = make(map[string]struct{})
		}
		imageMap[img.Parent][img.ID] = struct{}{}
	}

	// Loop on the children of the given image and check the config
	var match *image.Image
	for elem := range imageMap[imgID] {
		img, ok := images[elem]
		if !ok {
			return nil, fmt.Errorf("unable to find image %q", elem)
		}
		if runconfig.Compare(&img.ContainerConfig, config) {
			if match == nil || match.Created.Before(img.Created) {
				match = img
			}
		}
	}
	return match, nil
}

// tempDir returns the default directory to use for temporary files.
func tempDir(rootDir string, rootUID, rootGID int) (string, error) {
	var tmpDir string
	if tmpDir = os.Getenv("DOCKER_TMPDIR"); tmpDir == "" {
		tmpDir = filepath.Join(rootDir, "tmp")
	}
	return tmpDir, idtools.MkdirAllAs(tmpDir, 0700, rootUID, rootGID)
}

func (daemon *Daemon) setHostConfig(container *Container, hostConfig *runconfig.HostConfig) error {

	// Do not lock while creating volumes since this could be calling out to external plugins
	// Don't want to block other actions, like `docker ps` because we're waiting on an external plugin
	if err := daemon.registerMountPoints(container, hostConfig); err != nil {
		return err
	}

	container.Lock()
	defer container.Unlock()

	// Register any links from the host config before starting the container
	if err := daemon.registerLinks(container, hostConfig); err != nil {
		return err
	}

	container.hostConfig = hostConfig
	container.toDisk()
	return nil
}

func setDefaultMtu(config *Config) {
	// do nothing if the config does not have the default 0 value.
	if config.Mtu != 0 {
		return
	}
	config.Mtu = defaultNetworkMtu
	if routeMtu, err := getDefaultRouteMtu(); err == nil {
		config.Mtu = routeMtu
	}
}

var errNoDefaultRoute = errors.New("no default route was found")

// verifyContainerSettings performs validation of the hostconfig and config
// structures.
func (daemon *Daemon) verifyContainerSettings(hostConfig *runconfig.HostConfig, config *runconfig.Config) ([]string, error) {

	// First perform verification of settings common across all platforms.
	if config != nil {
		if config.WorkingDir != "" {
			config.WorkingDir = filepath.FromSlash(config.WorkingDir) // Ensure in platform semantics
			if !system.IsAbs(config.WorkingDir) {
				return nil, fmt.Errorf("The working directory '%s' is invalid. It needs to be an absolute path.", config.WorkingDir)
			}
		}

		if len(config.StopSignal) > 0 {
			_, err := signal.ParseSignal(config.StopSignal)
			if err != nil {
				return nil, err
			}
		}
	}

	if hostConfig == nil {
		return nil, nil
	}

	for port := range hostConfig.PortBindings {
		_, portStr := nat.SplitProtoPort(string(port))
		if _, err := nat.ParsePort(portStr); err != nil {
			return nil, fmt.Errorf("Invalid port specification: %q", portStr)
		}
		for _, pb := range hostConfig.PortBindings[port] {
			_, err := nat.NewPort(nat.SplitProtoPort(pb.HostPort))
			if err != nil {
				return nil, fmt.Errorf("Invalid port specification: %q", pb.HostPort)
			}
		}
	}

	// Now do platform-specific verification
	return verifyPlatformContainerSettings(daemon, hostConfig, config)
}

func (daemon *Daemon) kill(c *Container, sig int) error {
	if daemon.execDriver != nil {
		return daemon.execDriver.Kill(c.command, sig)
	} else {
		return nil
	}
}

func configureVolumes(config *Config, rootUID, rootGID int) (*store.VolumeStore, error) {
	volumesDriver, err := local.New(config.Root, rootUID, rootGID)
	if err != nil {
		return nil, err
	}

	volumedrivers.Register(volumesDriver, volumesDriver.Name())
	s := store.New()
	s.AddAll(volumesDriver.List())

	return s, nil
}

// AuthenticateToRegistry checks the validity of credentials in authConfig
func (daemon *Daemon) AuthenticateToRegistry(authConfig *cliconfig.AuthConfig) (string, error) {
	return daemon.RegistryService.Auth(authConfig)
}

// SearchRegistryForImages queries the registry for images matching
// term. authConfig is used to login.
func (daemon *Daemon) SearchRegistryForImages(term string,
	authConfig *cliconfig.AuthConfig,
	headers map[string][]string) (*registry.SearchResults, error) {
	return daemon.RegistryService.Search(term, authConfig, headers)
}

// IsShuttingDown tells whether the daemon is shutting down or not
func (daemon *Daemon) IsShuttingDown() bool {
	return daemon.shutdown
}

// GetContainerStats collects all the stats published by a container
func (daemon *Daemon) GetContainerStats(container *Container) (*execdriver.ResourceStats, error) {
	return nil, nil
}

func (daemon *Daemon) getNetworkStats(c *Container) ([]*libcontainer.NetworkInterface, error) {
	return nil, nil
}

// newBaseContainer creates a new container with its initial
// configuration based on the root storage from the daemon.
func (daemon *Daemon) newBaseContainer(id string) *Container {
	return newBaseContainer(id, daemon.containerRoot(id))
}

func convertLnNetworkStats(name string, stats *lntypes.InterfaceStatistics) *libcontainer.NetworkInterface {
	return nil
}

func (daemon *Daemon) Setup() error {
	if err := daemon.driver.Setup(); err != nil {
		return err
	}
	return nil
}
