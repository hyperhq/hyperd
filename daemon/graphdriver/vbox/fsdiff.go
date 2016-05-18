package vbox

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/chrootarchive"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/golang/glog"
	apitypes "github.com/hyperhq/hyperd/types"
	"github.com/hyperhq/hyperd/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/govbox"
)

func (d *Driver) ApplyDiff(id, parent string, diff archive.Reader) (size int64, err error) {
	// Mount the root filesystem so we can apply the diff/layer.
	layerFs, err := d.Get(id, "")
	if err != nil {
		return 0, err
	}
	defer d.Put(id)

	start := time.Now().UTC()
	glog.V(1).Info("Start untar layer")
	if size, err = chrootarchive.ApplyLayer(layerFs, diff); err != nil {
		return 0, err
	}
	glog.V(1).Infof("Untar time: %vs", time.Now().UTC().Sub(start).Seconds())
	root := path.Join(utils.HYPER_ROOT, "vbox")
	idDisk := fmt.Sprintf("%s/images/%s.vdi", root, id)
	if _, err = os.Stat(idDisk); err != nil {
		return 0, err
	}
	if err = d.VmMountLayer(id); err != nil {
		return 0, err
	}
	// XXX should remove the image/container's directory
	return size, err
}

func (d *Driver) Diff(id, parent string) (diff archive.Archive, err error) {
	if d.daemon == nil {
		if err := d.Setup(); err != nil {
			return nil, err
		}
	}
	var (
		podData string
		tgtDisk string = ""
		code    int
		cause   string
	)
	srcDisk := fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), id)
	if parent != "" {
		tgtDisk = fmt.Sprintf("%s/images/%s.vdi", d.RootPath(), parent)
	}
	outDir := path.Join(utils.HYPER_ROOT, "tar")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, err
	}
	uuid, err := virtualbox.GetMediumUUID(srcDisk)
	if err == nil {
		srcDisk = uuid
	}
	// create pod
	podId := "diff-" + id[:10]
	podData, err = MakeDiffPod(podId, "puller:latest", id, srcDisk, tgtDisk, outDir)
	if err != nil {
		return nil, err
	}

	var podSpec apitypes.UserPod
	err = json.Unmarshal([]byte(podData), &podSpec)
	if err != nil {
		return nil, err
	}

	p, err := daemon.CreatePod(podId, &podSpec)
	if err != nil {
		glog.Errorf("can not create pod %s", podData)
		return nil, err
	}
	defer d.daemon.CleanPod(podId)

	// start vm
	vmId := fmt.Sprintf("%s-%s", d.pullVm, utils.RandStr(10, "alpha"))
	vm, err := d.daemon.StartVm(vmId, 1, 64, false)
	if err != nil {
		glog.Error(err)
		return nil, err
	}

	// wait for cmd finish
	Status, err := vm.GetResponseChan()
	if err != nil {
		glog.Error(err)
		d.daemon.KillVm(vmId)
		return nil, err
	}
	defer vm.ReleaseResponseChan(Status)

	code, cause, err = d.daemon.StartInternal(p, vmId, nil, false, []*hypervisor.TtyIO{})
	if err != nil {
		glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
		d.daemon.KillVm(vmId)
		return nil, err
	}

	var vmResponse *types.VmResponse
	for {
		vmResponse = <-Status
		if vmResponse.VmId == vmId {
			if vmResponse.Code == types.E_VM_SHUTDOWN {
				glog.Infof("vm shutdown")
				break
			}
		}
	}

	tarFile := outDir + "/" + id + ".tar"
	if _, err := os.Stat(tarFile); err != nil {
		// If the parent is nil, the first layer is also nil.
		// So we may not got tar file
		if parent == "" {
			layerFs := fmt.Sprintf("%s/diff/%s", d.RootPath(), id)
			archive, err := archive.Tar(layerFs, archive.Uncompressed)
			if err != nil {
				return nil, err
			}
			return ioutils.NewReadCloserWrapper(archive, func() error {
				err := archive.Close()
				return err
			}), nil
		} else {
			return nil, fmt.Errorf("the out tar file is not exist")
		}
	}
	f, err := os.Open(tarFile)
	if err != nil {
		return nil, err
	}
	var archive io.ReadCloser
	archive = ioutil.NopCloser(f)
	glog.Infof("Diff between %s and %s", id, parent)
	return ioutils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		return err
	}), nil
}

func (d *Driver) Changes(id, parent string) ([]archive.Change, error) {
	return nil, nil
}

func (d *Driver) DiffSize(id, parent string) (size int64, err error) {
	return 0, nil
}
