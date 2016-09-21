package integration

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/hyperhq/hyperd/lib/promise"
	"github.com/hyperhq/hyperd/types"

	"github.com/docker/docker/pkg/stdcopy"
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

type StreamExtractor interface {
	Extract(orig []byte) ([]byte, []byte, error)
}

type RawExtractor struct{}

const (
	// Stdin represents standard input stream type.
	Stdin stdcopy.StdType = iota
	// Stdout represents standard output stream type.
	Stdout
	// Stderr represents standard error steam type.
	Stderr

	stdWriterPrefixLen = 8
	stdWriterFdIndex   = 0
	stdWriterSizeIndex = 4
)

type StdcopyExtractor struct {
	readingHead bool
	current     stdcopy.StdType
	remain      int

	headbuf []byte
	headlen int
}

func NewExtractor(tty bool) StreamExtractor {
	if tty {
		return &RawExtractor{}
	}
	return &StdcopyExtractor{
		readingHead: true,
		headbuf:     make([]byte, stdWriterPrefixLen),
	}
}

func (r *RawExtractor) Extract(orig []byte) ([]byte, []byte, error) {
	return orig, nil, nil
}

func (s *StdcopyExtractor) Extract(orig []byte) ([]byte, []byte, error) {
	var (
		stdout = []byte{}
		stderr = []byte{}
	)
	for len(orig) > 0 {
		if s.readingHead {
			hrl := stdWriterPrefixLen - s.headlen //hrl -- head remain length
			if len(orig) < hrl {
				copy(s.headbuf[s.headlen:], orig)
				s.headlen += len(orig)
				return stdout, stderr, nil
			}

			copy(s.headbuf[s.headlen:], orig[:hrl])
			orig = orig[hrl:]
			s.headlen = 0

			stype := stdcopy.StdType(s.headbuf[stdWriterFdIndex])
			if stype != Stdout && stype != Stderr {
				return stdout, stderr, fmt.Errorf("invalid stream type %x", stype)
			}

			s.current = stype
			s.remain = int(binary.BigEndian.Uint32(s.headbuf[stdWriterSizeIndex : stdWriterSizeIndex+4]))
			s.readingHead = false
		}

		var (
			msg []byte
			ml  int
		)
		if len(orig) < s.remain {
			s.remain -= len(orig)
			ml = len(orig)
		} else {
			ml = s.remain
			s.readingHead = true
			s.remain = 0
		}

		msg = orig[:ml]
		orig = orig[ml:]

		switch s.current {
		case Stdout:
			stdout = append(stdout, msg...)
		case Stderr:
			stderr = append(stderr, msg...)
		}
	}
	return stdout, stderr, nil

}

