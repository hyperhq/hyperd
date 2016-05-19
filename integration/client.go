package integration

import (
	"io"
	"time"

	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// HyperClient is the gRPC client for hyperd
type HyperClient struct {
	client types.PublicAPIClient
	ctx    context.Context
}

// NewHyperClient creates a new *HyperClient
func NewHyperClient(server string) (*HyperClient, error) {
	conn, err := grpc.Dial(server, grpc.WithInsecure(), grpc.WithTimeout(5*time.Second))
	if err != nil {
		return nil, err
	}

	return &HyperClient{
		client: types.NewPublicAPIClient(conn),
		ctx:    context.Background(),
	}, nil
}

// GetPodInfo gets pod info by podID
func (c *HyperClient) GetPodInfo(podID string) (*types.PodInfo, error) {
	request := types.PodInfoRequest{
		PodID: podID,
	}
	pod, err := c.client.PodInfo(c.ctx, &request)
	if err != nil {
		return nil, err
	}

	return pod.PodInfo, nil
}

// GetPodList get a list of Pods
func (c *HyperClient) GetPodList() ([]*types.PodListResult, error) {
	request := types.PodListRequest{}
	podList, err := c.client.PodList(
		c.ctx,
		&request,
	)
	if err != nil {
		return nil, err
	}

	return podList.PodList, nil
}

// GetVMList gets a list of VMs
func (c *HyperClient) GetVMList() ([]*types.VMListResult, error) {
	req := types.VMListRequest{}
	vmList, err := c.client.VMList(
		c.ctx,
		&req,
	)
	if err != nil {
		return nil, err
	}

	return vmList.VmList, nil
}

// GetContainerList gets a list of containers
func (c *HyperClient) GetContainerList(auxiliary bool) ([]*types.ContainerListResult, error) {
	req := types.ContainerListRequest{
		Auxiliary: auxiliary,
	}
	containerList, err := c.client.ContainerList(
		c.ctx,
		&req,
	)
	if err != nil {
		return nil, err
	}

	return containerList.ContainerList, nil
}

// GetContainerInfo gets container info by container name or id
func (c *HyperClient) GetContainerInfo(container string) (*types.ContainerInfo, error) {
	req := types.ContainerInfoRequest{
		Container: container,
	}
	cinfo, err := c.client.ContainerInfo(
		c.ctx,
		&req,
	)
	if err != nil {
		return nil, err
	}

	return cinfo.ContainerInfo, nil
}

// GetContainerLogs gets container log by container name or id
func (c *HyperClient) GetContainerLogs(container string) ([]byte, error) {
	req := types.ContainerLogsRequest{
		Container:  container,
		Follow:     false,
		Timestamps: false,
		Tail:       "",
		Since:      "",
		Stdout:     true,
		Stderr:     true,
	}
	stream, err := c.client.ContainerLogs(
		c.ctx,
		&req,
	)
	if err != nil {
		return nil, err
	}

	ret := []byte{}
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			if req.Follow == true {
				continue
			}
			break
		}
		if err != nil {
			return nil, err
		}
		ret = append(ret, res.Log...)
	}

	return ret, nil
}

// GetImageList gets a list of images
func (c *HyperClient) GetImageList() ([]*types.ImageInfo, error) {
	req := types.ImageListRequest{}
	imageList, err := c.client.ImageList(
		c.ctx,
		&req,
	)
	if err != nil {
		return nil, err
	}

	return imageList.ImageList, nil
}

// CreateVM creates a new VM
func (c *HyperClient) CreateVM(cpu, memory int32) (string, error) {
	req := types.VMCreateRequest{
		Cpu:    cpu,
		Memory: memory,
	}
	vm, err := c.client.VMCreate(
		c.ctx,
		&req,
	)
	if err != nil {
		return "", err
	}

	return vm.VmID, nil
}

// RemoveVM removes a vm by id
func (c *HyperClient) RemoveVM(vmID string) (*types.VMRemoveResponse, error) {
	req := types.VMRemoveRequest{
		VmID: vmID,
	}
	resp, err := c.client.VMRemove(
		c.ctx,
		&req,
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// CreatePod creates a pod
func (c *HyperClient) CreatePod(spec *types.UserPod) (string, error) {
	req := types.PodCreateRequest{
		PodSpec: spec,
	}
	resp, err := c.client.PodCreate(
		c.ctx,
		&req,
	)
	if err != nil {
		return "", err
	}

	return resp.PodID, nil
}

// RemovePod removes a pod by podID
func (c *HyperClient) RemovePod(podID string) error {
	_, err := c.client.PodRemove(
		c.ctx,
		&types.PodRemoveRequest{PodID: podID},
	)

	if err != nil {
		return err
	}

	return nil
}
