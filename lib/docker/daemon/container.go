package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/libcontainer/label"

	"github.com/hyperhq/hyper/lib/docker/daemon/network"
	"github.com/hyperhq/hyper/lib/docker/image"
	"github.com/hyperhq/hyper/lib/docker/nat"
	"github.com/hyperhq/hyper/lib/docker/pkg/archive"
	"github.com/hyperhq/hyper/lib/docker/pkg/broadcastwriter"
	"github.com/hyperhq/hyper/lib/docker/pkg/ioutils"
	"github.com/hyperhq/hyper/lib/docker/pkg/symlink"
	"github.com/hyperhq/hyper/lib/docker/runconfig"
	"github.com/hyperhq/runv/lib/glog"
)

var (
	ErrNotATTY               = errors.New("The PTY is not a file")
	ErrNoTTY                 = errors.New("No PTY found")
	ErrContainerStart        = errors.New("The container failed to start. Unknown error")
	ErrContainerStartTimeout = errors.New("The container failed to start due to timed out.")
)

type StreamConfig struct {
	stdout    *broadcastwriter.BroadcastWriter
	stderr    *broadcastwriter.BroadcastWriter
	stdin     io.ReadCloser
	stdinPipe io.WriteCloser
}

// CommonContainer holds the settings for a container which are applicable
// across all platforms supported by the daemon.
type CommonContainer struct {
	StreamConfig

	*State `json:"State"` // Needed for remote api version <= 1.11
	root   string         // Path to the "home" of the container, including metadata.
	basefs string         // Path to the graphdriver mountpoint

	ID                       string
	Created                  time.Time
	Path                     string
	Args                     []string
	Config                   *runconfig.Config
	ImageID                  string `json:"Image"`
	NetworkSettings          *network.Settings
	ResolvConfPath           string
	HostnamePath             string
	HostsPath                string
	LogPath                  string
	Name                     string
	Driver                   string
	MountLabel, ProcessLabel string
	RestartCount             int
	UpdateDns                bool

	hostConfig *runconfig.HostConfig
	daemon     *Daemon
}

func (container *Container) FromDisk() error {
	pth, err := container.jsonPath()
	if err != nil {
		return err
	}

	jsonSource, err := os.Open(pth)
	if err != nil {
		return err
	}
	defer jsonSource.Close()

	dec := json.NewDecoder(jsonSource)

	// Load container settings
	// udp broke compat of docker.PortMapping, but it's not used when loading a container, we can skip it
	if err := dec.Decode(container); err != nil && !strings.Contains(err.Error(), "docker.PortMapping") {
		return err
	}

	if err := label.ReserveLabel(container.ProcessLabel); err != nil {
		return err
	}
	return container.readHostConfig()
}

func (container *Container) toDisk() error {
	data, err := json.Marshal(container)
	if err != nil {
		return err
	}

	pth, err := container.jsonPath()
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(pth, data, 0666); err != nil {
		return err
	}

	return container.WriteHostConfig()
}

func (container *Container) ToDisk() error {
	container.Lock()
	err := container.toDisk()
	container.Unlock()
	return err
}

func (container *Container) readHostConfig() error {
	container.hostConfig = &runconfig.HostConfig{}
	// If the hostconfig file does not exist, do not read it.
	// (We still have to initialize container.hostConfig,
	// but that's OK, since we just did that above.)
	pth, err := container.hostConfigPath()
	if err != nil {
		return err
	}

	_, err = os.Stat(pth)
	if os.IsNotExist(err) {
		return nil
	}

	f, err := os.Open(pth)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewDecoder(f).Decode(&container.hostConfig)
}

func (container *Container) WriteHostConfig() error {
	data, err := json.Marshal(container.hostConfig)
	if err != nil {
		return err
	}

	pth, err := container.hostConfigPath()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(pth, data, 0666)
}

func (container *Container) LogEvent(action string) {
	/*
		d := container.daemon
		d.EventsService.Log(
			action,
			container.ID,
			container.Config.Image,
		)
	*/
}

