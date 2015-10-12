package client

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/pkg/timeutils"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdLogs(args ...string) error {
	var opts struct {
		Follow bool   `short:"f" long:"follow" default:"false" default-mask:"-" description:"Follow log output"`
		Since  string `long:"since" value-name:"\"\"" description:"Show logs since timestamp"`
		Times  bool   `short:"t" long:"timestamps" default:"false" default-mask:"-" description:"Show timestamps"`
		Tail   string `long:"tail" value-name:"\"all\"" description:"Number of lines to show from the end of the logs"`
	}

	var parser = gflag.NewParser(&opts, gflag.Default|gflag.IgnoreUnknown)
	parser.Usage = "logs CONTAINER [OPTIONS...]\n\nFetch the logs of a container"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	if len(args) <= 1 {
		return fmt.Errorf("%s ERROR: Can not accept the 'logs' command without argument!\n", os.Args[0])
	}

	v := url.Values{}
	v.Set("container", args[1])
	v.Set("stdout", "yes")
	v.Set("stderr", "yes")

	if opts.Since != "" {
		v.Set("since", timeutils.GetTimestamp(opts.Since, time.Now()))
	}

	if opts.Times {
		v.Set("timestamps", "yes")
	}

	if opts.Follow {
		v.Set("follow", "yes")
	}
	v.Set("tail", opts.Tail)

	headers := http.Header(make(map[string][]string))
	return cli.stream("GET", "/container/logs?"+v.Encode(), nil, cli.out, headers)
}
