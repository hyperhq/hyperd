package dockerfile

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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api"
	"github.com/docker/docker/builder/dockerfile/parser"
	"github.com/docker/docker/image"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/httputils"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/progressreader"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/pkg/stringutils"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/docker/pkg/tarsum"
	"github.com/docker/docker/pkg/urlutil"
	"github.com/golang/glog"
	hyperdaemon "github.com/hyperhq/hyper/daemon"
	"github.com/hyperhq/hyper/lib/docker/builder"
	"github.com/hyperhq/hyper/lib/docker/daemon"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (b *Builder) commit(id string, autoCmd *stringutils.StrSlice, comment string) error {
	if b.disableCommit {
		return nil
	}
	if b.image == "" && !b.noBaseImage {
		return fmt.Errorf("Please provide a source image with `from` prior to commit")
	}
	b.runConfig.Image = b.image
	if id == "" {
		cmd := b.runConfig.Cmd
		if runtime.GOOS != "windows" {
			b.runConfig.Cmd = stringutils.NewStrSlice("/bin/sh", "-c", "#(nop) "+comment)
		} else {
			b.runConfig.Cmd = stringutils.NewStrSlice("cmd", "/S /C", "REM (nop) "+comment)
		}
		defer func(cmd *stringutils.StrSlice) { b.runConfig.Cmd = cmd }(cmd)

		if hit, err := b.probeCache(); err != nil {
			return err
		} else if hit {
			return nil
		}
		container, err := b.create()
		if err != nil {
			return err
		}
		id = container.ID

		if err := b.docker.Mount(container); err != nil {
			return err
		}
		defer b.docker.Unmount(container)
	}

	// Note: Actually copy the struct
	autoConfig := *b.runConfig
	autoConfig.Cmd = autoCmd

	commitCfg := &daemon.ContainerCommitConfig{
		Author: b.maintainer,
		Pause:  true,
		Config: &autoConfig,
	}

	// Commit the container
	image, err := b.docker.Commit(id, commitCfg)
	if err != nil {
		return err
	}
	b.docker.Retain(b.id, image.ID)
	b.activeImages = append(b.activeImages, image.ID)
	b.image = image.ID
	return nil
}

type copyInfo struct {
	builder.FileInfo
	decompress bool
}