// Evaluates `path` in the scope of the container's basefs, with proper path
// sanitisation. Symlinks are all scoped to the basefs of the container, as
// though the container's basefs was `/`.
//
// The basefs of a container is the host-facing path which is bind-mounted as
// `/` inside the container. This method is essentially used to access a
// particular path inside the container as though you were a process in that
// container.
//
// NOTE: The returned path is *only* safely scoped inside the container's basefs
//       if no component of the returned path changes (such as a component
//       symlinking to a different path) between using this method and using the
//       path. See symlink.FollowSymlinkInScope for more details.
func (container *Container) GetResourcePath(path string) (string, error) {
	cleanPath := filepath.Join("/", path)
	return symlink.FollowSymlinkInScope(filepath.Join(container.basefs, cleanPath), container.basefs)
}

// Evaluates `path` in the scope of the container's root, with proper path
// sanitisation. Symlinks are all scoped to the root of the container, as
// though the container's root was `/`.
//
// The root of a container is the host-facing configuration metadata directory.
// Only use this method to safely access the container's `container.json` or
// other metadata files. If in doubt, use container.GetResourcePath.
//
// NOTE: The returned path is *only* safely scoped inside the container's root
//       if no component of the returned path changes (such as a component
//       symlinking to a different path) between using this method and using the
//       path. See symlink.FollowSymlinkInScope for more details.
func (container *Container) GetRootResourcePath(path string) (string, error) {
	cleanPath := filepath.Join("/", path)
	return symlink.FollowSymlinkInScope(filepath.Join(container.root, cleanPath), container.root)
}

func (container *Container) Start() (err error) {
	container.Lock()
	defer container.Unlock()

	if container.Running {
		return nil
	}

	if container.removalInProgress || container.Dead {
		return fmt.Errorf("Container is marked for removal and cannot be started.")
	}

	// if we encounter an error during start we need to ensure that any other
	// setup has been cleaned up properly
	defer func() {
		if err != nil {
			container.setError(err)
			// if no one else has set it, make sure we don't leave it at zero
			if container.ExitCode == 0 {
				container.ExitCode = 128
			}
			container.toDisk()
			container.cleanup()
			container.LogEvent("die")
		}
	}()

	if err := container.Mount(); err != nil {
		return err
	}
	if err := container.initializeNetworking(); err != nil {
		return err
	}
	linkedEnv, err := container.setupLinkedContainers()
	if err != nil {
		return err
	}
	if err := container.setupWorkingDirectory(); err != nil {
		return err
	}
	env := container.createDaemonEnvironment(linkedEnv)
	if err := populateCommand(container, env); err != nil {
		return err
	}
	return container.waitForStart()
}

func (container *Container) Run() error {
	if err := container.Start(); err != nil {
		return err
	}
	container.WaitStop(-1 * time.Second)
	return nil
}

func (container *Container) Output() (output []byte, err error) {
	pipe := container.StdoutPipe()
	defer pipe.Close()
	if err := container.Start(); err != nil {
		return nil, err
	}
	output, err = ioutil.ReadAll(pipe)
	container.WaitStop(-1 * time.Second)
	return output, err
}

// StreamConfig.StdinPipe returns a WriteCloser which can be used to feed data
// to the standard input of the container's active process.
// Container.StdoutPipe and Container.StderrPipe each return a ReadCloser
// which can be used to retrieve the standard output (and error) generated
// by the container's active process. The output (and error) are actually
// copied and delivered to all StdoutPipe and StderrPipe consumers, using
// a kind of "broadcaster".

func (streamConfig *StreamConfig) StdinPipe() io.WriteCloser {
	return streamConfig.stdinPipe
}

func (streamConfig *StreamConfig) StdoutPipe() io.ReadCloser {
	reader, writer := io.Pipe()
	streamConfig.stdout.AddWriter(writer, "")
	return ioutils.NewBufReader(reader)
}

func (streamConfig *StreamConfig) StderrPipe() io.ReadCloser {
	reader, writer := io.Pipe()
	streamConfig.stderr.AddWriter(writer, "")
	return ioutils.NewBufReader(reader)
}

func (streamConfig *StreamConfig) StdoutLogPipe() io.ReadCloser {
	reader, writer := io.Pipe()
	streamConfig.stdout.AddWriter(writer, "stdout")
	return ioutils.NewBufReader(reader)
}

func (streamConfig *StreamConfig) StderrLogPipe() io.ReadCloser {
	reader, writer := io.Pipe()
	streamConfig.stderr.AddWriter(writer, "stderr")
	return ioutils.NewBufReader(reader)
}

