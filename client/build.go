package client

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	rand "github.com/hyperhq/hyperd/utils"

	"github.com/docker/docker/api"
	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/pkg/symlink"
	"github.com/docker/docker/reference"
	gflag "github.com/jessevdk/go-flags"
)

// hyperctl build [OPTIONS] PATH
func (cli *HyperClient) HyperCmdBuild(args ...string) error {
	var opts struct {
		ImageName      string `long:"tag" short:"t" default:"" value-name:"\"\"" default-mask:"-" description:"Repository name (and optionally a tag) to be applied to the resulting image in case of success"`
		DockerfileName string `long:"file" short:"f" default:"" value-name:"\"\"" default-mask:"-" description:"Customized docker file"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "build [OPTIONS] PATH\n\nBuild a new image from the source code at PATH"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("%s: \"build\" requires a minimum of 1 argument, See 'hyperctl build --help'.", os.Args[0])
	}
	var (
		filename = ""
		context  archive.Archive
		name     = ""
	)
	root := args[0]
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

	f, err := os.Open(filepath.Join(root, ".dockerignore"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	defer f.Close()

	var excludes []string
	if err == nil {
		excludes, err = dockerignore.ReadAll(f)
		if err != nil {
			return err
		}
	}

	if err := ValidateContextDirectory(root, excludes); err != nil {
		return fmt.Errorf("Error checking context: '%s'.", err)
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

	if err := ValidateContextDirectory(root, excludes); err != nil {
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
		var progBuff io.Writer = cli.out

		progressOutput := streamformatter.NewStreamFormatter().NewProgressOutput(progBuff, true)

		body = progress.NewProgressReader(context, progressOutput, 0, "", "Sending build context to Docker daemon")
	}

	if opts.ImageName == "" {
		// set a image name
		name = rand.RandStr(10, "alphanum")
	} else {
		name = opts.ImageName
		if _, err := reference.ParseNamed(name); err != nil {
			return err
		}
	}
	output, ctype, err := cli.client.Build(name, context != nil, body)
	if err != nil {
		return err
	}
	return cli.readStreamOutput(output, ctype, false, cli.out, cli.err)
}

// validateContextDirectory checks if all the contents of the directory
// can be read and returns an error if some files can't be read
// symlinks which point to non-existing files don't trigger an error
func ValidateContextDirectory(srcPath string, excludes []string) error {
	contextRoot := filepath.Join(srcPath, ".")

	return filepath.Walk(contextRoot, func(filePath string, f os.FileInfo, err error) error {
		// skip this directory/file if it's not in the path, it won't get added to the context
		if relFilePath, err := filepath.Rel(contextRoot, filePath); err != nil {
			return err
		} else if skip, err := fileutils.Matches(relFilePath, excludes); err != nil {
			return err
		} else if skip {
			if f.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("can't stat '%s'", filePath)
			}
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		// skip checking if symlinks point to non-existing files, such symlinks can be useful
		// also skip named pipes, because they hanging on open
		if f.Mode()&(os.ModeSymlink|os.ModeNamedPipe) != 0 {
			return nil
		}

		if !f.IsDir() {
			currentFile, err := os.Open(filePath)
			if err != nil && os.IsPermission(err) {
				return fmt.Errorf("no permission to read from '%s'", filePath)
			}
			currentFile.Close()
		}
		return nil
	})
}