func (b *Builder) runContextCommand(args []string, allowRemote bool, allowLocalDecompression bool, cmdName string) error {
	if b.context == nil {
		return fmt.Errorf("No context given. Impossible to use %s", cmdName)
	}

	if len(args) < 2 {
		return fmt.Errorf("Invalid %s format - at least two arguments required", cmdName)
	}

	// Work in daemon-specific filepath semantics
	dest := filepath.FromSlash(args[len(args)-1]) // last one is always the dest

	b.runConfig.Image = b.image

	var infos []copyInfo

	// Loop through each src file and calculate the info we need to
	// do the copy (e.g. hash value if cached).  Don't actually do
	// the copy until we've looked at all src files
	var err error
	for _, orig := range args[0 : len(args)-1] {
		var fi builder.FileInfo
		decompress := allowLocalDecompression
		if urlutil.IsURL(orig) {
			if !allowRemote {
				return fmt.Errorf("Source can't be a URL for %s", cmdName)
			}
			fi, err = b.download(orig)
			if err != nil {
				return err
			}
			defer os.RemoveAll(filepath.Dir(fi.Path()))
			decompress = false
			infos = append(infos, copyInfo{fi, decompress})
			continue
		}
		// not a URL
		subInfos, err := b.calcCopyInfo(cmdName, orig, allowLocalDecompression, true)
		if err != nil {
			return err
		}

		infos = append(infos, subInfos...)
	}

	if len(infos) == 0 {
		return fmt.Errorf("No source files were specified")
	}
	if len(infos) > 1 && !strings.HasSuffix(dest, string(os.PathSeparator)) {
		return fmt.Errorf("When using %s with more than one source file, the destination must be a directory and end with a /", cmdName)
	}

	// For backwards compat, if there's just one info then use it as the
	// cache look-up string, otherwise hash 'em all into one
	var srcHash string
	var origPaths string

	if len(infos) == 1 {
		fi := infos[0].FileInfo
		origPaths = fi.Name()
		if hfi, ok := fi.(builder.Hashed); ok {
			srcHash = hfi.Hash()
		}
	} else {
		var hashs []string
		var origs []string
		for _, info := range infos {
			fi := info.FileInfo
			origs = append(origs, fi.Name())
			if hfi, ok := fi.(builder.Hashed); ok {
				hashs = append(hashs, hfi.Hash())
			}
		}
		hasher := sha256.New()
		hasher.Write([]byte(strings.Join(hashs, ",")))
		srcHash = "multi:" + hex.EncodeToString(hasher.Sum(nil))
		origPaths = strings.Join(origs, " ")
	}

	cmd := b.runConfig.Cmd
	if runtime.GOOS != "windows" {
		b.runConfig.Cmd = stringutils.NewStrSlice("/bin/sh", "-c", fmt.Sprintf("#(nop) %s %s in %s", cmdName, srcHash, dest))
	} else {
		b.runConfig.Cmd = stringutils.NewStrSlice("cmd", "/S", "/C", fmt.Sprintf("REM (nop) %s %s in %s", cmdName, srcHash, dest))
	}
	defer func(cmd *stringutils.StrSlice) { b.runConfig.Cmd = cmd }(cmd)

	if hit, err := b.probeCache(); err != nil {
		return err
	} else if hit {
		return nil
	}

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

	podString, err := MakeCopyPod(podId, b.image, b.runConfig.WorkingDir, tempSrcDir, dest, shellDir)
	if err != nil {
		return err
	}
	err = b.Hyperdaemon.CreatePod(podId, podString, false)
	if err != nil {
		return err
	}
	// Get the container
	var (
		containerId string = ""
		container   *daemon.Container
	)

	ps, ok := b.Hyperdaemon.PodList.GetStatus(podId)
	if !ok {
		return fmt.Errorf("Cannot find pod %s", podId)
	}
	for _, i := range ps.Containers {
		containerId = i.Id
	}
	container, err = b.Daemon.Get(containerId)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	/*
		container, _, err := b.docker.Create(b.runConfig, nil)
		if err != nil {
			return err
		}
		defer b.docker.Unmount(container)
	*/
	b.tmpPods[podId] = struct{}{}
	b.tmpContainers[container.ID] = struct{}{}

	comment := fmt.Sprintf("%s %s in %s", cmdName, origPaths, dest)

	// Twiddle the destination when its a relative path - meaning, make it
	// relative to the WORKINGDIR
	if !system.IsAbs(dest) {
		hasSlash := strings.HasSuffix(dest, string(os.PathSeparator))
		dest = filepath.Join(string(os.PathSeparator), filepath.FromSlash(b.runConfig.WorkingDir), dest)

		// Make sure we preserve any trailing slash
		if hasSlash {
			dest += string(os.PathSeparator)
		}
	}

	for _, info := range infos {
		if err := b.docker.Copy(container, tempSrcDir, info.FileInfo, info.decompress); err != nil {
			return err
		}
		if strings.HasSuffix(dest, string(os.PathSeparator)) == true {
			fmt.Fprintf(copyshell, fmt.Sprintf("cp /tmp/src/%s %s\n", info.FileInfo.Name(), filepath.Join(dest, info.FileInfo.Name())))
		} else {
			fmt.Fprintf(copyshell, fmt.Sprintf("cp /tmp/src/%s %s\n", info.FileInfo.Name(), dest))
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

		bo := &hypervisor.BootConfig{
			CPU:    1,
			Memory: 512,
			Kernel: b.Hyperdaemon.Kernel,
			Initrd: b.Hyperdaemon.Initrd,
			Bios:   b.Hyperdaemon.Bios,
			Cbfs:   b.Hyperdaemon.Cbfs,
			Vbox:   b.Hyperdaemon.VboxImage,
		}

		vm = b.Hyperdaemon.NewVm(b.Name, 1, 512, false, types.VM_KEEP_AFTER_FINISH)

		err = vm.Launch(bo)
		if err != nil {
			return err
		}

		b.Hyperdaemon.AddVm(vm)
	}
	if vm.Status == types.S_VM_IDLE {
		code, cause, err := b.Hyperdaemon.StartPod(podId, "", b.Name, nil, false, false, types.VM_KEEP_AFTER_FINISH, []*hypervisor.TtyIO{})
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			b.Hyperdaemon.KillVm(b.Name)
			return err
		}
		vm = b.Hyperdaemon.VmList[b.Name]
		// wait for cmd finish
		Status, err := vm.GetResponseChan()
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		defer vm.ReleaseResponseChan(Status)

		var vmResponse *types.VmResponse
		for {
			vmResponse = <-Status
			if vmResponse.VmId == b.Name {
				if vmResponse.Code == types.E_POD_FINISHED {
					glog.Infof("Got E_POD_FINISHED code response")
					break
				}
			}
		}

		pod, ok := b.Hyperdaemon.PodList.Get(podId)
		if !ok {
			return fmt.Errorf("Cannot find pod %s", podId)
		}
		pod.SetVM(b.Name, vm)

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

	if err := b.commit(container.ID, cmd, comment); err != nil {
		return err
	}
	return nil
}

func (b *Builder) download(srcURL string) (fi builder.FileInfo, err error) {
	// get filename from URL
	u, err := url.Parse(srcURL)
	if err != nil {
		return
	}
	path := filepath.FromSlash(u.Path) // Ensure in platform semantics
	if strings.HasSuffix(path, string(os.PathSeparator)) {
		path = path[:len(path)-1]
	}
	parts := strings.Split(path, string(os.PathSeparator))
	filename := parts[len(parts)-1]
	if filename == "" {
		err = fmt.Errorf("cannot determine filename from url: %s", u)
		return
	}

	// Initiate the download
	resp, err := httputils.Download(srcURL)
	if err != nil {
		return
	}

	// Prepare file in a tmp dir
	tmpDir, err := ioutils.TempDir("", "docker-remote")
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpDir)
		}
	}()
	tmpFileName := filepath.Join(tmpDir, filename)
	tmpFile, err := os.OpenFile(tmpFileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return
	}

	// Download and dump result to tmp file
	if _, err = io.Copy(tmpFile, progressreader.New(progressreader.Config{
		In: resp.Body,
		// TODO: make progressreader streamformatter agnostic
		Out:       b.Stdout.(*streamformatter.StdoutFormatter).Writer,
		Formatter: b.Stdout.(*streamformatter.StdoutFormatter).StreamFormatter,
		Size:      resp.ContentLength,
		NewLines:  true,
		ID:        "",
		Action:    "Downloading",
	})); err != nil {
		tmpFile.Close()
		return
	}
	fmt.Fprintln(b.Stdout)
	// ignoring error because the file was already opened successfully
	tmpFileSt, err := tmpFile.Stat()
	if err != nil {
		return
	}
	tmpFile.Close()

	// Set the mtime to the Last-Modified header value if present
	// Otherwise just remove atime and mtime
	mTime := time.Time{}

	lastMod := resp.Header.Get("Last-Modified")
	if lastMod != "" {
		// If we can't parse it then just let it default to 'zero'
		// otherwise use the parsed time value
		if parsedMTime, err := http.ParseTime(lastMod); err == nil {
			mTime = parsedMTime
		}
	}

	if err = system.Chtimes(tmpFileName, mTime, mTime); err != nil {
		return
	}

	// Calc the checksum, even if we're using the cache
	r, err := archive.Tar(tmpFileName, archive.Uncompressed)
	if err != nil {
		return
	}
	tarSum, err := tarsum.NewTarSum(r, true, tarsum.Version1)
	if err != nil {
		return
	}
	if _, err = io.Copy(ioutil.Discard, tarSum); err != nil {
		return
	}
	hash := tarSum.Sum(nil)
	r.Close()
	return &builder.HashedFileInfo{FileInfo: builder.PathFileInfo{FileInfo: tmpFileSt, FilePath: tmpFileName}, FileHash: hash}, nil
}

