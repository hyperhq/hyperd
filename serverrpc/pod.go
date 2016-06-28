package serverrpc

import (
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	runvtypes "github.com/hyperhq/runv/hypervisor/types"
	"golang.org/x/net/context"
)

// PodCreate creates a pod by PodSpec
func (s *ServerRPC) PodCreate(ctx context.Context, req *types.PodCreateRequest) (*types.PodCreateResponse, error) {
	glog.V(3).Infof("PodCreate with request %s", req.String())

	pod, err := s.daemon.CreatePod(req.PodID, req.PodSpec)
	if err != nil {
		glog.Errorf("CreatePod failed: %v", err)
		return nil, err
	}

	return &types.PodCreateResponse{
		PodID: pod.Id,
	}, nil
}

// PodStart starts a pod by podID
func (s *ServerRPC) PodStart(stream types.PublicAPI_PodStartServer) error {
	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	glog.V(3).Infof("PodStart with request %s", req.String())

	if !req.Attach {
		_, _, err := s.daemon.StartPod(nil, nil, req.PodID, req.VmID, req.Attach)
		if err != nil {
			glog.Errorf("StartPod failed: %v", err)
			return err
		}

		// Send an empty message to client, so client could wait for start complete
		if err := stream.Send(&types.PodStartMessage{}); err != nil {
			return err
		}

		return nil
	}

	ir, iw := io.Pipe()
	or, ow := io.Pipe()

	go func() {
		for {
			cmd, err := stream.Recv()
			if err != nil {
				return
			}

			if _, err := iw.Write(cmd.Data); err != nil {
				return
			}
		}

	}()

	go func() {
		for {
			res := make([]byte, 512)
			n, err := or.Read(res)
			if err != nil {
				return
			}

			if err := stream.Send(&types.PodStartMessage{Data: res[:n]}); err != nil {
				return
			}
		}
	}()

	if _, _, err := s.daemon.StartPod(ir, ow, req.PodID, req.VmID, req.Attach); err != nil {
		glog.Errorf("StartPod failed: %v", err)
		return err
	}

	return nil
}

