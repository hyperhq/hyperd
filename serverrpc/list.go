package serverrpc

import (
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ContainerList implements GET /list?item=container
func (s *ServerRPC) ContainerList(ctx context.Context, req *types.ContainerListRequest) (*types.ContainerListResponse, error) {
	glog.V(3).Infof("ContainerList with request %s", req.String())

	containerList, err := s.daemon.List("container", req.PodID, req.VmID, req.Auxiliary)
	if err != nil {
		glog.Errorf("ContainerList error: %v", err)
		return nil, err
	}

	if _, ok := containerList["cData"]; !ok {
		return nil, nil
	}

	result := make([]*types.ContainerListResult, 0, 1)
	for _, c := range containerList["cData"] {
		cStrings := strings.Split(c, ":")
		if len(cStrings) != 4 {
			glog.Errorf("Parse container info %s error", c)
			continue
		}
		cID := cStrings[0]
		cName := cStrings[1]
		podID := cStrings[2]
		status := cStrings[3]
		result = append(result, &types.ContainerListResult{
			ContainerID:   cID,
			ContainerName: cName,
			PodID:         podID,
			Status:        status,
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
	podList, err := s.daemon.List("pod", req.PodID, req.VmID, false)
	if err != nil {
		glog.Errorf("PodList error: %v", err)
		return nil, err
	}

	if _, ok := podList["podData"]; !ok {
		return nil, nil
	}

	for _, c := range podList["podData"] {
		cStrings := strings.Split(c, ":")
		if len(cStrings) != 4 {
			glog.Errorf("Parse pod info %s error", c)
			continue
		}
		podID := cStrings[0]
		podName := cStrings[1]
		vmID := cStrings[2]
		status := cStrings[3]
		result = append(result, &types.PodListResult{
			PodID:   podID,
			PodName: podName,
			VmID:    vmID,
			Status:  status,
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
	vmList, err := s.daemon.List("vm", req.PodID, req.VmID, false)
	if err != nil {
		glog.Errorf("VmList error: %v", err)
		return nil, err
	}

	if _, ok := vmList["vmData"]; !ok {
		return nil, nil
	}

	for _, c := range vmList["vmData"] {
		cStrings := strings.Split(c, ":")
		if len(cStrings) != 3 {
			glog.Errorf("Parse vm info %s failed", c)
			continue
		}
		vmID := cStrings[0]
		podID := cStrings[1]
		status := cStrings[2]
		result = append(result, &types.VMListResult{
			VmID:   vmID,
			PodID:  podID,
			Status: status,
		})
	}

	return &types.VMListResponse{
		VmList: result,
	}, nil
}