// PostAttach attach to a container or pod by id
func (c *HyperClient) PostAttach(id string, tty bool) error {
	stream, err := c.client.Attach(c.ctx)
	if err != nil {
		return err
	}

	extractor := NewExtractor(tty)

	req := types.AttachMessage{
		ContainerID: id,
	}
	if err := stream.Send(&req); err != nil {
		return err
	}

	cmd := types.AttachMessage{
		Data: []byte("echo Hello Hyper\n"),
	}
	if err := stream.Send(&cmd); err != nil {
		return err
	}

	res, err := stream.Recv()
	if err != nil {
		return err
	}

	out, _, err := extractor.Extract(res.Data)
	if err != nil {
		return err
	}

	if string(out) != "Hello Hyper\n" {
		return fmt.Errorf("post attach response error\n")
	}

	return nil
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

// CreateContainer creates a container
func (c *HyperClient) CreateContainer(podID string, spec *types.UserContainer) (string, error) {
	req := types.ContainerCreateRequest{
		PodID:         podID,
		ContainerSpec: spec,
	}
	resp, err := c.client.ContainerCreate(c.ctx, &req)
	if err != nil {
		return "", err
	}

	return resp.ContainerID, nil
}

// RenameContainer renames a container
func (c *HyperClient) RenameContainer(oldName string, newName string) error {
	req := types.ContainerRenameRequest{
		OldContainerName: oldName,
		NewContainerName: newName,
	}
	_, err := c.client.ContainerRename(c.ctx, &req)

	if err != nil {
		return err
	}

	return nil
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

// ContainerExecCreate creates exec in a container
func (c *HyperClient) ContainerExecCreate(container string, command []string, tty bool) (string, error) {
	req := types.ExecCreateRequest{
		ContainerID: container,
		Command:     command,
		Tty:         tty,
	}
	resp, err := c.client.ExecCreate(c.ctx, &req)
	if err != nil {
		return "", err
	}

	return resp.ExecID, nil
}

// ContainerExecStart starts exec in a container with input stream in and output stream out
func (c *HyperClient) ContainerExecStart(containerId, execId string, stdin io.ReadCloser, stdout, stderr io.Writer, tty bool) error {
	request := types.ExecStartRequest{
		ContainerID: containerId,
		ExecID:      execId,
	}
	stream, err := c.client.ExecStart(context.Background())
	if err != nil {
		return err
	}
	if err := stream.Send(&request); err != nil {
		return err
	}
	extractor := NewExtractor(tty)
	var recvStdoutError chan error
	if stdout != nil || stderr != nil {
		recvStdoutError = promise.Go(func() (err error) {
			for {
				in, err := stream.Recv()
				if err != nil && err != io.EOF {
					return err
				}
				if in != nil && in.Stdout != nil {
					so, se, ee := extractor.Extract(in.Stdout)
					if ee != nil {
						return ee
					}
					if len(so) > 0 {
						nw, ew := stdout.Write(so)
						if ew != nil {
							return ew
						}
						if nw != len(so) {
							return io.ErrShortWrite
						}
					}
					if len(se) > 0 {
						nw, ew := stdout.Write(se)
						if ew != nil {
							return ew
						}
						if nw != len(se) {
							return io.ErrShortWrite
						}

					}
				}
				if err == io.EOF {
					break
				}
			}
			return nil
		})
	}
	if stdin != nil {
		go func() error {
			defer stream.CloseSend()
			buf := make([]byte, 32)
			for {
				nr, err := stdin.Read(buf)
				if nr > 0 {
					if err := stream.Send(&types.ExecStartRequest{Stdin: buf[:nr]}); err != nil {
						return err
					}
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
			}
			return nil
		}()
	}
	if stdout != nil || stderr != nil {
		if err := <-recvStdoutError; err != nil {
			return err
		}
	}

	return nil
}

// StartPod starts a pod by podID
func (c *HyperClient) StartPod(podID, vmID string, attach bool) error {
	stream, err := c.client.PodStart(context.Background())
	if err != nil {
		return err
	}

	req := types.PodStartMessage{
		PodID:  podID,
		VmID:   vmID,
		Attach: attach,
	}
	if err := stream.Send(&req); err != nil {
		return err
	}

	if attach {
		if _, err := stream.Recv(); err != nil {
			return err
		}

		return nil
	}

	cmd := types.PodStartMessage{
		Data: []byte("ls\n"),
	}
	if err := stream.Send(&cmd); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}

	return nil
}

// StopPod stops a pod
func (c *HyperClient) StopPod(podID string) (int, string, error) {
	resp, err := c.client.PodStop(c.ctx, &types.PodStopRequest{
		PodID: podID,
	})
	if err != nil {
		return -1, "", err
	}

	return int(resp.Code), resp.Cause, nil
}

// PausePod pauses a pod
func (c *HyperClient) PausePod(podID string) error {
	_, err := c.client.PodPause(c.ctx, &types.PodPauseRequest{
		PodID: podID,
	})
	if err != nil {
		return err
	}

	return nil
}

// UnpausePod unpauses a pod
func (c *HyperClient) UnpausePod(podID string) error {
	_, err := c.client.PodUnpause(c.ctx, &types.PodUnpauseRequest{
		PodID: podID,
	})
	if err != nil {
		return err
	}

	return nil
}

// PodSignal sends a signal to all containers of specified pod
func (c *HyperClient) PodSignal(podID string, signal int64) error {
	_, err := c.client.PodSignal(c.ctx, &types.PodSignalRequest{
		PodID:  podID,
		Signal: signal,
	})
	if err != nil {
		return err
	}

	return nil
}

// Wait gets exitcode by container and processId
func (c *HyperClient) Wait(container, processId string, noHang bool) (int32, error) {
	request := types.WaitRequest{
		Container: container,
		ProcessId: processId,
		NoHang:    noHang,
	}
	response, err := c.client.Wait(c.ctx, &request)
	if err != nil {
		return -1, err
	}

	return response.ExitCode, nil
}

func (c *HyperClient) PullImage(image, tag string, out io.Writer) error {
	request := types.ImagePullRequest{
		Image: image,
		Tag:   tag,
	}

	stream, err := c.client.ImagePull(c.ctx, &request)
	if err != nil {
		return err
	}

	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if out != nil {
			n, err := out.Write(res.Data)
			if err != nil {
				return err
			}
			if n != len(res.Data) {
				return io.ErrShortWrite
			}
		}
	}

	return nil
}

func (c *HyperClient) RemoveImage(image string) error {
	_, err := c.client.ImageRemove(c.ctx, &types.ImageRemoveRequest{Image: image})
	return err
}

func (c *HyperClient) PushImage(repo, tag string, out io.Writer) error {
	request := types.ImagePushRequest{
		Repo: repo,
		Tag:  tag,
	}

	stream, err := c.client.ImagePush(c.ctx, &request)
	if err != nil {
		return err
	}

	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if out != nil {
			n, err := out.Write(res.Data)
			if err != nil {
				return err
			}
			if n != len(res.Data) {
				return io.ErrShortWrite
			}
		}
	}

	return nil
}

