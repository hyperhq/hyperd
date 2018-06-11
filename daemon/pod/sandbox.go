package pod

import (
	"github.com/hyperhq/hypercontainer-utils/hlog"
	//	"github.com/hyperhq/runv/factory"
	//	"github.com/hyperhq/runv/hypervisor"
	apitypes "github.com/hyperhq/hyperd/types"
	vc "github.com/kata-containers/runtime/virtcontainers"
	//	"runtime"
	//	"strings"
)

const (
	defaultHypervisor = vc.QemuHypervisor
	defaultProxy      = vc.KataBuiltInProxyType
	defaultShim       = vc.KataBuiltInShimType
	defaultAgent      = vc.KataContainersAgent

	DefaultKernel = "/usr/share/kata-containers/vmlinuz.container"
	DefaultInitrd = "/usr/share/kata-containers/kata-containers-initrd.img"
	DefaultHyper  = "/usr/bin/qemu-lite-system-x86_64"
)

const (
	maxReleaseRetry = 3
)

func startSandbox(spec *apitypes.UserPod, kernel, initrd string) (sandbox *vc.Sandbox, err error) {
	var (
		DEFAULT_CPU = 1
		DEFAULT_MEM = 128
	)

	if spec.Resource.Vcpu <= 0 {
		spec.Resource.Vcpu = int32(DEFAULT_CPU)
	}
	if spec.Resource.Memory <= 0 {
		spec.Resource.Memory = int32(DEFAULT_MEM)
	}

	resource := vc.Resources{
		Memory: uint(spec.Resource.Memory),
	}

	if kernel == "" {
		kernel = DefaultKernel
	}
	if initrd == "" {
		initrd = DefaultInitrd
	}

	sandboxConfig := vc.SandboxConfig{
		ID:       spec.Id,
		Hostname: spec.Hostname,
		VMConfig: resource,

		HypervisorType: defaultHypervisor,
		HypervisorConfig: vc.HypervisorConfig{
			HypervisorPath: DefaultHyper,
			KernelPath:     kernel,
			InitrdPath:     initrd,
		},

		AgentType:   defaultAgent,
		AgentConfig: vc.KataAgentConfig{},

		ProxyType:   defaultProxy,
		ProxyConfig: vc.ProxyConfig{},

		ShimType:   defaultShim,
		ShimConfig: vc.ShimConfig{},

		NetworkModel:  vc.CNMNetworkModel,
		NetworkConfig: vc.NetworkConfig{},
	}
	vcsandbox, err := vc.CreateSandbox(sandboxConfig)
	sandbox, _ = vcsandbox.(*vc.Sandbox)
	if err != nil {
		hlog.Log(ERROR, "failed to create a sandbox")
	}

	return sandbox, err
}

func dissociateSandbox(sandbox *vc.Sandbox, retry int) error {
	if sandbox == nil {
		return nil
	}

	err := sandbox.Release()
	if err != nil {
		hlog.Log(WARNING, "SB[%s] failed to release sandbox: %v", sandbox.ID(), err)
		hlog.Log(INFO, "SB[%s] shutdown because of failed release", sandbox.ID())
		sandbox.Delete()
		_, err = vc.StopSandbox(sandbox.ID())
		return err
	}
	return nil
}
