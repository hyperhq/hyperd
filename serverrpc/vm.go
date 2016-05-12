package serverrpc

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// VMCreate implements POST /vm/create
func (s *ServerRPC) VMCreate(ctx context.Context, req *types.VMCreateRequest) (*types.VMCreateResponse, error) {
	glog.V(3).Infof("VMCreate with request %s", req.String())

	if req.Cpu == 0 || req.Memory == 0 {
		return nil, fmt.Errorf("Cpu and memory is required for VMCreate")
	}

	vm, err := s.daemon.CreateVm(int(req.Cpu), int(req.Memory), false)
	if err != nil {
		glog.Errorf("CreateVm failed: %v", err)
		return nil, err
	}

	return &types.VMCreateResponse{
		VmID: vm.Id,
	}, nil
}

// VMRemove implements DELETE /vm
func (s *ServerRPC) VMRemove(ctx context.Context, req *types.VMRemoveRequest) (*types.VMRemoveResponse, error) {
	glog.V(3).Infof("VMRemove with request %s", req.String())

	if req.VmID == "" {
		return nil, fmt.Errorf("VmID is required for VMRemove")
	}

	code, cause, err := s.daemon.KillVm(req.VmID)
	if err != nil {
		glog.Errorf("KillVm %s failed: %v", req.VmID, err)
		return nil, err
	}

	return &types.VMRemoveResponse{
		Code:  int32(code),
		Cause: cause,
	}, nil
}