// DeleteService deletes user service by podID and service content
func (c *HyperClient) DeleteService(podID string, services []*types.UserService) error {
	_, err := c.client.ServiceDelete(
		c.ctx,
		&types.ServiceDelRequest{PodID: podID, Services: services},
	)

	if err != nil {
		return err
	}
	return nil
}

// ListService lists user services by podID
func (c *HyperClient) ListService(podID string) ([]*types.UserService, error) {
	resp, err := c.client.ServiceList(
		c.ctx,
		&types.ServiceListRequest{PodID: podID},
	)

	if err != nil {
		return nil, err
	}

	return resp.Services, nil
}

// GetPodStats get stats of Pod by podID
func (c *HyperClient) GetPodStats(podID string) (*types.PodStats, error) {
	statsResponse, err := c.client.PodStats(
		c.ctx,
		&types.PodStatsRequest{PodID: podID},
	)

	if err != nil {
		return nil, err
	}

	return statsResponse.PodStats, nil
}

// AddService adds user service by podID and service content
func (c *HyperClient) AddService(podID string, services []*types.UserService) error {
	_, err := c.client.ServiceAdd(
		c.ctx,
		&types.ServiceAddRequest{PodID: podID, Services: services},
	)

	if err != nil {
		return err
	}

	return nil
}

// UpdateService updates user service by podID and service content
func (c *HyperClient) UpdateService(podID string, services []*types.UserService) error {
	_, err := c.client.ServiceUpdate(
		c.ctx,
		&types.ServiceUpdateRequest{PodID: podID, Services: services},
	)
	if err != nil {
		return err
	}

	return nil
}

// SetPodLabels sets labels to Pod by podID
func (c *HyperClient) SetPodLabels(podID string, override bool, labels map[string]string) error {
	_, err := c.client.SetPodLabels(
		c.ctx,
		&types.PodLabelsRequest{PodID: podID, Override: override, Labels: labels},
	)

	if err != nil {
		return err
	}

	return nil
}

// Info gets system info of hyperd
func (c *HyperClient) Info() (*types.InfoResponse, error) {
	info, err := c.client.Info(
		c.ctx,
		&types.InfoRequest{},
	)

	if err != nil {
		return nil, err
	}

	return info, nil
}

// Ping checks if hyperd is running (returns 'OK' on success)
func (c *HyperClient) Ping() (*types.PingResponse, error) {
	resp, err := c.client.Ping(
		c.ctx,
		&types.PingRequest{},
	)

	if err != nil {
		return nil, err
	}

	return resp, nil
}
