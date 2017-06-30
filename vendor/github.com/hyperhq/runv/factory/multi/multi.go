package multi

import (
	"sort"

	"github.com/hyperhq/runv/factory/base"
	"github.com/hyperhq/runv/factory/single"
	"github.com/hyperhq/runv/hypervisor"
)

type Factory []base.Factory

func (f Factory) GetVm(cpu, mem int) (*hypervisor.Vm, error) {
	for _, b := range f {
		config := b.Config()
		if config.CPU <= cpu && config.Memory <= mem {
			return single.New(b).GetVm(cpu, mem)
		}
	}
	boot := *f[0].Config()
	return single.Dummy(boot).GetVm(cpu, mem)
}

func (f Factory) CloseFactory() {
	for _, b := range f {
		b.CloseFactory()
	}
}

type sortingFactory []base.Factory

func (f sortingFactory) Len() int      { return len(f) }
func (f sortingFactory) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f sortingFactory) Less(i, j int) bool {
	ci, cj := f[i].Config(), f[j].Config()
	return ci.CPU < cj.CPU || (ci.CPU == cj.CPU && ci.Memory < cj.Memory)
}

func New(bases []base.Factory) Factory {
	sort.Sort(sort.Reverse(sortingFactory(bases)))
	return Factory(bases)
}
