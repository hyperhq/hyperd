// +build linux

package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/hyperhq/runv/hypervisor/types"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdReplace(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("\"replace\" requires a minimum of 1 argument, \"%s replace --help\" may provide more details.\n", os.Args[0])
	}

	var opts struct {
		OldPod  string `short:"o" long:"oldpod" value-name:"\"\"" description:"The Pod which will be replaced, must be 'running' status"`
		NewPod  string `short:"n" long:"newpod" value-name:"\"\"" description:"The Pod which will be running, must be 'pending' status"`
		PodFile string `short:"f" long:"file" value-name:"\"\"" description:"The Pod file is used to create a new POD and run"`
		Yaml    bool   `short:"y" long:"yaml" default:"false" default-mask:"-" description:"The Pod file is based on Yaml file"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)
	parser.Usage = "replace --oldpod POD_ID --newpod POD_ID [--file POD_FILE]\n\nReplace the pod in a running VM with a new one"
	args, err := parser.Parse()
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}
	oldPodId := opts.OldPod
	if oldPodId == "" {
		return fmt.Errorf("Please provide the old pod which you want to replace!")
	}
	newPodId := opts.NewPod
	newPodFile := opts.PodFile
	if newPodId == "" && newPodFile == "" {
		return fmt.Errorf("Please provide the new pod ID or pod file to replace the old pod %s", oldPodId)
	}

	vmId, err := cli.GetPodInfo(oldPodId)
	if err != nil {
		return err
	}
	// we need to stop the old pod, but leave the vm run
	code, cause, err := cli.StopPod(oldPodId, "no")
	if err != nil {
		return err
	}
	if code != types.E_POD_STOPPED {
		return fmt.Errorf("Error code is %d, cause is %s", code, cause)
	}
	if newPodId != "" {
		if _, err := cli.StartPod(newPodId, vmId, false); err != nil {
			return err
		}
		fmt.Printf("Successfully replaced the old pod(%s) with new pod(%s)\n", oldPodId, newPodId)
		return nil
	}
	if newPodFile != "" {
		jsonbody, err := cli.JsonFromFile(newPodFile, opts.Yaml, false)
		if err != nil {
			return err
		}

		newPodId, err := cli.CreatePod(jsonbody, false)
		if err != nil {
			return err
		}
		if _, err := cli.StartPod(newPodId, vmId, false); err != nil {
			return err
		}
		fmt.Printf("Successfully replaced the old pod(%s) with new pod file(%s)\n", oldPodId, newPodFile)
		return nil
	}
	return nil
}
