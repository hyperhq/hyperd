package builder

// internals for handling commands. Covers many areas and a lot of
// non-contiguous functionality. Please read the comments.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	//	hyperdaemon "github.com/hyperhq/hyper/daemon"
	"github.com/hyperhq/hyper/lib/docker/builder/parser"
	"github.com/hyperhq/hyper/lib/docker/daemon"
	"github.com/hyperhq/hyper/lib/docker/graph"
	imagepkg "github.com/hyperhq/hyper/lib/docker/image"
	"github.com/hyperhq/hyper/lib/docker/pkg/archive"
	"github.com/hyperhq/hyper/lib/docker/pkg/chrootarchive"
	"github.com/hyperhq/hyper/lib/docker/pkg/httputils"
	"github.com/hyperhq/hyper/lib/docker/pkg/ioutils"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers"
	"github.com/hyperhq/hyper/lib/docker/pkg/progressreader"
	"github.com/hyperhq/hyper/lib/docker/pkg/system"
	"github.com/hyperhq/hyper/lib/docker/pkg/tarsum"
	"github.com/hyperhq/hyper/lib/docker/pkg/urlutil"
	"github.com/hyperhq/hyper/lib/docker/registry"
	"github.com/hyperhq/hyper/lib/docker/runconfig"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func (b *Builder) readContext(context io.Reader) error {
	tmpdirPath, err := ioutil.TempDir("", "docker-build")
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	decompressedStream, err := archive.DecompressStream(context)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	if b.context, err = tarsum.NewTarSum(decompressedStream, true, tarsum.Version0); err != nil {
		glog.Error(err.Error())
		return err
	}

	if err := chrootarchive.Untar(b.context, tmpdirPath, nil); err != nil {
		glog.Error(err.Error())
		return err
	}

	b.contextPath = tmpdirPath
	return nil
}

func (b *Builder) commit(id string, autoCmd *runconfig.Command, comment string) error {
	if b.disableCommit {
		return nil
	}
	if b.image == "" && !b.noBaseImage {
		return fmt.Errorf("Please provide a source image with `from` prior to commit")
	}
	b.Config.Image = b.image
	if id == "" {
		cmd := b.Config.Cmd
		if runtime.GOOS != "windows" {
			b.Config.Cmd = runconfig.NewCommand("/bin/sh", "-c", "#(nop) "+comment)
		} else {
			b.Config.Cmd = runconfig.NewCommand("cmd", "/S /C", "REM (nop) "+comment)
		}
		defer func(cmd *runconfig.Command) { b.Config.Cmd = cmd }(cmd)

		hit, err := b.probeCache()
		if err != nil {
			return err
		}
		if hit {
			return nil
		}

		container, err := b.create()
		if err != nil {
			return err
		}
		id = container.ID
	}
	container, err := b.Daemon.Get(id)
	if err != nil {
		return err
	}

	// Note: Actually copy the struct
	autoConfig := *b.Config
	autoConfig.Cmd = autoCmd

	// Commit the container
	image, err := b.Daemon.Commit(container, "", "", "", b.maintainer, true, &autoConfig)
	if err != nil {
		return err
	}
	b.image = image.ID
	return nil
}

type copyInfo struct {
	origPath   string
	destPath   string
	hash       string
	decompress bool
	tmpDir     string
}

