package pod

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/version"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/daemon/daemondb"
	"github.com/hyperhq/hyperd/types"
	apitypes "github.com/hyperhq/hyperd/types"
)

// MigrateLagecyData migrate lagecy persistence data to current layout.
func MigrateLagecyPersistentData(db *daemondb.DaemonDB, podFactory func() *PodFactory) (err error) {
	num := 0
	count := 0
	defer func() {
		logInfo := fmt.Sprintf("Migrate lagecy persistent pod data, found: %d, migrated: %d", num, count)
		if err == nil {
			hlog.Log(INFO, logInfo)
		} else {
			hlog.Log(ERROR, "%s, but failed with %v", logInfo, err)
		}
	}()
	list, err := db.LagecyListPod()
	if err != nil {
		return err
	}
	num = len(list)
	if num == 0 {
		return nil
	}
	ch := db.LagecyGetAllPods()
	if ch == nil {
		err = fmt.Errorf("cannot list pods in daemondb")
		return err
	}
	for {
		item, ok := <-ch
		if !ok {
			break
		}
		if item == nil {
			err = fmt.Errorf("error during get pods from daemondb")
			return err
		}

		podID := string(item.K[4:])

		hlog.Log(TRACE, "try to migrate lagecy pod %s from daemondb", podID)

		var podSpec apitypes.UserPod
		if err = json.Unmarshal(item.V, &podSpec); err != nil {
			return err
		}

		factory := podFactory()

		// fill in corresponding container id in pod spec
		if err = setupContanerID(factory, podID, &podSpec); err != nil {
			return err
		}
		// convert some lagecy volume field to current format
		if err = setupVolumes(factory.db, podID, item.V, &podSpec); err != nil {
			return err
		}

		if err = persistLagecyPod(factory, &podSpec); err != nil {
			return err
		}

		var vmID string
		if vmID, err = db.LagecyGetP2V(podID); err != nil {
			hlog.Log(DEBUG, "no existing VM for pod %s: %v", podID, err)
		} else {
			var vmData []byte
			if vmData, err = db.LagecyGetVM(vmID); err != nil {
				return err
			}
			// save sandbox data in current layout
			sandboxInfo := types.SandboxPersistInfo{
				Id:          vmID,
				PersistInfo: vmData,
			}
			err = saveMessage(db, fmt.Sprintf(SB_KEY_FMT, podSpec.Id), &sandboxInfo, nil, "sandbox info")
			if err != nil {
				return err
			}
		}

		errs := purgeLagecyPersistPod(db, podID)
		if len(errs) != 0 {
			hlog.Log(DEBUG, "%v", errs)
		}
		count++
	}
	return nil
}

func setupVolumes(db *daemondb.DaemonDB, podID string, persist []byte, podSpec *apitypes.UserPod) (err error) {
	var (
		vinfo       []byte
		specMap     map[string]interface{}
		raw_volumes []interface{}
	)
	if err = json.Unmarshal(persist, &specMap); err != nil {
		return err
	}
	raw_raw_volumes, success := specMap["volumes"]
	if success {
		raw_volumes, success = raw_raw_volumes.([]interface{})
	}
	for i, vol := range podSpec.Volumes {
		// copy lagecy field Volumes[i].Driver to Format
		if success {
			raw_vol, ok := raw_volumes[i].(map[string]interface{})
			if ok {
				raw_driver, ok := raw_vol["driver"]
				if ok {
					driver, ok := raw_driver.(string)
					if ok {
						vol.Format = driver
					}
				}
			}
		}
		// replace podID with spec.Id in hostvolume path
		vol.Source = strings.Replace(vol.Source, podID, podSpec.Id, 1)
		// replace podId with spec.Id in volume persist key
		vinfo, err = db.GetPodVolume(podID, vol.Name)
		if err == nil {
			if err = db.UpdatePodVolume(podSpec.Id, vol.Name, vinfo); err != nil {
				return err
			}
		}
	}
	return nil
}

func purgeLagecyPersistPod(db *daemondb.DaemonDB, podID string) []error {
	var errs []error
	err := db.LagecyDeleteVMByPod(podID)
	if err != nil {
		errs = append(errs, fmt.Errorf("remove vm: %v", err))
	}
	if err = db.LagecyDeletePod(podID); err != nil {
		errs = append(errs, fmt.Errorf("remove pod: %v", err))
	}
	if err = db.LagecyDeleteP2C(podID); err != nil {
		errs = append(errs, fmt.Errorf("remove pod container: %v", err))
	}
	if err = db.DeletePodVolumes(podID); err != nil {
		errs = append(errs, fmt.Errorf("remove pod volumes: %v", err))
	}
	return errs
}

func setupContanerID(factory *PodFactory, podID string, spec *apitypes.UserPod) error {
	cIDs, err := factory.db.LagecyGetP2C(podID)
	if err != nil {
		return err
	}
	for _, cID := range cIDs {
		r, err := factory.engine.ContainerInspect(cID, false, version.Version("1.21"))
		if err == nil {
			rsp, ok := r.(*dockertypes.ContainerJSON)
			if !ok {
				hlog.Log(ERROR, "fail to got loaded container info: %v", r)
				return nil
			}
			n := strings.TrimLeft(rsp.Name, "/")
			found := false
			for _, ctr := range spec.Containers {
				if ctr.Name == n {
					ctr.Id = cID
					found = true
					break
				}
			}
			if !found {
				err = fmt.Errorf("cannot find a match container with ID(%s)", cID)
				hlog.Log(ERROR, err)
				return err
			}
		}
	}
	return nil
}

func persistLagecyPod(factory *PodFactory, spec *apitypes.UserPod) error {
	p, err := newXPod(factory, spec)
	if err != nil {
		return err
	}
	if err = p.initResources(spec, false); err != nil {
		return err
	}

	if err = p.savePod(); err != nil {
		return err
	}
	return nil
}
