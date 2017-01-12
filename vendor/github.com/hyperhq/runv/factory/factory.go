// package factory defines the full function factory interface
// package base defines the base factory interface
// package cache direct and template implement base.Factory
// package single and multi implement fatory.Factory
package factory

import (
	"encoding/json"
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

func NewFromConfigs(kernel, initrd string, configs []FactoryConfig) Factory {
	bases := make([]base.Factory, len(configs))
	for i, c := range configs {
		var b base.Factory
		if c.Template {
			b = template.New(hypervisor.BaseDir+"/template", c.Cpu, c.Memory, kernel, initrd)
		} else {
			b = direct.New(c.Cpu, c.Memory, kernel, initrd)
		}
		bases[i] = cache.New(c.Cache, b)
	}

	if len(bases) == 0 {
		// skip GetVm from the base factory
		return single.New(direct.New(1000000, 1000000, kernel, initrd))
	} else if len(bases) == 1 {
		return single.New(bases[0])
	} else {
		return multi.New(bases)
	}
}

// vmFactoryPolicy = [FactoryConfig,]*FactoryConfig
// FactoryConfig   = {["cache":NUMBER,]["template":true|false,]"cpu":NUMBER,"memory":NUMBER}
func NewFromPolicy(kernel, initrd string, policy string) Factory {
	var configs []FactoryConfig
	jsonString := "[" + policy + "]"
	err := json.Unmarshal([]byte(jsonString), &configs)
	if err != nil && policy != "none" {
		glog.Errorf("Incorrect policy: %s", policy)
	}
	return NewFromConfigs(kernel, initrd, configs)
}
