package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ContainerList implements GET /list?item=container
func (s *ServerRPC) ContainerList(ctx context.Context, req *types.ContainerListRequest) (*types.ContainerListResponse, error) {
	glog.V(3).Infof("ContainerList with request %s", req.String())

	containerList, err := s.daemon.ListContainers(req.PodID, req.VmID, req.Auxiliary)
	if err != nil {
		glog.Errorf("ContainerList error: %v", err)
		return nil, err
	}

	result := make([]*types.ContainerListResult, 0, 1)
	for _, c := range containerList {
		result = append(result, &types.ContainerListResult{
			ContainerID:   c.Id,
			ContainerName: c.Name,
			PodID:         c.PodId,
			Status:        s.daemon.GetContainerStatus(c.Status),
		})
	}

	return &types.ContainerListResponse{
		ContainerList: result,
	}, nil
}

// PodList implements GET /list?item=pod
func (s *ServerRPC) PodList(ctx context.Context, req *types.PodListRequest) (*types.PodListResponse, error) {
	glog.V(3).Infof("PodList with request %s", req.String())

	result := make([]*types.PodListResult, 0, 1)
	podList, err := s.daemon.ListPods(req.PodID, req.VmID)
	if err != nil {
		glog.Errorf("PodList error: %v", err)
		return nil, err
	}

	for _, p := range podList {
		vmID := ""
		if p.VM != nil {
			vmID = p.VM.Id
		}

		result = append(result, &types.PodListResult{
			PodID:     p.Id,
			PodName:   p.Spec.Name,
			Labels:    p.Spec.Labels,
			CreatedAt: p.CreatedAt,
			VmID:      vmID,
			Status:    s.daemon.GetPodStatus(p.PodStatus.Status, p.Spec.Type),
		})
	}

	return &types.PodListResponse{
		PodList: result,
	}, nil
}

// VMList implements GET /list?item=vm
func (s *ServerRPC) VMList(ctx context.Context, req *types.VMListRequest) (*types.VMListResponse, error) {
	glog.V(3).Infof("VMList with request %s", req.String())

	result := make([]*types.VMListResult, 0, 1)
	vmList, err := s.daemon.ListVMs(req.PodID, req.VmID)
	if err != nil {
		glog.Errorf("VmList error: %v", err)
		return nil, err
	}

	for _, vm := range vmList {
		podID := ""
		if vm.Pod != nil {
			podID = vm.Pod.Id
		}

		result = append(result, &types.VMListResult{
			VmID:   vm.Id,
			PodID:  podID,
			Status: s.daemon.GetVMStatus(vm.Status),
		})
	}

	return &types.VMListResponse{
		VmList: result,
	}, nil
}