func (b *Builder) runContextCommand(args []string, allowRemote bool, allowDecompression bool, cmdName string) error {
	if b.context == nil {
		return fmt.Errorf("No context given. Impossible to use %s", cmdName)
	}

	if len(args) < 2 {
		return fmt.Errorf("Invalid %s format - at least two arguments required", cmdName)
	}

	dest := args[len(args)-1] // last one is always the dest

	copyInfos := []*copyInfo{}

	b.Config.Image = b.image

	defer func() {
		for _, ci := range copyInfos {
			if ci.tmpDir != "" {
				os.RemoveAll(ci.tmpDir)
			}
		}
	}()

	// Loop through each src file and calculate the info we need to
	// do the copy (e.g. hash value if cached).  Don't actually do
	// the copy until we've looked at all src files
	for _, orig := range args[0 : len(args)-1] {
		if err := calcCopyInfo(
			b,
			cmdName,
			&copyInfos,
			orig,
			dest,
			allowRemote,
			allowDecompression,
			true,
		); err != nil {
			glog.Error(err.Error())
			return err
		}
	}

	if len(copyInfos) == 0 {
		return fmt.Errorf("No source files were specified")
	}

	if len(copyInfos) > 1 && !strings.HasSuffix(dest, "/") {
		return fmt.Errorf("When using %s with more than one source file, the destination must be a directory and end with a /", cmdName)
	}

	// For backwards compat, if there's just one CI then use it as the
	// cache look-up string, otherwise hash 'em all into one
	var srcHash string
	var origPaths string

	if len(copyInfos) == 1 {
		srcHash = copyInfos[0].hash
		origPaths = copyInfos[0].origPath
	} else {
		var hashs []string
		var origs []string
		for _, ci := range copyInfos {
			hashs = append(hashs, ci.hash)
			origs = append(origs, ci.origPath)
		}
		hasher := sha256.New()
		hasher.Write([]byte(strings.Join(hashs, ",")))
		srcHash = "multi:" + hex.EncodeToString(hasher.Sum(nil))
		origPaths = strings.Join(origs, " ")
	}

	cmd := b.Config.Cmd
	if runtime.GOOS != "windows" {
		b.Config.Cmd = runconfig.NewCommand("/bin/sh", "-c", fmt.Sprintf("#(nop) %s %s in %s", cmdName, srcHash, dest))
	} else {
		b.Config.Cmd = runconfig.NewCommand("cmd", "/S /C", fmt.Sprintf("REM (nop) %s %s in %s", cmdName, srcHash, dest))
	}
	defer func(cmd *runconfig.Command) { b.Config.Cmd = cmd }(cmd)

	hit, err := b.probeCache()
	if err != nil {
		return err
	}

	if hit {
		return nil
	}

	b.Config.Image = b.image
	config := *b.Config

	// Create the Pod
	podId := fmt.Sprintf("buildpod-%s", utils.RandStr(10, "alpha"))
	tempSrcDir := fmt.Sprintf("/var/run/hyper/temp/%s/", podId)
	if err := os.MkdirAll(tempSrcDir, 0755); err != nil {
		glog.Errorf(err.Error())
		return err
	}
	if _, err := os.Stat(tempSrcDir); err != nil {
		glog.Errorf(err.Error())
		return err
	}
	shellDir := fmt.Sprintf("/var/run/hyper/shell/%s/", podId)
	if err := os.MkdirAll(shellDir, 0755); err != nil {
		glog.Errorf(err.Error())
		return err
	}
	copyshell, err1 := os.Create(shellDir + "/exec-copy.sh")
	if err1 != nil {
		glog.Errorf(err1.Error())
		return err1
	}
	fmt.Fprintf(copyshell, "#!/bin/sh\n")

	podString, err := MakeCopyPod(podId, b.image, b.Config.WorkingDir, tempSrcDir, dest, shellDir)
	if err != nil {
		return err
	}
	err = b.Hyperdaemon.CreatePod(podId, podString, config, false)
	if err != nil {
		return err
	}
	// Get the container
	var (
		containerId = ""
		container   *daemon.Container
	)
	for _, i := range b.Hyperdaemon.PodList[podId].Containers {
		containerId = i.Id
	}
	container, err = b.Daemon.Get(containerId)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	b.TmpContainers[container.ID] = struct{}{}
	b.TmpPods[podId] = struct{}{}
	for _, ci := range copyInfos {
		glog.V(1).Infof("container %s, origPath %s, destPath %s", container.ID, ci.origPath, ci.destPath)
		// Copy the source files to tempSrcDir
		if err := b.addContext(container, ci.origPath, tempSrcDir, ci.decompress); err != nil {
			glog.Error(err.Error())
			return err
		}
		if strings.HasSuffix(dest, "/") == true {
			fmt.Fprintf(copyshell, fmt.Sprintf("cp /tmp/src/%s %s\n", ci.origPath, ci.destPath))
		} else {
			fmt.Fprintf(copyshell, fmt.Sprintf("cp /tmp/src/%s %s\n", ci.origPath, dest))
			break
		}
	}

	fmt.Fprintf(copyshell, "umount /tmp/src/\n")
	fmt.Fprintf(copyshell, "umount /tmp/shell/\n")
	fmt.Fprintf(copyshell, "rm -rf /tmp/shell/\n")
	fmt.Fprintf(copyshell, "rm -rf /tmp/src/\n")
	copyshell.Close()
	// start or replace pod
	vm, ok := b.Hyperdaemon.VmList[b.Name]
	if !ok {
		glog.Warningf("can not find VM(%s)", b.Name)
	}
	if !ok || vm.Status == types.S_VM_IDLE {
		bo := &hypervisor.BootConfig{
			CPU:    1,
			Memory: 512,
			Kernel: b.Hyperdaemon.Kernel,
			Initrd: b.Hyperdaemon.Initrd,
			Bios:   b.Hyperdaemon.Bios,
			Cbfs:   b.Hyperdaemon.Cbfs,
		}

		vm = b.Hyperdaemon.NewVm(b.Name, 1, 512, false, types.VM_KEEP_AFTER_FINISH)

		err = vm.Launch(bo)
		if err != nil {
			return err
		}

		b.Hyperdaemon.AddVm(vm)
		code, cause, err := b.Hyperdaemon.StartPod(podId, "", b.Name, nil, false, false, types.VM_KEEP_AFTER_FINISH)
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			b.Hyperdaemon.KillVm(b.Name)
			return err
		}
		vm = b.Hyperdaemon.VmList[b.Name]
		// wait for cmd finish
		_, _, ret3, err := vm.GetQemuChan()
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		subVmStatus := ret3.(chan *types.QemuResponse)
		var vmResponse *types.QemuResponse
		for {
			vmResponse = <-subVmStatus
			if vmResponse.VmId == b.Name {
				if vmResponse.Code == types.E_POD_FINISHED {
					glog.Infof("Got E_POD_FINISHED code response")
					break
				}
			}
		}

		b.Hyperdaemon.PodList[podId].Vm = b.Name
		// release pod from VM
		glog.Warningf("start stop pod")
		code, cause, err = b.Hyperdaemon.StopPod(podId, "no")
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			b.Hyperdaemon.KillVm(b.Name)
			return err
		}
		glog.Warningf("stop pod finish")
	} else {
		glog.Errorf("Vm is not IDLE")
		return fmt.Errorf("Vm is not IDLE")
	}

	glog.Warningf("begin commit")
	if err := b.commit(container.ID, cmd, fmt.Sprintf("%s %s in %s", cmdName, origPaths, dest)); err != nil {
		return err
	}

	return nil
}

