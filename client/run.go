package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	gflag "github.com/jessevdk/go-flags"

	apitype "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/runv/lib/term"
)

// hyperctl run [OPTIONS] image [COMMAND] [ARGS...]
func (cli *HyperClient) HyperCmdRun(args ...string) error {
	var (
		parser *gflag.Parser
		opts   = &RunFlags{}
		err    error
	)
	parser = gflag.NewParser(opts, gflag.Default|gflag.IgnoreUnknown|gflag.PassAfterNonOption)
	parser.Usage = "run [OPTIONS] IMAGE [COMMAND] [ARG...]\n\nCreate and start a pod"
	args, err = parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	specjson, err := cli.ParseCommonOptions(&opts.CommonFlags, false, args...)
	if err != nil {
		return err
	}

	var (
		podId  string
		spec   apitype.UserPod
		code   int
		attach = opts.Attach
		tty    = false
		res    chan error
	)

	t1 := time.Now()

	err = json.Unmarshal([]byte(specjson), &spec)
	if err != nil {
		return err
	}

	podId, code, err = cli.client.CreatePod(&spec)
	if err != nil {
		if code == http.StatusNotFound {
			err = cli.PullImages(&spec)
			if err != nil {
				return err
			}
			podId, code, err = cli.client.CreatePod(&spec)
		}
		if err != nil {
			return err
		}
	}
	if opts.PodFile == "" {
		attach = !opts.Detach
	}
	if !attach {
		fmt.Printf("POD id is %s\n", podId)
	}

	if opts.Remove {
		defer func() {
			rmerr := cli.client.RmPod(podId)
			if rmerr != nil {
				fmt.Fprintf(cli.out, "failed to rm pod, %v\n", rmerr)
			}
		}()
	}

	if attach && len(spec.Containers) > 0 {
		res = make(chan error, 1)
		tty = spec.Tty || spec.Containers[0].Tty
		p, err := cli.client.GetPodInfo(podId)
		if err != nil {
			fmt.Fprintf(cli.err, "failed to get pod info: %v", err)
			return err
		}
		if tty {
			cli.monitorTtySize(p.Spec.Containers[0].ContainerID, "")
			oldState, err := term.SetRawTerminal(cli.inFd)
			if err != nil {
				return err
			}
			defer term.RestoreTerminal(cli.inFd, oldState)
		}
		cname := spec.Containers[0].Name

		go func() {
			res <- cli.client.Attach(cname, tty, cli.in, cli.out, cli.err)
		}()
	}

	err = cli.client.StartPod(podId)
	if err != nil {
		return err
	}

	if !attach {
		t2 := time.Now()
		fmt.Printf("Time to run a POD is %d ms\n", (t2.UnixNano()-t1.UnixNano())/1000000)
	}

	if res != nil {
		err = <-res
		// a container having tty may detach from the terminal, don't wait
		return cli.client.GetExitCode(spec.Containers[0].Name, "", !tty)
	}

	return nil
}

type CreateOptions struct {
	JsonBytes []byte
	PodId     string
	Remove    bool
}
