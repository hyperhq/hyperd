package vbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/pkg/idtools"
	"github.com/golang/glog"
	hyperdaemon "github.com/hyperhq/hyperd/daemon"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/govbox"
)

type Driver struct {
	rootPath string
	baseVdi  string
	pullVm   string
	disks    map[string]bool
	uidMaps  []idtools.IDMap
	gidMaps  []idtools.IDMap
	daemon   *hyperdaemon.Daemon
}

var backingFs = "<unknown>"
var daemon *hyperdaemon.Daemon

func Register(d *hyperdaemon.Daemon) {
	daemon = d
	graphdriver.Register("vbox", Init)
}

func Init(root string, options []string, uidMaps, gidMaps []idtools.IDMap) (graphdriver.Driver, error) {
	if err := supportsVbox(); err != nil {
		return nil, graphdriver.ErrNotSupported
	}

	fsMagic, err := graphdriver.GetFSMagic(root)
	if err != nil {
		return nil, err
	}
	if fsName, ok := graphdriver.FsNames[fsMagic]; ok {
		backingFs = fsName
	}

	d := &Driver{
		rootPath: root,
	}

	paths := []string{
		"images",
		"diff",
		"layers",
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		if os.IsExist(err) {
			return d, nil
		}
		return nil, err
	}

	for _, p := range paths {
		if err := os.MkdirAll(path.Join(root, p), 0755); err != nil {
			return nil, err
		}
	}
	vdi := fmt.Sprintf("%s/images/base.vdi", root)
	if _, err := os.Stat(vdi); err != nil {
		glog.Error(err)
		return nil, err
	}
	d.baseVdi = vdi
	d.pullVm = "hyper-mac-pull-vm"
	d.disks = make(map[string]bool, 1024)
	d.uidMaps = uidMaps
	d.gidMaps = gidMaps

	return d, nil
}

// Return a nil error if the system does not support differencing
// image
func supportsVbox() error {
	_, err := exec.LookPath("vboxmanage")
	if err != nil {
		return graphdriver.ErrNotSupported
	}
	return nil
}

func (d *Driver) RootPath() string {
	return d.rootPath
}

func (d *Driver) BaseImage() string {
	return d.baseVdi
}

func (d *Driver) Setup() (err error) {
	var (
		vm        *hypervisor.Vm
		ids       []string
		parentIds []string
	)

	d.daemon = daemon
	vm, err = d.daemon.StartVm(d.pullVm, 1, 64, false)
	if err != nil {
		glog.Error(err)
		return err
	}
	defer func() {
		if err != nil {
			d.daemon.KillVm(vm.Id)
		}
	}()

	if err = d.daemon.WaitVmStart(vm); err != nil {
		glog.Error(err)
		return err
	}

	if err = virtualbox.RegisterDisk(d.pullVm, d.pullVm, d.BaseImage(), 4); err != nil {
		glog.Error(err)
		return err
	}
	ids, err = loadIds(path.Join(d.RootPath(), "layers"))
	if err != nil {
		return err
	}

	for _, id := range ids {
		if d.disks[id] == true {
			continue
		}
		parentIds, err = getParentIds(d.RootPath(), id)
		if err != nil {
			glog.Warningf(err.Error())
			continue
		}
		for _, cid := range parentIds {
			if d.disks[cid] == true {
				continue
			}
			d.Exists(cid)
			d.disks[cid] = true
		}
		d.disks[id] = true
	}

	return nil
}

func (d *Driver) String() string {
	return "vbox"
}

func (d *Driver) Status() [][2]string {
	status := [][2]string{
		{"Root Dir", d.RootPath()},
		{"Backing Filesystem", backingFs},
	}
	return status
}

// GetMetadata not implemented
func (a *Driver) GetMetadata(id string) (map[string]string, error) {
	return nil, nil
}

func (d *Driver) Exists(id string) bool {
	disk := fmt.Sprintf("%s/%s.vdi", path.Join(d.RootPath(), "images"), id)
	if _, err := os.Lstat(disk); err != nil {
		return false
	}
	if _, err := virtualbox.GetMediumUUID(disk); err != nil {
		if err := virtualbox.RegisterDisk(d.pullVm, d.pullVm, disk, 4); err != nil {
			return false
		}
	}
	return true
}