func calcCopyInfo(b *Builder, cmdName string, cInfos *[]*copyInfo, origPath string, destPath string, allowRemote bool, allowDecompression bool, allowWildcards bool) error {

	if origPath != "" && origPath[0] == '/' && len(origPath) > 1 {
		origPath = origPath[1:]
	}
	origPath = strings.TrimPrefix(origPath, "./")

	// Twiddle the destPath when its a relative path - meaning, make it
	// relative to the WORKINGDIR
	if !filepath.IsAbs(destPath) {
		hasSlash := strings.HasSuffix(destPath, "/")
		destPath = filepath.Join("/", b.Config.WorkingDir, destPath)

		// Make sure we preserve any trailing slash
		if hasSlash {
			destPath += "/"
		}
	}

	// In the remote/URL case, download it and gen its hashcode
	if urlutil.IsURL(origPath) {
		if !allowRemote {
			return fmt.Errorf("Source can't be a URL for %s", cmdName)
		}

		ci := copyInfo{}
		ci.origPath = origPath
		ci.hash = origPath // default to this but can change
		ci.destPath = destPath
		ci.decompress = false
		*cInfos = append(*cInfos, &ci)

		// Initiate the download
		resp, err := httputils.Download(ci.origPath)
		if err != nil {
			return err
		}

		// Create a tmp dir
		tmpDirName, err := ioutil.TempDir(b.contextPath, "docker-remote")
		if err != nil {
			return err
		}
		ci.tmpDir = tmpDirName

		// Create a tmp file within our tmp dir
		tmpFileName := filepath.Join(tmpDirName, "tmp")
		tmpFile, err := os.OpenFile(tmpFileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}

		// Download and dump result to tmp file
		if _, err := io.Copy(tmpFile, progressreader.New(progressreader.Config{
			In:        resp.Body,
			Out:       b.OutOld,
			Formatter: b.StreamFormatter,
			Size:      int(resp.ContentLength),
			NewLines:  true,
			ID:        "",
			Action:    "Downloading",
		})); err != nil {
			tmpFile.Close()
			return err
		}
		fmt.Fprintf(b.OutStream, "\n")
		tmpFile.Close()

		// Set the mtime to the Last-Modified header value if present
		// Otherwise just remove atime and mtime
		times := make([]syscall.Timespec, 2)

		lastMod := resp.Header.Get("Last-Modified")
		if lastMod != "" {
			mTime, err := http.ParseTime(lastMod)
			// If we can't parse it then just let it default to 'zero'
			// otherwise use the parsed time value
			if err == nil {
				times[1] = syscall.NsecToTimespec(mTime.UnixNano())
			}
		}

		if err := system.UtimesNano(tmpFileName, times); err != nil {
			return err
		}

		ci.origPath = filepath.Join(filepath.Base(tmpDirName), filepath.Base(tmpFileName))

		// If the destination is a directory, figure out the filename.
		if strings.HasSuffix(ci.destPath, "/") {
			u, err := url.Parse(origPath)
			if err != nil {
				return err
			}
			path := u.Path
			if strings.HasSuffix(path, "/") {
				path = path[:len(path)-1]
			}
			parts := strings.Split(path, "/")
			filename := parts[len(parts)-1]
			if filename == "" {
				return fmt.Errorf("cannot determine filename from url: %s", u)
			}
			ci.destPath = ci.destPath + filename
		}

		// Calc the checksum, even if we're using the cache
		r, err := archive.Tar(tmpFileName, archive.Uncompressed)
		if err != nil {
			return err
		}
		tarSum, err := tarsum.NewTarSum(r, true, tarsum.Version0)
		if err != nil {
			return err
		}
		if _, err := io.Copy(ioutil.Discard, tarSum); err != nil {
			return err
		}
		ci.hash = tarSum.Sum(nil)
		r.Close()

		return nil
	}

	// Deal with wildcards
	if allowWildcards && ContainsWildcards(origPath) {
		for _, fileInfo := range b.context.GetSums() {
			if fileInfo.Name() == "" {
				continue
			}
			match, _ := filepath.Match(origPath, fileInfo.Name())
			if !match {
				continue
			}

			// Note we set allowWildcards to false in case the name has
			// a * in it
			calcCopyInfo(b, cmdName, cInfos, fileInfo.Name(), destPath, allowRemote, allowDecompression, false)
		}
		return nil
	}

	// Must be a dir or a file

	if err := b.checkPathForAddition(origPath); err != nil {
		return err
	}
	fi, _ := os.Stat(filepath.Join(b.contextPath, origPath))

	ci := copyInfo{}
	ci.origPath = origPath
	ci.hash = origPath
	ci.destPath = destPath
	ci.decompress = allowDecompression
	*cInfos = append(*cInfos, &ci)

	// Deal with the single file case
	if !fi.IsDir() {
		// This will match first file in sums of the archive
		fis := b.context.GetSums().GetFile(ci.origPath)
		if fis != nil {
			ci.hash = "file:" + fis.Sum()
		}
		return nil
	}

	// Must be a dir
	var subfiles []string
	absOrigPath := filepath.Join(b.contextPath, ci.origPath)

	// Add a trailing / to make sure we only pick up nested files under
	// the dir and not sibling files of the dir that just happen to
	// start with the same chars
	if !strings.HasSuffix(absOrigPath, "/") {
		absOrigPath += "/"
	}

	// Need path w/o / too to find matching dir w/o trailing /
	absOrigPathNoSlash := absOrigPath[:len(absOrigPath)-1]

	for _, fileInfo := range b.context.GetSums() {
		absFile := filepath.Join(b.contextPath, fileInfo.Name())
		// Any file in the context that starts with the given path will be
		// picked up and its hashcode used.  However, we'll exclude the
		// root dir itself.  We do this for a coupel of reasons:
		// 1 - ADD/COPY will not copy the dir itself, just its children
		//     so there's no reason to include it in the hash calc
		// 2 - the metadata on the dir will change when any child file
		//     changes.  This will lead to a miss in the cache check if that
		//     child file is in the .dockerignore list.
		if strings.HasPrefix(absFile, absOrigPath) && absFile != absOrigPathNoSlash {
			subfiles = append(subfiles, fileInfo.Sum())
		}
	}
	sort.Strings(subfiles)
	hasher := sha256.New()
	hasher.Write([]byte(strings.Join(subfiles, ",")))
	ci.hash = "dir:" + hex.EncodeToString(hasher.Sum(nil))

	return nil
}

