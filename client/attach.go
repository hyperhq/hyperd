package client

import (
    "fmt"
    "io"
    "net/url"
    "strings"

    "hyper/lib/promise"
	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdAttach(args ...string) error {
	var parser = gflag.NewParser(nil, gflag.Default)
	parser.Usage = "attach CONTAINER\n\nattach to the tty of a specified container in a pod"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
    if len(args) == 1 {
        return fmt.Errorf("Can not accept the 'attach' command without Container ID!")
    }
    var (
        podName = args[1]
        hostname = ""
        tag = cli.GetTag()
        containerId = podName
    )

    v := url.Values{}
    if strings.Contains(podName, "pod-") {
        hostname, err = cli.GetPodInfo(podName)
        if err != nil {
            return err
        }
        if hostname == "" {
            return fmt.Errorf("The pod(%s) does not exist, please create it before exec!", podName)
        }
        containerId, err = cli.GetContainerByPod(podName)
        if err != nil {
            return err
        }
        v.Set("type", "container")
        v.Set("value", containerId)
    } else {
        v.Set("type", "container")
        v.Set("value", containerId)
    }
    v.Set("tag", tag)

    var (
        hijacked    = make(chan io.Closer)
        errCh       chan error
    )
    // Block the return until the chan gets closed
    defer func() {
        fmt.Printf("End of CmdExec(), Waiting for hijack to finish.\n")
        if _, ok := <-hijacked; ok {
            fmt.Printf("Hijack did not finish (chan still open)\n")
        }
    }()

    errCh = promise.Go(func() error {
        return cli.hijack("POST", "/attach?"+v.Encode(), true, cli.in, cli.out, cli.out, hijacked, nil, hostname)
    })

    if err := cli.monitorTtySize(podName, tag); err != nil {
        fmt.Printf("Monitor tty size fail for %s!\n", podName)
    }

    // Acknowledge the hijack before starting
    select {
    case closer := <-hijacked:
    // Make sure that hijack gets closed when returning. (result
    // in closing hijack chan and freeing server's goroutines.
        if closer != nil {
            defer closer.Close()
        }
    case err := <-errCh:
        if err != nil {
            fmt.Printf("Error hijack: %s", err.Error())
            return err
        }
    }

    if err := <-errCh; err != nil {
        fmt.Printf("Error hijack: %s", err.Error())
        return err
    }
    fmt.Printf("Success to attach to pod(%s)\n", podName)
    return nil
}
