package template

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory/base"
	"github.com/hyperhq/runv/factory/direct"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/utils"
	"github.com/hyperhq/runv/template"
)

type templateFactory struct {
	s *template.TemplateVmConfig
}

func New(templateRoot string, b hypervisor.BootConfig) base.Factory {
	var vmName string

	for {
		vmName = fmt.Sprintf("template-vm-%s", utils.RandStr(10, "alpha"))
		if _, err := os.Stat(filepath.Join(templateRoot, vmName)); os.IsNotExist(err) {
			break
		}
	}
	s, err := template.CreateTemplateVM(filepath.Join(templateRoot, vmName), vmName, b)
	if err != nil {
		glog.Errorf("failed to create template factory: %v", err)
		glog.V(3).Infof("use direct factory instead")
		return direct.New(b)
	}
	return &templateFactory{s: s}
}

func NewFromExisted(s *template.TemplateVmConfig) base.Factory {
	return &templateFactory{s: s}
}

func (t *templateFactory) Config() *hypervisor.BootConfig {
	return t.s.BootConfigFromTemplate()
}

func (t *templateFactory) GetBaseVm() (*hypervisor.Vm, error) {
	return t.s.NewVmFromTemplate("")
}

func (t *templateFactory) CloseFactory() {
	t.s.Destroy()
}