func (d *Driver) Create(id, parent, mountLabel string) error {
	if err := d.createDirsFor(id); err != nil {
		glog.Error(err)
		return err
	}

	// create the disk and mount it to id's diff dir
	if err := d.createDisk(id, parent); err != nil {
		glog.Error(err)
		return err
	}

	// Write the layers metadata
	f, err := os.Create(fmt.Sprintf("%s/%s", path.Join(d.RootPath(), "layers"), id))
	if err != nil {
		return err
	}
	defer f.Close()

	if parent != "" {
		ids, err := getParentIds(d.RootPath(), parent)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintln(f, parent); err != nil {
			return err
		}
		for _, i := range ids {
			if _, err := fmt.Fprintln(f, i); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Driver) createDirsFor(id string) error {
	paths := []string{
		"diff",
	}

	for _, p := range paths {
		if err := os.MkdirAll(path.Join(d.RootPath(), p, id), 0755); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) createDisk(id, parent string) error {
	// create a raw image
	if _, err := os.Stat(fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), id)); err == nil {
		return nil
	}
	var (
		parentDisk string = d.BaseImage()
		idDisk     string = fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), id)
	)
	if parent != "" {
		parentDisk = fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), parent)
	}
	params := fmt.Sprintf("vboxmanage createhd --filename %s --diffparent %s --format VDI", idDisk, parentDisk)
	cmd := exec.Command("/bin/sh", "-c", params)
	if output, err := cmd.CombinedOutput(); err != nil {
		glog.Warningf(string(output))
		if strings.Contains(string(output), "not found in the media registry") {
			if err := virtualbox.RegisterDisk(d.pullVm, d.pullVm, parentDisk, 4); err != nil {
				return err
			}
		}
	}
	os.Chmod(idDisk, 0755)
	params = fmt.Sprintf("vboxmanage closemedium %s", idDisk)
	cmd = exec.Command("/bin/sh", "-c", params)
	if output, err := cmd.CombinedOutput(); err != nil {
		glog.Error(err)
		return fmt.Errorf("error to run vboxmanage closemedium, %s", output)
	}
	return nil
}

func (d *Driver) Remove(id string) error {
	if !d.Exists(id) {
		return nil
	}

	vdi := fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), id)
	virtualbox.UnregisterDisk("", vdi)
	if err := os.RemoveAll(vdi); err != nil && !os.IsNotExist(err) {
		return err
	}

	diff := path.Join(d.RootPath(), "diff", id)
	if err := os.RemoveAll(diff); err != nil && !os.IsNotExist(err) {
		return err
	}
	mp := fmt.Sprintf("%s/layers/%s", d.RootPath(), id)
	if err := os.RemoveAll(mp); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (d *Driver) Get(id, mountLabel string) (string, error) {
	mnt := path.Join(d.RootPath(), "diff", id)
	if st, err := os.Stat(mnt); err != nil {
		return "", err
	} else if !st.IsDir() {
		return "", fmt.Errorf("%s: is not a directory", mnt)
	}
	return mnt, nil
}

func (d *Driver) Put(id string) error {
	return nil
}

func (d *Driver) Cleanup() error {
	m, err := virtualbox.GetMachine(d.pullVm)
	m.Poweroff()
	ids, err := loadIds(path.Join(d.RootPath(), "layers"))
	if err != nil {
		return err
	}

	for _, id := range ids {
		_ = id
	}
	return nil
}

func (d *Driver) VmMountLayer(id string) error {
	if daemon == nil {
		if err := d.Setup(); err != nil {
			return err
		}
	}

	var (
		diffSrc = fmt.Sprintf("%s/diff/%s", d.RootPath(), id)
		volDst  = fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), id)
	)
	podstring, err := MakeMountPod("mac-vm-disk-mount-layer", "puller:latest", id, diffSrc, volDst)
	if err != nil {
		return err
	}
	podId := fmt.Sprintf("pull-%s", utils.RandStr(10, "alpha"))
	vmId := fmt.Sprintf("%s-%s", d.pullVm, utils.RandStr(10, "alpha"))

	var podSpec apitypes.UserPod
	err = json.Unmarshal([]byte(podstring), &podSpec)
	if err != nil {
		return err
	}

	p, err := daemon.CreatePod(podId, &podSpec)
	if err != nil {
		glog.Errorf("can not create pod %s", podstring)
		return err
	}
	defer daemon.CleanPod(podId)

	vm, err := d.daemon.StartVm(vmId, 1, 64, false)
	if err != nil {
		glog.Error(err)
		return err
	}

	// wait for cmd finish
	Status, err := vm.GetResponseChan()
	if err != nil {
		d.daemon.KillVm(vmId)
		glog.Error(err)
		return err
	}
	defer vm.ReleaseResponseChan(Status)

	code, cause, err := daemon.StartInternal(p, vmId, nil, false, []*hypervisor.TtyIO{})
	if err != nil {
		d.daemon.KillVm(vmId)
		glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
		return err
	}

	var vmResponse *types.VmResponse
	for {
		vmResponse = <-Status
		if vmResponse.VmId == vmId {
			if vmResponse.Code == types.E_VM_SHUTDOWN {
				glog.Infof("vm sthudown")
				break
			}
		}
	}

	return nil
}