func (b *Builder) calcCopyInfo(cmdName, origPath string, allowLocalDecompression, allowWildcards bool) ([]copyInfo, error) {

	// Work in daemon-specific OS filepath semantics
	origPath = filepath.FromSlash(origPath)

	if origPath != "" && origPath[0] == os.PathSeparator && len(origPath) > 1 {
		origPath = origPath[1:]
	}
	origPath = strings.TrimPrefix(origPath, "."+string(os.PathSeparator))

	// Deal with wildcards
	if allowWildcards && containsWildcards(origPath) {
		var copyInfos []copyInfo
		if err := b.context.Walk("", func(path string, info builder.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Name() == "" {
				// Why are we doing this check?
				return nil
			}
			if match, _ := filepath.Match(origPath, path); !match {
				return nil
			}

			// Note we set allowWildcards to false in case the name has
			// a * in it
			subInfos, err := b.calcCopyInfo(cmdName, path, allowLocalDecompression, false)
			if err != nil {
				return err
			}
			copyInfos = append(copyInfos, subInfos...)
			return nil
		}); err != nil {
			return nil, err
		}
		return copyInfos, nil
	}

	// Must be a dir or a file

	statPath, fi, err := b.context.Stat(origPath)
	if err != nil {
		return nil, err
	}

	copyInfos := []copyInfo{{FileInfo: fi, decompress: allowLocalDecompression}}

	hfi, handleHash := fi.(builder.Hashed)
	if !handleHash {
		return copyInfos, nil
	}

	// Deal with the single file case
	if !fi.IsDir() {
		hfi.SetHash("file:" + hfi.Hash())
		return copyInfos, nil
	}
	// Must be a dir
	var subfiles []string
	err = b.context.Walk(statPath, func(path string, info builder.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// we already checked handleHash above
		subfiles = append(subfiles, info.(builder.Hashed).Hash())
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(subfiles)
	hasher := sha256.New()
	hasher.Write([]byte(strings.Join(subfiles, ",")))
	hfi.SetHash("dir:" + hex.EncodeToString(hasher.Sum(nil)))

	return copyInfos, nil
}

func containsWildcards(name string) bool {
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

func (b *Builder) processImageFrom(img *image.Image) error {
	b.image = img.ID

	if img.Config != nil {
		b.runConfig = img.Config
	}

	// The default path will be blank on Windows (set by HCS)
	if len(b.runConfig.Env) == 0 && daemon.DefaultPathEnv != "" {
		b.runConfig.Env = append(b.runConfig.Env, "PATH="+daemon.DefaultPathEnv)
	}

	// Process ONBUILD triggers if they exist
	if nTriggers := len(b.runConfig.OnBuild); nTriggers != 0 {
		word := "trigger"
		if nTriggers > 1 {
			word = "triggers"
		}
		fmt.Fprintf(b.Stderr, "# Executing %d build %s...\n", nTriggers, word)
	}

	// Copy the ONBUILD triggers, and remove them from the config, since the config will be committed.
	onBuildTriggers := b.runConfig.OnBuild
	b.runConfig.OnBuild = []string{}

	// parse the ONBUILD triggers by invoking the parser
	for _, step := range onBuildTriggers {
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

			if err := b.dispatch(i, n); err != nil {
				return err
			}
		}
	}

	return nil
}

// probeCache checks if `b.docker` implements builder.ImageCache and image-caching
// is enabled (`b.UseCache`).
// If so attempts to look up the current `b.image` and `b.runConfig` pair with `b.docker`.
// If an image is found, probeCache returns `(true, nil)`.
// If no image is found, it returns `(false, nil)`.
// If there is any error, it returns `(false, err)`.
func (b *Builder) probeCache() (bool, error) {
	c, ok := b.docker.(builder.ImageCache)
	if !ok || !b.UseCache || b.cacheBusted {
		return false, nil
	}
	cache, err := c.GetCachedImage(b.image, b.runConfig)
	if err != nil {
		return false, err
	}
	if len(cache) == 0 {
		logrus.Debugf("[BUILDER] Cache miss: %s", b.runConfig.Cmd)
		b.cacheBusted = true
		return false, nil
	}

	fmt.Fprintf(b.Stdout, " ---> Using cache\n")
	logrus.Debugf("[BUILDER] Use cached version: %s", b.runConfig.Cmd)
	b.image = string(cache)

	// TODO: remove once Commit can take a tag parameter.
	b.docker.Retain(b.id, b.image)
	b.activeImages = append(b.activeImages, b.image)

	return true, nil
}

func (b *Builder) run(c *daemon.Container) error {
	//start the pod
	var (
		mycontainer *hypervisor.Container
		code        int
		cause       string
		err         error
	)
	b.Hyperdaemon.PodList.Find(func(p *hyperdaemon.Pod) bool {
		ps := p.Status()
		if ps == nil {
			return false
		}
		for _, con := range ps.Containers {
			if con.Id == c.ID {
				mycontainer = con
				return true
			}
		}
		return false
	})

	if mycontainer == nil {
		return fmt.Errorf("can not find that container(%s)", c.ID)
	}
	podId := mycontainer.PodId
	// start or replace pod
	vm, ok := b.Hyperdaemon.VmList[b.Name]
	if !ok {
		glog.Warningf("can not find VM(%s)", b.Name)

		bo := &hypervisor.BootConfig{
			CPU:    1,
			Memory: 512,
			Kernel: b.Hyperdaemon.Kernel,
			Initrd: b.Hyperdaemon.Initrd,
			Bios:   b.Hyperdaemon.Bios,
			Cbfs:   b.Hyperdaemon.Cbfs,
			Vbox:   b.Hyperdaemon.VboxImage,
		}

		vm = b.Hyperdaemon.NewVm(b.Name, 1, 512, false, types.VM_KEEP_AFTER_FINISH)

		err = vm.Launch(bo)
		if err != nil {
			return err
		}

		b.Hyperdaemon.AddVm(vm)
	}
	if vm.Status == types.S_VM_IDLE {
		code, cause, err = b.Hyperdaemon.StartPod(podId, "", b.Name, nil, false, false, types.VM_KEEP_AFTER_FINISH, []*hypervisor.TtyIO{})
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			b.Hyperdaemon.KillVm(b.Name)
			return err
		}
		vm = b.Hyperdaemon.VmList[b.Name]
		// wait for cmd finish
		Status, err := vm.GetResponseChan()
		if err != nil {
			glog.Error(err.Error())
			return err
		}
		defer vm.ReleaseResponseChan(Status)
		var vmResponse *types.VmResponse
		for {
			vmResponse = <-Status
			if vmResponse.VmId == b.Name {
				if vmResponse.Code == types.E_POD_FINISHED {
					glog.Infof("Got E_POD_FINISHED code response")
					break
				}
			}
		}

		pod, ok := b.Hyperdaemon.PodList.Get(podId)
		if !ok {
			return fmt.Errorf("Cannot find pod %s", podId)
		}
		pod.SetVM(b.Name, vm)
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

func (b *Builder) clearTmp() {
	for p := range b.tmpPods {
		b.Hyperdaemon.CleanPod(p)
		delete(b.tmpPods, p)
		fmt.Fprintf(b.Stdout, "Removing intermediate pod %s\n", p)
	}
}

// readDockerfile reads a Dockerfile from the current context.
func (b *Builder) readDockerfile() error {
	// If no -f was specified then look for 'Dockerfile'. If we can't find
	// that then look for 'dockerfile'.  If neither are found then default
	// back to 'Dockerfile' and use that in the error message.
	if b.DockerfileName == "" {
		b.DockerfileName = api.DefaultDockerfileName
		if _, _, err := b.context.Stat(b.DockerfileName); os.IsNotExist(err) {
			lowercase := strings.ToLower(b.DockerfileName)
			if _, _, err := b.context.Stat(lowercase); err == nil {
				b.DockerfileName = lowercase
			}
		}
	}

	f, err := b.context.Open(b.DockerfileName)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Cannot locate specified Dockerfile: %s", b.DockerfileName)
		}
		return err
	}
	if f, ok := f.(*os.File); ok {
		// ignoring error because Open already succeeded
		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("Unexpected error reading Dockerfile: %v", err)
		}
		if fi.Size() == 0 {
			return fmt.Errorf("The Dockerfile (%s) cannot be empty", b.DockerfileName)
		}
	}
	b.dockerfile, err = parser.Parse(f)
	f.Close()
	if err != nil {
		return err
	}

	// After the Dockerfile has been parsed, we need to check the .dockerignore
	// file for either "Dockerfile" or ".dockerignore", and if either are
	// present then erase them from the build context. These files should never
	// have been sent from the client but we did send them to make sure that
	// we had the Dockerfile to actually parse, and then we also need the
	// .dockerignore file to know whether either file should be removed.
	// Note that this assumes the Dockerfile has been read into memory and
	// is now safe to be removed.
	if dockerIgnore, ok := b.context.(builder.DockerIgnoreContext); ok {
		dockerIgnore.Process([]string{b.DockerfileName})
	}
	return nil
}

// determine if build arg is part of built-in args or user
// defined args in Dockerfile at any point in time.
func (b *Builder) isBuildArgAllowed(arg string) bool {
	if _, ok := BuiltinAllowedBuildArgs[arg]; ok {
		return true
	}
	if _, ok := b.allowedBuildArgs[arg]; ok {
		return true
	}
	return false
}