func (container *Container) isNetworkAllocated() bool {
	return container.NetworkSettings.IPAddress != ""
}

// cleanup releases any network resources allocated to the container along with any rules
// around how containers are linked together.  It also unmounts the container's root filesystem.
func (container *Container) cleanup() {
	container.ReleaseNetwork()

	disableAllActiveLinks(container)

	if err := container.Unmount(); err != nil {
		glog.Errorf("%v: Failed to umount filesystem: %v", container.ID, err)
	}
}

func (container *Container) KillSig(sig int) error {
	glog.V(1).Infof("Sending %d to %s", sig, container.ID)
	container.Lock()
	defer container.Unlock()

	// We could unpause the container for them rather than returning this error
	if container.Paused {
		return fmt.Errorf("Container %s is paused. Unpause the container before stopping", container.ID)
	}

	if !container.Running {
		return nil
	}

	// if the container is currently restarting we do not need to send the signal
	// to the process.  Telling the monitor that it should exit on it's next event
	// loop is enough
	if container.Restarting {
		return nil
	}

	return nil
}

// Wrapper aroung KillSig() suppressing "no such process" error.
func (container *Container) killPossiblyDeadProcess(sig int) error {
	err := container.KillSig(sig)
	if err == syscall.ESRCH {
		glog.V(1).Infof("Cannot kill process (pid=%d) with signal %d: no such process.", container.GetPid(), sig)
		return nil
	}
	return err
}

func (container *Container) Kill() error {
	if !container.IsRunning() {
		return nil
	}

	// 1. Send SIGKILL
	if err := container.killPossiblyDeadProcess(9); err != nil {
		// While normally we might "return err" here we're not going to
		// because if we can't stop the container by this point then
		// its probably because its already stopped. Meaning, between
		// the time of the IsRunning() call above and now it stopped.
		// Also, since the err return will be exec driver specific we can't
		// look for any particular (common) error that would indicate
		// that the process is already dead vs something else going wrong.
		// So, instead we'll give it up to 2 more seconds to complete and if
		// by that time the container is still running, then the error
		// we got is probably valid and so we return it to the caller.

		if container.IsRunning() {
			container.WaitStop(2 * time.Second)
			if container.IsRunning() {
				return err
			}
		}
	}

	// 2. Wait for the process to die, in last resort, try to kill the process directly
	if err := killProcessDirectly(container); err != nil {
		return err
	}

	container.WaitStop(-1 * time.Second)
	return nil
}

func (container *Container) Stop(seconds int) error {
	if !container.IsRunning() {
		return nil
	}

	// 1. Send a SIGTERM
	if err := container.killPossiblyDeadProcess(15); err != nil {
		glog.Infof("Failed to send SIGTERM to the process, force killing")
		if err := container.killPossiblyDeadProcess(9); err != nil {
			return err
		}
	}

	// 2. Wait for the process to exit on its own
	if _, err := container.WaitStop(time.Duration(seconds) * time.Second); err != nil {
		glog.Infof("Container %v failed to exit within %d seconds of SIGTERM - using the force", container.ID, seconds)
		// 3. If it doesn't, then send SIGKILL
		if err := container.Kill(); err != nil {
			container.WaitStop(-1 * time.Second)
			return err
		}
	}

	container.LogEvent("stop")
	return nil
}

func (container *Container) Restart(seconds int) error {
	// Avoid unnecessarily unmounting and then directly mounting
	// the container when the container stops and then starts
	// again
	if err := container.Mount(); err == nil {
		defer container.Unmount()
	}

	if err := container.Stop(seconds); err != nil {
		return err
	}

	if err := container.Start(); err != nil {
		return err
	}

	container.LogEvent("restart")
	return nil
}

func (container *Container) Resize(h, w int) error {
	if !container.IsRunning() {
		return fmt.Errorf("Cannot resize container %s, container is not running", container.ID)
	}
	return nil
}

func (container *Container) Export() (archive.Archive, error) {
	if err := container.Mount(); err != nil {
		return nil, err
	}

	archive, err := archive.Tar(container.basefs, archive.Uncompressed)
	if err != nil {
		container.Unmount()
		return nil, err
	}
	arch := ioutils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		container.Unmount()
		return err
	})
	container.LogEvent("export")
	return arch, err
}

