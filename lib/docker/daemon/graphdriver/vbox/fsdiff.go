package vbox

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/hyper/lib/docker/pkg/archive"
	"github.com/hyperhq/hyper/lib/docker/pkg/chrootarchive"
	"github.com/hyperhq/hyper/lib/docker/pkg/ioutils"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/govbox"
)

func (d *Driver) ApplyDiff(id, parent string, diff archive.ArchiveReader) (size int64, err error) {
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

	// start or replace pod
	vm, ok := d.daemon.VmList[d.pullVm]
	if !ok {
		return nil, fmt.Errorf("can not find VM(%s)", d.pullVm)
	}
	if vm.Status == types.S_VM_IDLE {
		code, cause, err = d.daemon.StartPod(podId, podData, d.pullVm, nil, false, true, types.VM_KEEP_AFTER_SHUTDOWN, []*hypervisor.TtyIO{})
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			d.daemon.KillVm(d.pullVm)
			return nil, err
		}
		vm := d.daemon.VmList[d.pullVm]
		// wait for cmd finish
		Status, err := vm.GetResponseChan()
		if err != nil {
			glog.Error(err.Error())
			return nil, err
		}
		defer vm.ReleaseResponseChan(Status)

		var vmResponse *types.VmResponse
		for {
			vmResponse = <-Status
			if vmResponse.VmId == d.pullVm {
				if vmResponse.Code == types.E_POD_FINISHED {
					glog.Infof("Got E_POD_FINISHED code response")
					break
				}
			}
		}

		pod, ok := d.daemon.PodList.Get(podId)
		if !ok {
			glog.Errorf("pod %s does not exist", podId)
			return nil, fmt.Errorf("pod %s does not exist", podId)
		}
		pod.SetVM(d.pullVm, vm)

		// release pod from VM
		code, cause, err = d.daemon.StopPod(podId, "no")
		if err != nil {
			glog.Errorf("Code is %d, Cause is %s, %s", code, cause, err.Error())
			d.daemon.KillVm(d.pullVm)
			return nil, err
		}
		d.daemon.CleanPod(podId)
	} else {
		glog.Errorf("pull vm should not be associated")
		return nil, fmt.Errorf("pull vm is not idle")
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
