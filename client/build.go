package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hyperhq/hyper/lib/docker/api"
	"github.com/hyperhq/hyper/lib/docker/graph/tags"
	"github.com/hyperhq/hyper/lib/docker/pkg/archive"
	"github.com/hyperhq/hyper/lib/docker/pkg/fileutils"
	"github.com/hyperhq/hyper/lib/docker/pkg/parsers"
	"github.com/hyperhq/hyper/lib/docker/pkg/progressreader"
	"github.com/hyperhq/hyper/lib/docker/pkg/streamformatter"
	"github.com/hyperhq/hyper/lib/docker/pkg/symlink"
	"github.com/hyperhq/hyper/lib/docker/registry"
	"github.com/hyperhq/hyper/lib/docker/utils"
	rand "github.com/hyperhq/hyper/utils"

	gflag "github.com/jessevdk/go-flags"
)

// hyper build [OPTIONS] PATH
func (cli *HyperClient) HyperCmdBuild(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s ERROR: Can not accept the 'run' command without argument!\n", os.Args[0])
	}
	var opts struct {
		ImageName      string `long:"tag" short:"t" default:"" value-name:"\"\"" default-mask:"-" description:"Repository name (and optionally a tag) to be applied to the resulting image in case of success"`
		DockerfileName string `long:"file" short:"f" default:"" value-name:"\"\"" default-mask:"-" description:"Customized docker file"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "build [OPTIONS] PATH\n\nBuild a new image from the source code at PATH"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("%s: \"build\" requires a minimum of 1 argument, See 'hyper build --help'.", os.Args[0])
	}
	var (
		filename = ""
		context  archive.Archive
		name     = ""
	)
	root := args[1]
	if _, err := os.Stat(root); err != nil {
		return err
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	filename = opts.DockerfileName // path to Dockerfile

	if opts.DockerfileName == "" {
		// No -f/--file was specified so use the default
		opts.DockerfileName = api.DefaultDockerfileName
		filename = filepath.Join(absRoot, opts.DockerfileName)

		// Just to be nice ;-) look for 'dockerfile' too but only
		// use it if we found it, otherwise ignore this check
		if _, err = os.Lstat(filename); os.IsNotExist(err) {
			tmpFN := path.Join(absRoot, strings.ToLower(opts.DockerfileName))
			if _, err = os.Lstat(tmpFN); err == nil {
				opts.DockerfileName = strings.ToLower(opts.DockerfileName)
				filename = tmpFN
			}
		}
	}

	origDockerfile := opts.DockerfileName // used for error msg
	if filename, err = filepath.Abs(filename); err != nil {
		return err
	}

	// Verify that 'filename' is within the build context
	filename, err = symlink.FollowSymlinkInScope(filename, absRoot)
	if err != nil {
		return fmt.Errorf("The Dockerfile (%s) must be within the build context (%s)", origDockerfile, root)
	}

	// Now reset the dockerfileName to be relative to the build context
	opts.DockerfileName, err = filepath.Rel(absRoot, filename)
	if err != nil {
		return err
	}
	// And canonicalize dockerfile name to a platform-independent one
	opts.DockerfileName, err = archive.CanonicalTarNameForPath(opts.DockerfileName)
	if err != nil {
		return fmt.Errorf("Cannot canonicalize dockerfile path %s: %v", opts.DockerfileName, err)
	}

	if _, err = os.Lstat(filename); os.IsNotExist(err) {
		return fmt.Errorf("Cannot locate Dockerfile: %s", origDockerfile)
	}
	var includes = []string{"."}

	excludes, err := utils.ReadDockerIgnore(path.Join(root, ".dockerignore"))
	if err != nil {
		return err
	}

	// If .dockerignore mentions .dockerignore or the Dockerfile
	// then make sure we send both files over to the daemon
	// because Dockerfile is, obviously, needed no matter what, and
	// .dockerignore is needed to know if either one needs to be
	// removed.  The deamon will remove them for us, if needed, after it
	// parses the Dockerfile.
	keepThem1, _ := fileutils.Matches(".dockerignore", excludes)
	keepThem2, _ := fileutils.Matches(opts.DockerfileName, excludes)
	if keepThem1 || keepThem2 {
		includes = append(includes, ".dockerignore", opts.DockerfileName)
	}

	if err := utils.ValidateContextDirectory(root, excludes); err != nil {
		return fmt.Errorf("Error checking context: '%s'.", err)
	}
	options := &archive.TarOptions{
		Compression:     archive.Uncompressed,
		ExcludePatterns: excludes,
		IncludeFiles:    includes,
	}
	context, err = archive.TarWithOptions(root, options)
	if err != nil {
		return err
	}
	var body io.Reader
	// Setup an upload progress bar
	// FIXME: ProgressReader shouldn't be this annoying to use
	if context != nil {
		sf := streamformatter.NewStreamFormatter()
		body = progressreader.New(progressreader.Config{
			In:        context,
			Out:       os.Stdout,
			Formatter: sf,
			NewLines:  true,
			ID:        "",
			Action:    "Sending build context to Docker daemon",
		})
	}

	if opts.ImageName == "" {
		// set a image name
		name = rand.RandStr(10, "alphanum")
	} else {
		name = opts.ImageName
		repository, tag := parsers.ParseRepositoryTag(name)
		if err := registry.ValidateRepositoryName(repository); err != nil {
			return err
		}
		if len(tag) > 0 {
			if err := tags.ValidateTagName(tag); err != nil {
				return err
			}
		}
	}
	v := url.Values{}
	v.Set("name", name)
	headers := http.Header(make(map[string][]string))
	if context != nil {
		headers.Set("Content-Type", "application/tar")
	}
	err = cli.stream("POST", "/image/build?"+v.Encode(), body, cli.out, headers)
	if err != nil {
		return err
	}
	return nil
}
