package template

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

// The TemplateVm will be booted, paused, saved, and killed. The TemplateVm
// is not existed any more but just the states left. The states includes two
// parts, the memory is StatePath/memory and devices states
// is StatePath/state
//
// New Vm can be booted from the saved TemplateVm states with all the initial
// memory is shared(copy-on-write) with the TemplateVm(statePath/memory)
//
// Phoenix rising from the ashes

type TemplateVmConfig struct {
	StatePath string `json:"statepath"`
	Driver    string `json:"driver"`
	Config    hypervisor.BootConfig
}

func CreateTemplateVM(statePath, vmName string, b hypervisor.BootConfig) (t *TemplateVmConfig, err error) {
	if b.BootToBeTemplate || b.BootFromTemplate || b.MemoryPath != "" || b.DevicesStatePath != "" {
		return nil, fmt.Errorf("Error boot config for template")
	}
	b.MemoryPath = statePath + "/memory"
	b.DevicesStatePath = statePath + "/state"

	config := &TemplateVmConfig{
		StatePath: statePath,
		Driver:    hypervisor.HDriver.Name(),
		Config:    b,
	}
	config.Config.BootFromTemplate = true

	defer func() {
		if err != nil {
			config.Destroy()
		}
	}()

	// prepare statePath
	if err := os.MkdirAll(statePath, 0700); err != nil {
		glog.Infof("create template state path failed: %v", err)
		return nil, err
	}
	flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV)
	opts := fmt.Sprintf("size=%dM", b.Memory+8)
	if err = syscall.Mount("tmpfs", statePath, "tmpfs", flags, opts); err != nil {
		glog.Infof("mount template state path failed: %v", err)
		return nil, err
	}
	if f, err := os.Create(statePath + "/memory"); err != nil {
		glog.Infof("create memory path failed: %v", err)
		return nil, err
	} else {
		f.Close()
	}

	// launch vm
	b.BootToBeTemplate = true
	vm, err := hypervisor.GetVm(vmName, &b, true)
	if err != nil {
		return nil, err
	}
	defer vm.Kill()

	// pasue and save devices state
	if err = vm.Pause(true); err != nil {
		glog.Infof("failed to pause template vm:%v", err)
		return nil, err
	}
	if err = vm.Save(statePath + "/state"); err != nil {
		glog.Infof("failed to save template vm states: %v", err)
		return nil, err
	}

	// TODO: qemu driver's qmp doesn't wait migration finish.
	// so we wait here. We should fix it in the qemu driver side.
	time.Sleep(1 * time.Second)

	configData, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return nil, err
	}
	configFile := filepath.Join(statePath, "config.json")
	err = ioutil.WriteFile(configFile, configData, 0644)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return nil, err
	}

	return config, nil
}

func (t *TemplateVmConfig) BootConfigFromTemplate() *hypervisor.BootConfig {
	b := t.Config
	return &b
}

// boot vm from template, the returned vm is paused
func (t *TemplateVmConfig) NewVmFromTemplate(vmName string) (*hypervisor.Vm, error) {
	return hypervisor.GetVm(vmName, t.BootConfigFromTemplate(), false)
}

func (t *TemplateVmConfig) Destroy() {
	for i := 0; i < 5; i++ {
		err := syscall.Unmount(t.StatePath, 0)
		if err != nil {
			glog.Infof("Failed to unmount the template state path: %v", err)
		} else {
			break
		}
		time.Sleep(time.Second) // TODO: only sleep&retry when unmount() returns EBUSY
	}
	err := os.Remove(t.StatePath)
	if err != nil {
		glog.Infof("Failed to remove the template state path: %v", err)
	}
}