func (container *Container) Mount() error {
	return container.daemon.Mount(container)
}

func (container *Container) changes() ([]archive.Change, error) {
	return container.daemon.Changes(container)
}

func (container *Container) Changes() ([]archive.Change, error) {
	container.Lock()
	defer container.Unlock()
	return container.changes()
}

func (container *Container) GetImage() (*image.Image, error) {
	if container.daemon == nil {
		return nil, fmt.Errorf("Can't get image of unregistered container")
	}
	return container.daemon.graph.Get(container.ImageID)
}

func (container *Container) Unmount() error {
	return container.daemon.Unmount(container)
}

func (container *Container) hostConfigPath() (string, error) {
	return container.GetRootResourcePath("hostconfig.json")
}

func (container *Container) jsonPath() (string, error) {
	return container.GetRootResourcePath("config.json")
}

// This method must be exported to be used from the lxc template
// This directory is only usable when the container is running
func (container *Container) RootfsPath() string {
	return container.basefs
}

func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("Invalid empty id")
	}
	return nil
}

func (container *Container) Copy(resource string) (io.ReadCloser, error) {
	container.Lock()
	defer container.Unlock()
	var err error
	if err := container.Mount(); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			// unmount the container's rootfs
			container.Unmount()
		}
	}()
	basePath, err := container.GetResourcePath(resource)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(basePath)
	if err != nil {
		return nil, err
	}
	var filter []string
	if !stat.IsDir() {
		d, f := filepath.Split(basePath)
		basePath = d
		filter = []string{f}
	} else {
		filter = []string{filepath.Base(basePath)}
		basePath = filepath.Dir(basePath)
	}
	archive, err := archive.TarWithOptions(basePath, &archive.TarOptions{
		Compression:  archive.Uncompressed,
		IncludeFiles: filter,
	})
	if err != nil {
		return nil, err
	}
	reader := ioutils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		container.Unmount()
		return err
	})
	container.LogEvent("copy")
	return reader, nil
}

// Returns true if the container exposes a certain port
func (container *Container) Exposes(p nat.Port) bool {
	_, exists := container.Config.ExposedPorts[p]
	return exists
}

func (container *Container) HostConfig() *runconfig.HostConfig {
	return container.hostConfig
}

func (container *Container) SetHostConfig(hostConfig *runconfig.HostConfig) {
	container.hostConfig = hostConfig
}

func (container *Container) getLogConfig() runconfig.LogConfig {
	cfg := container.hostConfig.LogConfig
	if cfg.Type != "" { // container has log driver configured
		return cfg
	}
	// Use daemon's default log config for containers
	return container.daemon.defaultLogConfig
}

func (container *Container) waitForStart() error {
	return nil
}

func (container *Container) GetProcessLabel() string {
	// even if we have a process label return "" if we are running
	// in privileged mode
	if container.hostConfig.Privileged {
		return ""
	}
	return container.ProcessLabel
}

func (container *Container) GetMountLabel() string {
	if container.hostConfig.Privileged {
		return ""
	}
	return container.MountLabel
}

func (c *Container) LogDriverType() string {
	c.Lock()
	defer c.Unlock()
	if c.hostConfig.LogConfig.Type == "" {
		return c.daemon.defaultLogConfig.Type
	}
	return c.hostConfig.LogConfig.Type
}

// Code c/c from io.Copy() modified to handle escape sequence
func copyEscapable(dst io.Writer, src io.ReadCloser) (written int64, err error) {
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			// ---- Docker addition
			// char 16 is C-p
			if nr == 1 && buf[0] == 16 {
				nr, er = src.Read(buf)
				// char 17 is C-q
				if nr == 1 && buf[0] == 17 {
					if err := src.Close(); err != nil {
						return 0, err
					}
					return 0, nil
				}
			}
			// ---- End of docker
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}

func (container *Container) shouldRestart() bool {
	return container.hostConfig.RestartPolicy.Name == "always" ||
		(container.hostConfig.RestartPolicy.Name == "on-failure" && container.ExitCode != 0)
}

func (container *Container) GetDaemon() *Daemon {
	return container.daemon
}

func (container *Container) SetDaemon(daemon *Daemon) {
	container.daemon = daemon
}
