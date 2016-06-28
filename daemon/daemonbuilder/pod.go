package daemonbuilder

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/builder"
	derr "github.com/docker/docker/errors"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/chrootarchive"
	"github.com/docker/docker/pkg/idtools"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/engine-api/types"
	containertypes "github.com/docker/engine-api/types/container"
	"github.com/golang/glog"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

// ContainerAttach attaches streams to the container cID. If stream is true, it streams the output.
func (d Docker) ContainerAttach(cId string, stdin io.ReadCloser, stdout, stderr io.Writer, stream bool) error {
	<-d.hyper.Ready

	err := d.Daemon.Attach(stdin, ioutils.NopWriteCloser(stdout), cId)
	if err != nil {
		return err
	}

	code, err := d.Daemon.ExitCode(cId, "")
	if err != nil {
		return err
	}

	if code == 0 {
		return nil
	}

	return &jsonmessage.JSONError{
		Message: fmt.Sprintf("The container '%s' returned a non-zero code: %d", cId, code),
		Code:    code,
	}
}

func (d Docker) Commit(cId string, cfg *types.ContainerCommitConfig) (string, error) {
	// give copy pod a chance to run
	podId, ok := d.hyper.CopyPods[cId]
	if !ok {
		return d.Daemon.Commit(cId, cfg)
	}

	copyshell, err := os.OpenFile(filepath.Join("/var/run/hyper/shell", podId, "exec-copy.sh"), os.O_RDWR|os.O_APPEND, 0755)
	if err != nil {
		glog.Errorf(err.Error())
		return "", err
	}

	fmt.Fprintf(copyshell, "umount /tmp/src/\n")
	fmt.Fprintf(copyshell, "umount /tmp/shell/\n")
	fmt.Fprintf(copyshell, "rm -rf /tmp/shell/\n")
	fmt.Fprintf(copyshell, "rm -rf /tmp/src/\n")

	copyshell.Close()

	go func() {
		<-d.hyper.Ready
	}()

	err = d.ContainerStart(cId, nil)
	if err != nil {
		return "", err
	}

	_, err = d.ContainerWait(cId, -1)
	if err != nil {
		return "", err
	}

	return d.Daemon.Commit(cId, cfg)
}

// BuilderCopy copies/extracts a source FileInfo to a destination path inside a container
// specified by a container object.
// TODO: make sure callers don't unnecessarily convert destPath with filepath.FromSlash (Copy does it already).
// BuilderCopy should take in abstract paths (with slashes) and the implementation should convert it to OS-specific paths.
func (d Docker) BuilderCopy(cId string, destPath string, src builder.FileInfo, decompress bool) error {
	// add copy item to exec-copy.sh
	podId, ok := d.hyper.CopyPods[cId]
	if !ok {
		return fmt.Errorf("%s is not a copy pod", cId)
	}

	copyshell, err := os.OpenFile(filepath.Join("/var/run/hyper/shell", podId, "exec-copy.sh"), os.O_RDWR|os.O_APPEND, 0755)
	if err != nil {
		glog.Errorf(err.Error())
		return err
	}
	fmt.Fprintf(copyshell, fmt.Sprintf("cp -r /tmp/src/%s %s\n", src.Name(), destPath))
	copyshell.Close()

	srcPath := src.Path()
	destExists := true
	destDir := false
	rootUID, rootGID := d.Daemon.GetRemappedUIDGID()

	// Work in daemon-local OS specific file paths
	destPath = filepath.Join("/var/run/hyper/temp/", podId, filepath.FromSlash(src.Name()))
	// Preserve the trailing slash
	// TODO: why are we appending another path separator if there was already one?
	if strings.HasSuffix(destPath, string(os.PathSeparator)) || destPath == "." {
		destDir = true
	}

	destStat, err := os.Stat(destPath)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Errorf("Error performing os.Stat on %s. %s", destPath, err)
			return err
		}
		destExists = false
	}

	uidMaps, gidMaps := d.Daemon.GetUIDGIDMaps()
	archiver := &archive.Archiver{
		Untar:   chrootarchive.Untar,
		UIDMaps: uidMaps,
		GIDMaps: gidMaps,
	}

	if src.IsDir() {
		// copy as directory
		if err := archiver.CopyWithTar(srcPath, destPath); err != nil {
			return err
		}
		return fixPermissions(srcPath, destPath, rootUID, rootGID, destExists)
	}
	if decompress && archive.IsArchivePath(srcPath) {
		// Only try to untar if it is a file and that we've been told to decompress (when ADD-ing a remote file)

		// First try to unpack the source as an archive
		// to support the untar feature we need to clean up the path a little bit
		// because tar is very forgiving.  First we need to strip off the archive's
		// filename from the path but this is only added if it does not end in slash
		tarDest := destPath
		if strings.HasSuffix(tarDest, string(os.PathSeparator)) {
			tarDest = filepath.Dir(destPath)
		}

		// try to successfully untar the orig
		err := archiver.UntarPath(srcPath, tarDest)
		if err != nil {
			glog.Errorf("Couldn't untar to %s: %v", tarDest, err)
		}
		return err
	}

	// only needed for fixPermissions, but might as well put it before CopyFileWithTar
	if destDir || (destExists && destStat.IsDir()) {
		destPath = filepath.Join(destPath, src.Name())
	}

	if err := idtools.MkdirAllNewAs(filepath.Dir(destPath), 0755, rootUID, rootGID); err != nil {
		return err
	}
	if err := archiver.CopyFileWithTar(srcPath, destPath); err != nil {
		return err
	}

	return fixPermissions(srcPath, destPath, rootUID, rootGID, destExists)
}