func ContainsWildcards(name string) bool {
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '\\' {
			i++
		} else if ch == '*' || ch == '?' || ch == '[' {
			return true
		}
	}
	return false
}

func (b *Builder) pullImage(name string) (*imagepkg.Image, error) {
	remote, tag := parsers.ParseRepositoryTag(name)
	if tag == "" {
		tag = "latest"
	}

	pullRegistryAuth := b.AuthConfig
	if len(b.ConfigFile.AuthConfigs) > 0 {
		// The request came with a full auth config file, we prefer to use that
		repoInfo, err := b.Daemon.RegistryService.ResolveRepository(remote)
		if err != nil {
			return nil, err
		}
		resolvedAuth := registry.ResolveAuthConfig(b.ConfigFile, repoInfo.Index)
		pullRegistryAuth = &resolvedAuth
	}

	imagePullConfig := &graph.ImagePullConfig{
		AuthConfig: pullRegistryAuth,
		OutStream:  ioutils.NopWriteCloser(b.OutOld),
	}

	if err := b.Daemon.Repositories().Pull(remote, tag, imagePullConfig); err != nil {
		return nil, err
	}

	image, err := b.Daemon.Repositories().LookupImage(name)
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (b *Builder) processImageFrom(img *imagepkg.Image) error {
	b.image = img.ID

	if img.Config != nil {
		b.Config = img.Config
	}

	if len(b.Config.Env) == 0 {
		b.Config.Env = append(b.Config.Env, "PATH="+daemon.DefaultPathEnv)
	}

	// Process ONBUILD triggers if they exist
	if nTriggers := len(b.Config.OnBuild); nTriggers != 0 {
		fmt.Fprintf(b.ErrStream, "# Executing %d build triggers\n", nTriggers)
	}

	// Copy the ONBUILD triggers, and remove them from the config, since the config will be committed.
	onBuildTriggers := b.Config.OnBuild
	b.Config.OnBuild = []string{}

	// parse the ONBUILD triggers by invoking the parser
	for stepN, step := range onBuildTriggers {
		ast, err := parser.Parse(strings.NewReader(step))
		if err != nil {
			return err
		}

		for i, n := range ast.Children {
			switch strings.ToUpper(n.Value) {
			case "ONBUILD":
				return fmt.Errorf("Chaining ONBUILD via `ONBUILD ONBUILD` isn't allowed")
			case "MAINTAINER", "FROM":
				return fmt.Errorf("%s isn't allowed as an ONBUILD trigger", n.Value)
			}

			fmt.Fprintf(b.OutStream, "Trigger %d, %s\n", stepN, step)

			if err := b.dispatch(i, n); err != nil {
				return err
			}
		}
	}

	return nil
}

// probeCache checks to see if image-caching is enabled (`b.UtilizeCache`)
// and if so attempts to look up the current `b.image` and `b.Config` pair
// in the current server `b.Daemon`. If an image is found, probeCache returns
// `(true, nil)`. If no image is found, it returns `(false, nil)`. If there
// is any error, it returns `(false, err)`.
func (b *Builder) probeCache() (bool, error) {
	if !b.UtilizeCache || b.cacheBusted {
		return false, nil
	}

	cache, err := b.Daemon.ImageGetCached(b.image, b.Config)
	if err != nil {
		return false, err
	}
	if cache == nil {
		glog.V(1).Infof("[BUILDER] Cache miss")
		b.cacheBusted = true
		return false, nil
	}

	fmt.Fprintf(b.OutStream, " ---> Using cache\n")
	glog.V(1).Infof("[BUILDER] Use cached version")
	b.image = cache.ID
	return true, nil
}

func (b *Builder) run(c *daemon.Container) error {
	if b.Verbose {
	}

	//start the pod
	var (
		mycontainer *hypervisor.Container
		code        int
		cause       string
		err         error
	)
	for _, p := range b.Hyperdaemon.PodList {
		for _, con := range p.Containers {
			if con.Id == c.ID {
				mycontainer = con
				break
			}
		}
		if mycontainer != nil {
			break
		}
	}

	if mycontainer == nil {
		return fmt.Errorf("can not find that container(%s)", c.ID)
	}
	podId := mycontainer.PodId
	// start or replace pod
	vm, ok := b.Hyperdaemon.VmList[b.Name]
	if !ok {
		glog.Warningf("can not find VM(%s)", b.Name)
	}
	if !ok || vm.Status == types.S_VM_IDLE {
		code, cause, err = b.Hyperdaemon.StartPod(podId, "", b.Name, nil, false, false, types.VM_KEEP_AFTER_FINISH)
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			b.Hyperdaemon.KillVm(b.Name)
			return err
		}
		vm = b.Hyperdaemon.VmList[b.Name]
		// wait for cmd finish
		_, _, ret3, err := vm.GetQemuChan()
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		subVmStatus := ret3.(chan *types.QemuResponse)
		var vmResponse *types.QemuResponse
		for {
			vmResponse = <-subVmStatus
			if vmResponse.VmId == b.Name {
				if vmResponse.Code == types.E_POD_FINISHED {
					glog.Infof("Got E_POD_FINISHED code response")
					break
				}
			}
		}

		b.Hyperdaemon.PodList[podId].Vm = b.Name
		// release pod from VM
		code, cause, err = b.Hyperdaemon.StopPod(podId, "no")
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			b.Hyperdaemon.KillVm(b.Name)
			return err
		}
	} else {
		glog.Errorf("Vm is not IDLE")
		return fmt.Errorf("Vm is not IDLE")
	}

	return nil
}

