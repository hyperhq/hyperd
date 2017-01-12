package qemu

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

//implement the hypervisor.HypervisorDriver interface
type QemuDriver struct {
	executable string
}

//implement the hypervisor.DriverContext interface
type QemuContext struct {
	driver      *QemuDriver
	qmp         chan QmpInteraction
	waitQmp     chan int
	wdt         chan string
	qmpSockName string
	qemuPidFile string
	qemuLogFile *QemuLogFile
	cpus        int
	process     *os.Process
}

func qemuContext(ctx *hypervisor.VmContext) *QemuContext {
	return ctx.DCtx.(*QemuContext)
}

func InitDriver() *QemuDriver {
	cmd, err := exec.LookPath(QEMU_SYSTEM_EXE)
	if err != nil {
		return nil
	}

	return &QemuDriver{
		executable: cmd,
	}
}

func (qd *QemuDriver) Name() string {
	return "qemu"
}

func (qd *QemuDriver) InitContext(homeDir string) hypervisor.DriverContext {
	if _, err := os.Stat(QemuLogDir); os.IsNotExist(err) {
		os.Mkdir(QemuLogDir, 0755)
	}

	logFile := QemuLogDir + "/" + homeDir[strings.Index(homeDir, "vm-"):len(homeDir)-1] + ".log"
	if _, err := os.Create(logFile); err != nil {
		glog.Errorf("create qemu log file failed: %v", err)
	}
	qemuLogFile := &QemuLogFile{
		Name:   logFile,
		Offset: 0,
	}

	return &QemuContext{
		driver:      qd,
		qmp:         make(chan QmpInteraction, 128),
		wdt:         make(chan string, 16),
		waitQmp:     make(chan int, 1),
		qmpSockName: homeDir + QmpSockName,
		qemuPidFile: homeDir + QemuPidFile,
		qemuLogFile: qemuLogFile,
		process:     nil,
	}
}

func (qd *QemuDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "qemu" {
		return nil, errors.New("wrong driver type in persist info")
	}

	var sock string
	var log QemuLogFile
	var proc *os.Process = nil
	var err error

	s, ok := persisted["qmpSock"]
	if !ok {
		return nil, errors.New("cannot read the qmp socket info from persist info")
	} else {
		switch s.(type) {
		case string:
			sock = s.(string)
		default:
			return nil, errors.New("wrong sock name type in persist info")
		}
	}

	p, ok := persisted["pid"]
	if !ok {
		return nil, errors.New("cannot read the pid info from persist info")
	} else {
		switch p.(type) {
		case float64:
			proc, err = os.FindProcess(int(p.(float64)))
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("wrong pid field type in persist info")
		}
	}

	l, ok := persisted["log"]
	if !ok {
		return nil, errors.New("cannot read the qemu log filename info from persist info")
	}
	if bytes, err := json.Marshal(l); err != nil {
		return nil, fmt.Errorf("wrong qemu log filename type in persist info: %v", err)
	} else if err = json.Unmarshal(bytes, &log); err != nil {
		return nil, fmt.Errorf("wrong qemu log filename type in persist info: %v", err)
	}

	return &QemuContext{
		driver:      qd,
		qmp:         make(chan QmpInteraction, 128),
		wdt:         make(chan string, 16),
		waitQmp:     make(chan int, 1),
		qmpSockName: sock,
		qemuLogFile: &log,
		process:     proc,
	}, nil
}

func (qc *QemuContext) Launch(ctx *hypervisor.VmContext) {
	go launchQemu(qc, ctx)
	go qmpHandler(ctx)
}

func (qc *QemuContext) Associate(ctx *hypervisor.VmContext) {
	go associateQemu(ctx)
	go qmpHandler(ctx)
}

func (qc *QemuContext) Dump() (map[string]interface{}, error) {
	if qc.process == nil {
		return nil, errors.New("can not serialize qemu context: no process running")
	}

	return map[string]interface{}{
		"hypervisor": "qemu",
		"qmpSock":    qc.qmpSockName,
		"log":        *qc.qemuLogFile,
		"pid":        qc.process.Pid,
	}, nil
}

func (qc *QemuContext) Shutdown(ctx *hypervisor.VmContext) {
	qmpQemuQuit(ctx, qc)
}

func (qc *QemuContext) Kill(ctx *hypervisor.VmContext) {
	defer func() {
		err := recover()
		if glog.V(1) && err != nil {
			glog.Info("kill qemu, but channel has already been closed")
		}
	}()
	qc.wdt <- "kill"
}