func (d Docker) ContainerStart(cId string, hostConfig *containertypes.HostConfig) (err error) {
	var vm *hypervisor.Vm

	podId := ""
	if _, ok := d.hyper.CopyPods[cId]; ok {
		podId = d.hyper.CopyPods[cId]
	} else if _, ok := d.hyper.BasicPods[cId]; ok {
		podId = d.hyper.BasicPods[cId]
	} else {
		return fmt.Errorf("container %s doesn't belong to pod", cId)
	}

	defer func() {
		d.hyper.Ready <- true
		if err != nil && d.hyper.Vm != nil {
			if d.hyper.Status != nil {
				d.hyper.Vm.ReleaseResponseChan(d.hyper.Status)
				d.hyper.Status = nil
			}
			glog.Infof("ContainerStart failed, KillVm")
			d.Daemon.KillVm(d.hyper.Vm.Id)
			d.hyper.Vm = nil
		}
	}()

	vmId := "buildevm-" + utils.RandStr(10, "number")
	if vm, err = d.Daemon.StartVm(vmId, 1, 512, false); err != nil {
		return
	}
	d.hyper.Vm = vm

	if d.hyper.Status, err = vm.GetResponseChan(); err != nil {
		return
	}

	if _, _, err = d.Daemon.StartPod(nil, nil, podId, vm.Id, false); err != nil {
		return
	}

	return nil
}

func (d Docker) ContainerWait(cId string, timeout time.Duration) (int, error) {
	//FIXME: implement timeout
	if d.hyper.Vm == nil {
		return -1, fmt.Errorf("no vm is running")
	}

	var podId string

	copyId, isCopyPod := d.hyper.CopyPods[cId]
	basicId, isBasicPod := d.hyper.BasicPods[cId]

	switch {
	case isCopyPod:
		podId = copyId
	case isBasicPod:
		podId = basicId
	default:
		return -1, fmt.Errorf("container %s doesn't belong to pod", cId)
	}

	d.Daemon.PodWait(podId)

	// release pod from VM
	glog.Warningf("pod finished, cleanup")
	d.hyper.Vm.ReleaseResponseChan(d.hyper.Status)
	d.hyper.Vm = nil
	d.hyper.Status = nil

	return 0, nil
}

// Override the Docker ContainerCreate interface, create pod to run command
func (d Docker) ContainerCreate(params types.ContainerCreateConfig) (types.ContainerCreateResponse, error) {
	var podString string
	var err error

	if params.Config == nil {
		return types.ContainerCreateResponse{}, derr.ErrorCodeEmptyConfig
	}

	podId := fmt.Sprintf("buildpod-%s", utils.RandStr(10, "alpha"))
	// Hack here, container created by ADD/COPY only has Config
	if params.HostConfig != nil {
		podString, err = MakeBasicPod(podId, params.Config)
	} else {
		podString, err = MakeCopyPod(podId, params.Config)
	}

	if err != nil {
		return types.ContainerCreateResponse{}, err
	}

	var podSpec apitypes.UserPod
	err = json.Unmarshal([]byte(podString), &podSpec)
	if err != nil {
		return types.ContainerCreateResponse{}, err
	}

	pod, err := d.Daemon.CreatePod(podId, &podSpec)
	if err != nil {
		return types.ContainerCreateResponse{}, err
	}

	if len(pod.Status().Containers) != 1 {
		return types.ContainerCreateResponse{}, fmt.Errorf("container count in pod is incorrect")
	}
	cId := pod.Status().Containers[0].Id
	if params.HostConfig != nil {
		d.hyper.BasicPods[cId] = podId
		glog.Infof("basic containerId %s, podId %s", cId, podId)
	} else {
		d.hyper.CopyPods[cId] = podId
		glog.Infof("copy containerId %s, podId %s", cId, podId)
	}

	return types.ContainerCreateResponse{ID: cId}, nil
}