// PodRemove removes a pod by podID
func (s *ServerRPC) PodRemove(ctx context.Context, req *types.PodRemoveRequest) (*types.PodRemoveResponse, error) {
	glog.V(3).Infof("PodRemove with request %s", req.String())

	if req.PodID == "" {
		return nil, fmt.Errorf("PodID is required for PodRemove")
	}

	code, cause, err := s.daemon.CleanPod(req.PodID)
	if err != nil {
		glog.Errorf("CleanPod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodRemoveResponse{
		Cause: cause,
		Code:  int32(code),
	}, nil
}

// PodStop stops a pod
func (s *ServerRPC) PodStop(ctx context.Context, req *types.PodStopRequest) (*types.PodStopResponse, error) {
	glog.V(3).Infof("PodStop with request %s", req.String())

	code, cause, err := s.daemon.StopPod(req.PodID)
	if err != nil {
		glog.Errorf("StopPod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodStopResponse{
		Cause: cause,
		Code:  int32(code),
	}, nil
}

// PodSignal sends a singal to all containers of specified pod
func (s *ServerRPC) PodSignal(ctx context.Context, req *types.PodSignalRequest) (*types.PodSignalResponse, error) {
	glog.V(3).Infof("PodSignal with request %s", req.String())

	err := s.daemon.KillPodContainers(req.PodID, "", req.Signal)
	if err != nil {
		glog.Errorf("KillPodContainers %s with signal %d failed: %v", req.PodID, req.Signal, err)
		return nil, err
	}

	return &types.PodSignalResponse{}, nil
}

// PodPause pauses a pod
func (s *ServerRPC) PodPause(ctx context.Context, req *types.PodPauseRequest) (*types.PodPauseResponse, error) {
	glog.V(3).Infof("PodPause with request %s", req.String())

	err := s.daemon.PausePod(req.PodID)
	if err != nil {
		glog.Errorf("PausePod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodPauseResponse{}, nil
}

// PodUnpause unpauses a pod
func (s *ServerRPC) PodUnpause(ctx context.Context, req *types.PodUnpauseRequest) (*types.PodUnpauseResponse, error) {
	glog.V(3).Infof("PodUnpause with request %s", req.String())

	err := s.daemon.UnpausePod(req.PodID)
	if err != nil {
		glog.Errorf("UnpausePod %s failed: %v", req.PodID, err)
		return nil, err
	}

	return &types.PodUnpauseResponse{}, nil
}

// PodLabels sets the labels of Pod
func (s *ServerRPC) SetPodLabels(c context.Context, req *types.PodLabelsRequest) (*types.PodLabelsResponse, error) {
	glog.V(3).Infof("Set pod labels with request %v", req.String())

	err := s.daemon.SetPodLabels(req.PodID, req.Override, req.Labels)
	if err != nil {
		glog.Errorf("PodLabels error: %v", err)
		return nil, err
	}

	return &types.PodLabelsResponse{}, nil
}

// PodStats get stats (runvtypes.PodStats) of Pod
func (s *ServerRPC) PodStats(c context.Context, req *types.PodStatsRequest) (*types.PodStatsResponse, error) {
	glog.V(3).Infof("Get pod stats with request %v", req.String())

	statsObject, err := s.daemon.GetPodStats(req.PodID)
	if err != nil {
		glog.Errorf("PodStats error: %v", err)
		return nil, err
	}

	stats := statsObject.(*runvtypes.PodStats)

	return &types.PodStatsResponse{
		PodStats: convertRunvStatsToGrpcTypes(stats),
	}, nil
}

func convertRunvStatsToGrpcTypes(stats *runvtypes.PodStats) *types.PodStats {
	grpcPodStats := &types.PodStats{}
	grpcPodStats.Cpu = convertToGrpcCpuStats(stats.Cpu)
	grpcPodStats.Block = convertToGrpcBlockStats(stats.Block)
	grpcPodStats.Memory = convertToGrpcMemoryStats(stats.Memory)
	grpcPodStats.Network = convertToGrpcNetworkStats(stats.Network)
	grpcPodStats.Timestamp = stats.Timestamp.Unix()

	for _, fs := range stats.Filesystem {
		grpcPodStats.Filesystem = append(grpcPodStats.Filesystem, convertRunvFsToGrpcType(fs))
	}

	for _, cStats := range stats.ContainersStats {
		var containerStats *types.ContainersStats
		containerStats.ContainerID = cStats.ContainerID
		containerStats.Cpu = convertToGrpcCpuStats(cStats.Cpu)
		containerStats.Memory = convertToGrpcMemoryStats(cStats.Memory)
		containerStats.Block = convertToGrpcBlockStats(cStats.Block)
		containerStats.Network = convertToGrpcNetworkStats(cStats.Network)
		for _, fs := range cStats.Filesystem {
			containerStats.Filesystem = append(containerStats.Filesystem, convertRunvFsToGrpcType(fs))
		}
		containerStats.Timestamp = cStats.Timestamp.Unix()
		grpcPodStats.ContainersStats = append(grpcPodStats.ContainersStats, containerStats)
	}
	return grpcPodStats
}

func convertToGrpcCpuStats(stats runvtypes.CpuStats) *types.CpuStats {
	return &types.CpuStats{
		Usage: &types.CpuUsage{
			Total:  stats.Usage.Total,
			PerCpu: stats.Usage.PerCpu,
			User:   stats.Usage.User,
			System: stats.Usage.System,
		},
		LoadAverage: stats.LoadAverage,
	}
}

func convertToGrpcMemoryStats(stats runvtypes.MemoryStats) *types.MemoryStats {
	return &types.MemoryStats{
		Usage:      stats.Usage,
		WorkingSet: stats.WorkingSet,
		Failcnt:    stats.Failcnt,
		ContainerData: &types.MemoryStatsMemoryData{
			Pgfault:    stats.ContainerData.Pgfault,
			Pgmajfault: stats.ContainerData.Pgmajfault,
		},
		HierarchicalData: &types.MemoryStatsMemoryData{
			Pgfault:    stats.ContainerData.Pgfault,
			Pgmajfault: stats.ContainerData.Pgmajfault,
		},
	}
}

func convertToGrpcBlockStats(stats runvtypes.BlkioStats) *types.BlkioStats {
	return &types.BlkioStats{
		IoServiceBytesRecursive: covertToGrpcBlockEntry(stats.IoServiceBytesRecursive),
		IoServicedRecursive:     covertToGrpcBlockEntry(stats.IoServicedRecursive),
		IoQueuedRecursive:       covertToGrpcBlockEntry(stats.IoQueuedRecursive),
		IoServiceTimeRecursive:  covertToGrpcBlockEntry(stats.IoServiceTimeRecursive),
		IoWaitTimeRecursive:     covertToGrpcBlockEntry(stats.IoWaitTimeRecursive),
		IoMergedRecursive:       covertToGrpcBlockEntry(stats.IoMergedRecursive),
		IoTimeRecursive:         covertToGrpcBlockEntry(stats.IoTimeRecursive),
		SectorsRecursive:        covertToGrpcBlockEntry(stats.SectorsRecursive),
	}
}

func convertToGrpcNetworkStats(stats runvtypes.NetworkStats) *types.NetworkStats {
	return &types.NetworkStats{
		Interfaces: convertToGrpcInterfaceStats(stats.Interfaces),
		Tcp:        convertToGrpcTcpStats(stats.Tcp),
		Tcp6:       convertToGrpcTcpStats(stats.Tcp6),
	}
}

func convertToGrpcTcpStats(stats runvtypes.TcpStat) *types.TcpStat {
	return &types.TcpStat{
		Established: stats.Established,
		SynSent:     stats.SynSent,
		SynRecv:     stats.SynRecv,
		FinWait1:    stats.FinWait1,
		FinWait2:    stats.FinWait2,
		TimeWait:    stats.TimeWait,
		Close:       stats.Close,
		CloseWait:   stats.CloseWait,
		LastAck:     stats.LastAck,
		Listen:      stats.Listen,
		Closing:     stats.Closing,
	}
}

func convertToGrpcInterfaceStats(iStats []runvtypes.InterfaceStats) []*types.InterfaceStats {
	var result []*types.InterfaceStats
	for _, f := range iStats {
		item := &types.InterfaceStats{
			Name:      f.Name,
			RxBytes:   f.RxBytes,
			RxPackets: f.RxPackets,
			RxErrors:  f.RxErrors,
			RxDropped: f.RxDropped,
			TxBytes:   f.TxBytes,
			TxPackets: f.TxPackets,
			TxErrors:  f.TxErrors,
			TxDropped: f.TxDropped,
		}
		result = append(result, item)
	}

	return result
}

func covertToGrpcBlockEntry(bStats []runvtypes.BlkioStatEntry) []*types.BlkioStatEntry {
	var result []*types.BlkioStatEntry
	for _, b := range bStats {
		item := &types.BlkioStatEntry{
			Name:   b.Name,
			Type:   b.Type,
			Source: b.Source,
			Major:  b.Major,
			Minor:  b.Minor,
			Stat:   b.Stat,
		}
		result = append(result, item)
	}
	return result
}

func convertRunvFsToGrpcType(fs runvtypes.FsStats) *types.FsStats {
	var grpcFs *types.FsStats
	grpcFs.Device = fs.Device
	grpcFs.Limit = fs.Limit
	grpcFs.Usage = fs.Usage
	grpcFs.Available = fs.Available
	grpcFs.ReadsCompleted = fs.ReadsCompleted
	grpcFs.ReadsMerged = fs.ReadsMerged
	grpcFs.SectorsRead = fs.SectorsRead
	grpcFs.ReadTime = fs.ReadTime
	grpcFs.WritesCompleted = fs.WritesCompleted
	grpcFs.WritesMerged = fs.WritesMerged
	grpcFs.SectorsWritten = fs.SectorsWritten
	grpcFs.WriteTime = fs.WriteTime
	grpcFs.IoInProgress = fs.IoInProgress
	grpcFs.IoTime = fs.IoTime
	grpcFs.WeightedIoTime = fs.WeightedIoTime
	return grpcFs
}