func (qc *QemuContext) Stats(ctx *hypervisor.VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (qc *QemuContext) Close() {
	qc.wdt <- "quit"
	<-qc.waitQmp
	qc.qemuLogFile.Close()
	close(qc.qmp)
	close(qc.wdt)
}

func (qc *QemuContext) Pause(ctx *hypervisor.VmContext, pause bool, result chan<- error) {
	commands := make([]*QmpCommand, 1)

	if pause {
		commands[0] = &QmpCommand{
			Execute: "stop",
		}
	} else {
		commands[0] = &QmpCommand{
			Execute: "cont",
		}
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: func(err error) {
			result <- err
		},
	}
}

func (qc *QemuContext) AddDisk(ctx *hypervisor.VmContext, sourceType string, blockInfo *hypervisor.DiskDescriptor, result chan<- hypervisor.VmEvent) {
	name := blockInfo.Name
	filename := blockInfo.Filename
	format := blockInfo.Format
	id := blockInfo.ScsiId

	if format == "rbd" {
		if blockInfo.Options != nil {
			keyring := blockInfo.Options["keyring"]
			user := blockInfo.Options["user"]
			if keyring != "" && user != "" {
				filename += ":id=" + user + ":key=" + keyring
			}

			monitors := blockInfo.Options["monitors"]
			for i, m := range strings.Split(monitors, ";") {
				monitor := strings.Replace(m, ":", "\\:", -1)
				if i == 0 {
					filename += ":mon_host=" + monitor
					continue
				}
				filename += ";" + monitor
			}
		}
	}

	newDiskAddSession(ctx, qc, name, sourceType, filename, format, id, result)
}

func (qc *QemuContext) RemoveDisk(ctx *hypervisor.VmContext, blockInfo *hypervisor.DiskDescriptor, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	id := blockInfo.ScsiId

	newDiskDelSession(ctx, qc, id, callback, result)
}

func (qc *QemuContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	newNetworkAddSession(ctx, qc, host.Id, host.Fd, guest.Device, host.Mac, guest.Index, guest.Busaddr, result)
}

func (qc *QemuContext) RemoveNic(ctx *hypervisor.VmContext, n *hypervisor.InterfaceCreated, callback hypervisor.VmEvent, result chan<- hypervisor.VmEvent) {
	newNetworkDelSession(ctx, qc, n.DeviceName, callback, result)
}

func (qc *QemuContext) SetCpus(ctx *hypervisor.VmContext, cpus int, result chan<- error) {
	currcpus := qc.cpus

	if cpus < currcpus {
		result <- fmt.Errorf("can't reduce cpus number from %d to %d", currcpus, cpus)
		return
	} else if cpus == currcpus {
		result <- nil
		return
	}

	commands := make([]*QmpCommand, cpus-currcpus)
	for id := currcpus; id < cpus; id++ {
		commands[id-currcpus] = &QmpCommand{
			Execute: "cpu-add",
			Arguments: map[string]interface{}{
				"id": id,
			},
		}
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: func(err error) {
			if err == nil {
				qc.cpus = cpus
			}
			result <- err
		},
	}
}

func (qc *QemuContext) AddMem(ctx *hypervisor.VmContext, slot, size int, result chan<- error) {
	commands := make([]*QmpCommand, 2)
	commands[0] = &QmpCommand{
		Execute: "object-add",
		Arguments: map[string]interface{}{
			"qom-type": "memory-backend-ram",
			"id":       "mem" + strconv.Itoa(slot),
			"props":    map[string]interface{}{"size": int64(size) << 20},
		},
	}
	commands[1] = &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "pc-dimm",
			"id":     "dimm" + strconv.Itoa(slot),
			"memdev": "mem" + strconv.Itoa(slot),
		},
	}
	qc.qmp <- &QmpSession{
		commands: commands,
		respond:  func(err error) { result <- err },
	}
}

func (qc *QemuContext) Save(ctx *hypervisor.VmContext, path string, result chan<- error) {
	commands := make([]*QmpCommand, 2)

	commands[0] = &QmpCommand{
		Execute: "migrate-set-capabilities",
		Arguments: map[string]interface{}{
			"capabilities": []map[string]interface{}{
				{
					"capability": "bypass-shared-memory",
					"state":      true,
				},
			},
		},
	}
	commands[1] = &QmpCommand{
		Execute: "migrate",
		Arguments: map[string]interface{}{
			"uri": fmt.Sprintf("exec:cat>%s", path),
		},
	}
	if !ctx.Boot.BootToBeTemplate {
		commands = commands[1:]
	}

	// TODO: use query-migrate to query until completed
	qc.qmp <- &QmpSession{
		commands: commands,
		respond:  func(err error) { result <- err },
	}
}

func (qc *QemuDriver) SupportLazyMode() bool {
	return false
}