func (b *Builder) checkPathForAddition(orig string) error {
	origPath := filepath.Join(b.contextPath, orig)
	origPath, err := filepath.EvalSymlinks(origPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file or directory", orig)
		}
		return err
	}
	contextPath, err := filepath.EvalSymlinks(b.contextPath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(origPath, contextPath) {
		return fmt.Errorf("Forbidden path outside the build context: %s (%s)", orig, origPath)
	}
	if _, err := os.Stat(origPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file or directory", orig)
		}
		return err
	}
	return nil
}

func (b *Builder) addContext(container *daemon.Container, orig, dest string, decompress bool) error {
	var (
		err        error
		destExists = true
		origPath   = filepath.Join(b.contextPath, orig)
		destPath   string
	)
	destPath = dest
	destStat, err := os.Stat(destPath)
	if err != nil {
		if !os.IsNotExist(err) {
			glog.Error(err.Error())
			return err
		}
		destExists = false
	}

	fi, err := os.Stat(origPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: no such file or directory", orig)
		}
		glog.Error(err.Error())
		return err
	}

	if fi.IsDir() {
		err := copyAsDirectory(origPath, destPath, destExists)
		if err != nil {
			glog.Error(err.Error())
		}
		return err
	}

	// If we are adding a remote file (or we've been told not to decompress), do not try to untar it
	if decompress {
		// First try to unpack the source as an archive
		// to support the untar feature we need to clean up the path a little bit
		// because tar is very forgiving.  First we need to strip off the archive's
		// filename from the path but this is only added if it does not end in / .
		tarDest := destPath
		if strings.HasSuffix(tarDest, "/") {
			tarDest = filepath.Dir(destPath)
		}

		// try to successfully untar the orig
		if err := chrootarchive.UntarPath(origPath, tarDest); err == nil {
			return nil
		} else if err != io.EOF {
			glog.Infof("Couldn't untar %s to %s: %s", origPath, tarDest, err)
		}
	}

	if err := system.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		glog.Error(err.Error())
		return err
	}
	if err := chrootarchive.CopyWithTar(origPath, destPath); err != nil {
		glog.Error(err.Error())
		return err
	}

	resPath := destPath
	if destExists && destStat.IsDir() {
		resPath = filepath.Join(destPath, filepath.Base(origPath))
	}
	_ = resPath
	/*
		if err := fixPermissions(origPath, resPath, 0, 0, destExists); err != nil {
			glog.Error(err.Error())
			return err
		}
	*/
	return nil
}

func copyAsDirectory(source, destination string, destExisted bool) error {
	if err := chrootarchive.CopyWithTar(source, destination); err != nil {
		glog.Error(err.Error())
		return err
	}
	return fixPermissions(source, destination, 0, 0, destExisted)
}

func (b *Builder) clearTmp() {
	for p := range b.TmpPods {
		b.Hyperdaemon.CleanPod(p)
		delete(b.TmpPods, p)
		fmt.Fprintf(b.OutStream, "Removing intermediate pod %s\n", p)
	}
}
