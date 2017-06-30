// package factory defines the full function factory interface
// package base defines the base factory interface
// package cache direct and template implement base.Factory
// package single and multi implement fatory.Factory
package factory

import (
	"encoding/json"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory/base"
	"github.com/hyperhq/runv/factory/cache"
	"github.com/hyperhq/runv/factory/direct"
	"github.com/hyperhq/runv/factory/multi"
	"github.com/hyperhq/runv/factory/single"
	"github.com/hyperhq/runv/factory/template"
	"github.com/hyperhq/runv/hypervisor"
)

type Factory interface {
	GetVm(cpu, mem int) (*hypervisor.Vm, error)
	CloseFactory()
}

type FactoryConfig struct {
	Cache    int  `json:"cache"`
	Template bool `json:"template"`
	Cpu      int  `json:"cpu"`
	Memory   int  `json:"memory"`
}

func NewFromConfigs(bootConfig hypervisor.BootConfig, configs []FactoryConfig) Factory {
	bases := make([]base.Factory, len(configs))
	for i, c := range configs {
		var b base.Factory
		boot := bootConfig
		boot.CPU = c.Cpu
		boot.Memory = c.Memory
		if c.Template {
			b = template.New(filepath.Join(hypervisor.BaseDir, "template"), boot)
		} else {
			b = direct.New(boot)
		}
		bases[i] = cache.New(c.Cache, b)
	}

	if len(bases) == 0 {
		return single.Dummy(bootConfig)
	} else if len(bases) == 1 {
		return single.New(bases[0])
	} else {
		return multi.New(bases)
	}
}

// vmFactoryPolicy = [FactoryConfig,]*FactoryConfig
// FactoryConfig   = {["cache":NUMBER,]["template":true|false,]"cpu":NUMBER,"memory":NUMBER}
func NewFromPolicy(bootConfig hypervisor.BootConfig, policy string) Factory {
	var configs []FactoryConfig
	jsonString := "[" + policy + "]"
	err := json.Unmarshal([]byte(jsonString), &configs)
	if err != nil && policy != "none" {
		glog.Errorf("Incorrect policy: %s", policy)
	}
	return NewFromConfigs(bootConfig, configs)
}