func MakeCopyPod(podId string, config *containertypes.Config) (string, error) {
	tempSrcDir := filepath.Join("/var/run/hyper/temp/", podId)
	if err := os.MkdirAll(tempSrcDir, 0755); err != nil {
		glog.Errorf(err.Error())
		return "", err
	}
	if _, err := os.Stat(tempSrcDir); err != nil {
		glog.Errorf(err.Error())
		return "", err
	}
	shellDir := filepath.Join("/var/run/hyper/shell/", podId)
	if err := os.MkdirAll(shellDir, 0755); err != nil {
		glog.Errorf(err.Error())
		return "", err
	}
	copyshell, err1 := os.Create(filepath.Join(shellDir, "exec-copy.sh"))
	if err1 != nil {
		glog.Errorf(err1.Error())
		return "", err1
	}

	fmt.Fprintf(copyshell, "#!/bin/sh\n")
	copyshell.Close()

	return MakePod(podId, tempSrcDir, shellDir, config, []string{"/bin/sh", "/tmp/shell/exec-copy.sh"}, []string{})
}

func MakeBasicPod(podId string, config *containertypes.Config) (string, error) {
	return MakePod(podId, "", "", config, config.Cmd.Slice(), config.Entrypoint.Slice())
}

func MakePod(podId, src, shellDir string, config *containertypes.Config, cmds, entrys []string) (string, error) {
	if config.Image == "" {
		return "", fmt.Errorf("image can not be null")
	}

	var (
		env           = []pod.UserEnvironmentVar{}
		containerList = []pod.UserContainer{}
		volList       = []pod.UserVolume{}
		cVols         = []pod.UserVolumeReference{}
	)
	if src != "" {
		myVol1 := pod.UserVolumeReference{
			Path:     "/tmp/src/",
			Volume:   "source",
			ReadOnly: false,
		}
		myVol2 := pod.UserVolumeReference{
			Path:     "/tmp/shell/",
			Volume:   "shell",
			ReadOnly: false,
		}
		cVols = append(cVols, myVol1)
		cVols = append(cVols, myVol2)
		vol1 := pod.UserVolume{
			Name:   "source",
			Source: src,
			Driver: "vfs",
		}
		vol2 := pod.UserVolume{
			Name:   "shell",
			Source: shellDir,
			Driver: "vfs",
		}
		volList = append(volList, vol1)
		volList = append(volList, vol2)
	}

	var container = pod.UserContainer{
		Image:         config.Image,
		Command:       cmds,
		Workdir:       config.WorkingDir,
		Entrypoint:    entrys,
		Tty:           config.Tty,
		Ports:         []pod.UserContainerPort{},
		Envs:          env,
		Volumes:       cVols,
		Files:         []pod.UserFileReference{},
		RestartPolicy: "never",
	}
	containerList = append(containerList, container)

	var userPod = &pod.UserPod{
		Name:       podId,
		Containers: containerList,
		Resource:   pod.UserResource{Vcpu: 1, Memory: 512},
		Files:      []pod.UserFile{},
		Volumes:    volList,
		Tty:        config.Tty,
	}

	jsonString, err := utils.JSONMarshal(userPod, true)
	if err != nil {
		return "", err
	}

	return string(jsonString), nil
}

func (d Docker) ContainerRm(name string, config *types.ContainerRmConfig) error {
	podId := ""
	if _, ok := d.hyper.CopyPods[name]; ok {
		podId = d.hyper.CopyPods[name]
		delete(d.hyper.CopyPods, name)
	} else if _, ok := d.hyper.BasicPods[name]; ok {
		podId = d.hyper.BasicPods[name]
		delete(d.hyper.BasicPods, name)
	} else {
		return d.Daemon.ContainerRm(name, config)
	}

	glog.Infof("ContainerRm pod id %s", podId)
	d.Daemon.CleanPod(podId)

	return nil
}

func (d Docker) Cleanup() {
	for _, podId := range d.hyper.CopyPods {
		d.Daemon.CleanPod(podId)
	}

	for _, podId := range d.hyper.BasicPods {
		d.Daemon.CleanPod(podId)
	}

	close(d.hyper.Ready)
	if d.hyper.Vm != nil {
		if d.hyper.Status != nil {
			d.hyper.Vm.ReleaseResponseChan(d.hyper.Status)
		}

		d.Daemon.KillVm(d.hyper.Vm.Id)
	}
}
